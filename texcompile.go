package main

// LaTeX 导出 PDF：使用电脑上已安装的 TeX 发行版（TeX Live / MiKTeX），在临时目录中编译，
// 解析日志给出中文的错误说明（带行号），PDF 暂存 30 分钟供查看或保存。
// 只能在运行工作台的电脑上使用：LaTeX 可以读取电脑上的文件，不对外部设备开放。

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

type texEngine struct {
	Found    bool   `json:"found"`
	Dist     string `json:"dist"` // TeX Live / MiKTeX
	Version  string `json:"version"`
	XeLaTeX  string `json:"xelatex"`
	PdfLaTeX string `json:"pdflatex"`
	BibTeX   string `json:"-"`
	Biber    string `json:"-"`
	MiKTeX   bool   `json:"-"`
	Tectonic string `json:"tectonic,omitempty"`
}

var (
	texMu     sync.Mutex
	texCached *texEngine
	texAt     time.Time
)

func texCandidates(name string) []string {
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	var out []string
	if p, err := exec.LookPath(name); err == nil {
		out = append(out, p)
	}
	if runtime.GOOS == "windows" {
		var pats []string
		for _, root := range []string{`C:\texlive`, `D:\texlive`, `E:\texlive`} {
			pats = append(pats, filepath.Join(root, "*", "bin", "windows", exe), filepath.Join(root, "*", "bin", "win32", exe), filepath.Join(root, "*", "bin", "win64", exe))
		}
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			pats = append(pats, filepath.Join(la, "Programs", "MiKTeX", "miktex", "bin", "x64", exe))
		}
		for _, pf := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if pf != "" {
				pats = append(pats, filepath.Join(pf, "MiKTeX", "miktex", "bin", "x64", exe), filepath.Join(pf, "MiKTeX", "miktex", "bin", exe))
			}
		}
		for _, pat := range pats {
			m, _ := filepath.Glob(pat)
			sortDesc(m) // 多个年份时用最新的
			out = append(out, m...)
		}
	}
	return out
}

func sortDesc(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] > xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func first(xs []string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return ""
}

// findTeX 查找 TeX 发行版（结果缓存 5 分钟；refresh 为 true 时重新查找）。
func findTeX(refresh bool) texEngine {
	texMu.Lock()
	defer texMu.Unlock()
	if texCached != nil && !refresh && time.Since(texAt) < 5*time.Minute {
		return *texCached
	}
	e := texEngine{XeLaTeX: first(texCandidates("xelatex")), PdfLaTeX: first(texCandidates("pdflatex"))}
	bin := filepath.Dir(first([]string{e.XeLaTeX, e.PdfLaTeX}))
	if e.XeLaTeX != "" || e.PdfLaTeX != "" {
		e.Found = true
		for _, n := range []string{"bibtex", "biber"} {
			p := filepath.Join(bin, n)
			if runtime.GOOS == "windows" {
				p += ".exe"
			}
			if _, err := os.Stat(p); err != nil {
				p = first(texCandidates(n))
			}
			if n == "bibtex" {
				e.BibTeX = p
			} else {
				e.Biber = p
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, first([]string{e.XeLaTeX, e.PdfLaTeX}), "--version")
		hideConsole(cmd)
		out, _ := cmd.Output()
		v := strings.SplitN(string(out), "\n", 2)[0]
		e.Version = strings.TrimSpace(v)
		e.MiKTeX = strings.Contains(v, "MiKTeX") || strings.Contains(strings.ToLower(bin), "miktex")
		e.Dist = map[bool]string{true: "MiKTeX", false: "TeX Live"}[e.MiKTeX]
	} else if t := first(tectonicCandidates()); t != "" {
		// 没有 TeX Live / MiKTeX 时用便携版 Tectonic
		e.Found, e.Dist, e.Tectonic = true, "Tectonic", t
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, t, "--version")
		hideConsole(cmd)
		out, _ := cmd.Output()
		e.Version = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	}
	texCached, texAt = &e, time.Now()
	return e
}

type texIssue struct {
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	Msg  string `json:"msg"`
	Hint string `json:"hint,omitempty"`
}

type texResult struct {
	OK       bool       `json:"ok"`
	Engine   string     `json:"engine"`
	Seconds  float64    `json:"seconds"`
	Errors   []texIssue `json:"errors"`
	Warnings []texIssue `json:"warnings"`
	LogTail  string     `json:"log_tail"`
	PDFID    string     `json:"pdf_id,omitempty"`
	Pages    int        `json:"pages,omitempty"`
	pdf      []byte
}

