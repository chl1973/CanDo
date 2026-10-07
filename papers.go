package main

// 论文检索：
//   - 在线检索：OpenAlex 公开数据库（英文为主，收录部分中文期刊）；
//   - 知网 / 万方 / 百度学术：没有开放接口，前端提供一键跳转，并支持导入它们导出的题录（RIS / EndNote / NoteExpress / Refworks）；
//   - 收入资料库：有开放全文 PDF 的，经服务器代为下载后在浏览器解析；没有的，保存题录和摘要（明确标注“不是全文”）；
//   - GB/T 7714 参考文献格式；检索过程留痕（项目中老师可查看）。

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
)

type Paper struct {
	ID         string   `json:"id"`
	DOI        string   `json:"doi"`
	Title      string   `json:"title"`
	Authors    []string `json:"authors"`
	Year       int      `json:"year"`
	Venue      string   `json:"venue"`
	Volume     string   `json:"volume"`
	Issue      string   `json:"issue"`
	FirstPage  string   `json:"first_page"`
	LastPage   string   `json:"last_page"`
	Type       string   `json:"type"`
	Cited      int      `json:"cited"`
	IsOA       bool     `json:"is_oa"`
	PDFURL     string   `json:"pdf_url"`
	LandingURL string   `json:"landing_url"`
	Abstract   string   `json:"abstract"`
	Language   string   `json:"language"`
	Source     string   `json:"source"` // openalex / import
	GBT        string   `json:"gbt"`
}

type LoggedPaper struct {
	Title      string    `json:"title"`
	DOI        string    `json:"doi"`
	MaterialID string    `json:"material_id"`
	Mode       string    `json:"mode"` // 全文 / 题录
	At         time.Time `json:"at"`
}

type SearchLog struct {
	ID        string        `json:"id"`
	ProjectID string        `json:"project_id"`
	UserID    int           `json:"user_id"`
	Kind      string        `json:"kind"` // search / import
	Query     string        `json:"query"`
	Filters   string        `json:"filters"`
	Count     int           `json:"count"`
	At        time.Time     `json:"at"`
	Added     []LoggedPaper `json:"added"`
}

// ---------------- GB/T 7714 ----------------

func hasCJK(s string) bool {
	for _, r := range s {
		if isCJK(r) {
			return true
		}
	}
	return false
}

func gbtName(n string) string {
	n = strings.TrimSpace(n)
	if n == "" || hasCJK(n) {
		return n
	}
	if strings.Contains(n, ",") { // “Smith, John A.”
		parts := strings.SplitN(n, ",", 2)
		n = strings.TrimSpace(parts[1]) + " " + strings.TrimSpace(parts[0])
	}
	f := strings.Fields(strings.ReplaceAll(n, ".", " "))
	if len(f) == 1 {
		return strings.ToUpper(f[0])
	}
	out := strings.ToUpper(f[len(f)-1])
	for _, g := range f[:len(f)-1] {
		for _, part := range strings.Split(g, "-") {
			rs := []rune(part)
			if len(rs) > 0 {
				out += " " + string(unicode.ToUpper(rs[0]))
			}
		}
	}
	return out
}

func gbtType(t string) string {
	switch t {
	case "book", "book-chapter", "monograph":
		return "M"
	case "dissertation", "thesis":
		return "D"
	case "proceedings", "proceedings-article", "conference":
		return "C"
	case "report":
		return "R"
	case "standard":
		return "S"
	case "preprint", "posted-content":
		return "EB/OL"
	case "dataset":
		return "DS"
	case "webpage":
		return "EB/OL"
	}
	return "J"
}

