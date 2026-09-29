package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func (c *client) async(path string, body any, title string) string {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-KY", "1")
	req.Header.Set("X-KY-Async", "1")
	req.Header.Set("X-KY-Title", url.QueryEscape(title))
	req.Header.Set("X-KY-Link", url.QueryEscape("#/writing/guide"))
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	json.Unmarshal(raw, &m)
	if resp.StatusCode != 202 {
		c.t.Fatalf("应立即返回 202：%d %s", resp.StatusCode, raw)
	}
	return m["job_id"].(string)
}

func TestBackgroundJobs(t *testing.T) {
	_, srv := newEnv(t)
	tc, s1, _, _, _ := setupTeam(t, srv)
	chatJSON = goodModel
	tc.ok("PUT", "/api/settings", map[string]any{"llm_base_url": "http://fake", "llm_model": "m", "llm_key": "k"})
	release := make(chan struct{})
	var running, peak int32
	var mu sync.Mutex
	chatJSON = func(c ModelCfg, system, user string) (map[string]any, error) {
		if strings.Contains(system, "带新同学写论文") {
			n := atomic.AddInt32(&running, 1)
			mu.Lock()
			if n > peak {
				peak = n
			}
			mu.Unlock()
			<-release
			atomic.AddInt32(&running, -1)
			if strings.Contains(user, "出错") {
				return nil, &LLMError{"模型服务暂时不可用"}
			}
			return map[string]any{"sections": []any{map[string]any{"name": "摘要", "questions": []any{"做了什么？"}}}, "next": []any{"写初稿"}}, nil
		}
		return goodModel(c, system, user)
	}
	defer func() { chatJSON = goodModel }()

	// 同一个人发起 5 个任务：立即返回；同时运行的不超过 3 个
	var ids []string
	for i := 0; i < 5; i++ {
		topic := "题目" + itoa(i)
		if i == 4 {
			topic = "会出错的题目"
		}
		ids = append(ids, s1.async("/api/writing/outline", map[string]any{"profile": "course", "topic": topic}, topic))
	}
	time.Sleep(300 * time.Millisecond)
	r := s1.ok("GET", "/api/jobs", nil)
	if n := int(r["running"].(float64)); n != 5 {
		t.Fatalf("5 个任务都应在进行或排队：%v", r)
	}
	queued := 0
	for _, x := range r["jobs"].([]any) {
		j := x.(map[string]any)
		if j["status"] == "queued" {
			queued++
		}
		if j["label"] != "AI 列提纲" || j["link"] != "#/writing/guide" {
			t.Fatalf("任务信息不对 %v", j)
		}
	}
	if queued != 2 || atomic.LoadInt32(&running) != 3 {
		t.Fatalf("每人最多同时运行 3 个：排队 %d，运行 %d", queued, running)
	}
	// 别人看不到我的任务
	if code, _ := tc.do("GET", "/api/jobs/"+ids[0], nil); code != 404 {
		t.Fatal("别人不应看到我的任务")
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		r = s1.ok("GET", "/api/jobs", nil)
		if r["running"].(float64) == 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if peak > 3 {
		t.Fatalf("同时运行峰值 %d 超过 3", peak)
	}
	j := s1.ok("GET", "/api/jobs/"+ids[0], nil)
	if j["job"].(map[string]any)["status"] != "done" || j["result"].(map[string]any)["profile"] != "course" {
		t.Fatalf("任务结果不对 %v", j)
	}
	e := s1.ok("GET", "/api/jobs/"+ids[4], nil)["job"].(map[string]any)
	if e["status"] != "error" || !strings.Contains(e["error"].(string), "暂时不可用") {
		t.Fatalf("出错的任务应带原因 %v", e)
	}
	// 不带异步请求头时照常同步返回
	release2 := s1.ok("POST", "/api/writing/outline", map[string]any{"profile": "course", "topic": "同步"})
	if release2["profile"] != "course" {
		t.Fatal("同步调用应照常返回")
	}
	s1.ok("DELETE", "/api/jobs/"+ids[0], nil)
	if code, _ := s1.do("GET", "/api/jobs/"+ids[0], nil); code != 404 {
		t.Fatal("移除后应不存在")
	}
}