var reCJK = regexp.MustCompile(`[\p{Han}]`)

func needsXe(src string) bool {
	return reCJK.MatchString(src) || strings.Contains(src, "ctex") || strings.Contains(src, "xeCJK") || strings.Contains(src, "fontspec")
}

var (
	reFileLine   = regexp.MustCompile(`^(.*?\.(?:tex|sty|cls|bbl)):(\d+): (.*)$`)
	reBangErr    = regexp.MustCompile(`^! (.*)$`)
	reTexLineNo  = regexp.MustCompile(`^l\.(\d+)`)
	reRefWarn    = regexp.MustCompile(`LaTeX Warning: (Reference|Citation) [` + "`" + `'"]?([^'"]+?)['"]? on page \d+ undefined on input line (\d+)`)
	reMissingChr = regexp.MustCompile(`Missing character: There is no (.) \(U\+([0-9A-F]+)\)`)
	reMissingSty = regexp.MustCompile("File `([^']+)' not found")
	rePages      = regexp.MustCompile(`Output written on .*\((\d+) pages?`)
)

func texHint(msg string, e texEngine) string {
	switch {
	case reMissingSty.MatchString(msg):
		f := reMissingSty.FindStringSubmatch(msg)[1]
		if strings.HasSuffix(f, ".sty") || strings.HasSuffix(f, ".cls") {
			if e.MiKTeX {
				return "缺少宏包 " + f + "：MiKTeX 会提示自动安装，请在 MiKTeX Console 里允许“自动安装缺少的宏包”后重试"
			}
			return "缺少宏包 " + f + "：在命令行运行 tlmgr install " + strings.TrimSuffix(strings.TrimSuffix(f, ".sty"), ".cls") + "，或安装完整版 TeX Live"
		}
		return "找不到文件 " + f + "：检查文件名和路径（区分大小写，建议用正斜杠 /）"
	case strings.Contains(msg, "Undefined control sequence"):
		return "有命令拼错了，或者没有加载对应的宏包"
	case strings.Contains(msg, "Missing $ inserted"):
		return "数学符号（如 ^ _ \\alpha）写在了公式环境外面，需要放进 $...$ 里"
	case strings.Contains(msg, "Extra }") || strings.Contains(msg, "Missing } inserted") || strings.Contains(msg, "Runaway argument") || strings.Contains(msg, "File ended while scanning"):
		return "花括号 { } 不配对（可能少了一个 }）"
	case strings.Contains(msg, "Environment") && strings.Contains(msg, "undefined"):
		return "环境名写错了，或缺少定义它的宏包"
	case strings.Contains(msg, "\\begin{") && strings.Contains(msg, "ended by \\end{"):
		return "\\begin 和 \\end 不匹配"
	case strings.Contains(msg, "Misplaced alignment tab character &"):
		return "& 只能用在表格或对齐环境里；正文中的 & 要写成 \\&"
	case strings.Contains(msg, "Font") && strings.Contains(msg, "not found"):
		return "找不到字体：换成电脑上已有的字体，或去掉字体设置用默认字体"
	case strings.Contains(msg, "Emergency stop") || strings.Contains(msg, "Fatal error"):
		return "编译提前停止，请先看上面的第一个错误"
	}
	return ""
}

