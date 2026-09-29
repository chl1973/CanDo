package main

// 文献数据库访问：OpenAlex（主）+ Crossref（备用）。
//
// OpenAlex 从 2026 年起按“每日额度”计费：不填密钥每天约 0.1 美元额度（全文检索每次约 0.001 美元，约 100 次），
// 而且同一出口 IP 共用——校园网里很多人一起用时很快就会用完，返回 429。
// 填一个免费密钥（openalex.org 注册 30 秒）额度提高到每天 1 美元（约 1000 次检索）。
// 这里做了四件事：支持密钥（团队 / 个人）、区分“今日额度用完”和“一时太快”、缓存相同检索、额度用完或连不上时自动改用 Crossref。

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var crossrefBase = "https://api.crossref.org"

// ---------------- 额度状态 ----------------

type oaQuota struct {
	Remaining float64
	Limit     float64
	PerSearch float64 // 一次检索消耗的额度（从响应头得到）
	ResetAt   time.Time
	Exhausted time.Time // 在此时间之前视为额度已用完
	At        time.Time
}

var (
	oaMu    sync.Mutex
	oaState = map[string]*oaQuota{} // key：密钥的前 8 位；"" 表示没有密钥
)

func quotaKey(key string) string {
	if len(key) > 8 {
		return key[:8]
	}
	return key
}

func oaGet(key string) oaQuota {
	oaMu.Lock()
	defer oaMu.Unlock()
	if q := oaState[quotaKey(key)]; q != nil {
		return *q
	}
	return oaQuota{}
}

func oaUpdate(key string, h http.Header, exhausted bool) {
	oaMu.Lock()
	defer oaMu.Unlock()
	q := oaState[quotaKey(key)]
	if q == nil {
		q = &oaQuota{}
		oaState[quotaKey(key)] = q
	}
	q.At = time.Now()
	if v, err := strconv.ParseFloat(h.Get("X-RateLimit-Remaining"), 64); err == nil {
		q.Remaining = v
	}
	if v, err := strconv.ParseFloat(h.Get("X-RateLimit-Limit"), 64); err == nil {
		q.Limit = v
	}
	if v, err := strconv.ParseFloat(h.Get("X-RateLimit-Credits-Used"), 64); err == nil && v > 0 {
		q.PerSearch = v
	}
	if v, err := strconv.ParseFloat(h.Get("X-RateLimit-Reset"), 64); err == nil && v > 0 {
		q.ResetAt = time.Now().Add(time.Duration(v) * time.Second)
	} else if q.ResetAt.Before(time.Now()) {
		// 没有给出时按 UTC 零点（北京时间 8 点）重置
		n := time.Now().UTC()
		q.ResetAt = time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, time.UTC)
	}
	if exhausted {
		q.Exhausted = q.ResetAt
	}
}

func oaExhausted(key string) bool {
	q := oaGet(key)
	return time.Now().Before(q.Exhausted)
}

func resetText(t time.Time) string {
	if t.IsZero() {
		return "明天"
	}
	lt := t.In(time.Local)
	if lt.Format("20060102") == time.Now().Format("20060102") {
		return "今天 " + lt.Format("15:04")
	}
	return "明天 " + lt.Format("15:04")
}

var (
	errOAQuota = errors.New("quota")
	errOABusy  = errors.New("busy")
	errOAKey   = errors.New("OpenAlex 密钥无效，请在“设置 → 文献数据库”重新填写")
)

// ---------------- 检索缓存 ----------------

type cachedSearch struct {
	papers []Paper
	total  int
	source string
	at     time.Time
}

var (
	scMu    sync.Mutex
	scCache = map[string]cachedSearch{}
)

func cacheGet(k string) (cachedSearch, bool) {
	scMu.Lock()
	defer scMu.Unlock()
	c, ok := scCache[k]
	if ok && time.Since(c.at) > 12*time.Hour {
		delete(scCache, k)
		ok = false
	}
	return c, ok
}

