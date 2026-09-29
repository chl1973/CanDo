package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func resetScholar() {
	oaMu.Lock()
	oaState = map[string]*oaQuota{}
	oaMu.Unlock()
	scMu.Lock()
	scCache = map[string]cachedSearch{}
	scMu.Unlock()
}

type quotaMock struct {
	mu        sync.Mutex
	oaCalls   []string // 收到的 api_key
	crCalls   int
	mode      string // ok / quota / busy-once
	busyCount int
	lastCR    string
}

func (m *quotaMock) servers(t *testing.T) (*httptest.Server, *httptest.Server) {
	oa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		k := r.URL.Query().Get("api_key")
		m.oaCalls = append(m.oaCalls, k)
		if k == "badkey123" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("X-RateLimit-Limit", "1")
		w.Header().Set("X-RateLimit-Reset", "3600")
		if m.mode == "quota" && k == "" {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"Daily budget exceeded"}`))
			return
		}
		if m.mode == "busy-once" && m.busyCount == 0 {
			m.busyCount++
			w.Header().Set("X-RateLimit-Remaining", "0.8")
			w.WriteHeader(429)
			return
		}
		w.Header().Set("X-RateLimit-Remaining", "0.95")
		w.Header().Set("X-RateLimit-Credits-Used", "0.001")
		json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{"count": 1}, "results": []any{
			map[string]any{"id": "W1", "display_name": "Deep learning for rivers", "publication_year": 2020, "type": "article",
				"authorships": []any{map[string]any{"author": map[string]any{"display_name": "John Smith"}}}}}})
	}))
	cr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.crCalls++
		m.lastCR = r.URL.RawQuery
		m.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"total-results": 57, "items": []any{
			map[string]any{"DOI": "10.1/ABC", "title": []any{"Deep learning for <i>rivers</i>"}, "type": "journal-article",
				"author":          []any{map[string]any{"given": "John", "family": "Smith"}, map[string]any{"given": "三", "family": "张"}},
				"container-title": []any{"Water Research"}, "issued": map[string]any{"date-parts": []any{[]any{2020, 5}}},
				"volume": "12", "issue": "3", "page": "1-10", "is-referenced-by-count": 42,
				"abstract": "<jats:title>Abstract</jats:title><jats:p>We study rivers.</jats:p>"}}}})
	}))
	t.Cleanup(oa.Close)
	t.Cleanup(cr.Close)
	return oa, cr
}

func TestScholarQuotaFallback(t *testing.T) {
	resetScholar()
	defer resetScholar()
	_, srv := newEnv(t)
	tc, s1, _, _, _ := setupTeam(t, srv)
	m := &quotaMock{mode: "quota"}
	oa, cr := m.servers(t)
	oldOA, oldCR := openAlexBase, crossrefBase
	openAlexBase, crossrefBase = oa.URL, cr.URL
	defer func() { openAlexBase, crossrefBase = oldOA, oldCR }()

	// 没有密钥、今日额度用完 → 自动改用 Crossref
	r := s1.ok("POST", "/api/papers/search", map[string]any{"query": "river", "year_from": 2015, "sort": "cited"})
	if r["source"] != "Crossref" || !strings.Contains(r["notice"].(string), "额度已用完") || !strings.Contains(r["notice"].(string), "密钥") {
		t.Fatalf("应改用 Crossref 并说明原因 %v", r)
	}
	p := r["results"].([]any)[0].(map[string]any)
	if p["title"] != "Deep learning for rivers" || p["abstract"] != "We study rivers." || p["gbt"] != "SMITH J, 张三. Deep learning for rivers[J]. Water Research, 2020, 12(3): 1-10. DOI: 10.1/abc." {
		t.Fatalf("Crossref 结果转换不对 %v", p)
	}
	if !strings.Contains(m.lastCR, "from-pub-date%3A2015") || !strings.Contains(m.lastCR, "sort=is-referenced-by-count") {
		t.Fatalf("Crossref 参数不对 %s", m.lastCR)
	}
	// 额度用完后不再反复请求 OpenAlex
	n := len(m.oaCalls)
	s1.ok("POST", "/api/papers/search", map[string]any{"query": "lake"})
	if len(m.oaCalls) != n {
		t.Fatal("额度用完后应直接用备用库，不再请求 OpenAlex")
	}
	// 相同检索走缓存
	c := m.crCalls
	r = s1.ok("POST", "/api/papers/search", map[string]any{"query": "lake"})
	if m.crCalls != c || r["cached"] != true {
		t.Fatal("相同检索应使用缓存")
	}
	st := s1.ok("GET", "/api/scholar/status", nil)
	if st["exhausted"] != true || st["using"] != "none" {
		t.Fatalf("状态不对 %v", st)
	}
	// 填个人密钥：错误的被拒绝，正确的使用独立额度
	if code, _ := s1.do("PUT", "/api/scholar/key", map[string]any{"scope": "me", "key": "badkey123"}); code != 400 {
		t.Fatal("无效密钥应拒绝")
	}
	if code, _ := s1.do("PUT", "/api/scholar/key", map[string]any{"scope": "team", "key": "goodkey123"}); code != 403 {
		t.Fatal("学生不能设置团队密钥")
	}
	st = s1.ok("PUT", "/api/scholar/key", map[string]any{"scope": "me", "key": "goodkey123"})
	if st["using"] != "mine" {
		t.Fatalf("应使用个人密钥 %v", st)
	}
	r = s1.ok("POST", "/api/papers/search", map[string]any{"query": "forest"})
	if r["source"] != "OpenAlex" || m.oaCalls[len(m.oaCalls)-1] != "goodkey123" {
		t.Fatalf("有密钥时应用 OpenAlex 并带上密钥 %v %v", r["source"], m.oaCalls)
	}
	if st = s1.ok("GET", "/api/scholar/status", nil); st["searches_left"].(float64) != 950 {
		t.Fatalf("应估算剩余次数 %v", st)
	}
	// 团队密钥：其他人也能用；管理员可设联系邮箱
	tc.ok("PUT", "/api/scholar/key", map[string]any{"scope": "team", "key": "teamkey123", "contact_email": "lab@example.edu"})
	r = tc.ok("POST", "/api/papers/search", map[string]any{"query": "desert"})
	if m.oaCalls[len(m.oaCalls)-1] != "teamkey123" || r["source"] != "OpenAlex" {
		t.Fatal("应使用团队密钥")
	}
	// 一时太快（429 但额度还有）→ 稍等重试
	resetScholar()
	m.mode = "busy-once"
	r = tc.ok("POST", "/api/papers/search", map[string]any{"query": "ocean"})
	if r["source"] != "OpenAlex" || m.busyCount != 1 {
		t.Fatal("短暂限流应重试成功")
	}
	// 引用核验查重：额度用完时改查 Crossref（带联系邮箱）
	resetScholar()
	m.mode = "quota"
	st2, note, mt, _, doi := checkExists("Deep learning for rivers", "", "lab@example.edu")
	if st2 != "found" || !strings.Contains(note, "Crossref") || mt != "Deep learning for rivers" || doi != "https://doi.org/10.1/abc" || !strings.Contains(m.lastCR, "mailto=lab%40example.edu") {
		t.Fatalf("核验改查 Crossref 不对 %s %s %s %s %s", st2, note, mt, doi, m.lastCR)
	}
}
