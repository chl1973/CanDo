package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTeXCompile(t *testing.T) {
	e := findTeX(true)
	if !e.Found {
		t.Skip("没有安装 LaTeX")
	}
	_, srv := newEnv(t)
	tc, _, _, _, _ := setupTeam(t, srv)
	// 公式片段（含中文）
	r := tc.ok("POST", "/api/latex/compile", map[string]any{"source": `\frac{a^2+b^2}{2} = c^2 \quad \text{（勾股）}`, "kind": "formula"})
	res := r["result"].(map[string]any)
	if res["ok"] != true || res["pdf_id"] == nil || !strings.Contains(res["engine"].(string), "XeLaTeX") {
		t.Fatalf("公式应编译成功 %v", res)
	}
	resp, _ := tc.hc.Get(srv.URL + "/api/latex/pdf/" + res["pdf_id"].(string))
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatal("应能取回 PDF")
	}
	resp.Body.Close()
	// TikZ 图
	r = tc.ok("POST", "/api/latex/compile", map[string]any{"source": "\\begin{tikzpicture}\\draw[->] (0,0) -- (2,1) node[right]{开始};\\end{tikzpicture}", "kind": "tikz"})
	if r["result"].(map[string]any)["ok"] != true || !strings.Contains(r["document"].(string), "standalone") {
		t.Fatalf("TikZ 应编译成功 %v", r["result"])
	}
	// 错误：给出行号和中文说明
	r = tc.ok("POST", "/api/latex/compile", map[string]any{"source": "\\documentclass{article}\n\\begin{document}\nHello \\foo{x}\n$x^2\n\\end{document}\n"})
	res = r["result"].(map[string]any)
	errs := res["errors"].([]any)
	if res["ok"] == true || len(errs) == 0 {
		t.Fatalf("应报告错误 %v", res)
	}
	e0 := errs[0].(map[string]any)
	if e0["line"].(float64) != 3 || !strings.Contains(e0["hint"].(string), "拼错") {
		t.Fatalf("错误说明不对 %v", e0)
	}
	// 缺少宏包
	r = tc.ok("POST", "/api/latex/compile", map[string]any{"source": "\\documentclass{article}\n\\usepackage{nosuchpkgxyz}\n\\begin{document}x\\end{document}\n"})
	if es := r["result"].(map[string]any)["errors"].([]any); len(es) == 0 || !strings.Contains(es[0].(map[string]any)["hint"].(string), "nosuchpkgxyz") {
		t.Fatalf("缺少宏包应说明 %v", r["result"])
	}
}

func TestRunTeXBib(t *testing.T) {
	e := findTeX(false)
	if !e.Found || e.BibTeX == "" {
		t.Skip("没有 BibTeX")
	}
	dir := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(dir, "refs.bib"), []byte("@article{smith2020,author={Smith, John},title={Rivers},journal={Water},year={2020}}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "paper.tex"), []byte("\\documentclass{article}\n\\begin{document}\nSee \\cite{smith2020} and \\ref{nolabel}.\n\\bibliographystyle{plain}\n\\bibliography{refs}\n\\end{document}\n"), 0o644)
	res := runTeX(e, dir, "paper.tex", out, false, 90*time.Second)
	if res.pdf == nil {
		t.Fatalf("应生成 PDF %+v", res.Errors)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "smith2020") {
			t.Fatal("BibTeX 应解析引用")
		}
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "nolabel") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应提示未定义的标签 %+v", res.Warnings)
	}
	if _, err := os.Stat(filepath.Join(dir, "paper.aux")); err == nil {
		t.Fatal("中间文件不应写到源文件夹")
	}
}