func cachePut(k string, c cachedSearch) {
	scMu.Lock()
	defer scMu.Unlock()
	if len(scCache) >= 300 {
		var oldK string
		var oldT time.Time
		for k2, v := range scCache {
			if oldK == "" || v.at.Before(oldT) {
				oldK, oldT = k2, v.at
			}
		}
		delete(scCache, oldK)
	}
	c.at = time.Now()
	scCache[k] = c
}

// ---------------- 密钥 ----------------

func (a *App) openAlexKey(me *Me) string {
	var k string
	a.store.View(func(db *DB) {
		if u := db.User(me.ID); u != nil && u.OpenAlexKey != "" {
			k = a.dec(u.OpenAlexKey)
			return
		}
		k = a.dec(db.Settings.OpenAlexKey)
	})
	return k
}

func (a *App) contactEmail() string {
	var e string
	a.store.View(func(db *DB) { e = db.Settings.ContactEmail })
	return e
}

// ---------------- 统一检索入口 ----------------

type scholarResult struct {
	Papers []Paper
	Total  int
	Source string
	Notice string
	Cached bool
}

func (a *App) scholarSearch(me *Me, q string, yearFrom, yearTo int, oaOnly bool, sortBy string, page int) (scholarResult, error) {
	key := a.openAlexKey(me)
	ck := strings.Join([]string{strings.ToLower(q), itoa(yearFrom), itoa(yearTo), strconv.FormatBool(oaOnly), sortBy, itoa(max(page, 1))}, "|")
	if c, ok := cacheGet(ck); ok {
		return scholarResult{Papers: c.papers, Total: c.total, Source: c.source, Cached: true, Notice: "12 小时内检索过相同内容，直接显示上次的结果（不消耗额度）"}, nil
	}
	var oaErr error
	if !oaExhausted(key) {
		papers, total, err := searchOpenAlex(key, q, yearFrom, yearTo, oaOnly, sortBy, page)
		if err == errOABusy {
			time.Sleep(1500 * time.Millisecond)
			papers, total, err = searchOpenAlex(key, q, yearFrom, yearTo, oaOnly, sortBy, page)
		}
		if err == nil {
			cachePut(ck, cachedSearch{papers: papers, total: total, source: "OpenAlex"})
			return scholarResult{Papers: papers, Total: total, Source: "OpenAlex"}, nil
		}
		if err == errOAKey {
			return scholarResult{}, err
		}
		oaErr = err
	} else {
		oaErr = errOAQuota
	}
	// 备用：Crossref
	why := ""
	switch oaErr {
	case errOAQuota:
		q := oaGet(key)
		why = "OpenAlex 今日免费额度已用完（" + resetText(q.ResetAt) + " 重置）"
		if key == "" {
			why += "。不填密钥时额度很少，而且同一个校园网的人共用"
		}
	case errOABusy:
		why = "OpenAlex 暂时繁忙"
	default:
		why = "连不上 OpenAlex（" + oaErr.Error() + "）"
	}
	papers, total, err := searchCrossref(a.contactEmail(), q, yearFrom, yearTo, sortBy, page)
	if err != nil {
		if oaErr == errOAQuota {
			return scholarResult{}, errors.New(why + "，备用的 Crossref 也暂时不可用。" + map[bool]string{true: "请在“设置 → 文献数据库”填一个免费的 OpenAlex 密钥（每天约 1000 次），或", false: "请"}[key == ""] + "稍后再试，也可以先用知网/万方检索后导入题录")
		}
		return scholarResult{}, errors.New(why + "，备用的 Crossref 也不可用（" + err.Error() + "）。请确认这台电脑能上网，或先用知网/万方检索后导入题录")
	}
	cachePut(ck, cachedSearch{papers: papers, total: total, source: "Crossref"})
	notice := why + "，已自动改用 Crossref 检索。Crossref 的结果多数没有摘要，也不能判断能否免费下载全文"
	if oaOnly {
		notice += "（“只看可免费下载全文”这次没有生效）"
	}
	if oaErr == errOAQuota && key == "" {
		notice += "。在“设置 → 文献数据库”填一个免费的 OpenAlex 密钥，每天可检索约 1000 次"
	}
	return scholarResult{Papers: papers, Total: total, Source: "Crossref", Notice: notice + "。"}, nil
}