func gbt(p Paper) string {
	var names []string
	cjk := hasCJK(p.Title)
	for i, a := range p.Authors {
		if i == 3 {
			if cjk {
				names = append(names, "等")
			} else {
				names = append(names, "et al")
			}
			break
		}
		if n := gbtName(a); n != "" {
			names = append(names, n)
		}
	}
	var b strings.Builder
	if len(names) > 0 {
		b.WriteString(strings.Join(names, ", ") + ". ")
	}
	b.WriteString(strings.TrimRight(p.Title, ".。") + "[" + gbtType(p.Type) + "]. ")
	var tail []string
	if p.Venue != "" {
		tail = append(tail, p.Venue)
	}
	if p.Year > 0 {
		tail = append(tail, itoa(p.Year))
	}
	s := strings.Join(tail, ", ")
	if p.Volume != "" {
		s += ", " + p.Volume
		if p.Issue != "" {
			s += "(" + p.Issue + ")"
		}
	} else if p.Issue != "" {
		s += "(" + p.Issue + ")"
	}
	if p.FirstPage != "" {
		s += ": " + p.FirstPage
		if p.LastPage != "" && p.LastPage != p.FirstPage {
			s += "-" + p.LastPage
		}
	}
	if s != "" {
		b.WriteString(s + ".")
	}
	if p.DOI != "" {
		b.WriteString(" DOI: " + p.DOI + ".")
	}
	return strings.TrimSpace(b.String())
}

// ---------------- OpenAlex ----------------

var paperClient = &http.Client{Timeout: 20 * time.Second}

