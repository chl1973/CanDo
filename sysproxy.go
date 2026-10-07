package main

// 出门走哪条路：环境变量里的代理 → Windows 的“系统代理” → 直连。
// Go 程序默认只看环境变量；浏览器用的是系统代理（Clash 等工具的“系统代理”开关改的就是它）。
// 不读系统代理，就会出现“浏览器能打开 GitHub，CanDo 却连不上”。
// 代理常常开在本机（127.0.0.1:7890），而下载全文、读取网页、导入技能都禁止连内网地址——
// 所以这里把“我们自己选中的代理地址”单独放行，目标地址仍然不许是内网。

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var errPrivateTarget = errors.New("禁止访问内网地址")

// 测试时可以替换
var (
	envProxy       = http.ProxyFromEnvironment
	systemProxyFor = func(target *url.URL) *url.URL { return parseSystemProxy(cachedSystemProxySetting(), target.Scheme) }
)

var proxyDialOK sync.Map // "ip:port" → true：允许拨号的代理地址

// chooseProxy 给 http.Transport 用：决定这次请求走不走代理。
func chooseProxy(req *http.Request) (*url.URL, error) {
	host := strings.ToLower(req.URL.Hostname())
	private := host == "localhost" || strings.HasSuffix(host, ".local")
	if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
		private = true
	}
	if private {
		if allowPrivateFetch {
			return nil, nil // 测试：直连
		}
		return nil, errPrivateTarget
	}
	u, err := envProxy(req)
	if err != nil {
		return nil, err
	}
	if u == nil {
		u = systemProxyFor(req.URL)
	}
	if u == nil {
		return nil, nil // 直连：拨号时再检查目标是不是内网
	}
	// 走代理时，拨号看到的是代理的地址，看不到目标；所以在这里先查一次目标的地址。
	// 查不到（例如本地 DNS 被污染或超时）就交给代理去解析。
	if !allowPrivateFetch && net.ParseIP(host) == nil {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		ips, _ := net.DefaultResolver.LookupIPAddr(ctx, host)
		cancel()
		for _, a := range ips {
			if blockedIP(a.IP) {
				return nil, errPrivateTarget
			}
		}
	}
	allowProxyDial(u)
	return u, nil
}

// allowProxyDial 记下代理的地址，拨号检查时放行（哪怕它在本机或局域网）。
func allowProxyDial(u *url.URL) {
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		proxyDialOK.Store(net.JoinHostPort(ip.String(), port), true)
		return
	}
	if _, seen := proxyDialOK.Load("name:" + host + ":" + port); seen {
		return
	}
	proxyDialOK.Store("name:"+host+":"+port, true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ips, _ := net.DefaultResolver.LookupIPAddr(ctx, host)
	for _, a := range ips {
		proxyDialOK.Store(net.JoinHostPort(a.IP.String(), port), true)
	}
}

// dialAllowed 拨号前的检查：内网地址只有在它是我们选中的代理时才允许。
func dialAllowed(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !blockedIP(ip) || allowPrivateFetch {
		return nil
	}
	if _, ok := proxyDialOK.Load(net.JoinHostPort(ip.String(), port)); ok {
		return nil
	}
	return errPrivateTarget
}

// ---------------- Windows 系统代理 ----------------

var sysProxyCache struct {
	sync.Mutex
	at  time.Time
	val string
}

// cachedSystemProxySetting 读系统代理设置，30 秒内不重复读。
func cachedSystemProxySetting() string {
	sysProxyCache.Lock()
	defer sysProxyCache.Unlock()
	if sysProxyCache.at.IsZero() || time.Since(sysProxyCache.at) > 30*time.Second {
		sysProxyCache.val, sysProxyCache.at = readSystemProxySetting(), time.Now()
	}
	return sysProxyCache.val
}

// parseSystemProxy 解析系统代理的几种写法，返回这次请求该用的代理：
//
//	127.0.0.1:7890
//	http=127.0.0.1:7890;https=127.0.0.1:7890
//	socks=127.0.0.1:7891
//	http://127.0.0.1:7890
func parseSystemProxy(setting, scheme string) *url.URL {
	byProto, all := map[string]string{}, ""
	for _, part := range strings.Split(setting, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok && !strings.Contains(k, "/") {
			byProto[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		} else if all == "" {
			all = part
		}
	}
	addr, socks := byProto[scheme], false
	if addr == "" {
		addr = all
	}
	if addr == "" && byProto["socks"] != "" {
		addr, socks = byProto["socks"], true
	}
	if addr == "" {
		return nil
	}
	if !strings.Contains(addr, "://") {
		if socks {
			addr = "socks5://" + addr
		} else {
			addr = "http://" + addr
		}
	}
	u, err := url.Parse(addr)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u
	case "socks", "socks4", "socks4a": // Go 只会说 SOCKS5；Clash、v2ray 这类工具的 socks 端口都支持
		u.Scheme = "socks5"
		return u
	}
	return nil
}
