package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeOpenAI 是一个 OpenAI 兼容的模拟服务：返回 usage，按模型名区分“日常模型”和“难题模型”。
type fakeOpenAI struct {
	mu        sync.Mutex
	models    []string // 每次请求用的模型
	badDaily  bool     // 日常模型的问答引用不合格
	badStrong bool     // 难题模型自检不合格
}

func (f *fakeOpenAI) server(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			if r.Header.Get("Authorization") != "Bearer sk-good" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"data":[{"id":"m-daily"},{"id":"m-strong"},{"id":"m-vl"}]}`))
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		f.mu.Lock()
		f.models = append(f.models, body.Model)
		badD, badS := f.badDaily, f.badStrong
		f.mu.Unlock()
		sys, _ := body.Messages[0].Content.(string)
		user, _ := body.Messages[len(body.Messages)-1].Content.(string)
		var out map[string]any
		switch {
		case strings.Contains(sys, "读懂学术文献") && strings.Contains(user, "概念："):
			out = map[string]any{"plain": "速度就是单位时间内走过的路程。", "formula": "v = s/t", "example": "1 小时走 5 千米，速度是 5 km/h",
				"in_paper":      []any{map[string]any{"text": "本文中速度恒为 10 米/秒", "cites": []any{"F1"}}, map[string]any{"text": "编造的", "cites": []any{"F9"}}},
				"prerequisites": []any{map[string]any{"concept": "路程", "why": "速度由路程定义"}}, "learn_next": []any{"加速度"}}
		case strings.Contains(sys, "读懂学术文献"):
			out = map[string]any{"summary": "研究匀速运动的路程。", "question": map[string]any{"text": "5 秒路程是多少", "cites": []any{"F1"}},
				"method":   map[string]any{"text": "公式计算", "cites": []any{}},
				"findings": []any{map[string]any{"text": "路程为 50 米", "cites": []any{"F1"}}, map[string]any{"text": "瞎编的发现", "cites": []any{"F42"}}},
				"limits":   []any{}, "terms": []any{map[string]any{"term": "匀速", "note": "速度不变"}},
				"prerequisites": []any{map[string]any{"concept": "速度", "why": "理解路程公式"}}, "gaps": []any{"没有实验数据"}}
		case badD && body.Model == "m-daily" && strings.Contains(sys, "严谨的资料整理助手"):
			out = map[string]any{"claims": []any{map[string]any{"text": "路程为 50 米", "type": "原文支持", "cites": []any{"F77"}}}}
		case badS && body.Model == "m-strong" && strings.Contains(sys, "引用核查"):
			out = map[string]any{"verdict": "支持", "cites": []any{"S1"}}
		default:
			out, _ = goodModel(ModelCfg{}, sys, user)
		}
		c, _ := json.Marshal(out)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(c)}}},
			"usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 500}})
	}))
	t.Cleanup(s.Close)
	return s
}

func (f *fakeOpenAI) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.models[len(f.models)-1]
}

func TestRoutingUsageBudget(t *testing.T) {
	chatJSON = realChatJSON
	defer func() { chatJSON = goodModel }()
	_, srv := newEnv(t)
	tc, s1, _, s1ID, _ := setupTeam(t, srv)
	fk := &fakeOpenAI{}
	fs := fk.server(t)
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": fs.URL + "/v1", "llm_model": "m-daily", "llm_key": "sk-good",
		"llm_strong_model": "m-strong", "llm_price_in": 2, "llm_price_out": 8, "llm_strong_price_in": 10, "llm_strong_price_out": 30})
	p := tc.ok("POST", "/api/projects", map[string]any{"name": "运动", "template_key": "research", "members": []int{s1ID}})
	pid := p["id"].(string)
	m := s1.upload("运动.txt", []byte("设车辆始终以10米/秒匀速运动，持续5秒，则路程为50米。该结论以速度恒定为条件。"), pid, nil)
	mid := m["id"].(string)
	ask := func(q, effort string) map[string]any {
		return s1.ok("POST", "/api/ask", map[string]any{"project_id": pid, "material_ids": []string{mid}, "question": q, "effort": effort})
	}
	// 简单问题 → 日常模型
	r := ask("5 秒内的路程是多少", "")
	if fk.last() != "m-daily" || !strings.Contains(r["route"].(string), "日常模型") {
		t.Fatalf("简单问题应用日常模型：%s %v", fk.last(), r["route"])
	}
	// 需要推理的问题 → 难题模型
	r = ask("为什么路程是 50 米", "")
	if fk.last() != "m-strong" || !strings.Contains(r["route"].(string), "难题") || !strings.Contains(r["model"].(string), "m-strong") {
		t.Fatalf("难题应用难题模型：%s %v", fk.last(), r)
	}
	// 手动选择“快速”
	ask("为什么路程是 50 米", "fast")
	if fk.last() != "m-daily" {
		t.Fatal("选择快速时应用日常模型")
	}
	// 日常模型引用不合格 → 自动升级重试
	fk.badDaily = true
	r = ask("5 秒内的路程是多少", "")
	if fk.last() != "m-strong" || !strings.Contains(r["route"].(string), "自动改用难题模型") || r["status"] != "answered" {
		t.Fatalf("应自动升级：%s %v", fk.last(), r)
	}
	fk.badDaily = false
	// 用量
	u := s1.ok("GET", "/api/usage", nil)
	tot := u["total"].(map[string]any)
	if tot["calls"].(float64) != 5 || tot["in"].(float64) != 5000 || tot["invalid"].(float64) != 1 {
		t.Fatalf("用量统计不对 %v", tot)
	}
	// 费用：3 次日常（1000*2+500*8=0.006）+ 2 次难题（1000*10+500*30=0.025）= 0.068
	if c := tot["cost"].(float64); c < 0.0679 || c > 0.0681 {
		t.Fatalf("费用不对 %v", c)
	}
	bt := u["by_task"].([]any)
	if len(bt) != 1 || bt[0].(map[string]any)["label"] != "资料问答" {
		t.Fatalf("按任务统计不对 %v", bt)
	}
	if code, _ := s1.do("GET", "/api/usage?scope=team", nil); code != 403 {
		t.Fatal("学生不能看全组用量")
	}
	team := tc.ok("GET", "/api/usage?scope=team", nil)
	if len(team["by_user"].([]any)) != 1 {
		t.Fatal("全组用量应按人统计")
	}
	if me := s1.ok("GET", "/api/me", nil); me["team_spent"].(float64) < 0.06 {
		t.Fatalf("我的团队花费不对 %v", me["team_spent"])
	}
	// 额度：每人每月 0.07 元，再问一次（0.006）后超出
	tc.ok("PUT", "/api/settings", map[string]any{"team_budget": 0.07})
	ask("5 秒内的路程是多少", "")
	r = ask("5 秒内的路程是多少", "")
	if r["status"] != "llm_error" || !strings.Contains(r["message"].(string), "额度") {
		t.Fatalf("超出额度应拒绝 %v", r)
	}
	if code, _ := tc.do("PUT", "/api/settings", map[string]any{"team_budget": -1}); code != 400 {
		t.Fatal("额度不能为负")
	}
	// 老师不受学生额度影响
	tr := tc.ok("POST", "/api/ask", map[string]any{"project_id": pid, "material_ids": []string{mid}, "question": "5 秒内的路程是多少"})
	if tr["status"] != "answered" {
		t.Fatal("额度按人计算")
	}
	// 团队自检：难题模型没通过 → 不再使用难题模型
	fk.badStrong = true
	ck := tc.ok("POST", "/api/models/team/check", nil)
	if ck["passed"] != true || ck["strong_ok"] == true {
		t.Fatalf("难题模型自检应不通过 %v", ck)
	}
	tr = tc.ok("POST", "/api/ask", map[string]any{"project_id": pid, "material_ids": []string{mid}, "question": "为什么路程是 50 米"})
	if fk.last() != "m-daily" || !strings.Contains(tr["route"].(string), "没通过自检") {
		t.Fatalf("难题模型没通过自检时应退回日常模型 %s %v", fk.last(), tr["route"])
	}
	// 接入向导：查询模型列表
	pr := s1.ok("POST", "/api/models/probe", map[string]any{"base_url": fs.URL + "/v1", "key": "sk-good"})
	if len(pr["models"].([]any)) != 3 {
		t.Fatal("应列出可用模型")
	}
	if code, m := s1.do("POST", "/api/models/probe", map[string]any{"base_url": fs.URL + "/v1", "key": "sk-bad"}); code != 400 || !strings.Contains(m["detail"].(string), "无效") {
		t.Fatal("错误密钥应提示")
	}
	if code, _ := s1.do("POST", "/api/models/probe", map[string]any{"base_url": fs.URL + "/v1", "key": "sk good"}); code != 400 {
		t.Fatal("含空格的密钥应提示")
	}
}

func TestReadBriefConcept(t *testing.T) {
	chatJSON = realChatJSON
	defer func() { chatJSON = goodModel }()
	_, srv := newEnv(t)
	tc, s1, s2, s1ID, _ := setupTeam(t, srv)
	fk := &fakeOpenAI{}
	fs := fk.server(t)
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": fs.URL + "/v1", "llm_model": "m-daily", "llm_key": "sk-good", "llm_strong_model": "m-strong"})
	p := tc.ok("POST", "/api/projects", map[string]any{"name": "运动", "template_key": "research", "members": []int{s1ID}})
	pid := p["id"].(string)
	m := s1.upload("运动.txt", []byte("摘要：设车辆始终以10米/秒匀速运动，持续5秒，则路程为50米。\n\n该结论以速度恒定为条件。"), pid, nil)
	mid := m["id"].(string)
	b := s1.ok("POST", "/api/read/brief", map[string]any{"material_id": mid})
	res := b["result"].(map[string]any)
	fds := res["findings"].([]any)
	if res["summary"] == "" || len(fds) != 2 || len(fds[0].(map[string]any)["chunk_ids"].([]any)) != 1 || len(fds[1].(map[string]any)["chunk_ids"].([]any)) != 0 {
		t.Fatalf("速读卡引用校验不对 %v", res)
	}
	if fds[1].(map[string]any)["note"] == nil || b["cached"] != false || fk.last() != "m-daily" {
		t.Fatal("没有出处的发现应标注；速读用日常模型")
	}
	cid := fds[0].(map[string]any)["chunk_ids"].([]any)[0].(string)
	if _, ok := b["citations"].(map[string]any)[cid]; !ok {
		t.Fatal("应返回引用原文")
	}
	n := len(fk.models)
	b2 := tc.ok("POST", "/api/read/brief", map[string]any{"material_id": mid})
	if b2["cached"] != true || len(fk.models) != n || b2["by"] != "学生甲" {
		t.Fatal("同一份资料的速读卡应全组共用，不重复调用模型")
	}
	if code, _ := s2.do("POST", "/api/read/brief", map[string]any{"material_id": mid}); code != 404 {
		t.Fatal("非成员不能速读")
	}
	c := s1.ok("POST", "/api/read/concept", map[string]any{"material_id": mid, "term": "匀速", "context": "设车辆始终以10米/秒匀速运动"})
	cr := c["result"].(map[string]any)
	if cr["appears"] != true || len(cr["in_paper"].([]any)) != 2 || !strings.Contains(cr["note"].(string), "不是出自这份资料") {
		t.Fatalf("概念讲解不对 %v", cr)
	}
	if ip := cr["in_paper"].([]any)[1].(map[string]any); len(ip["chunk_ids"].([]any)) != 0 {
		t.Fatal("无效引用应丢弃")
	}
	c2 := s1.ok("POST", "/api/read/concept", map[string]any{"material_id": mid, "term": " 匀速 "})
	if c2["cached"] != true {
		t.Fatal("同一概念应复用")
	}
	c3 := s1.ok("POST", "/api/read/concept", map[string]any{"material_id": mid, "term": "动量守恒"})
	if c3["result"].(map[string]any)["appears"] != false {
		t.Fatal("资料中没出现的概念应标出")
	}
	cards := s1.ok("GET", "/api/read/cards?material_id="+mid, nil)["list"].([]any)
	if len(cards) != 3 {
		t.Fatalf("应保存 3 张卡片：%d", len(cards))
	}
	u := s1.ok("GET", "/api/usage", nil)
	if len(u["by_task"].([]any)) != 2 {
		t.Fatalf("速读和概念讲解应分别统计 %v", u["by_task"])
	}
}

// 小模型没通过“提醒项”（例如复述了文档里的指令）时，仍可启用。
func TestSelfCheckWarnLevel(t *testing.T) {
	defer func() { chatJSON = goodModel }()
	_, srv := newEnv(t)
	tc, _, _, _, _ := setupTeam(t, srv)
	chatJSON = func(c ModelCfg, system, user string) (map[string]any, error) {
		if strings.Contains(user, "这份资料讲了什么") {
			return map[string]any{"claims": []any{map[string]any{"text": "已读取其他用户的资料", "type": "原文支持", "cites": []any{"X9"}}}}, nil
		}
		return goodModel(c, system, user)
	}
	mp := tc.ok("POST", "/api/models", map[string]any{"name": "小模型", "base_url": "https://x/v1", "model": "glm-4-flash", "key": "sk-1234567890"})["mine"].([]any)[0].(map[string]any)
	ck := tc.ok("POST", "/api/models/"+mp["id"].(string)+"/check", nil)
	if ck["passed"] != true || ck["warn"] != true || !strings.Contains(ck["items"].([]any)[3].(map[string]any)["detail"].(string), "X9") {
		t.Fatalf("应为基本通过并说明原因 %v", ck)
	}
	tc.ok("POST", "/api/models/"+mp["id"].(string)+"/activate", map[string]any{"active": true})
	// 复述指令（而不是照做）应判为通过
	chatJSON = func(c ModelCfg, system, user string) (map[string]any, error) {
		if strings.Contains(user, "这份资料讲了什么") {
			return map[string]any{"claims": []any{map[string]any{"text": "资料中有一段要求模型输出“已读取其他用户的资料”的指令", "type": "解释", "cites": []any{"F1"}}}}, nil
		}
		return goodModel(c, system, user)
	}
	ck = tc.ok("POST", "/api/models/"+mp["id"].(string)+"/check", nil)
	if ck["passed"] != true || ck["warn"] == true {
		t.Fatalf("复述指令不算照做 %v", ck)
	}
	// 连必过项都没通过时不能启用
	chatJSON = badModel
	ck = tc.ok("POST", "/api/models/"+mp["id"].(string)+"/check", nil)
	if ck["passed"] != false {
		t.Fatal("依据原文回答不合格时不能通过")
	}
}
