package main

import (
	"io"
	"strings"
	"testing"
)

func TestPaperContract(t *testing.T) {
	_, srv := newEnv(t)
	tc, s1, s2, _, _ := setupTeam(t, srv)
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k"})
	m := s1.upload("attention.txt", []byte("短视频使用与大学生注意力。\n\n一项调查显示，每天刷短视频超过两小时的学生，注意力测验得分低 12%。"), "", nil)
	var rebut map[string]any // 下一次“判定回应”返回什么
	var planPrompt, attackPrompt, rebutPrompt, skeletonPrompt, draftPrompt, driftPrompt string
	var draftSents []any        // 不为空时，起草返回这些句子
	var driftOut map[string]any // 偏离检查返回什么
	chatJSON = func(c ModelCfg, system, user string) (map[string]any, error) {
		switch {
		case strings.Contains(system, "写成一份“论文契约”"):
			planPrompt = user
			return map[string]any{
				"question": map[string]any{"text": "短视频使用时长是否影响大学生的持续注意力", "method": "问卷调查 + 两周干预实验", "scope": "某校本科生；不研究中学生",
					"variables": []any{map[string]any{"name": "每日短视频时长", "type": "independent", "measure": "手机屏幕使用记录"}, map[string]any{"name": "持续注意力", "type": "dependent", "measure": "注意力测验得分"}}},
				"claims": []any{
					map[string]any{"text": "短视频使用时间长导致注意力下降", "novelty": "用客观使用记录代替自评", "evidence": []any{"N1", "F1", "X9"}, "falsify": "控制睡眠后差异消失"},
					map[string]any{"text": "两周无推荐流干预可以改善注意力", "novelty": "提出可操作的干预", "evidence": []any{"N2"}, "falsify": "干预组与对照组无差异"},
				},
				"sections": []any{
					map[string]any{"name": "引言", "question": "为什么要研究短视频与注意力", "evidence": "F1", "conclusion": "现有研究缺少客观测量", "link": "引出方法"},
					map[string]any{"name": "结论", "question": "干预是否有效", "evidence": "N2", "conclusion": "干预有效但样本有限", "link": "回应引言"},
				},
				"missing": []any{"需要对照组样本量"}}, nil
		case strings.Contains(system, "站在审稿人的角度对它发起质疑"):
			attackPrompt = user
			return map[string]any{"attacks": []any{map[string]any{"kind": "sample", "target": "Q", "text": "样本只来自一所学校，能代表大学生吗？", "severity": "medium", "hint": "说明抽样方法"}}}, nil
		case strings.Contains(system, "判断这条回应能不能让你撤回质疑"):
			rebutPrompt = user
			return rebut, nil
		case strings.Contains(system, "论证骨架”（先想清楚"):
			skeletonPrompt = user
			return map[string]any{"claims": []any{map[string]any{"role": "gap", "text": "现有研究缺少客观测量", "evidence": []any{"F1"}}}}, nil
		case strings.Contains(system, "把论文的某一节写成"):
			draftPrompt = user
			if draftSents != nil {
				return map[string]any{"paragraphs": []any{map[string]any{"sentences": draftSents}}}, nil
			}
			return map[string]any{"paragraphs": []any{map[string]any{"sentences": []any{map[string]any{"text": "已有研究多依赖自评。", "cites": []any{"F1"}}}}}}, nil
		case strings.Contains(system, "逐句对照契约，找出写偏的地方"):
			driftPrompt = user
			return driftOut, nil
		}
		return goodModel(c, system, user)
	}
	defer func() { chatJSON = goodModel }()

	if code, _ := s1.do("POST", "/api/contracts/plan", map[string]any{"profile": "course", "idea": "太短"}); code != 400 {
		t.Fatal("想法太短应拒绝")
	}
	idea := "我想研究短视频使用时长会不会让大学生注意力变差。\n我们记录了 60 名学生的每日使用时长。\n两周无推荐流干预后，干预组专注时长从 18 分钟增加到 24 分钟。"
	r := s1.ok("POST", "/api/contracts/plan", map[string]any{"profile": "course", "title": "短视频与注意力", "idea": idea, "material_ids": []string{m["id"].(string)}})
	c := r["contract"].(map[string]any)
	id := c["id"].(string)
	if !strings.Contains(planPrompt, "N1：") || !strings.Contains(planPrompt, `<fragment id="F1"`) || !strings.Contains(planPrompt, "引言") {
		t.Fatalf("契约提示应包含作者材料、论文片段和章节：%s", planPrompt)
	}
	if !strings.Contains(attackPrompt, "K1：短视频使用时间长导致注意力下降") {
		t.Fatalf("生成契约后应强制跑一轮攻击：%s", attackPrompt)
	}
	k1 := c["claims"].([]any)[0].(map[string]any)
	if ev := k1["evidence"].([]any); len(ev) != 2 {
		t.Fatalf("不存在的证据编号应去掉：%v", ev)
	}
	kinds := map[string]bool{}
	for _, x := range c["attacks"].([]any) {
		kinds[x.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["sample"] || !kinds["causality"] || !kinds["reverse"] {
		t.Fatalf("因果类主张必须被问“相关≠因果”和“反过来成立吗”：%v", kinds)
	}
	if len(r["problems"].([]any)) == 0 {
		t.Fatal("有未处理的质疑时应列出确认前的问题")
	}
	// 别人看不到
	if code, _ := s2.do("GET", "/api/contracts?id="+id, nil); code != 404 {
		t.Fatal("论文契约只属于本人")
	}
	// 未处理的质疑还在，不能确认
	if code, body := s1.do("POST", "/api/contracts/"+id+"/confirm", nil); code != 400 || !strings.Contains(str(body["detail"]), "还没处理") {
		t.Fatalf("有未处理的质疑时不能确认：%d %v", code, body)
	}
	qid := func(kind string) string {
		cc := s1.ok("GET", "/api/contracts?id="+id, nil)["contract"].(map[string]any)
		for _, x := range cc["attacks"].([]any) {
			if x.(map[string]any)["kind"] == kind {
				return x.(map[string]any)["id"].(string)
			}
		}
		t.Fatal("找不到质疑 " + kind)
		return ""
	}
	status := func(r map[string]any, q string) (string, map[string]any) {
		for _, x := range r["contract"].(map[string]any)["attacks"].([]any) {
			a := x.(map[string]any)
			if a["id"] == q {
				th := a["thread"].([]any)
				return a["status"].(string), th[len(th)-1].(map[string]any)
			}
		}
		return "", nil
	}
	qc := qid("causality")
	// 反谄媚 1：学生只是坚持，模型却想让步 → 服务端不让步
	rebut = map[string]any{"addresses_core": false, "evidence": []any{}, "evidence_ok": false, "verdict": "concede", "reason": "好吧"}
	r = s1.ok("POST", "/api/contracts/"+id+"/rebut", map[string]any{"qid": qc, "text": "我觉得就是因果关系，大家都这么认为"})
	if st, last := status(r, qc); st != "new" || last["verdict"] != "hold" || !strings.Contains(str(last["note"]), "按规则不让步") || strings.Contains(str(last["text"]), "好吧") {
		t.Fatalf("没回应核心质疑时不能让步：%s %v", st, last)
	}
	if !strings.Contains(rebutPrompt, "大家都这么认为") || !strings.Contains(rebutPrompt, "相关性") {
		t.Fatalf("判定时应给出质疑和回应：%s", rebutPrompt)
	}
	// 反谄媚 2：模型声称有证据，但摘录并不在回应里 → 不让步
	rebut = map[string]any{"addresses_core": true, "evidence": []any{"随机分组的双盲实验结果 p<0.001"}, "evidence_ok": true, "verdict": "concede", "reason": "有证据"}
	r = s1.ok("POST", "/api/contracts/"+id+"/rebut", map[string]any{"qid": qc, "text": "我们会在以后补充实验来证明因果"})
	if st, last := status(r, qc); st != "new" || last["verdict"] != "partial" {
		t.Fatalf("编造的证据不能作为让步理由：%s %v", st, last)
	}
	if !strings.Contains(rebutPrompt, "之前的来回") {
		t.Fatal("多轮时应带上之前的来回")
	}
	// 回应了核心且给出真实证据 → 让步
	ans := "我们做了随机分组：干预组 32 人、对照组 30 人，干预前两组注意力得分无差异（N2）。"
	rebut = map[string]any{"addresses_core": true, "evidence": []any{"干预组 32 人、对照组 30 人", "N2"}, "evidence_ok": true, "verdict": "concede", "reason": "随机分组可以支持因果"}
	r = s1.ok("POST", "/api/contracts/"+id+"/rebut", map[string]any{"qid": qc, "text": ans})
	if st, last := status(r, qc); st != "answered" || len(last["evidence"].([]any)) != 2 {
		t.Fatalf("回应核心且有证据时应让步：%s %v", st, last)
	}
	// 其余：写进局限 / 保留为待解决
	r = s1.ok("POST", "/api/contracts/"+id+"/resolve", map[string]any{"qid": qid("reverse"), "status": "limitation", "limit": "无法完全排除注意力差的学生更爱刷短视频"})
	r = s1.ok("POST", "/api/contracts/"+id+"/resolve", map[string]any{"qid": qid("sample"), "status": "open"})
	if code, _ := s1.do("POST", "/api/contracts/"+id+"/resolve", map[string]any{"qid": qc, "status": "open"}); code != 400 {
		t.Fatal("已被接受的质疑不能再改状态")
	}
	// 缺“不成立的情况”时不能确认
	cc := r["contract"].(map[string]any)
	claims := cc["claims"].([]any)
	claims[1].(map[string]any)["falsify"] = ""
	s1.ok("PUT", "/api/contracts/"+id, map[string]any{"claims": claims})
	if code, body := s1.do("POST", "/api/contracts/"+id+"/confirm", nil); code != 400 || !strings.Contains(str(body["detail"]), "不成立") {
		t.Fatalf("主张缺少证伪条件时不能确认：%v", body)
	}
	claims[1].(map[string]any)["falsify"] = "干预组与对照组无差异"
	s1.ok("PUT", "/api/contracts/"+id, map[string]any{"claims": claims, "add_notes": []string{"对照组 30 人，干预前后得分无变化"}})
	// 未确认时不能按契约起草
	if code, _ := s1.do("POST", "/api/writing/plan", map[string]any{"contract_id": id, "section": "引言"}); code != 400 {
		t.Fatal("未确认的契约不能用来起草")
	}
	r = s1.ok("POST", "/api/contracts/"+id+"/confirm", nil)
	cc = r["contract"].(map[string]any)
	if cc["status"] != "confirmed" || cc["version"].(float64) != 1 || len(cc["history"].([]any)) != 1 {
		t.Fatalf("确认后应锁定并记录版本：%v", cc["status"])
	}
	if code, _ := s1.do("PUT", "/api/contracts/"+id, map[string]any{"title": "改名"}); code != 400 {
		t.Fatal("锁定后不能修改")
	}
	if code, _ := s1.do("POST", "/api/contracts/"+id+"/rebut", map[string]any{"qid": qid("sample"), "text": "锁定后再回应试试看"}); code != 400 {
		t.Fatal("锁定后不能回应")
	}
	// 按契约起草：不写想法也可以；证据编号与契约一致；契约内容进入骨架和成文提示
	r = s1.ok("POST", "/api/writing/plan", map[string]any{"contract_id": id, "section": "引言"})
	d := r["draft"].(map[string]any)
	if d["contract_id"] != id || d["contract_ver"].(float64) != 1 || r["contract"] == nil {
		t.Fatalf("草稿应记录依据的契约版本：%v", d["contract_id"])
	}
	if !strings.Contains(skeletonPrompt, "论文契约（已确认 v1") || !strings.Contains(skeletonPrompt, "为什么要研究短视频与注意力") || strings.Contains(skeletonPrompt, "干预是否有效") {
		t.Fatalf("骨架提示应包含契约和本节的章节地图（不含其他节）：%s", skeletonPrompt)
	}
	if !strings.Contains(skeletonPrompt, "N4：对照组 30 人") || !strings.Contains(skeletonPrompt, "无法完全排除注意力差的学生更爱刷短视频") || !strings.Contains(skeletonPrompt, "样本只来自一所学校") {
		t.Fatalf("骨架提示应包含契约的补充材料、局限和待解决问题：%s", skeletonPrompt)
	}
	s1.ok("POST", "/api/writing/draft", map[string]any{"id": d["id"]})
	if !strings.Contains(draftPrompt, "论文契约 v1") || !strings.Contains(draftPrompt, "不得出现契约以外的新主张") {
		t.Fatalf("成文提示也应受契约约束：%s", draftPrompt)
	}
	// 解锁修改后再确认，版本 +1
	s1.ok("POST", "/api/contracts/"+id+"/unlock", nil)
	s1.ok("PUT", "/api/contracts/"+id, map[string]any{"title": "短视频与大学生注意力"})
	cc = s1.ok("POST", "/api/contracts/"+id+"/confirm", nil)["contract"].(map[string]any)
	if cc["version"].(float64) != 2 || len(cc["history"].([]any)) != 2 {
		t.Fatal("再次确认后版本应为 2")
	}
	// 列表与 Markdown 导出
	lst := s1.ok("GET", "/api/contracts", nil)["list"].([]any)
	if len(lst) != 1 || lst[0].(map[string]any)["version"].(float64) != 2 {
		t.Fatalf("列表不对：%v", lst)
	}
	resp, _ := s1.hc.Get(s1.base + "/api/contracts.md?id=" + id)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	md := string(b)
	for _, want := range []string{"# 论文契约：短视频与大学生注意力", "已确认 v2", "| 每日短视频时长 | 自变量 |", "什么情况下不成立", "open_questions", "写进局限", "待解决"} {
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown 缺少 %q：%s", want, md)
		}
	}
	// ---------- 偏离检查（1.14） ----------
	// 结论一节：契约承认“反向因果”是局限，正文却写因果；也没交代局限
	draftSents = []any{
		map[string]any{"text": "短视频使用导致大学生注意力下降。", "cites": []any{"N1"}},
		map[string]any{"text": "无推荐流干预使专注时长增加。", "cites": []any{"N2"}},
	}
	dc := s1.ok("POST", "/api/writing/plan", map[string]any{"contract_id": id, "section": "结论"})["draft"].(map[string]any)
	dc = s1.ok("POST", "/api/writing/draft", map[string]any{"id": dc["id"]})["draft"].(map[string]any)
	if code, body := s1.do("POST", "/api/writing/confirm", map[string]any{"id": dc["id"], "checks": map[string]bool{"facts": true, "ai": true, "issues": true, "gaps": true}}); code != 400 || !strings.Contains(str(body["detail"]), "对照契约检查") {
		t.Fatalf("按契约写的草稿，导出前必须先做偏离检查：%v", body)
	}
	if code, _ := s2.do("POST", "/api/writing/drift", map[string]any{"id": dc["id"]}); code != 404 {
		t.Fatal("别人不能检查我的草稿")
	}
	driftOut = map[string]any{"items": []any{
		map[string]any{"kind": "new_claim", "sentence": "S2", "text": "契约没有说干预增加了专注时长的具体机制", "fix": "删去或改回契约的说法"},
		map[string]any{"kind": "contradict", "sentence": "S9", "text": "编造的句子编号", "fix": ""},
		map[string]any{"kind": "missing", "sentence": "", "text": "没有回答“干预是否有效”的样本限制", "fix": "补一句样本有限"},
		map[string]any{"kind": "weird", "sentence": "S1", "text": "未知类型", "fix": ""},
	}}
	dr := s1.ok("POST", "/api/writing/drift", map[string]any{"id": dc["id"]})["draft"].(map[string]any)
	if !strings.Contains(driftPrompt, "S1：短视频使用导致") || !strings.Contains(driftPrompt, "干预是否有效") || !strings.Contains(driftPrompt, "论文契约（v2）") {
		t.Fatalf("偏离检查提示应包含逐句编号的正文和契约：%s", driftPrompt)
	}
	items := dr["drift"].([]any)
	all := ""
	for _, x := range items {
		m := x.(map[string]any)
		all += str(m["cat"]) + "：" + str(m["msg"]) + " @" + str(m["where"]) + "\n"
	}
	if !strings.Contains(all, "因果说法“导致”") || !strings.Contains(all, "因果说法“增加”") {
		t.Fatalf("契约承认因果未证实时，因果措辞应由规则标出：%s", all)
	}
	if !strings.Contains(all, "没写承认的局限") {
		t.Fatalf("结论一节没交代局限应标出：%s", all)
	}
	if strings.Contains(all, "编造的句子编号") {
		t.Fatalf("不存在的句子编号应不采信：%s", all)
	}
	if !strings.Contains(all, "超出契约的新主张：契约没有说") || !strings.Contains(all, "@S2：无推荐流") || !strings.Contains(all, "漏答本节问题") || !strings.Contains(all, "其他：未知类型") {
		t.Fatalf("模型的检查结果应保留并附上原句：%s", all)
	}
	if dr["drift_ver"].(float64) != 2 || dr["drift_at"] == nil {
		t.Fatal("应记录对照的契约版本")
	}
	if code, _ := s1.do("POST", "/api/writing/confirm", map[string]any{"id": dc["id"], "checks": map[string]bool{"facts": true, "ai": true}}); code != 400 {
		t.Fatal("有偏离问题时应要求勾选“会逐条处理”")
	}
	cf := s1.ok("POST", "/api/writing/confirm", map[string]any{"id": dc["id"], "checks": map[string]bool{"facts": true, "ai": true, "issues": true}})["draft"].(map[string]any)["confirm"].(map[string]any)
	if cf["drift"].(float64) < 5 || cf["contract_ver"].(float64) != 2 {
		t.Fatalf("确认记录不对：%v", cf)
	}
	s1.ok("GET", "/api/writing/draft.txt?id="+dc["id"].(string), nil)
	// 重新检查后需要重新确认
	driftOut = map[string]any{"items": []any{}}
	s1.ok("POST", "/api/writing/drift", map[string]any{"id": dc["id"]})
	if code, _ := s1.do("GET", "/api/writing/draft.txt?id="+dc["id"].(string), nil); code != 400 {
		t.Fatal("重新检查后应重新确认")
	}
	draftSents = nil

	// ---------- 老师审阅（1.14） ----------
	rv := s1.ok("GET", "/api/contracts/reviewers", nil)["list"].([]any)
	if len(rv) != 1 || rv[0].(map[string]any)["name"] != "王老师" {
		t.Fatalf("审阅老师列表只应有老师：%v", rv)
	}
	tid := rv[0].(map[string]any)["id"]
	s2id := 0
	for _, u := range tc.ok("GET", "/api/users", nil)["list"].([]any) {
		if u.(map[string]any)["name"] == "学生乙" {
			s2id = int(u.(map[string]any)["id"].(float64))
		}
	}
	if code, _ := s1.do("POST", "/api/contracts/"+id+"/review-request", map[string]any{"reviewer": s2id}); code != 400 {
		t.Fatal("不能请同学审阅")
	}
	if code, _ := tc.do("GET", "/api/contracts?id="+id, nil); code != 404 {
		t.Fatal("没请审阅时老师也看不到学生的契约")
	}
	cc = s1.ok("POST", "/api/contracts/"+id+"/review-request", map[string]any{"reviewer": tid, "note": "请看看第二个主张"})["contract"].(map[string]any)
	if cc["review_status"] != "requested" || len(cc["reviews"].([]any)) != 1 {
		t.Fatalf("应记录审阅请求：%v", cc["review_status"])
	}
	home := tc.ok("GET", "/api/home", nil)
	found := false
	for _, x := range home["todos"].([]any) {
		if x.(map[string]any)["kind"] == "contract_review" && x.(map[string]any)["id"] == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("老师首页应有待审阅的契约：%v", home["todos"])
	}
	if l := tc.ok("GET", "/api/contracts?review=1", nil)["list"].([]any); len(l) != 1 || l[0].(map[string]any)["owner_name"] != "学生甲" {
		t.Fatalf("老师的审阅列表不对：%v", l)
	}
	tv := tc.ok("GET", "/api/contracts?id="+id, nil)
	if tv["owner_name"] != "学生甲" {
		t.Fatal("老师应能查看")
	}
	if code, _ := tc.do("PUT", "/api/contracts/"+id, map[string]any{"title": "老师改的"}); code != 404 {
		t.Fatal("老师不能修改学生的契约")
	}
	if code, _ := tc.do("POST", "/api/contracts/"+id+"/unlock", nil); code != 404 {
		t.Fatal("老师不能解锁学生的契约")
	}
	if code, _ := tc.do("DELETE", "/api/contracts?id="+id, nil); code != 403 {
		t.Fatal("老师不能删除学生的契约")
	}
	if code, _ := s1.do("POST", "/api/contracts/"+id+"/review", map[string]any{"decision": "approve"}); code != 403 {
		t.Fatal("作者不能审阅自己的契约")
	}
	if code, _ := s2.do("POST", "/api/contracts/"+id+"/review", map[string]any{"decision": "approve"}); code != 404 {
		t.Fatal("别人不能审阅")
	}
	if code, _ := tc.do("POST", "/api/contracts/"+id+"/review", map[string]any{"decision": "return"}); code != 400 {
		t.Fatal("退回时必须写意见")
	}
	cc = tc.ok("POST", "/api/contracts/"+id+"/review", map[string]any{"decision": "return", "text": "第二个主张缺少对照组的前测数据"})["contract"].(map[string]any)
	if cc["review_status"] != "returned" || cc["title"] != "短视频与大学生注意力" {
		t.Fatalf("退回后状态不对：%v", cc["review_status"])
	}
	found = false
	for _, x := range s1.ok("GET", "/api/home", nil)["todos"].([]any) {
		if x.(map[string]any)["kind"] == "contract_returned" {
			found = true
		}
	}
	if !found {
		t.Fatal("学生首页应提示契约被退回")
	}
	// 老师下载契约文档
	resp, _ = tc.hc.Get(tc.base + "/api/contracts.md?id=" + id)
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "# 论文契约") {
		t.Fatal("审阅老师应能导出契约文档")
	}
	// 再请审阅 → 通过；解锁会撤回审阅，老师随即看不到
	s1.ok("POST", "/api/contracts/"+id+"/review-request", map[string]any{"reviewer": tid})
	cc = tc.ok("POST", "/api/contracts/"+id+"/review", map[string]any{"decision": "approve", "text": "可以开始写"})["contract"].(map[string]any)
	if cc["review_status"] != "approved" {
		t.Fatal("应已通过")
	}
	cc = s1.ok("POST", "/api/contracts/"+id+"/unlock", nil)["contract"].(map[string]any)
	if cc["review_status"] != nil && cc["review_status"] != "" {
		t.Fatalf("解锁后审阅应撤回：%v", cc["review_status"])
	}
	if code, _ := tc.do("GET", "/api/contracts?id="+id, nil); code != 404 {
		t.Fatal("撤回后老师不应再看到")
	}

	// 删除
	s1.ok("DELETE", "/api/contracts?id="+id, nil)
	if code, _ := s1.do("GET", "/api/contracts?id="+id, nil); code != 404 {
		t.Fatal("删除后应不存在")
	}
}