type oaWork struct {
	ID              string `json:"id"`
	DOI             string `json:"doi"`
	DisplayName     string `json:"display_name"`
	PublicationYear int    `json:"publication_year"`
	Type            string `json:"type"`
	Language        string `json:"language"`
	CitedByCount    int    `json:"cited_by_count"`
	Authorships     []struct {
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	PrimaryLocation *struct {
		LandingPageURL string `json:"landing_page_url"`
		Source         *struct {
			DisplayName string `json:"display_name"`
		} `json:"source"`
	} `json:"primary_location"`
	BestOALocation *struct {
		PDFURL         string `json:"pdf_url"`
		LandingPageURL string `json:"landing_page_url"`
	} `json:"best_oa_location"`
	OpenAccess struct {
		IsOA  bool   `json:"is_oa"`
		OAURL string `json:"oa_url"`
	} `json:"open_access"`
	Biblio struct {
		Volume    string `json:"volume"`
		Issue     string `json:"issue"`
		FirstPage string `json:"first_page"`
		LastPage  string `json:"last_page"`
	} `json:"biblio"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
}

func invertAbstract(idx map[string][]int) string {
	if len(idx) == 0 {
		return ""
	}
	type wp struct {
		w string
		p int
	}
	var ws []wp
	for w, ps := range idx {
		for _, p := range ps {
			ws = append(ws, wp{w, p})
		}
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].p < ws[j].p })
	var b strings.Builder
	for i, x := range ws {
		if i > 0 && !(isCJK([]rune(x.w)[0]) && isCJK(lastRune(b.String()))) {
			b.WriteString(" ")
		}
		b.WriteString(x.w)
	}
	return b.String()
}

func lastRune(s string) rune {
	rs := []rune(s)
	if len(rs) == 0 {
		return ' '
	}
	return rs[len(rs)-1]
}

func (w oaWork) toPaper() Paper {
	p := Paper{ID: w.ID, Title: strings.TrimSpace(w.DisplayName), Year: w.PublicationYear, Type: w.Type, Language: w.Language,
		Cited: w.CitedByCount, IsOA: w.OpenAccess.IsOA, Source: "openalex", Volume: w.Biblio.Volume, Issue: w.Biblio.Issue,
		FirstPage: w.Biblio.FirstPage, LastPage: w.Biblio.LastPage, Abstract: invertAbstract(w.AbstractInvertedIndex)}
	p.DOI = strings.TrimPrefix(strings.TrimPrefix(w.DOI, "https://doi.org/"), "http://doi.org/")
	for _, a := range w.Authorships {
		if n := strings.TrimSpace(a.Author.DisplayName); n != "" {
			p.Authors = append(p.Authors, n)
		}
	}
	if w.PrimaryLocation != nil {
		p.LandingURL = w.PrimaryLocation.LandingPageURL
		if w.PrimaryLocation.Source != nil {
			p.Venue = w.PrimaryLocation.Source.DisplayName
		}
	}
	if w.BestOALocation != nil {
		p.PDFURL = w.BestOALocation.PDFURL
		if p.LandingURL == "" {
			p.LandingURL = w.BestOALocation.LandingPageURL
		}
	}
	if p.LandingURL == "" && p.DOI != "" {
		p.LandingURL = "https://doi.org/" + p.DOI
	}
	if p.Authors == nil {
		p.Authors = []string{}
	}
	p.GBT = gbt(p)
	return p
}

// ---------------- 题录导入（知网 / 万方 / 维普导出） ----------------

var reRISLine = regexp.MustCompile(`^([A-Z][A-Z0-9])  - ?(.*)$`)
var reENWLine = regexp.MustCompile(`^%([0-9A-Z@!#])\s+(.*)$`)
var reNELine = regexp.MustCompile(`^\{([^}]+)\}:\s*(.*)$`)
var reRWLine = regexp.MustCompile(`^(RT|A1|T1|JF|YR|VO|IS|OP|AB|DO|UL|K1|SR|PB|FD)\s+(.*)$`)

func splitAuthors(s string) []string {
	var out []string
	for _, a := range regexp.MustCompile(`[;；,，]`).Split(s, -1) {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func parsePages(p *Paper, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	parts := regexp.MustCompile(`\s*[-–~]\s*`).Split(s, 2)
	p.FirstPage = parts[0]
	if len(parts) == 2 {
		p.LastPage = parts[1]
	}
}

func typeFromText(t string) string {
	t = strings.ToLower(t)
	switch {
	case strings.Contains(t, "thes") || strings.Contains(t, "学位") || t == "thes":
		return "dissertation"
	case strings.Contains(t, "conf") || strings.Contains(t, "会议") || t == "cpaper":
		return "proceedings-article"
	case strings.Contains(t, "book") || strings.Contains(t, "图书"):
		return "book"
	}
	return "article"
}

// ParseCitationFile 识别 RIS / EndNote / NoteExpress / Refworks / GB/T 7714 文本。
func ParseCitationFile(text string) []Paper {
	text = strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(text, "\uFEFF"), "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")
	var out []Paper
	var cur *Paper
	flush := func() {
		if cur != nil && strings.TrimSpace(cur.Title) != "" {
			if cur.Authors == nil {
				cur.Authors = []string{}
			}
			cur.Source = "import"
			cur.GBT = gbt(*cur)
			out = append(out, *cur)
		}
		cur = nil
	}
	start := func() {
		flush()
		cur = &Paper{Type: "article"}
	}
	year := func(s string) int {
		y, _ := strconv.Atoi(reYear.FindString(s))
		return y
	}
	format := ""
	for _, ln := range lines {
		switch {
		case reRISLine.MatchString(ln):
			format = "ris"
		case reENWLine.MatchString(ln):
			format = "enw"
		case reNELine.MatchString(ln):
			format = "ne"
		case reRWLine.MatchString(ln) && format == "":
			format = "rw"
		}
		if format != "" {
			break
		}
	}
	if format == "" {
		// GB/T 7714 纯文本：每行一条
		for _, ln := range lines {
			ln = strings.TrimSpace(reRefStart.ReplaceAllString(ln, ""))
			if len([]rune(ln)) < 8 {
				continue
			}
			p := Paper{Title: refTitle(ln), Year: year(ln), Type: "article", Authors: []string{}}
			if loc := regexp.MustCompile(`^(.*?)[.．]\s`).FindStringSubmatch(ln); loc != nil && !strings.Contains(loc[1], "[") {
				p.Authors = splitAuthors(loc[1])
			}
			if m := regexp.MustCompile(`\]\s*[.．]\s*([^,，]+)[,，]`).FindStringSubmatch(ln); m != nil {
				p.Venue = strings.TrimSpace(m[1])
			}
			if p.Title != "" {
				cur = &p
				flush()
			}
		}
		return out
	}
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t")
		switch format {
		case "ris":
			m := reRISLine.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			k, v := m[1], strings.TrimSpace(m[2])
			if k == "TY" {
				start()
				cur.Type = map[string]string{"THES": "dissertation", "CONF": "proceedings-article", "CPAPER": "proceedings-article", "BOOK": "book", "CHAP": "book-chapter"}[v]
				if cur.Type == "" {
					cur.Type = "article"
				}
				continue
			}
			if cur == nil {
				start()
			}
			switch k {
			case "TI", "T1":
				cur.Title = v
			case "AU", "A1", "A2":
				cur.Authors = append(cur.Authors, v)
			case "PY", "Y1", "DA":
				if cur.Year == 0 {
					cur.Year = year(v)
				}
			case "JO", "JF", "T2", "JA":
				if cur.Venue == "" {
					cur.Venue = v
				}
			case "VL":
				cur.Volume = v
			case "IS":
				cur.Issue = v
			case "SP":
				cur.FirstPage = v
			case "EP":
				cur.LastPage = v
			case "DO":
				cur.DOI = v
			case "AB", "N2":
				cur.Abstract = v
			case "UR", "L1":
				if cur.LandingURL == "" {
					cur.LandingURL = v
				}
			case "ER":
				flush()
			}
		case "enw":
			m := reENWLine.FindStringSubmatch(ln)
			if m == nil {
				if strings.TrimSpace(ln) == "" {
					flush()
				}
				continue
			}
			k, v := m[1], strings.TrimSpace(m[2])
			if k == "0" {
				start()
				cur.Type = typeFromText(v)
				continue
			}
			if cur == nil {
				start()
			}
			switch k {
			case "T":
				cur.Title = v
			case "A":
				cur.Authors = append(cur.Authors, splitAuthors(v)...)
			case "D":
				cur.Year = year(v)
			case "J", "B":
				cur.Venue = v
			case "V":
				cur.Volume = v
			case "N":
				cur.Issue = v
			case "P":
				parsePages(cur, v)
			case "R":
				cur.DOI = v
			case "X":
				cur.Abstract = v
			case "U":
				cur.LandingURL = v
			}
		case "ne":
			m := reNELine.FindStringSubmatch(ln)
			if m == nil {
				if strings.TrimSpace(ln) == "" {
					flush()
				}
				continue
			}
			k, v := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
			if k == "Reference Type" {
				start()
				cur.Type = typeFromText(v)
				continue
			}
			if cur == nil {
				start()
			}
			switch k {
			case "Title":
				cur.Title = v
			case "Author":
				cur.Authors = append(cur.Authors, splitAuthors(v)...)
			case "Year":
				cur.Year = year(v)
			case "Journal", "Secondary Title":
				cur.Venue = v
			case "Volume":
				cur.Volume = v
			case "Issue":
				cur.Issue = v
			case "Pages":
				parsePages(cur, v)
			case "DOI":
				cur.DOI = v
			case "Abstract":
				cur.Abstract = v
			case "URL":
				cur.LandingURL = v
			}
		case "rw":
			m := reRWLine.FindStringSubmatch(ln)
			if m == nil {
				if strings.TrimSpace(ln) == "" {
					flush()
				}
				continue
			}
			k, v := m[1], strings.TrimSpace(m[2])
			if k == "RT" {
				start()
				cur.Type = typeFromText(v)
				continue
			}
			if cur == nil {
				start()
			}
			switch k {
			case "T1":
				cur.Title = v
			case "A1":
				cur.Authors = append(cur.Authors, splitAuthors(v)...)
			case "YR", "FD":
				if cur.Year == 0 {
					cur.Year = year(v)
				}
			case "JF":
				cur.Venue = v
			case "VO":
				cur.Volume = v
			case "IS":
				cur.Issue = v
			case "OP":
				parsePages(cur, v)
			case "DO":
				cur.DOI = v
			case "AB":
				cur.Abstract = v
			case "UL":
				cur.LandingURL = v
			}
		}
	}
	flush()
	return out
}

// ---------------- 开放全文下载（防止借机访问内网） ----------------

var allowPrivateFetch = false // 仅测试时置为 true

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

var pdfClient = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		Proxy: chooseProxy, // 环境变量里的代理 → Windows 系统代理 → 直连（见 sysproxy.go）
		DialContext: (&net.Dialer{Timeout: 15 * time.Second, Control: func(network, address string, c syscall.RawConn) error {
			return dialAllowed(address)
		}}).DialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("重定向次数过多")
		}
		return nil
	},
}

func (a *App) hPaperFetchPDF(w http.ResponseWriter, r *http.Request, me *Me) error {
	raw := r.URL.Query().Get("url")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errBad("链接无效")
	}
	if !allowPrivateFetch {
		if ip := net.ParseIP(u.Hostname()); ip != nil && blockedIP(ip) {
			return errBad("不能下载内网地址的文件")
		}
		if h := strings.ToLower(u.Hostname()); h == "localhost" || strings.HasSuffix(h, ".local") {
			return errBad("不能下载内网地址的文件")
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 KeyanWorkbench/"+AppVersion)
	req.Header.Set("Accept", "application/pdf,*/*;q=0.8")
	resp, err := pdfClient.Do(req)
	if err != nil {
		return errBad("下载全文失败：无法连接该网站（" + shortErr(err) + "）。可以打开原文页面手动下载 PDF 后上传")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errBad("下载全文失败（网站返回状态 " + itoa(resp.StatusCode) + "），可能需要在出版社网站手动下载")
	}
	head := make([]byte, 5)
	n, _ := io.ReadFull(resp.Body, head)
	if n < 5 || string(head) != "%PDF-" {
		return errBad("该链接打开的不是 PDF 全文（可能是网页或需要登录），请打开原文页面手动下载")
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(head)
	_, err = io.Copy(w, io.LimitReader(resp.Body, maxFileBytes))
	return nil
}

func shortErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "内网") {
		return "目标是内网地址"
	}
	if strings.Contains(s, "timeout") || strings.Contains(s, "deadline") {
		return "超时"
	}
	return "网络错误"
}

// ---------------- 接口 ----------------

func (a *App) checkPaperProject(me *Me, pid string, write bool) error {
	if pid == "" {
		return nil
	}
	var err error
	a.store.View(func(db *DB) {
		p := db.Project(pid)
		if p == nil || !canSee(me, p) {
			err = errNotFound("项目不存在或无权访问")
		} else if write && p.Status == "archived" {
			err = errBad("项目已归档")
		}
	})
	return err
}

func (a *App) addSearchLog(me *Me, pid, kind, q, filters string, count int) string {
	id := newID()
	a.store.Update(func(db *DB) error {
		db.SearchLogs = append(db.SearchLogs, &SearchLog{ID: id, ProjectID: pid, UserID: me.ID, Kind: kind, Query: q, Filters: filters, Count: count, At: now(), Added: []LoggedPaper{}})
		if len(db.SearchLogs) > 5000 {
			db.SearchLogs = db.SearchLogs[len(db.SearchLogs)-5000:]
		}
		if pid != "" {
			verb := "文献检索"
			if kind == "import" {
				verb = "导入题录"
			} else if kind == "zotero" {
				verb = "从 Zotero 导入"
			}
			db.Log(pid, me.ID, verb, q+"（"+itoa(count)+" 条）")
		}
		return nil
	})
	return id
}

func (a *App) hPaperSearch(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Query     string `json:"query"`
		YearFrom  int    `json:"year_from"`
		YearTo    int    `json:"year_to"`
		OAOnly    bool   `json:"oa_only"`
		Sort      string `json:"sort"`
		Page      int    `json:"page"`
		ProjectID string `json:"project_id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return errBad("请输入检索词")
	}
	if err := a.checkPaperProject(me, in.ProjectID, false); err != nil {
		return err
	}
	sr, err := a.scholarSearch(me, in.Query, in.YearFrom, in.YearTo, in.OAOnly, in.Sort, in.Page)
	if err != nil {
		return errBad(err.Error())
	}
	papers, total := sr.Papers, sr.Total
	var f []string
	if in.YearFrom > 0 || in.YearTo > 0 {
		switch {
		case in.YearTo == 0:
			f = append(f, itoa(in.YearFrom)+" 年以来")
		case in.YearFrom == 0:
			f = append(f, itoa(in.YearTo)+" 年及以前")
		default:
			f = append(f, "年份 "+itoa(in.YearFrom)+"–"+itoa(in.YearTo))
		}
	}
	if in.OAOnly {
		f = append(f, "仅免费全文")
	}
	if sr.Source != "OpenAlex" {
		f = append(f, "来源 "+sr.Source)
	}
	if in.Sort == "cited" {
		f = append(f, "按被引排序")
	} else if in.Sort == "new" {
		f = append(f, "按最新排序")
	}
	logID := ""
	if in.Page <= 1 {
		logID = a.addSearchLog(me, in.ProjectID, "search", in.Query, strings.Join(f, "，"), total)
	}
	writeJSON(w, 200, map[string]any{"results": papers, "total": total, "page": max(in.Page, 1), "log_id": logID, "source": sr.Source, "notice": sr.Notice, "cached": sr.Cached})
	return nil
}

const keywordRules = `你是科研文献检索助手，帮助本科生把研究问题拆成检索词。要求：
1. 给出中文和英文关键词（专业术语用规范译名），以及 3–6 条可以直接使用的检索式；
2. 检索式要简洁，适合在学术数据库中检索（英文检索式用于 OpenAlex，中文检索式用于知网、万方）；
3. 不要编造文献、作者或数据；只输出一个 JSON 对象。
输出格式：{"keywords_zh":["..."],"keywords_en":["..."],"queries":[{"query":"...","lang":"en|zh","note":"用途说明"}],"tips":"一两句检索建议"}`

func (a *App) hPaperKeywords(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Question  string `json:"question"`
		ProjectID string `json:"project_id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Question = strings.TrimSpace(in.Question)
	if in.Question == "" {
		return errBad("请先写下你的研究问题")
	}
	if err := a.checkPaperProject(me, in.ProjectID, false); err != nil {
		return err
	}
	cfg := a.modelFor(me, in.ProjectID, "keywords", "", in.Question, 0)
	out, err := chatJSON(cfg, keywordRules, "研究问题：\n"+in.Question)
	if err != nil {
		return errBad(err.Error())
	}
	clip := func(xs []string, n int) []string {
		if len(xs) > n {
			xs = xs[:n]
		}
		if xs == nil {
			xs = []string{}
		}
		return xs
	}
	var qs []map[string]string
	for _, x := range list(out["queries"]) {
		m := obj(x)
		if q := str(m["query"]); q != "" {
			lang := str(m["lang"])
			if lang != "zh" {
				if hasCJK(q) {
					lang = "zh"
				} else {
					lang = "en"
				}
			}
			qs = append(qs, map[string]string{"query": q, "lang": lang, "note": str(m["note"])})
		}
		if len(qs) >= 8 {
			break
		}
	}
	if qs == nil {
		qs = []map[string]string{}
	}
	writeJSON(w, 200, map[string]any{"keywords_zh": clip(strList(out["keywords_zh"]), 12), "keywords_en": clip(strList(out["keywords_en"]), 12),
		"queries": qs, "tips": str(out["tips"]), "model": cfg.Label(), "note": "以上检索词由 AI 生成，仅供参考"})
	return nil
}

func (a *App) hPaperImport(w http.ResponseWriter, r *http.Request, me *Me) error {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(6 << 20); err != nil {
		return errBad("文件过大（不超过 5 MB）")
	}
	defer r.MultipartForm.RemoveAll()
	pid := r.FormValue("project_id")
	if err := a.checkPaperProject(me, pid, false); err != nil {
		return err
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		return errBad("请选择题录文件")
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 5<<20))
	text, err := DecodeText(b)
	if err != nil {
		return errBad("题录文件不是 UTF-8 编码。请在知网导出时选择 UTF-8，或用记事本另存为 UTF-8 后再导入")
	}
	papers := ParseCitationFile(text)
	if len(papers) == 0 {
		return errBad("没有识别出题录。支持 RIS、EndNote、NoteExpress、Refworks 格式，以及每行一条的 GB/T 7714 参考文献")
	}
	logID := a.addSearchLog(me, pid, "import", "导入题录："+fh.Filename, "", len(papers))
	writeJSON(w, 200, map[string]any{"results": papers, "total": len(papers), "log_id": logID, "source": "导入的题录"})
	return nil
}

