package main

import (
	"strings"
	"testing"
)

func TestOCR(t *testing.T) {
	_, srv := newEnv(t)
	tc, s1, _, _, _ := setupTeam(t, srv)
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k", "llm_vision": true})
	var gotSys string
	chatVision = func(c ModelCfg, system, text string, imgs []chatImage) (string, error) {
		gotSys = system
		return "```\n第一章 匀速运动\n\n物体以恒定速度运动时，路程等于速度乘以时间，例如 $s=vt$。\n```", nil
	}
	defer func() {
		chatVision = func(c ModelCfg, s, x string, i []chatImage) (string, error) { return "", ErrLLMUnavailable }
	}()

	// 识别一页
	r := tc.ok("POST", "/api/ocr/page", map[string]any{"image": tinyPNG, "lang": "zh"})
	if !strings.Contains(gotSys, "OCR") || strings.Contains(r["text"].(string), "```") || !strings.Contains(r["text"].(string), "s=vt") {
		t.Fatalf("OCR 结果不对 %v", r)
	}

	// 扫描件：全部页没有文字 → 失败 → OCR 后可用
	f := tc.upload("scan.pdf", []byte("%PDF-1.4 fake"), "", []PDFPage{{PageIndex: 1}, {PageIndex: 2}})
	if f["status"] != "failed" || !strings.Contains(f["error"].(string), "OCR 识别") {
		t.Fatalf("扫描件应提示可以 OCR：%v", f)
	}
	id := f["id"].(string)
	page := func(i int, txt string) map[string]any { return map[string]any{"page_index": i, "text": txt} }
	m := tc.ok("POST", "/api/materials/"+id+"/ocr", map[string]any{"total_pages": 2, "model": "千问VL", "pages": []any{page(1, r["text"].(string))}})
	if m["status"] != "partial" || !strings.Contains(m["parse_note"].(string), "第 1 页的文字由 AI 识别") || !strings.Contains(m["parse_note"].(string), "第 2 页没有文字") {
		t.Fatalf("只识别了 1 页应为部分可用：%v", m)
	}
	cs := tc.ok("GET", "/api/materials/"+id+"/chunks", nil)["list"].([]any)
	if len(cs) == 0 || !strings.Contains(cs[0].(map[string]any)["location"].(string), "OCR 识别") {
		t.Fatalf("片段应标注 OCR：%v", cs)
	}
	first := cs[0].(map[string]any)["chunk_id"]
	// 重复提交第 1 页被忽略；补上第 2 页 → 可用，原片段编号不变
	m = tc.ok("POST", "/api/materials/"+id+"/ocr", map[string]any{"total_pages": 2, "pages": []any{page(1, "重复的内容重复的内容重复的内容"), page(2, "第二页：结论是路程与时间成正比。")}})
	if m["status"] != "ready" || len(m["ocr_pages"].([]any)) != 1 {
		t.Fatalf("补齐后应可用，只新增第 2 页：%v", m)
	}
	cs2 := tc.ok("GET", "/api/materials/"+id+"/chunks", nil)["list"].([]any)
	if cs2[0].(map[string]any)["chunk_id"] != first || len(cs2) != len(cs)+1 {
		t.Fatal("原有片段编号应保持不变，只追加新页")
	}
	// 问答能用到 OCR 文字
	found := false
	for _, c := range cs2 {
		found = found || strings.Contains(c.(map[string]any)["text"].(string), "成正比")
	}
	if !found {
		t.Fatal("应能检索到 OCR 文字")
	}
	// 别人不能给我的个人资料做 OCR；非 PDF 不需要；页码越界忽略
	if code, _ := s1.do("POST", "/api/materials/"+id+"/ocr", map[string]any{"total_pages": 2, "pages": []any{}}); code != 404 {
		t.Fatal("他人资料应不可见")
	}
	txt := tc.upload("a.txt", []byte("这是一段足够长的文字，用于测试文本资料。"), "", nil)
	if code, _ := tc.do("POST", "/api/materials/"+txt["id"].(string)+"/ocr", map[string]any{"total_pages": 1}); code != 400 {
		t.Fatal("非 PDF 应拒绝")
	}
	// 没有识图模型时给出清楚的提示
	tc.ok("PUT", "/api/settings", map[string]any{"llm_vision": false})
	if code, body := tc.do("POST", "/api/ocr/page", map[string]any{"image": tinyPNG}); code != 400 || !strings.Contains(body["detail"].(string), "能看图片") {
		t.Fatalf("无识图模型应提示：%d %v", code, body)
	}
}

func TestCleanOCR(t *testing.T) {
	if cleanOCR("（空白页）") != "" || cleanOCR("```\nabc\n```") != "abc" {
		t.Fatal("cleanOCR")
	}
}
