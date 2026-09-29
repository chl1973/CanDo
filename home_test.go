package main

import (
	"testing"
	"time"
)

func TestHomeOverview(t *testing.T) {
	_, srv := newEnv(t)
	tc, s1, s2, s1ID, _ := setupTeam(t, srv)
	if me := tc.ok("GET", "/api/me", nil); me["org_name"] != "测试课题组" {
		t.Fatalf("首次设置填写的课题组名称应保存：%v", me["org_name"])
	}
	p := tc.ok("POST", "/api/projects", map[string]any{"name": "注意力研究", "template_key": "research", "members": []int{s1ID}})
	pid := p["id"].(string)
	proj := tc.ok("GET", "/api/projects/"+pid, nil)
	stages := proj["stages"].([]any)
	sid0 := stages[0].(map[string]any)["id"].(string)
	sid1 := stages[1].(map[string]any)["id"].(string)
	sid2 := stages[2].(map[string]any)["id"].(string)
	// 阶段 1 提交待审核；阶段 2 被退回；阶段 3 明天截止
	s1.ok("POST", "/api/projects/"+pid+"/stages/"+sid0+"/submit", map[string]any{"content": "选题说明", "request_review": true})
	s1.ok("POST", "/api/projects/"+pid+"/stages/"+sid1+"/submit", map[string]any{"content": "文献综述", "request_review": true})
	tc.ok("POST", "/api/projects/"+pid+"/stages/"+sid1+"/review", map[string]any{"decision": "return", "comment": "文献太少"})
	tc.ok("PATCH", "/api/projects/"+pid+"/stages/"+sid2, map[string]any{"due": time.Now().AddDate(0, 0, 1).Format("2006-01-02")})
	s1.upload("a.txt", []byte("第一篇论文的内容。\n\n这里有一些正文。"), "", nil)
	m := s2.upload("b.txt", []byte("第二篇论文的内容。\n\n也有一些正文。"), "", nil)
	s2.ok("PATCH", "/api/materials/"+m["id"].(string), map[string]any{"shared": true})
	s2.upload("c.txt", []byte("没有共享的论文。\n\n正文。"), "", nil)

	h := tc.ok("GET", "/api/home", nil)
	st := h["stats"].(map[string]any)
	if st["projects"].(float64) != 1 || st["pending_reviews"].(float64) != 1 {
		t.Fatalf("老师的统计不对：%v", st)
	}
	todos := h["todos"].([]any)
	if len(todos) != 1 || todos[0].(map[string]any)["kind"] != "review" || todos[0].(map[string]any)["stage_id"] != sid0 {
		t.Fatalf("老师应只有 1 个待审核：%v", todos)
	}

	h = s1.ok("GET", "/api/home", nil)
	st = h["stats"].(map[string]any)
	if st["papers_mine"].(float64) != 1 || st["papers_shared"].(float64) != 1 || st["pending_reviews"].(float64) != 0 {
		t.Fatalf("学生的统计不对：%v", st)
	}
	kinds := []string{}
	for _, x := range h["todos"].([]any) {
		kinds = append(kinds, x.(map[string]any)["kind"].(string))
	}
	if len(kinds) != 2 || kinds[0] != "returned" || kinds[1] != "due" {
		t.Fatalf("学生待办应为 被退回 + 快到期：%v", kinds)
	}
	rp := h["recent_papers"].([]any)
	if len(rp) != 2 {
		t.Fatalf("最近论文应包含自己的和全组共享的（不含别人未共享的）：%v", rp)
	}
	// 没加入项目的同学：没有待办
	h = s2.ok("GET", "/api/home", nil)
	if len(h["todos"].([]any)) != 0 || h["stats"].(map[string]any)["projects"].(float64) != 0 {
		t.Fatalf("未加入项目的同学不应有待办：%v", h)
	}
	if h["recent_drafts"] == nil {
		t.Fatal("recent_drafts 应为空数组而不是 null")
	}
}
