package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLintLatex(t *testing.T) {
	good := "\\documentclass{article}\n\\begin{document}\n% 注释里的 { 和 $ 不算\n价格 100\\% 左右，$a^2+b^2=c^2$。\n\\begin{equation}\\label{eq:1}\n\\left( x \\right) \\rightarrow y\n\\end{equation}\n见式~\\eqref{eq:1}。\n\\begin{verbatim}\n{ $ \\begin{x}\n\\end{verbatim}\n\\end{document}\n"
	if w := LintLatex(good, false); len(w) != 0 {
		t.Fatalf("正常文档不应报错：%v", w)
	}
	cases := map[string]string{
		"\\begin{table}\n\\begin{tabular}{cc}\n\\end{table}\n": "不匹配",
		"\\textbf{abc\n":           "没有闭合",
		"a $x+y 的值\n\n下一段\n":       "没有配对",
		"$\\left( x$\n":            "\\left 与 \\right",
		"\\label{a}\n\\label{a}\n": "重复",
		"见 \\ref{fig:none}\n":      "没有定义",
		"\\end{itemize}\n":         "前面没有对应",
		"\\[ x \n":                 "没有对应的 \\]",
	}
	for src, want := range cases {
		w := strings.Join(LintLatex(src, false), "；")
		if !strings.Contains(w, want) {
			t.Fatalf("%q 应报告“%s”，得到：%s", src, want, w)
		}
	}
	if w := newLatexWarnings("\\label{a}\n\\label{a}\n", "\\label{a}\n\\label{a}\nok\n"); len(w) != 0 {
		t.Fatalf("原来就有的问题不算新问题：%v", w)
	}
}

func TestLineDiff(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"
	b := "1\n2\n3\n4\n5\n6\nsix\n8\n9\n10\n11\n12\n13"
	d := lineDiff(a, b)
	var ops string
	for _, x := range d {
		ops += x.Op
	}
	if ops != "~   -+     +" {
		t.Fatalf("差异不对：%q %+v", ops, d)
	}
}

func TestRepairLatexEscapes(t *testing.T) {
	cases := map[string]string{
		"\x0crac{a}{b}":        `\frac{a}{b}`,
		"\textbf{x} \theta":    `\textbf{x} \theta`,
		"a\newline b":          `a\newline b`,
		"\right)":              `\right)`,
		"第一行\ne.g. 第二行":        "第一行\ne.g. 第二行",
		"\tindented\n\\alpha":  "\tindented\n\\alpha",
		"x \x08egin{equation}": `x \begin{equation}`,
	}
	for in, want := range cases {
		if got := repairLatexEscapes(in); got != want {
			t.Fatalf("%q → %q，应为 %q", in, got, want)
		}
	}
}

var tinyPNG = "data:image/png;base64," + base64.StdEncoding.EncodeToString(selfcheckFormulaPNG)

func TestParseDataURL(t *testing.T) {
	if im, err := parseDataURL(tinyPNG); err != nil || im.Mime != "image/png" {
		t.Fatal(err)
	}
	if _, err := parseDataURL("data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(selfcheckFormulaPNG)); err == nil {
		t.Fatal("类型与内容不符应拒绝")
	}
	if _, err := parseDataURL("data:text/html;base64,PGh0bWw+"); err == nil {
		t.Fatal("非图片应拒绝")
	}
}

// scripted 按顺序返回预设的模型输出。
type scripted struct {
	mu    sync.Mutex
	steps []map[string]any
	seen  [][]chatMsg
}

func (s *scripted) fn(c ModelCfg, system string, msgs []chatMsg) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, append([]chatMsg(nil), msgs...))
	if len(s.steps) == 0 {
		return map[string]any{"reply": "完成"}, nil
	}
	x := s.steps[0]
	s.steps = s.steps[1:]
	return x, nil
}