// ---------------- OpenAlex ----------------

func searchOpenAlex(key, q string, yearFrom, yearTo int, oaOnly bool, sortBy string, page int) ([]Paper, int, error) {
	v := url.Values{}
	v.Set("search", q)
	v.Set("per-page", "20")
	if page < 1 {
		page = 1
	}
	v.Set("page", itoa(page))
	v.Set("select", "id,doi,display_name,publication_year,type,language,cited_by_count,authorships,primary_location,best_oa_location,open_access,biblio,abstract_inverted_index")
	var filters []string
	if yearFrom > 0 || yearTo > 0 {
		from, to := yearFrom, yearTo
		if from == 0 {
			from = 1900
		}
		if to == 0 {
			to = time.Now().Year() + 1
		}
		filters = append(filters, "publication_year:"+itoa(from)+"-"+itoa(to))
	}
	if oaOnly {
		filters = append(filters, "is_oa:true")
	}
	if len(filters) > 0 {
		v.Set("filter", strings.Join(filters, ","))
	}
	switch sortBy {
	case "cited":
		v.Set("sort", "cited_by_count:desc")
	case "new":
		v.Set("sort", "publication_date:desc")
	}
	if key != "" {
		v.Set("api_key", key)
	}
	resp, err := oaDo(key, openAlexBase+"/works?"+v.Encode())
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
		Results []oaWork `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(&out); err != nil {
		return nil, 0, errors.New("返回的内容无法识别")
	}
	papers := []Paper{}
	for _, w := range out.Results {
		if strings.TrimSpace(w.DisplayName) != "" {
			papers = append(papers, w.toPaper())
		}
	}
	return papers, out.Meta.Count, nil
}

var reQuotaBody = regexp.MustCompile(`(?i)(budget|credit|quota|daily|exceed|insufficient)`)

// oaDo 发起 OpenAlex 请求，并把额度相关的错误分类。
func oaDo(key, u string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "KeyanWorkbench/"+AppVersion)
	resp, err := paperClient.Do(req)
	if err != nil {
		return nil, errors.New("网络不通")
	}
	switch {
	case resp.StatusCode == 200:
		oaUpdate(key, resp.Header, false)
		return resp, nil
	case resp.StatusCode == 429:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		rem, remErr := strconv.ParseFloat(resp.Header.Get("X-RateLimit-Remaining"), 64)
		daily := (remErr == nil && rem <= 0) || reQuotaBody.Match(b)
		oaUpdate(key, resp.Header, daily)
		if daily {
			return nil, errOAQuota
		}
		return nil, errOABusy
	case (resp.StatusCode == 401 || resp.StatusCode == 403) && key != "":
		resp.Body.Close()
		return nil, errOAKey
	default:
		resp.Body.Close()
		return nil, errors.New("状态 " + itoa(resp.StatusCode))
	}
}

// ---------------- Crossref ----------------

type crItem struct {
	DOI    string   `json:"DOI"`
	Title  []string `json:"title"`
	Author []struct {
		Given  string `json:"given"`
		Family string `json:"family"`
		Name   string `json:"name"`
	} `json:"author"`
	Container []string `json:"container-title"`
	Issued    struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"issued"`
	Volume   string `json:"volume"`
	Issue    string `json:"issue"`
	Page     string `json:"page"`
	Type     string `json:"type"`
	Cited    int    `json:"is-referenced-by-count"`
	Abstract string `json:"abstract"`
	URL      string `json:"URL"`
	Language string `json:"language"`
}

