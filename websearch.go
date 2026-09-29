package main

// 联网搜索（1.14）：给本机智能体用。国内可用“博查”（open.bochaai.com），国外可用 Tavily。
// 密钥和模型密钥一样加密保存；每人可以填自己的，管理员也可以填一个团队共用的。

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	bochaURL  = "https://api.bochaai.com/v1/web-search"
	tavilyURL = "https://api.tavily.com/search"
)

var webProviders = map[string]string{"bocha": "博查（国内）", "tavily": "Tavily（国外）"}

type webResult struct {
	Title, URL, Snippet, Site, Date string
}

func webSearch(provider, key, query string, n int) ([]webResult, error) {
	var body []byte
	u := bochaURL
	if provider == "tavily" {
		u = tavilyURL
		body, _ = json.Marshal(map[string]any{"query": query, "max_results": n, "search_depth": "basic"})
	} else {
		body, _ = json.Marshal(map[string]any{"query": query, "summary": true, "count": n})
	}
	req, _ := http.NewRequest("POST", u, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Timeout: 25 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, errBad("连接搜索服务失败：" + err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, errBad("搜索服务说密钥无效或没有权限（" + itoa(resp.StatusCode) + "）")
	}
	if resp.StatusCode == 429 {
		return nil, errBad("搜索太频繁或额度用完了，请稍后再试")
	}
	if resp.StatusCode >= 400 {
		return nil, errBad("搜索服务返回错误 " + itoa(resp.StatusCode) + "：" + clipRunes(string(b), 200))
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil, errBad("搜索服务返回的内容无法解析")
	}
	var out []webResult
	if provider == "tavily" {
		for _, x := range list(m["results"]) {
			r := obj(x)
			out = append(out, webResult{Title: str(r["title"]), URL: str(r["url"]), Snippet: str(r["content"]), Date: str(r["published_date"])})
		}
	} else {
		data := obj(m["data"])
		if data == nil {
			data = m
		}
		if c, ok := m["code"].(float64); ok && c != 200 && c != 0 {
			return nil, errBad("搜索服务返回错误：" + clipRunes(str(m["msg"])+str(m["message"]), 200))
		}
		for _, x := range list(obj(data["webPages"])["value"]) {
			r := obj(x)
			sn := str(r["summary"])
			if sn == "" {
				sn = str(r["snippet"])
			}
			out = append(out, webResult{Title: str(r["name"]), URL: str(r["url"]), Snippet: sn, Site: str(r["siteName"]), Date: str(r["datePublished"])})
		}
	}
	return out, nil
}

// webSearchKey 返回可用的搜索服务和密钥：先用自己的，没有再用团队的
func (a *App) webSearchKey(me *Me) (provider, key, source string) {
	a.store.View(func(db *DB) {
		if u := db.User(me.ID); u != nil && u.WebSearchKey != "" {
			provider, key, source = u.WebSearchProvider, a.dec(u.WebSearchKey), "mine"
			return
		}
		if db.Settings.WebSearchKey != "" {
			provider, key, source = db.Settings.WebSearchProvider, a.dec(db.Settings.WebSearchKey), "team"
		}
	})
	if webProviders[provider] == "" {
		provider = "bocha"
	}
	return
}

func (a *App) toolSearchWeb(me *Me, query string) (string, string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", "", errBad("缺少搜索内容 query")
	}
	prov, key, _ := a.webSearchKey(me)
	if key == "" {
		return "", "", errBad("还没有设置联网搜索。请告诉用户：在“设置 → 联网搜索”里填写博查（国内）或 Tavily 的密钥即可使用。现在可以用 search_papers 检索学术文献，或用 fetch_url 读取已知网址")
	}
	rs, err := webSearch(prov, key, clipRunes(query, 200), 8)
	if err != nil {
		return "", "", err
	}
	if len(rs) == 0 {
		return "没有搜到结果，可以换个说法再搜。", "", nil
	}
	var b strings.Builder
	b.WriteString("搜索“" + query + "”（" + webProviders[prov] + "）的结果。这些是网上的内容，可能有误或过时；重要信息请用 fetch_url 打开原网页核对，回答时注明来源链接：\n")
	for i, r := range rs {
		b.WriteString("\n[" + itoa(i+1) + "] " + r.Title + "\n" + r.URL + "\n")
		if r.Site != "" || r.Date != "" {
			b.WriteString(strings.TrimSpace(r.Site+" "+r.Date) + "\n")
		}
		b.WriteString(clipRunes(strings.TrimSpace(r.Snippet), 400) + "\n")
	}
	return b.String(), "", nil
}

func (a *App) hWebSearchStatus(w http.ResponseWriter, r *http.Request, me *Me) error {
	var mine, team, mineProv, teamProv string
	a.store.View(func(db *DB) {
		if u := db.User(me.ID); u != nil && u.WebSearchKey != "" {
			mine, mineProv = maskKey(a.dec(u.WebSearchKey)), u.WebSearchProvider
		}
		if db.Settings.WebSearchKey != "" {
			team, teamProv = "已填写", db.Settings.WebSearchProvider
		}
	})
	_, _, using := a.webSearchKey(me)
	writeJSON(w, 200, map[string]any{"mine": mine, "mine_provider": mineProv, "team": team, "team_provider": teamProv, "using": using,
		"providers": webProviders, "admin": me.IsAdmin()})
	return nil
}

func (a *App) hWebSearchKey(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Scope    string `json:"scope"`
		Provider string `json:"provider"`
		Key      string `json:"key"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.Scope == "team" && !me.IsAdmin() {
		return errForbidden("只有管理员可以设置团队的搜索密钥")
	}
	in.Key = strings.TrimSpace(in.Key)
	if in.Key != "" {
		if webProviders[in.Provider] == "" {
			return errBad("请选择搜索服务")
		}
		if len(in.Key) > 200 || strings.ContainsAny(in.Key, " \n\t") {
			return errBad("密钥格式不对，请完整复制")
		}
		if _, err := webSearch(in.Provider, in.Key, "大学生科研", 1); err != nil {
			return errBad("用这个密钥试搜了一次，没有成功：" + err.Error())
		}
	}
	enc := ""
	if in.Key != "" {
		enc = a.enc(in.Key)
	}
	err := a.store.Update(func(db *DB) error {
		if in.Scope == "team" {
			db.Settings.WebSearchKey, db.Settings.WebSearchProvider = enc, in.Provider
			return nil
		}
		u := db.User(me.ID)
		if u == nil {
			return errNotFound("用户不存在")
		}
		u.WebSearchKey, u.WebSearchProvider = enc, in.Provider
		return nil
	})
	if err != nil {
		return err
	}
	return a.hWebSearchStatus(w, r, me)
}
