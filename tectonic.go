package main

// 便携版 LaTeX 编译器 Tectonic（1.14，参考同学的做法）：不用安装几个 GB 的 TeX Live，
// 一个 tectonic.exe 就能编译，缺少的宏包在第一次编译时自动下载（需要联网）。
// 查找顺序：已安装的 TeX Live / MiKTeX 优先；没有时再找 Tectonic（PATH、程序旁边的 latex_portable 文件夹、数据目录 tools\tectonic）。
// 设置里可以“一键下载”：从 Tectonic 官方 GitHub 发布页下载对应系统的版本。

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var (
	texToolsDir = ""                                                                           // 数据目录下的 tools（main 中设置）
	tectonicAPI = "https://api.github.com/repos/tectonic-typesetting/tectonic/releases/latest" // 测试时替换
)

const tectonicPage = "https://github.com/tectonic-typesetting/tectonic/releases"

func tectonicExe() string {
	if runtime.GOOS == "windows" {
		return "tectonic.exe"
	}
	return "tectonic"
}

func tectonicDir() string {
	if texToolsDir == "" {
		return ""
	}
	return filepath.Join(texToolsDir, "tectonic")
}

func tectonicCandidates() []string {
	var out []string
	if p, err := exec.LookPath("tectonic"); err == nil {
		out = append(out, p)
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		out = append(out, filepath.Join(d, "latex_portable", tectonicExe()), filepath.Join(d, tectonicExe()))
	}
	if d := tectonicDir(); d != "" {
		out = append(out, filepath.Join(d, tectonicExe()))
	}
	var ok []string
	for _, p := range out {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			ok = append(ok, p)
		}
	}
	return ok
}

var reTectonicErr = regexp.MustCompile(`(?m)^error: (?:([^:\n]+\.tex):(\d+): )?(.+)$`)

// runTectonic 用 Tectonic 编译（它会自动多次编译、运行 BibTeX）
func runTectonic(e texEngine, workDir, mainFile, outDir string, timeout time.Duration) texResult {
	start := time.Now()
	res := texResult{Engine: "Tectonic（第一次编译会自动下载宏包，需要联网）", Errors: []texIssue{}, Warnings: []texIssue{}}
	if timeout < 300*time.Second {
		timeout = 300 * time.Second // 第一次需要下载宏包
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stem := strings.TrimSuffix(filepath.Base(mainFile), filepath.Ext(mainFile))
	run := func(extra ...string) (string, error) {
		args := append([]string{"--keep-logs", "--untrusted", "--outdir", outDir}, extra...)
		args = append(args, mainFile)
		cmd := exec.CommandContext(ctx, e.Tectonic, args...)
		cmd.Dir = workDir
		hideConsole(cmd)
		cmd.Cancel = func() error { killTree(cmd); return nil }
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		cmd.Stdin = strings.NewReader("")
		err := cmd.Run()
		return buf.String(), err
	}
	// 主文件可能是临时副本：把原文件夹加入搜索路径，图片和 \input 的文件才能找到
	out, err := run("-Z", "search-path="+workDir)
	if err != nil && strings.Contains(out, "search-path") {
		out, err = run()
	}
	res.Seconds = float64(int(time.Since(start).Seconds()*10)) / 10
	if ctx.Err() != nil {
		res.Errors = append(res.Errors, texIssue{Msg: "编译超时（超过 " + itoa(int(timeout.Seconds())) + " 秒），已停止", Hint: "第一次编译要下载宏包，网络慢时可以再试一次"})
	}
	log := ""
	if b, e2 := os.ReadFile(filepath.Join(outDir, stem+".log")); e2 == nil {
		log = string(b)
	}
	errs, warns, pages := parseTeXLog(log, e)
	res.Errors = append(res.Errors, errs...)
	res.Warnings = warns
	res.Pages = pages
	if len(errs) == 0 && err != nil {
		for _, m := range reTectonicErr.FindAllStringSubmatch(out, 8) {
			it := texIssue{File: m[1], Msg: m[3]}
			it.Line = atoiSafe(m[2])
			if strings.Contains(m[3], "download") || strings.Contains(m[3], "network") || strings.Contains(m[3], "bundle") {
				it.Hint = "Tectonic 需要联网下载宏包，请检查网络后重试；校园网无法访问时，可以改装 TeX Live"
			}
			res.Errors = append(res.Errors, it)
		}
	}
	if pdf, e3 := os.ReadFile(filepath.Join(outDir, stem+".pdf")); e3 == nil && bytes.HasPrefix(pdf, []byte("%PDF")) {
		res.pdf = pdf
		res.OK = len(res.Errors) == 0
	}
	tail := log
	if tail == "" {
		tail = out
	}
	if len(tail) > 4000 {
		tail = tail[len(tail)-4000:]
	}
	res.LogTail = tail
	if res.pdf == nil && len(res.Errors) == 0 {
		res.Errors = append(res.Errors, texIssue{Msg: "没有生成 PDF", Hint: "请查看下面的日志"})
	}
	return res
}

// ---------------- 一键下载 ----------------

func tectonicAssetSuffix() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "x86_64-pc-windows-msvc.zip"
	case "linux/amd64":
		return "x86_64-unknown-linux-musl.tar.gz"
	case "linux/arm64":
		return "aarch64-unknown-linux-musl.tar.gz"
	case "darwin/arm64":
		return "aarch64-apple-darwin.tar.gz"
	case "darwin/amd64":
		return "x86_64-apple-darwin.tar.gz"
	}
	return ""
}

