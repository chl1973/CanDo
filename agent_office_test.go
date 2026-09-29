package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func copyFixture(t *testing.T, name, dst string) {
	b, err := os.ReadFile(filepath.Join("testdata", "office", name))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dst, b, 0o644)
}

func TestAgentOffice(t *testing.T) {
	e := newAgentEnv(t)
	W := e.W
	copyFixture(t, "report.docx", filepath.Join(W, "report.docx"))
	copyFixture(t, "data.xlsx", filepath.Join(W, "data.xlsx"))
	copyFixture(t, "slides.pptx", filepath.Join(W, "slides.pptx"))
	os.WriteFile(filepath.Join(W, "old.doc"), []byte{0xD0, 0xCF, 0x11, 0xE0}, 0o644)
	copyFixture(t, "report.docx", filepath.Join(e.R, "ro.docx"))
	docx := filepath.Join(W, "report.docx")
	r := e.run([]map[string]any{
		{"tool": "read_file", "args": map[string]any{"path": docx}},
		{"tool": "read_file", "args": map[string]any{"path": filepath.Join(W, "data.xlsx")}},
		{"tool": "read_file", "args": map[string]any{"path": filepath.Join(W, "slides.pptx")}},
		{"tool": "read_file", "args": map[string]any{"path": filepath.Join(W, "old.doc")}},
		{"tool": "docx_format", "args": map[string]any{"path": docx}},
		{"tool": "docx_edit", "args": map[string]any{"path": docx, "reason": "改摘要", "edits": []any{
			map[string]any{"op": "replace", "index": 1, "text": "摘要：本文用两周干预实验研究短视频与注意力。"},
			map[string]any{"op": "insert_after", "index": 3, "text": "补充的一段。"}}}},
		// 第二次修改在第一次的基础上（编号按修改后的版本）
		{"tool": "read_file", "args": map[string]any{"path": docx}},
		{"tool": "docx_edit", "args": map[string]any{"path": docx, "edits": []any{map[string]any{"op": "delete", "index": 5}}}},
		{"tool": "docx_edit", "args": map[string]any{"path": docx, "edits": []any{map[string]any{"op": "replace", "index": 5, "text": "x"}}}},            // 含图片的段落（前两次修改后是第 5 段）
		{"tool": "docx_edit", "args": map[string]any{"path": filepath.Join(e.R, "ro.docx"), "edits": []any{map[string]any{"op": "delete", "index": 0}}}}, // 只读
		{"tool": "make_docx", "args": map[string]any{"path": filepath.Join(W, "新报告.docx"), "content": "# 实验报告\n这是**正文**。\n\n| a | b |\n|---|---|\n| 1 | 2 |", "reason": "新建"}},
		{"tool": "make_docx", "args": map[string]any{"path": docx, "content": "# 覆盖"}},
		{"tool": "docx_images", "args": map[string]any{"path": docx}},
		{"tool": "docx_images", "args": map[string]any{"path": docx, "out_dir": filepath.Join(W, "图片"), "reason": "导出图片"}},
		{"tool": "search_web", "args": map[string]any{"query": "数学建模 国赛 论文格式"}},
	})
	res := resultTexts(r)
	must := func(i int, subs ...string) {
		for _, s := range subs {
			if !strings.Contains(res[i], s) {
				t.Fatalf("第 %d 步结果缺少 %q：\n%s", i, s, res[i])
			}
		}
	}
	must(0, "[1] (正文) 摘要：", "表格 1：2 行 × 3 列", "［含图片］")
	must(1, "=== 工作表：数据 ===", "2│张三\t88\tTRUE")
	must(2, "第 1 页", "开题报告")
	must(3, "失败", "另存为 .docx")
	must(4, "1 级标题", "图片 1 张")
	must(5, "已提交", "2 处修改")
	must(6, "包含待确认修改的版本", "[1] (正文) 摘要：本文用两周干预实验", "[4] (正文) 补充的一段。")
	must(7, "已提交")
	must(8, "失败", "含图片")
	must(9, "失败")
	must(10, "已生成 Word")
	must(11, "失败", "文件已存在")
	must(12, "1 张图片：image1.png")
	must(13, "已提交", "1 张图片")
	must(14, "失败", "设置 → 联网搜索")
	chs := r["changes"].([]any)
	byKind := map[string][]map[string]any{}
	for _, c := range chs {
		m := c.(map[string]any)
		byKind[m["kind"].(string)+"/"+str(m["sub"])+"/"+m["status"].(string)] = append(byKind[m["kind"].(string)+"/"+str(m["sub"])+"/"+m["status"].(string)], m)
	}
	if len(byKind["office/modify/rejected"]) != 1 || len(byKind["office/modify/pending"]) != 1 || len(byKind["office/create/pending"]) != 1 || len(byKind["files//pending"]) != 1 {
		t.Fatalf("修改建议不对：%v", byKind)
	}
	sid := r["_sid"].(string)
	orig, _ := os.ReadFile(docx)
	mod := byKind["office/modify/pending"][0]
	if !strings.Contains(mod["diff"].([]any)[0].(map[string]any)["text"].(string), "[1]") {
		t.Fatalf("合并后的改动说明应包含第一次的修改：%v", mod["diff"])
	}
	// 确认前没有写入
	if _, err := os.Stat(filepath.Join(W, "新报告.docx")); err == nil {
		t.Fatal("确认前不应写入")
	}
	for _, c := range []map[string]any{mod, byKind["office/create/pending"][0], byKind["files//pending"][0]} {
		e.tc.ok("POST", "/api/agent/sessions/"+sid+"/changes/"+c["id"].(string)+"/apply", nil)
	}
	b, _ := os.ReadFile(docx)
	d, err := parseDocxDoc(b)
	if err != nil {
		t.Fatal(err)
	}
	var ts []string
	for _, p := range d.paras {
		ts = append(ts, p.Text)
	}
	j := strings.Join(ts, "|")
	if !strings.Contains(j, "摘要：本文用两周干预实验") || !strings.Contains(j, "补充的一段。") || strings.Contains(j, "第二段：现有研究") || len(d.images) != 1 {
		t.Fatalf("两次修改都应写入，图片保留：%s", j)
	}
	nb, _ := os.ReadFile(filepath.Join(W, "新报告.docx"))
	if nd, err := parseDocxDoc(nb); err != nil || len(nd.tables) != 1 {
		t.Fatal("新建的 Word 应有效且有表格")
	}
	if _, err := os.Stat(filepath.Join(W, "图片", "image1.png")); err != nil {
		t.Fatal("图片应保存到文件夹")
	}
	// 撤销：恢复原文件、删除新建的文件
	for _, c := range []map[string]any{mod, byKind["office/create/pending"][0], byKind["files//pending"][0]} {
		e.tc.ok("POST", "/api/agent/sessions/"+sid+"/changes/"+c["id"].(string)+"/undo", nil)
	}
	if b2, _ := os.ReadFile(docx); string(b2) != string(orig) {
		t.Fatal("撤销后应恢复原来的 Word")
	}
	for _, p := range []string{filepath.Join(W, "新报告.docx"), filepath.Join(W, "图片")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("撤销后应删除 %s", p)
		}
	}
	// 冲突：提交修改后用户又在 Word 里改了文件，不覆盖
	r = e.run([]map[string]any{{"tool": "docx_edit", "args": map[string]any{"path": docx, "edits": []any{map[string]any{"op": "insert_after", "index": 0, "text": "新段"}}}}})
	c := r["changes"].([]any)
	last := c[len(c)-1].(map[string]any)
	os.WriteFile(docx, append(orig, ' '), 0o644)
	if code, _ := e.tc.do("POST", "/api/agent/sessions/"+r["_sid"].(string)+"/changes/"+last["id"].(string)+"/apply", nil); code != 400 {
		t.Fatal("文件被改动过时不应覆盖")
	}
}

