package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testSkillMD = "---\nname: 论文润色\ndescription: 按期刊风格润色段落\nlicense: Apache-2.0\n---\n1. 先读原文。\n2. 逐句改。\n"

// 没人在听的本机地址：连它一定失败，用来模拟“GitHub 连不上”
func deadURL(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return "http://" + addr + "/SKILL.md"
}

// 从 GitHub 导入技能：第一条线路连不上时换备用线路；都不行时告诉用户怎么手动导入。
func TestSkillImportFallback(t *testing.T) {
	allowPrivateFetch = true
	oldSrc, oldWait := skillSources, skillTryTimeout
	skillTryTimeout = 3 * time.Second
	defer func() { allowPrivateFetch, skillSources, skillTryTimeout = false, oldSrc, oldWait }()

	hits := 0
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/ok":
			io.WriteString(w, testSkillMD)
		case "/limited":
			w.WriteHeader(403)
		default:
			w.WriteHeader(404)
		}
	}))
	defer mirror.Close()
	e := newAgentEnv(t)
	gh := "https://github.com/o/r/tree/main/skills/polish"
	imp := func() (int, map[string]any) {
		return e.tc.do("POST", "/api/agent/skills/import", map[string]any{"url": gh})
	}

	// 1. 第一条连不上、第二条限流、第三条成功
	skillSources = func(raw string) []skillSource {
		if raw != "https://raw.githubusercontent.com/o/r/main/skills/polish/SKILL.md" {
			t.Fatalf("原始地址不对：%s", raw)
		}
		return []skillSource{{URL: deadURL(t)}, {URL: mirror.URL + "/limited"}, {URL: mirror.URL + "/ok"}}
	}
	code, r := imp()
	if code != 200 {
		t.Fatalf("应从备用线路导入成功：%d %v", code, r)
	}
	if sk := r["skill"].(map[string]any); sk["name"] != "论文润色" || sk["license"] != "Apache-2.0" || !strings.Contains(sk["body"].(string), "逐句改") {
		t.Fatalf("导入的内容不对：%v", sk)
	}
	if hits != 2 {
		t.Fatalf("应按顺序试到成功为止，实际请求 %d 次", hits)
	}

	// 2. GitHub 自己回答“没有这个文件”：不再试镜像
	hits = 0
	skillSources = func(string) []skillSource {
		return []skillSource{{URL: mirror.URL + "/none"}, {URL: mirror.URL + "/ok"}}
	}
	if code, r = imp(); code != 400 || !strings.Contains(errMsg(r), "没有找到 SKILL.md") || hits != 1 {
		t.Fatalf("第一条线路 404 应直接报没找到：%d %v（请求 %d 次）", code, r, hits)
	}

	// 3. 全都连不上：说清原因和手动导入的办法
	skillSources = func(string) []skillSource { return []skillSource{{URL: deadURL(t)}, {URL: deadURL(t)}} }
	if code, r = imp(); code != 400 || !strings.Contains(errMsg(r), "连不上 GitHub") || !strings.Contains(errMsg(r), "导入 SKILL.md 文件") {
		t.Fatalf("全部失败时应给出手动导入的办法：%d %v", code, r)
	}

	// 4. GitHub 连不上，镜像上也没有
	skillSources = func(string) []skillSource { return []skillSource{{URL: deadURL(t)}, {URL: mirror.URL + "/none"}} }
	if code, r = imp(); code != 400 || !strings.Contains(errMsg(r), "备用线路上也没有找到") {
		t.Fatalf("镜像 404 的提示不对：%d %v", code, r)
	}

	// 默认线路：原始地址在最前，后面是镜像和接口
	src := oldSrc("https://raw.githubusercontent.com/o/r/main/skills/polish/SKILL.md")
	want := []string{
		"https://raw.githubusercontent.com/o/r/main/skills/polish/SKILL.md",
		"https://cdn.jsdelivr.net/gh/o/r@main/skills/polish/SKILL.md",
		"https://fastly.jsdelivr.net/gh/o/r@main/skills/polish/SKILL.md",
		"https://api.github.com/repos/o/r/contents/skills/polish/SKILL.md?ref=main",
	}
	if len(src) != len(want) {
		t.Fatalf("线路数不对：%v", src)
	}
	for i := range want {
		if src[i].URL != want[i] {
			t.Fatalf("第 %d 条线路：%s，应为 %s", i+1, src[i].URL, want[i])
		}
	}
	if src[3].Accept == "" {
		t.Fatal("GitHub 接口要带 Accept 才返回原文")
	}
}