// parseTeXLog 从日志中提取错误和重要警告。
func parseTeXLog(log string, e texEngine) ([]texIssue, []texIssue, int) {
	var errs, warns []texIssue
	lines := strings.Split(strings.ReplaceAll(log, "\r\n", "\n"), "\n")
	seen := map[string]bool{}
	addErr := func(it texIssue) {
		k := it.File + itoa(it.Line) + it.Msg
		if seen[k] || len(errs) >= 20 {
			return
		}
		seen[k] = true
		it.Hint = texHint(it.Msg, e)
		errs = append(errs, it)
	}
	for i, ln := range lines {
		if m := reFileLine.FindStringSubmatch(ln); m != nil {
			line := 0
			for _, c := range m[2] {
				line = line*10 + int(c-'0')
			}
			addErr(texIssue{File: filepath.Base(m[1]), Line: line, Msg: strings.TrimSpace(m[3])})
			continue
		}
		if m := reBangErr.FindStringSubmatch(ln); m != nil {
			it := texIssue{Msg: strings.TrimSpace(m[1])}
			for j := i + 1; j < len(lines) && j < i+12; j++ {
				if lm := reTexLineNo.FindStringSubmatch(lines[j]); lm != nil {
					for _, c := range lm[1] {
						it.Line = it.Line*10 + int(c-'0')
					}
					break
				}
			}
			addErr(it)
		}
	}
	// “Emergency stop / Fatal error” 只是后果，前面已有具体错误时不再重复列出
	if len(errs) > 1 {
		keep := errs[:0]
		for _, it := range errs {
			if !strings.Contains(it.Msg, "Emergency stop") && !strings.Contains(it.Msg, "Fatal error") {
				keep = append(keep, it)
			}
		}
		if len(keep) > 0 {
			errs = keep
		}
	}
	missing := map[string]bool{}
	for _, m := range reRefWarn.FindAllStringSubmatch(log, -1) {
		k := m[1] + m[2]
		if seen[k] || len(warns) >= 15 {
			continue
		}
		seen[k] = true
		line := 0
		for _, c := range m[3] {
			line = line*10 + int(c-'0')
		}
		what := map[string]string{"Reference": "引用的标签", "Citation": "引用的参考文献"}[m[1]]
		warns = append(warns, texIssue{Line: line, Msg: what + " " + m[2] + " 没有定义", Hint: "检查 \\label / 参考文献条目的名字是否一致"})
	}
	for _, m := range reMissingChr.FindAllStringSubmatch(log, -1) {
		missing[m[1]] = true
	}
	if len(missing) > 0 {
		var cs []string
		for c := range missing {
			if len(cs) < 10 {
				cs = append(cs, c)
			}
		}
		warns = append(warns, texIssue{Msg: "字体里缺少这些字符，PDF 中会显示为空白：" + strings.Join(cs, " "), Hint: "中文请使用 ctex 文档类或宏包（\\documentclass{ctexart}），并用 XeLaTeX 编译"})
	}
	pages := 0
	if m := rePages.FindStringSubmatch(log); m != nil {
		for _, c := range m[1] {
			pages = pages*10 + int(c-'0')
		}
	}
	return errs, warns, pages
}