func (a *App) recordAdded(me *Me, logID string, lp LoggedPaper, pid string) {
	a.store.Update(func(db *DB) error {
		for _, l := range db.SearchLogs {
			if l.ID == logID && l.UserID == me.ID && l.ProjectID == pid {
				l.Added = append(l.Added, lp)
			}
		}
		if pid != "" {
			db.Log(pid, me.ID, "收入文献", lp.Title+"（"+lp.Mode+"）")
		}
		return nil
	})
}

// 没有开放全文时：保存题录和摘要（标注“不是全文”）
func (a *App) hPaperSaveRecord(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Paper       Paper  `json:"paper"`
		ProjectID   string `json:"project_id"`
		SearchLogID string `json:"search_log_id"`
		ZoteroKey   string `json:"zotero_key"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	out, err := a.saveRecord(me, in.Paper, in.ProjectID, in.SearchLogID, in.ZoteroKey)
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

// saveRecord 把一条题录（含摘要）收入资料库，标注“不是全文”。
func (a *App) saveRecord(me *Me, p Paper, pid, searchLogID, zoteroKey string) (map[string]any, error) {
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" {
		return nil, errBad("缺少题名")
	}
	if err := a.checkPaperProject(me, pid, true); err != nil {
		return nil, err
	}
	p.GBT = gbt(p)
	var b strings.Builder
	b.WriteString("# " + p.Title + "\n\n")
	if len(p.Authors) > 0 {
		b.WriteString("作者：" + strings.Join(p.Authors, "；") + "\n\n")
	}
	var meta []string
	if p.Venue != "" {
		meta = append(meta, "来源："+p.Venue)
	}
	if p.Year > 0 {
		meta = append(meta, "年份："+itoa(p.Year))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "；") + "\n\n")
	}
	if p.DOI != "" {
		b.WriteString("DOI：" + p.DOI + "\n\n")
	}
	if p.LandingURL != "" {
		b.WriteString("原文页面：" + p.LandingURL + "\n\n")
	}
	b.WriteString("参考文献格式（GB/T 7714，请核对）：" + p.GBT + "\n\n")
	b.WriteString("## 摘要\n\n")
	if strings.TrimSpace(p.Abstract) != "" {
		b.WriteString(strings.TrimSpace(p.Abstract) + "\n\n")
	} else {
		b.WriteString("（数据库中没有摘要）\n\n")
	}
	b.WriteString("说明：这是题录和摘要，不是论文全文。问答和引用核验只能依据摘要；如需逐句核对引用，请上传全文 PDF。\n")
	data := []byte(b.String())
	pieces, status, errMsg := ParseText(data)
	year := ""
	if p.Year > 0 {
		year = itoa(p.Year)
	}
	m := &Material{ID: newID(), OwnerID: me.ID, ProjectID: pid, Title: p.Title, Filename: "题录-" + safeName(p.Title) + ".md", Ftype: "md",
		Size: int64(len(data)), UploadedAt: now(), Status: status, Error: errMsg, ParseNote: "仅题录与摘要，不是全文",
		Author: strings.Join(p.Authors, "；"), SourceDate: year, DOI: p.DOI, SourceURL: p.LandingURL, ZoteroKey: clipRunes(zoteroKey, 80)}
	out, err := a.saveMaterial(me, m, data, pieces)
	if err != nil {
		return nil, err
	}
	if searchLogID != "" {
		a.recordAdded(me, searchLogID, LoggedPaper{Title: p.Title, DOI: p.DOI, MaterialID: m.ID, Mode: "题录", At: now()}, pid)
	} else if pid != "" {
		a.store.Update(func(db *DB) error { db.Log(pid, me.ID, "收入文献", p.Title+"（题录）"); return nil })
	}
	return out, nil
}

func safeName(s string) string {
	rs := []rune(regexp.MustCompile(`[\\/:*?"<>|\s]+`).ReplaceAllString(s, "_"))
	if len(rs) > 40 {
		rs = rs[:40]
	}
	return string(rs)
}

func (a *App) hPaperLogs(w http.ResponseWriter, r *http.Request, me *Me) error {
	pid := r.URL.Query().Get("project_id")
	if err := a.checkPaperProject(me, pid, false); err != nil {
		return err
	}
	out := []map[string]any{}
	a.store.View(func(db *DB) {
		for i := len(db.SearchLogs) - 1; i >= 0 && len(out) < 100; i-- {
			l := db.SearchLogs[i]
			if l.ProjectID != pid || (pid == "" && l.UserID != me.ID) {
				continue
			}
			out = append(out, map[string]any{"id": l.ID, "user": db.userName(l.UserID), "kind": l.Kind, "query": l.Query, "filters": l.Filters,
				"count": l.Count, "at": l.At, "added": l.Added})
		}
	})
	writeJSON(w, 200, out)
	return nil
}