var crTypes = map[string]string{"journal-article": "article", "proceedings-article": "proceedings-article", "book-chapter": "book-chapter",
	"book": "book", "monograph": "book", "edited-book": "book", "dissertation": "dissertation", "posted-content": "preprint",
	"report": "report", "dataset": "dataset", "standard": "standard"}

var reTags = regexp.MustCompile(`<[^>]+>`)

func (it crItem) toPaper() Paper {
	p := Paper{DOI: strings.ToLower(it.DOI), Volume: it.Volume, Issue: it.Issue, Type: crTypes[it.Type], Cited: it.Cited,
		LandingURL: it.URL, Language: it.Language, Source: "crossref", Authors: []string{}}
	if len(it.Title) > 0 {
		p.Title = strings.TrimSpace(reTags.ReplaceAllString(it.Title[0], ""))
	}
	for _, a := range it.Author {
		switch {
		case a.Name != "":
			p.Authors = append(p.Authors, a.Name)
		case hasCJK(a.Family + a.Given):
			p.Authors = append(p.Authors, a.Family+a.Given)
		case a.Given != "":
			p.Authors = append(p.Authors, a.Family+", "+a.Given)
		case a.Family != "":
			p.Authors = append(p.Authors, a.Family)
		}
	}
	if len(it.Container) > 0 {
		p.Venue = it.Container[0]
	}
	if len(it.Issued.DateParts) > 0 && len(it.Issued.DateParts[0]) > 0 {
		p.Year = it.Issued.DateParts[0][0]
	}
	if pg := strings.TrimSpace(it.Page); pg != "" {
		parts := strings.SplitN(pg, "-", 2)
		p.FirstPage = strings.TrimSpace(parts[0])
		if len(parts) == 2 {
			p.LastPage = strings.TrimSpace(parts[1])
		}
	}
	if it.Abstract != "" {
		p.Abstract = strings.TrimSpace(strings.Join(strings.Fields(reTags.ReplaceAllString(it.Abstract, " ")), " "))
		p.Abstract = strings.TrimPrefix(p.Abstract, "Abstract ")
	}
	p.GBT = gbt(p)
	return p
}

func crossrefGet(email, path string, v url.Values) (*http.Response, error) {
	if email != "" {
		v.Set("mailto", email)
	}
	req, _ := http.NewRequest("GET", crossrefBase+path+"?"+v.Encode(), nil)
	ua := "KeyanWorkbench/" + AppVersion
	if email != "" {
		ua += " (mailto:" + email + ")"
	}
	req.Header.Set("User-Agent", ua)
	resp, err := paperClient.Do(req)
	if err != nil {
		return nil, errors.New("网络不通")
	}
	if resp.StatusCode == 429 {
		resp.Body.Close()
		time.Sleep(1200 * time.Millisecond)
		if resp, err = paperClient.Do(req); err != nil {
			return nil, errors.New("网络不通")
		}
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		if resp.StatusCode == 429 {
			return nil, errors.New("太多人同时检索")
		}
		return nil, errors.New("状态 " + itoa(resp.StatusCode))
	}
	return resp, nil
}

