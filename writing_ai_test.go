package main

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
)

func TestGroupShare(t *testing.T) {
	_, srv := newEnv(t)
	tc, s1, s2, _, _ := setupTeam(t, srv)
	m := s1.upload("attention.txt", []byte("短视频使用与大学生注意力。\n\n一项调查显示，每天刷短视频超过两小时的学生，注意力测验得分低 12%。"), "", nil)
	id := m["id"].(string)
	if code, _ := s2.do("GET", "/api/materials/"+id, nil); code != 404 {
		t.Fatal("未共享时别人不应看到")
	}
	if code, _ := s2.do("PATCH", "/api/materials/"+id, map[string]any{"shared": true}); code != 404 {
		t.Fatal("别人不能共享我的论文")
	}
	r := s1.ok("PATCH", "/api/materials/"+id, map[string]any{"shared": true})
	if r["shared"] != true {
		t.Fatal("应已共享")
	}
	// 全组可见，但不出现在别人的“我的论文”里
	g := s2.ok("GET", "/api/materials?scope=group", nil)["list"].([]any)
	if len(g) != 1 || g[0].(map[string]any)["uploader"] != "学生甲" {
		t.Fatalf("全组共享列表不对 %v", g)
	}
	if n := len(s2.ok("GET", "/api/materials", nil)["list"].([]any)); n != 0 {
		t.Fatal("别人的“我的论文”不应包含共享论文")
	}
	if n := len(s2.ok("GET", "/api/materials?scope=all", nil)["list"].([]any)); n != 1 {
		t.Fatal("scope=all 应包含共享论文")
	}
	s2.ok("GET", "/api/materials/"+id+"/chunks", nil)
	// 只读：别人不能改、删、OCR
	if code, _ := s2.do("PATCH", "/api/materials/"+id, map[string]any{"author": "x"}); code != 403 {
		t.Fatalf("别人不能修改：%d", code)
	}
	if code, _ := s2.do("DELETE", "/api/materials/"+id, nil); code != 403 {
		t.Fatal("别人不能删除")
	}
	if code, _ := s2.do("POST", "/api/materials/"+id+"/ocr", map[string]any{"total_pages": 1}); code != 403 {
		t.Fatal("别人不能 OCR")
	}
	// 管理员可以取消共享（管理）
	tc.ok("PATCH", "/api/materials/"+id, map[string]any{"shared": false})
	if code, _ := s2.do("GET", "/api/materials/"+id, nil); code != 404 {
		t.Fatal("取消共享后别人不应再看到")
	}
}

