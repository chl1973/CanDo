package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

const defaultPort = 18765

func defaultDataDir() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "KeyanWorkbench", "data")
		}
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".keyan-workbench")
}

// probe 检查端口上是否已经运行着本程序。
func probe(port int) bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var h struct {
		App string `json:"app"`
	}
	json.NewDecoder(resp.Body).Decode(&h)
	return h.App == "kyws"
}

func main() {
	dataDir := flag.String("data", defaultDataDir(), "数据目录")
	portFlag := flag.Int("port", 0, "端口（默认 18765，被占用时自动顺延）")
	noBrowser := flag.Bool("no-browser", false, "启动后不自动打开窗口")
	resetAdminFlag := flag.Bool("reset-admin", false, "忘记管理员密码：重置为临时密码")
	flag.Parse()

	if *resetAdminFlag {
		msg, err := runResetAdmin(*dataDir)
		if err != nil {
			showError(err.Error())
			os.Exit(1)
		}
		showInfo(msg)
		return
	}

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		fatal("无法创建数据目录 " + *dataDir + "：" + err.Error())
	}
	lf, err := os.OpenFile(filepath.Join(*dataDir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		log.SetOutput(lf)
	}

	store, err := OpenStore(*dataDir)
	if err != nil {
		fatal("无法打开数据：" + err.Error())
	}
	// 上次异常退出时中断的核验任务
	store.Update(func(db *DB) error {
		for _, c := range db.CiteChecks {
			if c.Status == "running" {
				c.Status, c.Error = "failed", "程序重启导致核验中断，请重新运行"
			}
		}
		return nil
	})

	port := *portFlag
	if port == 0 {
		store.View(func(db *DB) { port = db.Settings.Port })
		if port == 0 {
			port = defaultPort
		}
	}
	// 单实例：已在运行则直接打开窗口
	if probe(port) {
		if !*noBrowser {
			openWindow("http://127.0.0.1:" + strconv.Itoa(port) + "/")
		}
		return
	}
	var ln net.Listener
	for p := port; p < port+20; p++ {
		ln, err = net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(p))
		if err == nil {
			port = p
			break
		}
		if probe(p) {
			if !*noBrowser {
				openWindow("http://127.0.0.1:" + strconv.Itoa(p) + "/")
			}
			return
		}
	}
	if ln == nil {
		fatal("找不到可用端口（" + strconv.Itoa(port) + " 起的 20 个端口都被占用）")
	}
	store.Update(func(db *DB) error { db.Settings.Port = port; return nil })

	app := &App{store: store, quit: make(chan struct{}), port: port, controlToken: writeControlToken(*dataDir), secret: loadSecret(*dataDir)}
	texToolsDir = filepath.Join(*dataDir, "tools")
	app.StartScheduler()
	srv := &http.Server{Handler: app.Routes(), ReadHeaderTimeout: 20 * time.Second, ErrorLog: log.Default()}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("程序异常退出：%v", r)
			panic(r)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-sig
		log.Printf("收到系统信号 %v，程序退出", s)
		select {
		case <-app.quit:
		default:
			close(app.quit)
		}
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Println("服务错误，程序退出：", err)
			close(app.quit)
		}
	}()
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	log.Printf("CanDo 可为 %s 已启动：%s（监听 0.0.0.0:%d）数据目录：%s", AppVersion, url, port, *dataDir)
	fmt.Println("CanDo 可为已启动：" + url)
	fmt.Println("数据目录：" + *dataDir)
	if !*noBrowser {
		openWindow(url)
	}
	<-app.quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("已正常退出（设置中点击了“退出工作台程序”或收到系统关闭信号）")
}

func fatal(msg string) {
	log.Println(msg)
	showError(msg)
	os.Exit(1)
}