func waitAgent(t *testing.T, c *client, sid string) map[string]any {
	for i := 0; i < 200; i++ {
		r := c.ok("GET", "/api/agent/sessions/"+sid, nil)
		if r["running"] == false {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("助手没有结束")
	return nil
}

func stepsText(r map[string]any) string {
	var b strings.Builder
	for _, x := range r["steps"].([]any) {
		st := x.(map[string]any)
		b.WriteString(st["kind"].(string) + ":" + st["text"].(string) + "\n")
		if d, ok := st["detail"].(string); ok {
			b.WriteString(d + "\n")
		}
	}
	return b.String()
}

func TestAgent(t *testing.T) {
	app, srv := newEnv(t)
	tc, s1, _, _, _ := setupTeam(t, srv)
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k"})
	defer func() {
		chatConv = func(c ModelCfg, system string, msgs []chatMsg) (map[string]any, error) { return nil, ErrLLMUnavailable }
	}()

	base := t.TempDir()
	W, R, O := filepath.Join(base, "论文"), filepath.Join(base, "参考"), filepath.Join(base, "其他")
	for _, d := range []string{W, R, O, filepath.Join(W, "chapters")} {
		os.MkdirAll(d, 0o755)
	}
	tex := "\\documentclass{article}\n\\begin{document}\n结果为 $\\frac{a}{b}$。\n\\end{document}\n"
	os.WriteFile(filepath.Join(W, "main.tex"), []byte(tex), 0o644)
	os.WriteFile(filepath.Join(W, "chapters", "intro.tex"), []byte("引言：短视频与注意力\n"), 0o644)
	os.WriteFile(filepath.Join(W, ".env"), []byte("API_KEY=xyz"), 0o644)
	os.WriteFile(filepath.Join(R, "ref.bib"), []byte("@article{a,title={X}}\n"), 0o644)
	os.WriteFile(filepath.Join(O, "secret.txt"), []byte("外面的文件"), 0o644)
	os.Symlink(O, filepath.Join(W, "link"))

	// 授权文件夹
	for _, bad := range []string{"/", app.store.dir, "/etc", "relative/path", filepath.Join(base, "不存在")} {
		if code, _ := tc.do("PUT", "/api/agent/folders", map[string]any{"folders": []any{map[string]any{"path": bad, "write": true}}}); code != 400 {
			t.Fatalf("应拒绝授权 %s", bad)
		}
	}
	fs := tc.ok("PUT", "/api/agent/folders", map[string]any{"folders": []any{map[string]any{"path": W, "write": true}, map[string]any{"path": R}}})["list"].([]any)
	if len(fs) != 2 || fs[1].(map[string]any)["write"] != false {
		t.Fatalf("授权文件夹不对 %v", fs)
	}
	// 外部设备不能用
	tc.ok("PUT", "/api/settings", map[string]any{"lan_enabled": true})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agent/sessions", strings.NewReader("{}"))
	req.RemoteAddr = "192.168.1.20:5555"
	req.Header.Set("X-KY", "1")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: tc.hc.Jar.Cookies(mustURL(srv.URL))[0].Value})
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("外部设备不能使用本机文件助手：%d", rec.Code)
	}
	if st := tc.ok("GET", "/api/agent/status", nil); st["local"] != true || st["configured"] != true {
		t.Fatalf("状态不对 %v", st)
	}

	// 一次完整的对话：查找 → 读取 → 修改
	sc := &scripted{steps: []map[string]any{
		{"say": "先找一下 tex 文件", "tool": "search_files", "args": map[string]any{"path": W, "name": "*.tex"}},
		{"tool": "read_file", "args": map[string]any{"path": filepath.Join(W, "main.tex")}},
		{"tool": "edit_file", "args": map[string]any{"path": filepath.Join(W, "main.tex"), "old": "$\x0crac{a}{b}$", "new": "$\\frac{a}{c}$", "reason": "按批注把分母改为 c"}},
		{"reply": "已提交修改，请确认"},
	}}
	chatConv = sc.fn
	sid := tc.ok("POST", "/api/agent/sessions", map[string]any{})["id"].(string)
	tc.ok("POST", "/api/agent/sessions/"+sid+"/messages", map[string]any{"text": "把分母改成 c", "images": []string{tinyPNG}})
	r := waitAgent(t, tc, sid)
	txt := stepsText(r)
	if !strings.Contains(txt, "chapters/intro.tex") && !strings.Contains(txt, filepath.Join("chapters", "intro.tex")) || !strings.Contains(txt, "3│结果为") || !strings.Contains(txt, "reply:已提交修改") {
		t.Fatalf("步骤不对：\n%s", txt)
	}
	if !strings.Contains(sc.seen[0][0].Text, "图片1") {
		t.Fatal("应告诉模型有附图")
	}
	if !strings.Contains(sc.seen[2][len(sc.seen[2])-1].Text, "不是用户指令") {
		t.Fatal("工具结果应标明是数据")
	}
	chs := r["changes"].([]any)
	if len(chs) != 1 {
		t.Fatalf("应有 1 条待确认修改 %v", chs)
	}
	ch := chs[0].(map[string]any)
	if ch["status"] != "pending" || ch["added"].(float64) != 1 || ch["removed"].(float64) != 1 {
		t.Fatalf("修改建议不对 %v", ch)
	}
	if b, _ := os.ReadFile(filepath.Join(W, "main.tex")); string(b) != tex {
		t.Fatal("确认前不能写入文件")
	}
	cid := ch["id"].(string)
	tc.ok("POST", "/api/agent/sessions/"+sid+"/changes/"+cid+"/apply", nil)
	if b, _ := os.ReadFile(filepath.Join(W, "main.tex")); !strings.Contains(string(b), `\frac{a}{c}`) {
		t.Fatalf("应用后应写入：%s", b)
	}
	backups, _ := filepath.Glob(filepath.Join(app.store.dir, "agent_backups", "*", cid+"_main.tex"))
	if len(backups) != 1 {
		t.Fatal("应备份原文件")
	}
	tc.ok("POST", "/api/agent/sessions/"+sid+"/changes/"+cid+"/undo", nil)
	if b, _ := os.ReadFile(filepath.Join(W, "main.tex")); string(b) != tex {
		t.Fatal("撤销后应恢复原文件")
	}
	if code, _ := tc.do("POST", "/api/agent/sessions/"+sid+"/changes/"+cid+"/apply", nil); code != 400 {
		t.Fatal("已处理的修改不能再应用")
	}
	if code, _ := s1.do("GET", "/api/agent/sessions/"+sid, nil); code != 404 {
		t.Fatal("别人的对话不能看")
	}

	// 直接检查各种越界
	s := app.agents[sid]
	me := &Me{ID: 1, Role: "admin"}
	bad := map[string]map[string]any{
		"授权外":     {"tool": "read_file", "path": filepath.Join(O, "secret.txt")},
		"跳出":      {"tool": "read_file", "path": filepath.Join(W, "..", "其他", "secret.txt")},
		"链接逃逸":    {"tool": "read_file", "path": filepath.Join(W, "link", "secret.txt")},
		"密钥文件":    {"tool": "read_file", "path": filepath.Join(W, ".env")},
		"数据目录":    {"tool": "list_dir", "path": app.store.dir},
		"只读文件夹":   {"tool": "write_file", "path": filepath.Join(R, "new.tex"), "content": "x"},
		"可执行文件":   {"tool": "write_file", "path": filepath.Join(W, "run.bat"), "content": "del *"},
		"相对路径":    {"tool": "read_file", "path": "main.tex"},
		"old 不唯一": {"tool": "edit_file", "path": filepath.Join(W, "main.tex"), "old": "\\", "new": "x"},
		"old 不存在": {"tool": "edit_file", "path": filepath.Join(W, "main.tex"), "old": "没有这句", "new": "x"},
		"没有这张图":   {"tool": "recognize_image", "image": "图片9"},
		"未知工具":    {"tool": "delete_file", "path": filepath.Join(W, "main.tex")},
	}
	for name, args := range bad {
		if _, _, err := app.execToolErr(s, me, args["tool"].(string), args); err == nil {
			t.Fatalf("%s 应被拒绝", name)
		}
	}
	// 新建文件 → 应用 → 撤销（删除）
	res, cid2, err := app.execToolErr(s, me, "write_file", map[string]any{"path": filepath.Join(W, "chapters", "method.tex"), "content": "\\section{方法}\n\\begin{itemize}\n", "reason": "新建方法一节"})
	if err != nil || cid2 == "" || !strings.Contains(res, "没有对应的 \\end{itemize}") {
		t.Fatalf("新建文件建议不对：%v %s", err, res)
	}
	tc.ok("POST", "/api/agent/sessions/"+sid+"/changes/"+cid2+"/apply", nil)
	if _, err := os.Stat(filepath.Join(W, "chapters", "method.tex")); err != nil {
		t.Fatal("应新建文件")
	}
	tc.ok("POST", "/api/agent/sessions/"+sid+"/changes/"+cid2+"/undo", nil)
	if _, err := os.Stat(filepath.Join(W, "chapters", "method.tex")); err == nil {
		t.Fatal("撤销应删除新建的文件")
	}
	// 连续两次 edit 合并为一条修改；读取时看到待确认版本
	app.execToolErr(s, me, "edit_file", map[string]any{"path": filepath.Join(W, "main.tex"), "old": "结果为", "new": "计算结果为"})
	_, cid3, _ := app.execToolErr(s, me, "edit_file", map[string]any{"path": filepath.Join(W, "main.tex"), "old": "{a}{b}", "new": "{a}{d}"})
	if rd, _, _ := app.execToolErr(s, me, "read_file", map[string]any{"path": filepath.Join(W, "main.tex")}); !strings.Contains(rd, "计算结果为 $\\frac{a}{d}$") || !strings.Contains(rd, "待确认") {
		t.Fatalf("应读到待确认版本：%s", rd)
	}
	// 冲突：文件在提议之后被改动
	os.WriteFile(filepath.Join(W, "main.tex"), []byte(tex+"% 用户自己改了\n"), 0o644)
	if code, m := tc.do("POST", "/api/agent/sessions/"+sid+"/changes/"+cid3+"/apply", nil); code != 400 || !strings.Contains(m["detail"].(string), "改动过") {
		t.Fatalf("应检测到冲突 %d %v", code, m)
	}
	if b, _ := os.ReadFile(filepath.Join(W, "main.tex")); !strings.Contains(string(b), "用户自己改了") {
		t.Fatal("冲突时不能覆盖用户的修改")
	}
	// 取消授权后不能再应用
	tc.ok("PUT", "/api/agent/folders", map[string]any{"folders": []any{map[string]any{"path": R}}})
	if _, _, err := app.execToolErr(s, me, "read_file", map[string]any{"path": filepath.Join(W, "main.tex")}); err == nil {
		t.Fatal("取消授权后应拒绝")
	}
	logs := tc.ok("GET", "/api/agent/logs", nil)["list"].([]any)
	if len(logs) < 5 {
		t.Fatal("应记录操作")
	}
}