func (a *App) hTectonicInstall(w http.ResponseWriter, r *http.Request, me *Me) error {
	if !isLoopback(r) {
		return errForbidden("只能在运行工作台的电脑上安装")
	}
	dir := tectonicDir()
	suffix := tectonicAssetSuffix()
	if dir == "" || suffix == "" {
		return errBad("这个系统暂不支持一键下载，请从 " + tectonicPage + " 手动下载")
	}
	manual := "。也可以手动下载：打开 " + tectonicPage + "，下载文件名以 " + suffix + " 结尾的版本，解压出 " + tectonicExe() + " 放到 " + dir + "，再点“重新检测”"
	hc := &http.Client{Timeout: 10 * time.Minute}
	req, _ := http.NewRequest("GET", tectonicAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CanDo/"+AppVersion)
	resp, err := hc.Do(req)
	if err != nil {
		return errBad("连不上 GitHub（" + err.Error() + "）" + manual)
	}
	var rel struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 {
		return errBad("读取 Tectonic 发布信息失败（" + itoa(resp.StatusCode) + "）" + manual)
	}
	url := ""
	for _, as := range rel.Assets {
		if strings.HasPrefix(as.Name, "tectonic-") && strings.HasSuffix(as.Name, suffix) {
			url = as.URL
		}
	}
	if url == "" {
		return errBad("没有找到适合这台电脑的版本" + manual)
	}
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "CanDo/"+AppVersion)
	resp, err = hc.Do(req)
	if err != nil || resp.StatusCode != 200 {
		code := 0
		if resp != nil {
			code = resp.StatusCode
			resp.Body.Close()
		}
		return errBad("下载失败（" + itoa(code) + "）" + manual)
	}
	pkg, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	resp.Body.Close()
	if err != nil {
		return errBad("下载中断，请重试" + manual)
	}
	bin, err := extractTectonic(pkg, suffix)
	if err != nil {
		return errBad(err.Error() + manual)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errBad("无法创建文件夹 " + dir)
	}
	tmp := filepath.Join(dir, tectonicExe()+".tmp")
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return errBad("写入失败：" + err.Error())
	}
	final := filepath.Join(dir, tectonicExe())
	os.Remove(final)
	if err := os.Rename(tmp, final); err != nil {
		return errBad("写入失败：" + err.Error())
	}
	e := findTeX(true)
	writeJSON(w, 200, map[string]any{"ok": true, "version": rel.Tag, "path": final, "engine": e})
	return nil
}

func extractTectonic(pkg []byte, suffix string) ([]byte, error) {
	want := tectonicExe()
	if strings.HasSuffix(suffix, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
		if err != nil {
			return nil, errBad("下载的文件不完整")
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == want {
				rc, err := f.Open()
				if err != nil {
					return nil, errBad("解压失败")
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 200<<20))
			}
		}
		return nil, errBad("压缩包里没有 " + want)
	}
	gz, err := gzip.NewReader(bytes.NewReader(pkg))
	if err != nil {
		return nil, errBad("下载的文件不完整")
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if filepath.Base(h.Name) == want && h.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
	return nil, errBad("压缩包里没有 " + want)
}
