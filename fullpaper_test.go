package main

import (
	"strings"
	"testing"
	"time"
)

// 准备：一份已确认的契约（v2）、三节草稿、两篇论文库里的论文
func setupFullPaper(t *testing.T) (*App, *client, *client, map[string]string) {
	app, srv := newEnv(t)
	tc, s1, _, _, _ := setupTeam(t, srv)
	me := int(tc.ok("GET", "/api/me", nil)["id"].(float64))
	t0 := time.Now().Add(-time.Hour)
	ids := map[string]string{"m1": "mat1", "m2": "mat2", "ct": "ct1", "abs": "dabs", "intro": "dintro", "conc": "dconc", "old": "dold"}
	app.store.Update(func(db *DB) error {
		db.Materials = append(db.Materials,
			&Material{ID: ids["m1"], OwnerID: me, Title: "短视频使用与大学生注意力", Author: "张三, 李四", SourceDate: "2023-05-01", Status: "ready"},
			&Material{ID: ids["m2"], OwnerID: me, Title: "推荐算法与内容切换", Status: "ready"})
		db.Contracts = append(db.Contracts, &PaperContract{ID: ids["ct"], OwnerID: me, Profile: "course", Lang: "zh", Title: "短视频与注意力", Status: "confirmed", Version: 2,
			Notes:    []wNote{{ID: "N1", Text: "干预组专注时长从 18 分钟增加到 24 分钟"}},
			Frags:    []wFrag{{ID: "F1", MaterialID: ids["m1"], ChunkID: "c1", Title: "短视频使用与大学生注意力", Text: "使用时长与注意力呈负相关"}},
			Claims:   []CClaim{{ID: "K1", Text: "减少推荐流能提高专注时长", Evidence: []string{"N1"}}, {ID: "K2", Text: "短视频使用与注意力下降相关", Evidence: []string{"F1"}}, {ID: "K3", Text: "效果可以推广到中学生"}},
			Sections: []CSection{{Name: "摘要"}, {Name: "引言"}, {Name: "正文"}, {Name: "结论"}},
			Attacks:  []CAttack{{ID: "Q1", Kind: "causality", Status: "limitation", Limit: "只有相关性证据"}}})
		conf := &draftConfirm{At: t0}
		db.Drafts = append(db.Drafts,
			&WDraft{ID: ids["abs"], OwnerID: me, Profile: "course", Lang: "zh", Section: "摘要", Stage: "draft", ContractID: ids["ct"], ContractVer: 2, Confirm: conf, UpdatedAt: t0,
				Frags:      []wFrag{{ID: "F1", MaterialID: ids["m1"], ChunkID: "c1"}},
				Paragraphs: [][]WSentence{{{Text: "本文研究短视频使用与注意力的关系。", Cites: []string{"F1"}}, {Text: "两周干预后专注时长增加。", Kind: "claim"}}}},
			&WDraft{ID: ids["intro"], OwnerID: me, Profile: "course", Lang: "zh", Section: "引言", Stage: "draft", ContractID: ids["ct"], ContractVer: 1, Confirm: conf, UpdatedAt: t0,
				Frags: []wFrag{{ID: "F1", MaterialID: ids["m1"], ChunkID: "c1"}, {ID: "F2", MaterialID: ids["m2"], ChunkID: "c2"}},
				Paragraphs: [][]WSentence{{{Text: "已有研究发现使用时长与注意力负相关。", Cites: []string{"F1"}}, {Text: "推荐算法导致频繁切换。", Cites: []string{"F2", "F1"}}},
					{{Text: "【需补充：国内研究现状】", Kind: "gap"}}}},
			&WDraft{ID: ids["conc"], OwnerID: me, Profile: "course", Lang: "zh", Section: "结论", Stage: "draft", ContractID: ids["ct"], ContractVer: 2, UpdatedAt: t0.Add(time.Minute),
				Notes:      []wNote{{ID: "N1", Text: "干预组专注时长从 18 分钟增加到 24 分钟"}},
				Paragraphs: [][]WSentence{{{Text: "干预组专注时长增加了 6 分钟。", Cites: []string{"N1"}}}}},
			// 更早的一份结论草稿（自由写），自动选择时应排在按契约写的后面
			&WDraft{ID: ids["old"], OwnerID: me, Profile: "course", Lang: "zh", Section: "结论", Stage: "draft", Idea: "旧的结论", UpdatedAt: t0.Add(2 * time.Minute),
				Paragraphs: [][]WSentence{{{Text: "旧结论。"}}}})
		return nil
	})
	return app, tc, s1, ids
}

