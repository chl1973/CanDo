package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// 假 OpenAlex：每个检索式返回 4 篇不同的文献（第二轮的检索式返回新文献）
func fakeOpenAlex(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		var res []map[string]any
		for i := 1; i <= 4; i++ {
			id := q + "-" + itoa(i)
			res = append(res, map[string]any{"id": "https://openalex.org/" + id, "display_name": "Study " + id + " on attention",
				"publication_year": 2020 + i, "authorships": []any{map[string]any{"author": map[string]any{"display_name": "A Author"}}},
				"primary_location":        map[string]any{"source": map[string]any{"display_name": "J Test"}},
				"abstract_inverted_index": map[string]any{"Short": []int{0}, "video": []int{1}, "reduces": []int{2}, "attention.": []int{3}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{"count": 4}, "results": res})
	}))
}

func waitJob(t *testing.T, tc *client, id string) map[string]any {
	for i := 0; i < 400; i++ {
		r := tc.ok("GET", "/api/research/"+id, nil)
		if r["status"] != "running" {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("调研没有结束")
	return nil
}

func TestResearch(t *testing.T) {
	resetScholar()
	oa := fakeOpenAlex(t)
	defer oa.Close()
	_, srv := newEnv(t)
	tc, s1, _, _, _ := setupTeam(t, srv)
	old := openAlexBase
	openAlexBase = oa.URL
	defer func() { openAlexBase = old }()
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k"})

	var mu sync.Mutex
	synthCalls := 0
	reID := regexp.MustCompile(`<paper id="(P\d+)">\n题名：Study (.+?) on attention`)
	chatJSON = func(c ModelCfg, system, user string) (map[string]any, error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(system, "检索助手"):
			return map[string]any{"queries": []any{map[string]any{"query": "short video attention"}, map[string]any{"query": "短视频 注意力"}, map[string]any{"query": "smartphone video attention"}}}, nil
		case strings.Contains(system, "筛选助手"):
			var items []any
			for _, m := range reID.FindAllStringSubmatch(user, -1) {
				score := 1
				if strings.HasSuffix(m[2], "-1") || strings.HasSuffix(m[2], "-2") {
					score = 3 // 每个检索式前两篇相关
				}
				items = append(items, map[string]any{"id": m[1], "score": score, "reason": "摘要讨论注意力", "fields": map[string]any{"方法": "问卷"}})
			}
			return map[string]any{"items": items}, nil
		case strings.Contains(system, "调研助手"):
			synthCalls++
			nq := []any{"attention span students", "short video attention"} // 第二条与已有重复，应被去掉
			if synthCalls >= 2 {
				nq = []any{"attention span students"} // 已用过 → 没有新方向
			}
			return map[string]any{"summary": "短视频使用与注意力下降有关",
				"sections": []any{map[string]any{"heading": "主要发现", "points": []any{
					map[string]any{"text": "多项研究发现注意力下降", "cites": []any{1, 2}},
					map[string]any{"text": "引用了不存在的文献", "cites": []any{999}}, // 无效出处，应丢弃
				}}},
				"gaps": []any{"缺少纵向研究"}, "next_queries": nq}, nil
		}
		return goodModel(c, system, user)
	}
	defer func() { chatJSON = goodModel }()

	// 文献筛选
	j := tc.ok("POST", "/api/research", map[string]any{"question": "短视频使用对大学生注意力的影响", "mode": "screen", "columns": []string{"方法"}})
	r := waitJob(t, tc, j["id"].(string))
	if r["status"] != "done" {
		t.Fatalf("筛选应完成：%v", r["log"])
	}
	ps := r["papers"].([]any)
	if len(ps) != 8 {
		t.Fatalf("两个检索式各 4 篇，应有 8 篇：%d %v", len(ps), r["log"])
	}
	p0 := ps[0].(map[string]any)
	if p0["screened"] != true || p0["fields"].(map[string]any)["方法"] != "问卷" || !strings.Contains(p0["paper"].(map[string]any)["gbt"].(string), "[J]") {
		t.Fatalf("筛选结果不对 %v", p0)
	}
	// 同时只能有一个进行中的调研；别人看不到
	if code, _ := s1.do("GET", "/api/research/"+j["id"].(string), nil); code != 404 {
		t.Fatal("别人不应看到")
	}

	// 调研报告：第 1 轮 → 综合 → 按缺口第 2 轮 → 综合（没有新方向）→ 结束
	j = tc.ok("POST", "/api/research", map[string]any{"question": "短视频使用对大学生注意力的影响", "mode": "report", "max_rounds": 3})
	r = waitJob(t, tc, j["id"].(string))
	if r["status"] != "done" {
		t.Fatalf("报告应完成：%v", r["log"])
	}
	rounds := r["rounds"].([]any)
	if len(rounds) != 2 || !strings.Contains(rounds[1].(map[string]any)["decision"].(string), "没有新的检索方向") {
		t.Fatalf("应进行两轮并说明停止原因：%v", rounds)
	}
	rep := r["report"].(map[string]any)
	pts := rep["sections"].([]any)[0].(map[string]any)["points"].([]any)
	if len(pts) != 1 {
		t.Fatalf("无效出处的结论应丢弃：%v", pts)
	}
	list := tc.ok("GET", "/api/research", nil)["list"].([]any)
	if len(list) != 2 {
		t.Fatalf("应列出两个调研：%v", list)
	}
	tc.ok("DELETE", "/api/research/"+j["id"].(string), nil)
	if code, _ := tc.do("GET", "/api/research/"+j["id"].(string), nil); code != 404 {
		t.Fatal("删除后应不存在")
	}
}