func searchCrossref(email, q string, yearFrom, yearTo int, sortBy string, page int) ([]Paper, int, error) {
	if page < 1 {
		page = 1
	}
	v := url.Values{}
	v.Set("query", q)
	v.Set("rows", "20")
	v.Set("offset", itoa((page-1)*20))
	v.Set("select", "DOI,title,author,container-title,issued,volume,issue,page,type,is-referenced-by-count,abstract,URL,language")
	var f []string
	if yearFrom > 0 {
		f = append(f, "from-pub-date:"+itoa(yearFrom))
	}
	if yearTo > 0 {
		f = append(f, "until-pub-date:"+itoa(yearTo))
	}
	if len(f) > 0 {
		v.Set("filter", strings.Join(f, ","))
	}
	switch sortBy {
	case "cited":
		v.Set("sort", "is-referenced-by-count")
		v.Set("order", "desc")
	case "new":
		v.Set("sort", "published")
		v.Set("order", "desc")
	}
	resp, err := crossrefGet(email, "/works", v)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Message struct {
			Total int      `json:"total-results"`
			Items []crItem `json:"items"`
		} `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(&out); err != nil {
		return nil, 0, errors.New("返回的内容无法识别")
	}
	papers := []Paper{}
	for _, it := range out.Message.Items {
		if p := it.toPaper(); p.Title != "" {
			papers = append(papers, p)
		}
	}
	return papers, out.Message.Total, nil
}

// ---------------- 设置：密钥与状态 ----------------

func (a *App) scholarStatus(me *Me) map[string]any {
	key := a.openAlexKey(me)
	var mine, team bool
	var email string
	a.store.View(func(db *DB) {
		if u := db.User(me.ID); u != nil {
			mine = u.OpenAlexKey != ""
		}
		team = db.Settings.OpenAlexKey != ""
		email = db.Settings.ContactEmail
	})
	q := oaGet(key)
	out := map[string]any{"my_key": mine, "team_key": team, "using": map[bool]string{true: "mine", false: map[bool]string{true: "team", false: "none"}[team]}[mine],
		"exhausted": time.Now().Before(q.Exhausted), "reset": resetText(q.ResetAt), "checked": !q.At.IsZero()}
	if me.IsAdmin() {
		out["contact_email"] = email
	}
	if !q.At.IsZero() && q.PerSearch > 0 {
		out["searches_left"] = int(q.Remaining/q.PerSearch + 1e-6)
	}
	return out
}

func (a *App) hScholarStatus(w http.ResponseWriter, r *http.Request, me *Me) error {
	writeJSON(w, 200, a.scholarStatus(me))
	return nil
}

var reOAKey = regexp.MustCompile(`^[A-Za-z0-9_\-]{8,128}$`)

func (a *App) hScholarKey(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Scope        string  `json:"scope"` // me / team
		Key          *string `json:"key"`
		ContactEmail *string `json:"contact_email"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.Scope == "team" && !me.IsAdmin() {
		return errForbidden("只有管理员可以设置团队的 OpenAlex 密钥")
	}
	if in.Key != nil {
		k := strings.TrimSpace(*in.Key)
		if k != "" {
			if !reOAKey.MatchString(k) {
				return errBad("密钥格式不对：请从 openalex.org/settings/api 复制完整的密钥")
			}
			// 用一次很便宜的请求验证密钥
			resp, err := oaDo(k, openAlexBase+"/works?per-page=1&select=id&filter=publication_year:2020&api_key="+url.QueryEscape(k))
			switch {
			case err == errOAKey:
				return errBad("OpenAlex 说这个密钥无效，请确认复制完整")
			case err == errOAQuota:
				// 密钥有效但今天额度已用完
			case err != nil && err != errOABusy:
				return errBad("暂时无法连接 OpenAlex 验证密钥（" + err.Error() + "），请稍后再试")
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
		enc := ""
		if k != "" {
			enc = a.enc(k)
		}
		err := a.store.Update(func(db *DB) error {
			if in.Scope == "team" {
				db.Settings.OpenAlexKey = enc
				return nil
			}
			u := db.User(me.ID)
			if u == nil {
				return errNotFound("用户不存在")
			}
			u.OpenAlexKey = enc
			return nil
		})
		if err != nil {
			return err
		}
	}
	if in.ContactEmail != nil {
		if !me.IsAdmin() {
			return errForbidden("只有管理员可以设置联系邮箱")
		}
		e := strings.TrimSpace(*in.ContactEmail)
		if e != "" && (!strings.Contains(e, "@") || len(e) > 100 || strings.ContainsAny(e, " <>\"")) {
			return errBad("邮箱格式不对")
		}
		a.store.Update(func(db *DB) error { db.Settings.ContactEmail = e; return nil })
	}
	writeJSON(w, 200, a.scholarStatus(me))
	return nil
}