func TestAIDraft(t *testing.T) {
	_, srv := newEnv(t)
	tc, s1, s2, _, _ := setupTeam(t, srv)
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k"})
	m := s1.upload("attention.txt", []byte("短视频使用与大学生注意力。\n\n一项调查显示，每天刷短视频超过两小时的学生，注意力测验得分低 12%。\n\n算法推荐会延长单次使用时长。"), "", nil)
	s1.ok("PATCH", "/api/materials/"+m["id"].(string), map[string]any{"shared": true, "author": "张三；李四；王五；赵六", "source_date": "2023"})
	var planPrompt, draftPrompt string
	chatJSON = func(c ModelCfg, system, user string) (map[string]any, error) {
		switch {
		case strings.Contains(system, "论证骨架”（先想清楚"):
			planPrompt = user
			return map[string]any{"claims": []any{
				map[string]any{"role": "background", "text": "短视频使用时间长与注意力下降有关", "evidence": []any{"F1", "X9"}},
				map[string]any{"role": "finding", "text": "我们的实验发现干预两周后专注时长提高", "evidence": []any{"N1"}},
				map[string]any{"role": "boundary", "text": "样本只来自一所学校", "evidence": []any{}},
			}, "missing": []any{"需要样本量"}, "advice": "先写缺口"}, nil
		case strings.Contains(system, "把论文的某一节写成"):
			draftPrompt = user
			return map[string]any{"paragraphs": []any{map[string]any{"sentences": []any{
				map[string]any{"text": "已有调查表明，长时间使用短视频的学生注意力得分更低。", "cites": []any{"F1"}},
				map[string]any{"text": "本研究的干预显著提高了专注时长。", "cites": []any{"C2"}},
				map[string]any{"text": "这一结论具有普遍意义。", "cites": []any{}},
				map[string]any{"text": "编造的出处。", "cites": []any{"F99"}},
				map[string]any{"text": "样本量为【需补充：样本量】人。", "cites": []any{}, "kind": "gap"},
			}}}, "notes": []any{"核对 12% 的出处"}}, nil
		}
		return goodModel(c, system, user)
	}
	defer func() { chatJSON = goodModel }()

	// 想法太短
	if code, _ := s2.do("POST", "/api/writing/plan", map[string]any{"profile": "course", "section": "引言", "idea": "太短"}); code != 400 {
		t.Fatal("想法太短应拒绝")
	}
	// s2 用全组共享的论文写作
	r := s2.ok("POST", "/api/writing/plan", map[string]any{"profile": "course", "section": "引言", "idea": "我想说明短视频使用会降低注意力，并提出一个两周的干预办法。\n我们的实验发现干预两周后专注时长提高了 20%。",
		"material_ids": []string{m["id"].(string)}})
	d := r["draft"].(map[string]any)
	if !strings.Contains(planPrompt, "漏斗") || !strings.Contains(planPrompt, "<fragment id=\"F1\"") || !strings.Contains(planPrompt, "N1：") {
		t.Fatalf("骨架提示应包含章节规则、片段和作者材料：%s", planPrompt)
	}
	cs := d["claims"].([]any)
	c0 := cs[0].(map[string]any)
	if len(c0["evidence"].([]any)) != 1 || cs[2].(map[string]any)["status"] != "needs_evidence" {
		t.Fatalf("无效证据应去掉，没有证据的论点应标为需要补充：%v", cs)
	}
	// 作者删掉一条论点后起草
	edited := []any{cs[0], cs[1]}
	r = s2.ok("POST", "/api/writing/draft", map[string]any{"id": d["id"], "claims": edited})
	if strings.Contains(draftPrompt, "样本只来自一所学校") {
		t.Fatal("应按作者修改后的骨架起草")
	}
	d = r["draft"].(map[string]any)
	ss := d["paragraphs"].([]any)[0].([]any)
	flag := func(i int) string { v, _ := ss[i].(map[string]any)["flag"].(string); return v }
	if flag(0) != "" || flag(2) == "" || flag(3) == "" || ss[4].(map[string]any)["kind"] != "gap" {
		t.Fatalf("句子校验不对：%v", ss)
	}
	if cites := ss[1].(map[string]any)["cites"].([]any); len(cites) != 1 || cites[0] != "N1" {
		t.Fatalf("论点编号应换成其证据：%v", cites)
	}
	refs := d["refs"].([]any)
	if len(refs) != 1 || !strings.Contains(refs[0].(string), "张三, 李四, 王五, 等") || !strings.Contains(refs[0].(string), "2023") {
		t.Fatalf("参考文献不对：%v", refs)
	}
	checks := ""
	for _, c := range d["checks"].([]any) {
		checks += c.(map[string]any)["msg"].(string) + "|"
	}
	if !strings.Contains(checks, "显著") || !strings.Contains(checks, "没有依据") || !strings.Contains(checks, "需补充") {
		t.Fatalf("规则检查不全：%s", checks)
	}
	// 导出前必须人工确认：有【需补充】和无依据的句子时，这两项也要勾选
	if code, _ := s2.do("GET", "/api/writing/draft.txt?id="+d["id"].(string), nil); code != 400 {
		t.Fatal("没有确认时不能导出")
	}
	if code, _ := s2.do("POST", "/api/writing/confirm", map[string]any{"id": d["id"], "checks": map[string]bool{"facts": true, "ai": true}}); code != 400 {
		t.Fatal("有需补充和无依据的句子时，应要求勾选对应项")
	}
	if code, _ := s1.do("POST", "/api/writing/confirm", map[string]any{"id": d["id"], "checks": map[string]bool{"facts": true, "ai": true, "gaps": true, "issues": true}}); code != 404 {
		t.Fatal("别人不能确认我的草稿")
	}
	cf := s2.ok("POST", "/api/writing/confirm", map[string]any{"id": d["id"], "checks": map[string]bool{"facts": true, "ai": true, "gaps": true, "issues": true}})["draft"].(map[string]any)["confirm"].(map[string]any)
	if cf["gaps"].(float64) != 1 || cf["flags"].(float64) < 2 {
		t.Fatalf("确认记录应记下当时的问题数：%v", cf)
	}
	if code, _ := s2.do("POST", "/api/writing/drift", map[string]any{"id": d["id"]}); code != 400 {
		t.Fatal("不是按契约写的草稿不能做偏离检查")
	}
	txt := s2.ok("GET", "/api/writing/draft.txt?id="+d["id"].(string), nil)
	p0 := txt["paragraphs"].([]any)[0].(string)
	if !regexp.MustCompile(`注意力得分更低\[1\]。`).MatchString(p0) {
		t.Fatalf("引用应换成 [1] 并放在句末标点前：%s", p0)
	}
	// Word 导出
	resp, _ := s2.hc.Get(s2.base + "/api/writing/draft.docx?id=" + d["id"].(string))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal("docx 无效")
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			x, _ := io.ReadAll(rc)
			if !strings.Contains(string(x), "AI 辅助起草稿") || !strings.Contains(string(x), "参考文献") {
				t.Fatal("docx 内容不对")
			}
		}
	}
	// 不选论文也能起草，列表字段不是 null
	r = s2.ok("POST", "/api/writing/plan", map[string]any{"profile": "course", "section": "结论", "idea": "我的结论是频繁切换内容会降低持续注意力，干预两周有效。"})
	r = s2.ok("POST", "/api/writing/draft", map[string]any{"id": r["draft"].(map[string]any)["id"]})
	nd := r["draft"].(map[string]any)
	for _, k := range []string{"ref_order", "refs", "checks", "frags", "missing", "draft_notes"} {
		if _, ok := nd[k].([]any); !ok {
			t.Fatalf("%s 应为数组：%v", k, nd[k])
		}
	}
	// 草稿列表只属于自己
	if n := len(s2.ok("GET", "/api/writing/drafts", nil)["list"].([]any)); n != 2 {
		t.Fatal("应有 2 份草稿")
	}
	if code, _ := s1.do("GET", "/api/writing/drafts?id="+d["id"].(string), nil); code != 404 {
		t.Fatal("别人不能看我的草稿")
	}
	// 取消共享后，别人就不能再用这篇论文起草
	s1.ok("PATCH", "/api/materials/"+m["id"].(string), map[string]any{"shared": false})
	if code, _ := s2.do("POST", "/api/writing/plan", map[string]any{"profile": "course", "section": "引言", "idea": "我想说明短视频使用会降低注意力，并提出一个两周的干预办法。", "material_ids": []string{m["id"].(string)}}); code != 404 {
		t.Fatalf("取消共享后应无权使用：%d", code)
	}
}