// runTeX 在 workDir 中编译 mainFile，输出到 outDir。会按需要运行 BibTeX/Biber 并重复编译。
// mainFile 可以是 workDir 里的相对路径，也可以是绝对路径（例如放在临时目录里、包含待确认修改的副本）。
func runTeX(e texEngine, workDir, mainFile, outDir string, forceXe bool, timeout time.Duration) texResult {
	start := time.Now()
	mainPath := mainFile
	if !filepath.IsAbs(mainPath) {
		mainPath = filepath.Join(workDir, mainFile)
	}
	src, _ := os.ReadFile(mainPath)
	if e.XeLaTeX == "" && e.PdfLaTeX == "" && e.Tectonic != "" {
		return runTectonic(e, workDir, mainFile, outDir, timeout)
	}
	eng, name := e.PdfLaTeX, "pdfLaTeX"
	if (forceXe || needsXe(string(src)) || eng == "") && e.XeLaTeX != "" {
		eng, name = e.XeLaTeX, "XeLaTeX"
	}
	res := texResult{Engine: name + "（" + e.Dist + "）", Errors: []texIssue{}, Warnings: []texIssue{}}
	if eng == "" {
		res.Errors = append(res.Errors, texIssue{Msg: "没有找到 LaTeX 编译程序"})
		return res
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stem := strings.TrimSuffix(filepath.Base(mainFile), filepath.Ext(mainFile))
	run := func(prog string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, prog, args...)
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(), "openout_any=p", "TEXMFOUTPUT="+outDir)
		hideConsole(cmd)
		cmd.Cancel = func() error { killTree(cmd); return nil }
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		cmd.Stdin = strings.NewReader("") // 出错时不要停下来等输入
		err := cmd.Run()
		return buf.String(), err
	}
	args := []string{"-interaction=nonstopmode", "-file-line-error", "-no-shell-escape", "-output-directory=" + outDir}
	if e.MiKTeX {
		args = append(args, "--enable-installer")
	}
	args = append(args, filepath.ToSlash(mainFile))
	var log string
	for pass := 1; pass <= 4; pass++ {
		_, err := run(eng, args...)
		if b, e2 := os.ReadFile(filepath.Join(outDir, stem+".log")); e2 == nil {
			log = string(b)
		}
		if ctx.Err() != nil {
			res.Errors = append(res.Errors, texIssue{Msg: "编译超时（超过 " + itoa(int(timeout.Seconds())) + " 秒），已停止", Hint: "文档可能太大，或 MiKTeX 正在等待安装宏包"})
			break
		}
		if pass == 1 {
			aux, _ := os.ReadFile(filepath.Join(outDir, stem+".aux"))
			if _, err := os.Stat(filepath.Join(outDir, stem+".bcf")); err == nil && e.Biber != "" {
				run(e.Biber, "--input-directory="+outDir, "--output-directory="+outDir, stem)
				continue
			}
			if bytes.Contains(aux, []byte(`\bibdata`)) && e.BibTeX != "" {
				// BibTeX 需要在输出目录中运行，同时能找到源文件目录里的 .bib
				cmd := exec.CommandContext(ctx, e.BibTeX, stem)
				cmd.Dir = outDir
				cmd.Env = append(os.Environ(), "BIBINPUTS="+workDir+string(os.PathListSeparator), "BSTINPUTS="+workDir+string(os.PathListSeparator))
				hideConsole(cmd)
				cmd.Run()
				continue
			}
		}
		if err != nil || !strings.Contains(log, "Rerun to get") && !strings.Contains(log, "Rerun LaTeX") && !(pass == 1 && strings.Contains(log, "undefined references")) {
			break
		}
	}
	res.Seconds = float64(int(time.Since(start).Seconds()*10)) / 10
	errs, warns, pages := parseTeXLog(log, e)
	res.Errors = append(res.Errors, errs...)
	res.Warnings = warns
	res.Pages = pages
	if pdf, err := os.ReadFile(filepath.Join(outDir, stem+".pdf")); err == nil && bytes.HasPrefix(pdf, []byte("%PDF")) {
		res.pdf = pdf
		res.OK = len(errs) == 0
	}
	if len(log) > 4000 {
		res.LogTail = log[len(log)-4000:]
	} else {
		res.LogTail = log
	}
	if res.pdf == nil && len(res.Errors) == 0 {
		res.Errors = append(res.Errors, texIssue{Msg: "没有生成 PDF", Hint: "请查看下面的日志"})
	}
	return res
}

// ---------------- PDF 暂存 ----------------

type storedPDF struct {
	data  []byte
	name  string
	owner int
	at    time.Time
}

var (
	pdfMu    sync.Mutex
	pdfStore = map[string]storedPDF{}
)

func keepPDF(owner int, name string, data []byte) string {
	pdfMu.Lock()
	defer pdfMu.Unlock()
	for k, v := range pdfStore {
		if time.Since(v.at) > 30*time.Minute {
			delete(pdfStore, k)
		}
	}
	id := newID()
	pdfStore[id] = storedPDF{data: data, name: name, owner: owner, at: time.Now()}
	return id
}

func getPDF(owner int, id string) (storedPDF, bool) {
	pdfMu.Lock()
	defer pdfMu.Unlock()
	p, ok := pdfStore[id]
	return p, ok && p.owner == owner
}

// ---------------- 片段包装 ----------------

// wrapLatex 把片段包装成可编译的文档。kind：formula 公式 / tikz 图 / 其他（表格、段落）。
func wrapLatex(src, kind string) string {
	src = strings.TrimSpace(src)
	if strings.Contains(src, `\documentclass`) {
		return src
	}
	cjk := needsXe(src)
	pkgs := `\usepackage{amsmath,amssymb,bm,booktabs,multirow,graphicx,xcolor}
\usepackage{tikz,pgfplots}
\usetikzlibrary{arrows.meta,positioning,shapes,calc,decorations.pathreplacing,patterns}
\pgfplotsset{compat=1.16}
`
	if strings.Contains(src, "circuitikz") || strings.Contains(src, `\draw (`) && strings.Contains(src, "to[") {
		pkgs += `\usepackage{circuitikz}` + "\n"
	}
	if kind == "tikz" || strings.HasPrefix(src, `\begin{tikzpicture}`) || strings.HasPrefix(src, `\begin{circuitikz}`) {
		head := `\documentclass[border=6pt]{standalone}` + "\n"
		if cjk {
			head += `\usepackage[UTF8]{ctex}` + "\n"
		}
		return head + pkgs + "\\begin{document}\n" + src + "\n\\end{document}\n"
	}
	head := `\documentclass{article}` + "\n"
	if cjk {
		head = `\documentclass[UTF8]{ctexart}` + "\n"
	}
	head += `\usepackage[margin=2.2cm]{geometry}` + "\n" + pkgs
	body := src
	if kind == "formula" && !strings.Contains(src, `\begin{`) && !strings.Contains(src, "$") && !strings.Contains(src, `\[`) {
		body = "\\[\n" + src + "\n\\]"
	}
	return head + "\\pagestyle{empty}\n\\begin{document}\n" + body + "\n\\end{document}\n"
}

