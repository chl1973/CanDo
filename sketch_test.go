package main

import (
	"strings"
	"testing"
)

func TestParseSketchAndSanitize(t *testing.T) {
	out := "===意图===\n一张两步流程图\n===TIKZ===\n```latex\n\\begin{tikzpicture}\\node (a) {开始};\\end{tikzpicture}\n```\n===SVG===\n<svg width=\"200\" onload=\"alert(1)\"><script>alert(2)</script><rect x=\"1\" onclick='x()'/><a href=\"http://evil\"><text>开始</text></a><image href=\"http://x/y.png\"/></svg>\n===说明===\n无\n"
	o := parseSketch(out)
	if o.Intent != "一张两步流程图" || !strings.HasPrefix(o.TikZ, `\begin{tikzpicture}`) || o.Note != "" {
		t.Fatalf("解析不对 %+v", o)
	}
	for _, bad := range []string{"script", "onload", "onclick", "http://evil", "<image"} {
		if strings.Contains(o.SVG, bad) {
			t.Fatalf("SVG 应清除 %s：%s", bad, o.SVG)
		}
	}
	if !strings.Contains(o.SVG, "xmlns=") || !strings.Contains(o.SVG, "<rect") {
		t.Fatalf("SVG 图形应保留 %s", o.SVG)
	}
	if sanitizeSVG("<div>not svg</div>") != "" {
		t.Fatal("不是 SVG 应丢弃")
	}
	// 模型没按格式输出时尽量找出代码
	o = parseSketch("好的：\\begin{tikzpicture}\\draw (0,0)--(1,1);\\end{tikzpicture}")
	if o.TikZ == "" {
		t.Fatal("应找出 tikzpicture")
	}
}

func TestSketchAPI(t *testing.T) {
	defer func() {
		chatVision = func(c ModelCfg, s, x string, i []chatImage) (string, error) { return "", ErrLLMUnavailable }
	}()
	_, srv := newEnv(t)
	tc, _, _, _, _ := setupTeam(t, srv)
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k", "llm_vision": true})
	var gotPrompt string
	chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) {
		gotPrompt = text
		return "===意图===\n函数 y=x^2 的图像\n===TIKZ===\n\\begin{tikzpicture}\\draw[->](-2,0)--(2,0);\\draw plot (\\x,{\\x*\\x});\\end{tikzpicture}\n===SVG===\n<svg viewBox=\"0 0 10 10\"><path d=\"M0 0 L5 5\"/></svg>\n===说明===\n把抖动的曲线理解为抛物线\n", nil
	}
	r := tc.ok("POST", "/api/sketch", map[string]any{"image": tinyPNG, "target": "function", "note": "画 y=x^2"})
	if !strings.Contains(r["intent"].(string), "y=x^2") || !strings.Contains(r["tikz"].(string), "plot") || !strings.Contains(r["note"].(string), "抛物线") {
		t.Fatalf("识别结果不对 %v", r)
	}
	if !strings.Contains(gotPrompt, "pgfplots") || !strings.Contains(gotPrompt, "画 y=x^2") {
		t.Fatal("应带上类型提示和补充说明")
	}
	chatRaw = func(c ModelCfg, system string, msgs []chatMsg, n int) (string, error) {
		if !strings.Contains(msgs[0].Text, "改成红色") {
			t.Fatal("应带上修改要求")
		}
		return "===TIKZ===\n\\begin{tikzpicture}\\draw[red] (0,0)--(1,1);\\end{tikzpicture}\n===SVG===\n<svg></svg>\n", nil
	}
	defer func() { chatRaw = realChatRaw }()
	r = tc.ok("POST", "/api/sketch/refine", map[string]any{"intent": "函数图像", "tikz": "\\begin{tikzpicture}\\end{tikzpicture}", "instruction": "曲线改成红色"})
	if !strings.Contains(r["tikz"].(string), "red") || r["intent"] != "函数图像" {
		t.Fatalf("调整结果不对 %v", r)
	}
}