func TestAgentWebSearchAndImagePath(t *testing.T) {
	e := newAgentEnv(t)
	var mu sync.Mutex
	var gotAuth, gotQuery string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		gotAuth, gotQuery = r.Header.Get("Authorization"), str(in["query"])
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"webPages": map[string]any{"value": []any{
			map[string]any{"name": "2026 年数学建模竞赛论文格式规范", "url": "https://example.edu/mcm", "snippet": "正文不超过 30 页", "summary": "论文格式规范：正文不超过 30 页，摘要一页。", "siteName": "竞赛组委会"},
		}}}})
	}))
	defer fake.Close()
	old := bochaURL
	bochaURL = fake.URL
	defer func() { bochaURL = old }()
	if code, body := e.tc.do("PUT", "/api/websearch/key", map[string]any{"scope": "me", "provider": "bocha", "key": "bad-key"}); code != 400 || !strings.Contains(str(body["detail"]), "无效") {
		t.Fatalf("无效密钥应拒绝：%v", body)
	}
	st := e.tc.ok("PUT", "/api/websearch/key", map[string]any{"scope": "me", "provider": "bocha", "key": "good-key"})
	if st["using"] != "mine" || strings.Contains(str(st["mine"]), "good-key") {
		t.Fatalf("应使用我的密钥且不回显：%v", st)
	}
	// 管理员的团队密钥：学生没有自己的密钥时用团队的
	// 图片路径
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	f, _ := os.Create(filepath.Join(e.W, "fig.png"))
	png.Encode(f, img)
	f.Close()
	e.tc.ok("PUT", "/api/settings", map[string]any{"llm_vision": true})
	var visN int
	chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) {
		visN = len(imgs)
		return "一张红点小图。", nil
	}
	defer func() {
		chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) { return "", ErrLLMUnavailable }
	}()
	r := e.run([]map[string]any{
		{"tool": "search_web", "args": map[string]any{"query": "数学建模 论文格式"}},
		{"tool": "recognize_image", "args": map[string]any{"path": filepath.Join(e.W, "fig.png"), "task": "describe", "instruction": "图里是什么"}},
		{"tool": "read_file", "args": map[string]any{"path": filepath.Join(e.W, "fig.png")}},
	})
	res := resultTexts(r)
	if !strings.Contains(res[0], "论文格式规范：正文不超过 30 页") || !strings.Contains(res[0], "https://example.edu/mcm") || !strings.Contains(res[0], "核对") {
		t.Fatalf("搜索结果不对：%s", res[0])
	}
	if gotQuery != "数学建模 论文格式" || gotAuth != "Bearer good-key" {
		t.Fatal("应带上密钥和查询词")
	}
	if !strings.Contains(res[1], "红点") || visN != 1 {
		t.Fatalf("应能看授权文件夹里的图片：%s", res[1])
	}
	if !strings.Contains(res[2], "recognize_image") {
		t.Fatalf("读图片时应提示用 recognize_image：%s", res[2])
	}
}