// ---------------- 接口 ----------------

func texInstallHelp() map[string]string {
	return map[string]string{
		"texlive":       "https://mirrors.tuna.tsinghua.edu.cn/CTAN/systems/texlive/Images/",
		"miktex":        "https://miktex.org/download",
		"note":          "最省事：在“AI 助手 → 本机智能体 → 设置”或导出 PDF 的提示里点“一键下载便携版 Tectonic”（约 20 MB，不用安装，第一次编译自动下载宏包）；需要完整环境时安装 TeX Live（清华镜像下载 ISO，双击 install-tl-windows.bat，约 30–60 分钟）或 MiKTeX。装好后点“重新检测”。",
		"tectonic_dir":  tectonicDir(),
		"tectonic_page": tectonicPage,
	}
}

func (a *App) hTeXStatus(w http.ResponseWriter, r *http.Request, me *Me) error {
	e := findTeX(r.URL.Query().Get("refresh") == "1")
	writeJSON(w, 200, map[string]any{"engine": e, "local": isLoopback(r), "help": texInstallHelp()})
	return nil
}

func (a *App) hTeXCompile(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Source string `json:"source"`
		Kind   string `json:"kind"`
		Name   string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Source) == "" {
		return errBad("没有要编译的内容")
	}
	if len(in.Source) > 2<<20 {
		return errBad("内容太长")
	}
	e := findTeX(false)
	if !e.Found {
		return errBad("这台电脑还没有安装 LaTeX（TeX Live 或 MiKTeX），请先安装，见“AI 助手 → 导出 PDF”里的说明")
	}
	dir, err := os.MkdirTemp(texTempBase(), "kyws-tex-")
	if err != nil {
		return errBad("无法创建临时文件夹")
	}
	defer os.RemoveAll(dir)
	doc := wrapLatex(in.Source, in.Kind)
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte(doc), 0o600)
	res := runTeX(e, dir, "main.tex", dir, false, 90*time.Second)
	if res.pdf != nil {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "latex"
		}
		res.PDFID = keepPDF(me.ID, safeName(name)+".pdf", res.pdf)
	}
	writeJSON(w, 200, map[string]any{"result": res, "document": doc})
	return nil
}

func (a *App) hTeXPDF(w http.ResponseWriter, r *http.Request, me *Me) error {
	p, ok := getPDF(me.ID, r.PathValue("id"))
	if !ok {
		return errNotFound("PDF 已过期（只保存 30 分钟），请重新编译")
	}
	w.Header().Set("Content-Type", "application/pdf")
	disp := "inline"
	if r.URL.Query().Get("download") == "1" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", disp+`; filename="document.pdf"; filename*=UTF-8''`+urlPathEscape(p.name))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(p.data)
	return nil
}

func urlPathEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte("-_.~", c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteString("%" + strings.ToUpper(hex2(c)))
		}
	}
	return b.String()
}

func hex2(c byte) string {
	const h = "0123456789abcdef"
	return string([]byte{h[c>>4], h[c&15]})
}

// texTempBase：Windows 用户名是中文或含空格时，部分 TeX 发行版处理临时路径会出错，改用系统盘根目录下的短路径。
func texTempBase() string {
	t := os.TempDir()
	if runtime.GOOS != "windows" {
		return ""
	}
	ascii := true
	for _, r := range t {
		if r > 127 || r == ' ' {
			ascii = false
			break
		}
	}
	if ascii {
		return ""
	}
	d := os.Getenv("SystemDrive")
	if d == "" {
		d = "C:"
	}
	p := d + `\kyws-tex`
	if os.MkdirAll(p, 0o700) != nil {
		return ""
	}
	return p
}