func levels(items []any, level string) []string {
	var out []string
	for _, x := range items {
		m := x.(map[string]any)
		if m["level"] == level {
			out = append(out, m["cat"].(string)+"："+m["msg"].(string))
		}
	}
	return out
}

func hasMsg(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

func TestFullPaperAssembleChecksExport(t *testing.T) {
	app, tc, s1, ids := setupFullPaper(t)
	r := tc.ok("POST", "/api/writing/full", map[string]any{"contract_id": ids["ct"]})
	paper := r["paper"].(map[string]any)
	pid := paper["id"].(string)
	if paper["title"] != "短视频与注意力" || paper["profile"] != "course" {
		t.Fatalf("应沿用契约的题目和论文类型：%v", paper)
	}
	picked := map[string]string{}
	for _, x := range r["parts"].([]any) {
		m := x.(map[string]any)
		picked[m["section"].(string)] = m["draft_id"].(string)
	}
	if picked["摘要"] != ids["abs"] || picked["引言"] != ids["intro"] || picked["结论"] != ids["conc"] || picked["正文"] != "" {
		t.Fatalf("自动选择不对（应优先按契约写的）：%v", picked)
	}
	errs := levels(r["checks"].([]any), "error")
	for _, want := range []string{"还没有填关键词", "“结论”还没有做导出前确认"} {
		if !hasMsg(errs, want) {
			t.Errorf("应有错误 %q：%v", want, errs)
		}
	}
	if r["can_export"] != false {
		t.Fatal("有错误时不能导出")
	}
	if code, _ := tc.do("GET", "/api/writing/full/"+pid+"/docx", nil); code != 400 {
		t.Fatal("有错误时导出应被拒绝", code)
	}
	if code, _ := tc.do("POST", "/api/writing/full/"+pid+"/confirm", map[string]any{"checks": map[string]bool{"facts": true, "ai": true, "issues": true}}); code != 400 {
		t.Fatal("有错误时不能确认", code)
	}

	// 补上关键词、确认结论那一节
	r = tc.ok("PUT", "/api/writing/full/"+pid, map[string]any{"keywords": []string{"短视频；注意力，大学生", "注意力"}})
	if kw := r["paper"].(map[string]any)["keywords"].([]any); len(kw) != 3 {
		t.Fatalf("关键词应拆分并去重：%v", kw)
	}
	app.store.Update(func(db *DB) error {
		for _, d := range db.Drafts {
			if d.ID == ids["conc"] {
				d.Confirm = &draftConfirm{At: time.Now()}
			}
		}
		return nil
	})
	r = tc.ok("GET", "/api/writing/full?id="+pid, nil)
	if errs := levels(r["checks"].([]any), "error"); len(errs) > 0 {
		t.Fatalf("不应再有错误：%v", errs)
	}
	warns := levels(r["checks"].([]any), "warn")
	for _, want := range []string{"契约 v1 写的", "因果说法“导致”", "局限", "K3 在契约里没有依据", "1 处【需补充】", "信息不全"} {
		if !hasMsg(warns, want) {
			t.Errorf("应有提醒 %q：%v", want, warns)
		}
	}
	for _, bad := range []string{"K1 的依据", "K2 的依据"} {
		if hasMsg(warns, bad) {
			t.Errorf("K1、K2 的依据都被引用了（按内容对应），不应提醒：%v", warns)
		}
	}
	if !hasMsg(levels(r["format"].([]any), "warn"), "摘要里有文献引用") {
		t.Errorf("格式检查（同一套规则）应发现摘要里有引用：%v", r["format"])
	}
	refs := r["refs"].([]any)
	if len(refs) != 2 || !strings.HasPrefix(refs[0].(string), "张三, 李四. 短视频使用与大学生注意力[J]") {
		t.Fatalf("参考文献应按全文首次引用的顺序统一编号：%v", refs)
	}
	// 引言里 F2(m2)、F1(m1) → 全文编号 [1,2]
	secs := r["sections"].([]any)
	intro := secs[1].(map[string]any)["paras"].([]any)[0].([]any)[1].(map[string]any)
	if nums := intro["nums"].([]any); len(nums) != 2 || nums[0].(float64) != 1 || nums[1].(float64) != 2 {
		t.Fatalf("引言第二句的全文编号应为 [1,2]：%v", intro)
	}

	// 确认：缺少“提醒”那一项不行
	if code, _ := tc.do("POST", "/api/writing/full/"+pid+"/confirm", map[string]any{"checks": map[string]bool{"facts": true, "ai": true}}); code != 400 {
		t.Fatal("有提醒时必须勾选 issues", code)
	}
	if code, _ := tc.do("GET", "/api/writing/full/"+pid+"/docx", nil); code != 400 {
		t.Fatal("没确认不能导出", code)
	}
	r = tc.ok("POST", "/api/writing/full/"+pid+"/confirm", map[string]any{"checks": map[string]bool{"facts": true, "ai": true, "issues": true}})
	if r["can_export"] != true {
		t.Fatal("确认后应可导出")
	}
	_, raw := tc.do("GET", "/api/writing/full/"+pid+"/docx", nil)
	di, err := parseDocx([]byte(raw["raw"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, b := range di.Blocks {
		all.WriteString(b.Text + "\n")
	}
	doc := all.String()
	for _, want := range []string{"AI 辅助起草的全文", "短视频与注意力", "关键词：短视频；注意力；大学生", "1 引言", "推荐算法导致频繁切换[1,2]。", "[1] 张三, 李四.", "参考文献"} {
		if !strings.Contains(doc, want) {
			t.Errorf("Word 里应有 %q：\n%s", want, doc)
		}
	}
	_, rawTex := tc.do("GET", "/api/writing/full/"+pid+"/tex", nil)
	tex := rawTex["raw"].(string)
	for _, want := range []string{`\documentclass[UTF8,zihao=-4]{ctexart}`, `\begin{abstract}`, `频繁切换~\cite{r1,r2}。`, `\bibitem{r2}`, `\gap{【需补充：国内研究现状】}`, `\section{引言}`} {
		if !strings.Contains(tex, want) {
			t.Errorf("LaTeX 里应有 %q", want)
		}
	}
	if ws := LintLatex(tex, false); len(ws) > 0 {
		t.Errorf("生成的 LaTeX 有静态问题：%v", ws)
	}
	// 本机有 LaTeX（含中文支持）时真的编译一次
	if e := findTeX(false); e.Found {
		code, res := tc.do("POST", "/api/writing/full/pdf", map[string]any{"id": pid})
		if code != 200 {
			t.Fatalf("编译全文失败：%d %v", code, res)
		}
		rr := res["result"].(map[string]any)
		if rr["ok"] != true || rr["pdf_id"] == nil {
			if strings.Contains(rr["log_tail"].(string), "ctexart.cls") {
				t.Log("这台电脑的 LaTeX 没有中文支持（ctex），跳过编译检查")
			} else {
				t.Fatalf("全文应能编译成 PDF：%v", rr)
			}
		}
	}

	// 改了题目 → 需要重新确认
	r = tc.ok("PUT", "/api/writing/full/"+pid, map[string]any{"title": "短视频推荐流与大学生专注时长"})
	if r["confirmed"] != false {
		t.Fatal("改题目后应需要重新确认")
	}
	tc.ok("POST", "/api/writing/full/"+pid+"/confirm", map[string]any{"checks": map[string]bool{"facts": true, "ai": true, "issues": true}})
	// 某一节重新起草 → 需要重新确认
	app.store.Update(func(db *DB) error {
		for _, d := range db.Drafts {
			if d.ID == ids["abs"] {
				d.UpdatedAt = time.Now()
			}
		}
		return nil
	})
	if code, _ := tc.do("GET", "/api/writing/full/"+pid+"/docx", nil); code != 400 {
		t.Fatal("某一节改动后应需要重新确认", code)
	}

	// 换成自由写的旧草稿：提醒不是按契约写的；同一份草稿不能用两次
	r = tc.ok("PUT", "/api/writing/full/"+pid, map[string]any{"parts": []any{map[string]any{"section": "摘要", "draft_id": ids["abs"]}, map[string]any{"section": "引言", "draft_id": ids["intro"]}, map[string]any{"section": "结论", "draft_id": ids["old"]}, map[string]any{"section": "正文", "skip": true}}})
	if !hasMsg(levels(r["checks"].([]any), "warn"), "不是按契约") {
		t.Fatal("应提醒结论不是按契约写的")
	}
	if !hasMsg(levels(r["checks"].([]any), "error"), "“结论”还没有做导出前确认") {
		t.Fatal("旧草稿没确认过，应报错")
	}
	if !hasMsg(levels(r["checks"].([]any), "warn"), "章节地图里有“正文”") {
		t.Fatal("契约里有正文、全文不写，应提醒")
	}
	r = tc.ok("PUT", "/api/writing/full/"+pid, map[string]any{"parts": []any{map[string]any{"section": "摘要", "draft_id": ids["abs"]}, map[string]any{"section": "引言", "draft_id": ids["abs"]}, map[string]any{"section": "结论", "draft_id": ids["conc"]}}})
	if !hasMsg(levels(r["checks"].([]any), "error"), "同一份草稿") {
		t.Fatal("同一份草稿用两次应报错")
	}
	// 必需的章节不能跳过
	r = tc.ok("PUT", "/api/writing/full/"+pid, map[string]any{"parts": []any{map[string]any{"section": "摘要", "skip": true}, map[string]any{"section": "引言", "draft_id": ids["intro"]}, map[string]any{"section": "结论", "draft_id": ids["conc"]}}})
	if !hasMsg(levels(r["checks"].([]any), "error"), "“摘要”是必需的章节") {
		t.Fatal("必需章节跳过应报错")
	}

	// 别人看不到、也不能用别人的草稿
	if code, _ := s1.do("GET", "/api/writing/full?id="+pid, nil); code != 404 {
		t.Fatal("别人不能看", code)
	}
	if code, _ := s1.do("GET", "/api/writing/full/"+pid+"/docx", nil); code != 404 {
		t.Fatal("别人不能导出", code)
	}
	s1p := s1.ok("POST", "/api/writing/full", map[string]any{"profile": "course"})["paper"].(map[string]any)["id"].(string)
	if code, _ := s1.do("PUT", "/api/writing/full/"+s1p, map[string]any{"parts": []any{map[string]any{"section": "引言", "draft_id": ids["intro"]}}}); code != 404 {
		t.Fatal("不能选别人的草稿", code)
	}
	if code, _ := s1.do("POST", "/api/writing/full", map[string]any{"contract_id": ids["ct"]}); code != 404 {
		t.Fatal("不能用别人的契约", code)
	}

	// 草稿被删除 → 报错；列表和删除
	app.store.Update(func(db *DB) error {
		for i, d := range db.Drafts {
			if d.ID == ids["intro"] {
				db.Drafts = append(db.Drafts[:i], db.Drafts[i+1:]...)
				break
			}
		}
		return nil
	})
	r = tc.ok("GET", "/api/writing/full?id="+pid, nil)
	if !hasMsg(levels(r["checks"].([]any), "error"), "已被删除") {
		t.Fatal("草稿被删除应报错")
	}
	if l := tc.ok("GET", "/api/writing/full", nil)["list"].([]any); len(l) != 1 {
		t.Fatalf("列表应有 1 篇：%v", l)
	}
	tc.ok("DELETE", "/api/writing/full?id="+pid, nil)
	if code, _ := tc.do("GET", "/api/writing/full?id="+pid, nil); code != 404 {
		t.Fatal("删除后应不存在")
	}
}

func TestFullPaperFreeWritingEnglish(t *testing.T) {
	app, srv := newEnv(t)
	tc, _, _, _, _ := setupTeam(t, srv)
	me := int(tc.ok("GET", "/api/me", nil)["id"].(float64))
	now := time.Now()
	app.store.Update(func(db *DB) error {
		db.Drafts = append(db.Drafts, &WDraft{ID: "e1", OwnerID: me, Profile: "ncomms", Lang: "en", Section: "Introduction", Stage: "draft", Confirm: &draftConfirm{At: now}, UpdatedAt: now,
			Paragraphs: [][]WSentence{{{Text: "Short videos are popular."}, {Text: "Attention spans may shrink."}}}})
		return nil
	})
	r := tc.ok("POST", "/api/writing/full", map[string]any{"profile": "ncomms", "title": "Short videos and attention"})
	paper := r["paper"].(map[string]any)
	if paper["lang"] != "en" {
		t.Fatal("应按论文类型选英文")
	}
	errs := levels(r["checks"].([]any), "error")
	for _, want := range []string{"“Abstract”是必需的章节", "“Results”是必需的章节"} {
		if !hasMsg(errs, want) {
			t.Errorf("应有错误 %q：%v", want, errs)
		}
	}
	if hasMsg(errs, "关键词") {
		t.Error("Nature Communications 没有关键词要求，不应报错")
	}
	if !hasMsg(levels(r["checks"].([]any), "info"), "Data availability") {
		t.Error("应提示自己补上 Data availability 声明")
	}
	// 用不存在的论文类型新建
	if code, _ := tc.do("POST", "/api/writing/full", map[string]any{"profile": "nope"}); code != 400 {
		t.Fatal("论文类型不存在应报错", code)
	}
}