// 系统代理的几种写法
func TestParseSystemProxy(t *testing.T) {
	for _, c := range []struct{ setting, scheme, want string }{
		{"127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"127.0.0.1:7890", "http", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:7890;https=127.0.0.1:7891", "https", "http://127.0.0.1:7891"},
		{"http=127.0.0.1:7890;https=127.0.0.1:7891", "http", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:7890", "https", ""}, // 只给了 http：https 直连，和 Windows 的做法一致
		{"socks=127.0.0.1:7891", "https", "socks5://127.0.0.1:7891"},
		{"http://proxy.lab:8080", "https", "http://proxy.lab:8080"},
		{"socks5://127.0.0.1:1080", "https", "socks5://127.0.0.1:1080"},
		{"", "https", ""},
		{" ; ", "https", ""},
		{"ftp://x:1", "https", ""},
	} {
		got := ""
		if u := parseSystemProxy(c.setting, c.scheme); u != nil {
			got = u.String()
		}
		if got != c.want {
			t.Fatalf("parseSystemProxy(%q, %q) = %q，应为 %q", c.setting, c.scheme, got, c.want)
		}
	}
}

// 代理开在本机时：允许连这个代理；目标是内网的仍然拒绝；本机别的端口仍然拒绝。
func TestLocalProxyAllowedButPrivateTargetsBlocked(t *testing.T) {
	seen := ""
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.String() // 普通 http 请求经过代理时，这里是完整网址
		io.WriteString(w, "经过代理")
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	oldEnv, oldSys := envProxy, systemProxyFor
	envProxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	systemProxyFor = func(*url.URL) *url.URL { return pu }
	defer func() { envProxy, systemProxyFor = oldEnv, oldSys }()
	if allowPrivateFetch {
		t.Fatal("这个测试要在禁止内网的状态下跑")
	}

	resp, err := pdfClient.Get("http://skills.invalid/a/SKILL.md")
	if err != nil {
		t.Fatalf("本机代理应该放行：%v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "经过代理" || seen != "http://skills.invalid/a/SKILL.md" {
		t.Fatalf("请求没有经过代理：%q %q", b, seen)
	}

	for _, target := range []string{"http://127.0.0.1:9/", "http://localhost:9/", "http://192.168.1.1/", "http://10.0.0.8/x", "http://[::1]:9/", "http://nas.local/"} {
		if _, err := pdfClient.Get(target); err == nil || !strings.Contains(err.Error(), "内网") {
			t.Fatalf("%s 应被拒绝（内网），得到 %v", target, err)
		}
	}
	if err := dialAllowed(pu.Host); err != nil {
		t.Fatalf("代理地址应允许拨号：%v", err)
	}
	if err := dialAllowed("127.0.0.1:9"); err == nil {
		t.Fatal("本机别的端口不应放行")
	}
	if err := dialAllowed("93.184.216.34:443"); err != nil {
		t.Fatalf("公网地址应允许：%v", err)
	}

	// 没有代理时：和以前一样，直连内网被拒绝
	systemProxyFor = func(*url.URL) *url.URL { return nil }
	if _, err := pdfClient.Get(proxy.URL); err == nil || !strings.Contains(err.Error(), "内网") {
		t.Fatalf("没有代理时直连本机应被拒绝：%v", err)
	}
}

func errMsg(r map[string]any) string {
	s, _ := r["detail"].(string)
	return s
}