func TestAgentRescueToolMarkup(t *testing.T) {
	e := newAgentEnv(t)
	os.WriteFile(filepath.Join(e.W, "a.txt"), []byte("你好"), 0o644)
	chatConv = realChatConv
	var calls int
	var mu sync.Mutex
	chatRaw = func(c ModelCfg, system string, msgs []chatMsg, n int) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return "我来读一下文件。<｜DSML｜function_calls><｜DSML｜invoke name=\"read_file\"><｜DSML｜parameter name=\"path\" string=\"true\">" + filepath.Join(e.W, "a.txt") + "</｜DSML｜parameter></｜DSML｜invoke></｜DSML｜function_calls>", nil
		}
		return `{"reply":"文件内容是你好"}`, nil
	}
	defer func() { chatRaw = realChatRaw }()
	sid := e.tc.ok("POST", "/api/agent/sessions", map[string]any{})["id"].(string)
	e.tc.ok("POST", "/api/agent/sessions/"+sid+"/messages", map[string]any{"text": "读 a.txt"})
	r := waitAgent(t, e.tc, sid)
	res := resultTexts(r)
	if len(res) != 1 || !strings.Contains(res[0], "你好") || calls != 2 {
		t.Fatalf("应把 DSML 标记转成工具调用：%v %d", res, calls)
	}
	if m := rescueToolMarkup(`{"name":"list_dir","arguments":{"path":"D:\\a"}}`); m == nil || m["tool"] != "list_dir" {
		t.Fatal("应识别 name/arguments 形式")
	}
	if rescueToolMarkup(`<invoke name="format_disk"><parameter name="x">1</parameter></invoke>`) != nil {
		t.Fatal("未知工具不应抢救")
	}
}