func TestLatexFromImage(t *testing.T) {
	_, srv := newEnv(t)
	tc, _, _, _, _ := setupTeam(t, srv)
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k"})
	var got []chatImage
	chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) {
		got = imgs
		if strings.Contains(text, "请把图片中的公式转写") {
			return "\\frac{a^{2}+b^{2}}{2} = c^{2}", nil
		}
		return "```latex\n\\begin{equation}\n\\frac{a^2+b^2}{2}=c^2\n\\end{equation}\n```", nil
	}
	if code, m := tc.do("POST", "/api/latex/from-image", map[string]any{"image": tinyPNG, "task": "formula"}); code != 400 || !strings.Contains(m["detail"].(string), "能看图片") {
		t.Fatal("没有识图模型时应说明")
	}
	tc.ok("PUT", "/api/settings", map[string]any{"llm_vision": true})
	r := tc.ok("POST", "/api/latex/from-image", map[string]any{"image": tinyPNG, "task": "auto"})
	if r["latex"] != "\\begin{equation}\n\\frac{a^2+b^2}{2}=c^2\n\\end{equation}" || len(got) != 1 || got[0].Mime != "image/png" {
		t.Fatalf("识别结果不对 %v", r)
	}
	// 个人识图模型：自检多一项“看图识别公式”
	mp := tc.ok("POST", "/api/models", map[string]any{"name": "千问VL", "base_url": "https://x/v1", "model": "qwen-vl-max", "key": "sk-1234567890", "vision": true})["mine"].([]any)[0].(map[string]any)
	ck := tc.ok("POST", "/api/models/"+mp["id"].(string)+"/check", nil)
	items := ck["items"].([]any)
	if ck["passed"] != true || items[len(items)-1].(map[string]any)["name"] != "看图识别公式" {
		t.Fatalf("识图自检不对 %v", ck)
	}
	chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) { return "x+y=1", nil }
	ck = tc.ok("POST", "/api/models/"+mp["id"].(string)+"/check", nil)
	if ck["passed"] != true || ck["warn"] != true {
		t.Fatalf("识图错误时应为“基本通过”（能用于文字任务）%v", ck)
	}
	tc.ok("PUT", "/api/settings", map[string]any{"llm_vision": false})
	tc.ok("POST", "/api/models/"+mp["id"].(string)+"/activate", map[string]any{"active": true})
	if code, m := tc.do("POST", "/api/latex/from-image", map[string]any{"image": tinyPNG, "task": "formula"}); code != 400 || !strings.Contains(m["detail"].(string), "能看图片") {
		t.Fatal("看图自检没通过的模型不能用于识图")
	}
	w := tc.ok("POST", "/api/latex/check", map[string]any{"text": "\\begin{align}a\n", "fragment": true})["warnings"].([]any)
	if len(w) == 0 {
		t.Fatal("应检查出问题")
	}
}
