package main

// Zotero 直连
//   · 本机 Zotero：Zotero 7 及以上的本地接口（http://127.0.0.1:23119/api/，只读）。只允许坐在运行工作台的电脑前使用，
//     因为读到的是这台电脑上登录的 Zotero 文库；PDF 直接从本机磁盘读取。保存到 Zotero 使用 Zotero 的浏览器插件接口（/connector/saveItems）。
//   · Zotero 云端：Web API v3 + 个人私人密钥（加密保存，每人一份），任何设备都能用，也能读课题组的群组文库。
// 导入的文献进入资料库时记下 Zotero 条目编号，避免重复导入；每次导入都记入检索记录。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	zoteroLocalBase = "http://127.0.0.1:23119"
	zoteroWebBase   = "https://api.zotero.org"
)

// ZoteroCloud 是某个成员的 Zotero 云端连接（密钥加密保存）。
type ZoteroCloud struct {
	UserID   int       `json:"user_id"`
	Username string    `json:"username"`
	KeyEnc   string    `json:"key_enc"`
	Library  bool      `json:"library"`
	Files    bool      `json:"files"`
	Write    bool      `json:"write"`
	Groups   string    `json:"groups"` // all / some / none
	SavedAt  time.Time `json:"saved_at"`
}

var (
	reZKey    = regexp.MustCompile(`^[A-Z0-9]{8}$`)
	reZLib    = regexp.MustCompile(`^(user|group:[0-9]{1,12})$`)
	reZAPIKey = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)
)

var zLocalClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// 云端接口：与下载全文同样禁止访问内网，且不自动跟随重定向（避免把密钥带到文件存储服务器）。
var zWebClient = &http.Client{
	Timeout:       40 * time.Second,
	Transport:     pdfClient.Transport,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

type zConn struct {
	local  bool
	key    string
	userID int
}

func (z zConn) base() string {
	if z.local {
		return zoteroLocalBase + "/api"
	}
	return zoteroWebBase
}

func (z zConn) prefix(lib string) (string, error) {
	if !reZLib.MatchString(lib) {
		return "", errBad("文库参数无效")
	}
	if lib == "user" {
		if z.local {
			return "/users/0", nil
		}
		return "/users/" + itoa(z.userID), nil
	}
	return "/groups/" + strings.TrimPrefix(lib, "group:"), nil
}

func (z zConn) name() string {
	if z.local {
		return "本机 Zotero"
	}
	return "Zotero 云端"
}

func (a *App) zoteroConn(r *http.Request, me *Me, src string) (zConn, error) {
	switch src {
	case "local":
		if !isLoopback(r) {
			return zConn{}, errForbidden("“本机 Zotero”只能在运行工作台的电脑上使用。在手机或其他电脑上，请到“设置 → 我的 Zotero”连接 Zotero 云端")
		}
		return zConn{local: true}, nil
	case "cloud":
		var zc *ZoteroCloud
		a.store.View(func(db *DB) {
			if u := db.User(me.ID); u != nil && u.Zotero != nil {
				c := *u.Zotero
				zc = &c
			}
		})
		if zc == nil {
			return zConn{}, errBad("还没有连接 Zotero 云端：请到“设置 → 我的 Zotero”填写 Zotero 私人密钥")
		}
		return zConn{key: a.dec(zc.KeyEnc), userID: zc.UserID}, nil
	}
	return zConn{}, errBad("请选择 Zotero 来源")
}

func (z zConn) request(method, path string, q url.Values, body []byte, hdr map[string]string) (*http.Response, error) {
	u := z.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Zotero-API-Version", "3")
	req.Header.Set("User-Agent", "KeyanWorkbench/"+AppVersion)
	if !z.local {
		req.Header.Set("Zotero-API-Key", z.key)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	cl := zWebClient
	if z.local {
		cl = zLocalClient
	}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, z.connErr(err)
	}
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, z.statusErr(resp.StatusCode, string(msg))
	}
	return resp, nil
}

func (z zConn) connErr(err error) error {
	if z.local {
		return errBad("没有连接到本机 Zotero：请先打开 Zotero（需要 Zotero 7 或更新版本），再点“重新连接”")
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errBad("连接 Zotero 云端超时，请检查运行工作台的电脑能否上网")
	}
	return errBad("无法连接 Zotero 云端（api.zotero.org），请检查运行工作台的电脑能否上网")
}

func (z zConn) statusErr(code int, body string) error {
	if z.local {
		switch {
		case code == 403:
			return errBad("本机 Zotero 没有开启本地接口：在 Zotero 中打开“编辑 → 设置 → 高级”，勾选“允许此计算机上的其他应用程序与 Zotero 通信”，然后重试")
		case code == 404 && strings.Contains(body, "No endpoint"):
			return errBad("本机 Zotero 版本太旧，不支持本地接口，请升级到 Zotero 7 或更新版本")
		case code == 404:
			return errNotFound("在本机 Zotero 中找不到该条目或附件（可能已删除，或附件文件还没同步到这台电脑）")
		}
		return errBad("本机 Zotero 返回错误（" + itoa(code) + "）")
	}
	switch code {
	case 403:
		return errBad("Zotero 云端拒绝访问：密钥无效，或这个密钥没有访问该文库的权限（可在 zotero.org/settings/keys 修改密钥权限）")
	case 404:
		return errNotFound("在 Zotero 云端找不到该条目或附件文件（可能附件没有同步到 Zotero 云端，或是“链接到文件”的附件）")
	case 429, 503:
		return errBad("Zotero 云端繁忙，请稍后再试")
	}
	return errBad("Zotero 云端返回错误（" + itoa(code) + "）")
}

func (z zConn) getJSON(path string, q url.Values, out any) (int, error) {
	resp, err := z.request("GET", path, q, nil, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 30<<20)).Decode(out); err != nil {
		return 0, errBad(z.name() + " 返回的数据无法识别")
	}
	total, _ := strconv.Atoi(resp.Header.Get("Total-Results"))
	return total, nil
}

// ---------------- 数据转换 ----------------

type zItem struct {
	Key     string `json:"key"`
	Library struct {
		Type string `json:"type"`
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"library"`
	Meta struct {
		ParsedDate  string `json:"parsedDate"`
		NumChildren int    `json:"numChildren"`
	} `json:"meta"`
	Data map[string]any `json:"data"`
}

// zid 是导入到资料库时记下的条目编号：文库类型:文库编号:条目键。本机与云端读到的是同一个文库时编号一致。
func (it zItem) zid() string {
	return it.Library.Type + ":" + itoa(it.Library.ID) + ":" + it.Key
}

var zTypeMap = map[string]string{
	"journalArticle": "article", "magazineArticle": "article", "newspaperArticle": "article",
	"book": "book", "bookSection": "book-chapter", "thesis": "thesis", "conferencePaper": "proceedings-article",
	"report": "report", "preprint": "preprint", "dataset": "dataset", "standard": "standard", "webpage": "webpage",
}

var reExtraDOI = regexp.MustCompile(`(?im)^\s*DOI:\s*(\S+)`)

func cleanDOI(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:", "DOI:"} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimSpace(s)
}

func zCreatorName(c map[string]any) string {
	if n := strings.TrimSpace(str(c["name"])); n != "" {
		return n
	}
	first, last := strings.TrimSpace(str(c["firstName"])), strings.TrimSpace(str(c["lastName"]))
	switch {
	case first == "":
		return last
	case last == "":
		return first
	case hasCJK(first + last):
		return last + first
	}
	return last + ", " + first
}

func zToPaper(it zItem) Paper {
	d := it.Data
	p := Paper{ID: it.Key, Title: strings.TrimSpace(str(d["title"])), Source: "zotero", Abstract: strings.TrimSpace(str(d["abstractNote"])),
		Language: str(d["language"]), LandingURL: strings.TrimSpace(str(d["url"])), Volume: str(d["volume"]), Issue: str(d["issue"])}
	var authors, others []string
	for _, x := range list(d["creators"]) {
		c := obj(x)
		n := zCreatorName(c)
		if n == "" {
			continue
		}
		if t := str(c["creatorType"]); t == "author" || t == "inventor" || t == "presenter" || t == "programmer" {
			authors = append(authors, n)
		} else {
			others = append(others, n)
		}
	}
	if len(authors) == 0 {
		authors = others
	}
	if authors == nil {
		authors = []string{}
	}
	p.Authors = authors
	if y := reYear.FindString(it.Meta.ParsedDate); y != "" {
		p.Year, _ = strconv.Atoi(y)
	} else if y := reYear.FindString(str(d["date"])); y != "" {
		p.Year, _ = strconv.Atoi(y)
	}
	for _, f := range []string{"publicationTitle", "proceedingsTitle", "bookTitle", "conferenceName", "university", "institution", "publisher", "websiteTitle", "repository"} {
		if v := strings.TrimSpace(str(d[f])); v != "" {
			p.Venue = v
			break
		}
	}
	if pg := strings.TrimSpace(str(d["pages"])); pg != "" {
		parts := strings.FieldsFunc(pg, func(r rune) bool { return r == '-' || r == '–' || r == '—' })
		if len(parts) > 0 {
			p.FirstPage = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			p.LastPage = strings.TrimSpace(parts[1])
		}
	}
	p.Type = zTypeMap[str(d["itemType"])]
	p.DOI = cleanDOI(str(d["DOI"]))
	if p.DOI == "" {
		if m := reExtraDOI.FindStringSubmatch(str(d["extra"])); m != nil {
			p.DOI = cleanDOI(m[1])
		}
	}
	if p.Title == "" {
		p.Title = strings.TrimSpace(str(d["filename"]))
	}
	p.GBT = gbt(p)
	return p
}

// paperToZotero 把检索结果转换成 Zotero 条目（Web API 格式；connector 为 true 时用浏览器插件接口格式）。
func paperToZotero(p Paper, collection string, connector bool) map[string]any {
	typ, venueField, doiOK, pagesOK := "journalArticle", "publicationTitle", true, true
	switch p.Type {
	case "proceedings-article", "proceedings", "conference":
		typ, venueField = "conferencePaper", "proceedingsTitle"
	case "book", "monograph":
		typ, venueField, doiOK, pagesOK = "book", "publisher", false, false
	case "book-chapter":
		typ, venueField, doiOK = "bookSection", "bookTitle", false
	case "dissertation", "thesis":
		typ, venueField, doiOK, pagesOK = "thesis", "university", false, false
	case "report":
		typ, venueField, doiOK = "report", "institution", false
	}
	it := map[string]any{"itemType": typ, "title": p.Title}
	var creators []map[string]any
	for _, n := range p.Authors {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		c := map[string]any{"creatorType": "author"}
		first, last := "", n
		if !hasCJK(n) {
			if i := strings.Index(n, ","); i > 0 {
				last, first = strings.TrimSpace(n[:i]), strings.TrimSpace(n[i+1:])
			} else if f := strings.Fields(n); len(f) > 1 {
				last, first = f[len(f)-1], strings.Join(f[:len(f)-1], " ")
			}
		}
		switch {
		case first != "":
			c["firstName"], c["lastName"] = first, last
		case connector:
			c["lastName"], c["fieldMode"] = last, 1
		default:
			c["name"] = last
		}
		creators = append(creators, c)
	}
	if creators == nil {
		creators = []map[string]any{}
	}
	it["creators"] = creators
	if p.Year > 0 {
		it["date"] = itoa(p.Year)
	}
	if p.Venue != "" {
		it[venueField] = p.Venue
	}
	if typ == "journalArticle" {
		if p.Volume != "" {
			it["volume"] = p.Volume
		}
		if p.Issue != "" {
			it["issue"] = p.Issue
		}
	}
	if pagesOK && p.FirstPage != "" {
		pg := p.FirstPage
		if p.LastPage != "" {
			pg += "-" + p.LastPage
		}
		it["pages"] = pg
	}
	if p.DOI != "" {
		if doiOK {
			it["DOI"] = p.DOI
		} else {
			it["extra"] = "DOI: " + p.DOI
		}
	}
	if u := p.LandingURL; u != "" {
		it["url"] = u
	} else if p.DOI != "" {
		it["url"] = "https://doi.org/" + p.DOI
	}
	if p.Abstract != "" {
		it["abstractNote"] = p.Abstract
	}
	if connector {
		it["tags"] = []string{"CanDo 可为"}
		it["attachments"] = []any{}
	} else {
		it["tags"] = []map[string]string{{"tag": "CanDo 可为"}}
		if collection != "" {
			it["collections"] = []string{collection}
		}
	}
	return it
}

// ---------------- 接口 ----------------

func (a *App) zoteroCloudView(me *Me) map[string]any {
	out := map[string]any{"configured": false}
	a.store.View(func(db *DB) {
		if u := db.User(me.ID); u != nil && u.Zotero != nil {
			z := u.Zotero
			out = map[string]any{"configured": true, "user_id": z.UserID, "username": z.Username, "key_masked": maskKey(a.dec(z.KeyEnc)),
				"library": z.Library, "files": z.Files, "write": z.Write, "groups": z.Groups, "saved_at": z.SavedAt}
		}
	})
	return out
}

func (a *App) hZoteroStatus(w http.ResponseWriter, r *http.Request, me *Me) error {
	out := map[string]any{"local_allowed": isLoopback(r), "cloud": a.zoteroCloudView(me)}
	if isLoopback(r) {
		z := zConn{local: true}
		var cols []zItem
		if _, err := z.getJSON("/users/0/collections", url.Values{"limit": {"1"}}, &cols); err != nil {
			out["local"] = map[string]any{"ok": false, "message": err.Error()}
		} else {
			out["local"] = map[string]any{"ok": true, "message": "已连接本机 Zotero"}
		}
	}
	writeJSON(w, 200, out)
	return nil
}

func (a *App) hZoteroCloudSave(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Key string `json:"key"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Key = strings.TrimSpace(in.Key)
	if !reZAPIKey.MatchString(in.Key) {
		return errBad("密钥格式不对：请复制 zotero.org/settings/keys 页面上新建的私人密钥（一串字母和数字）")
	}
	z := zConn{key: in.Key}
	var k struct {
		UserID   int    `json:"userID"`
		Username string `json:"username"`
		Access   struct {
			User struct {
				Library bool `json:"library"`
				Files   bool `json:"files"`
				Write   bool `json:"write"`
			} `json:"user"`
			Groups map[string]struct {
				Library bool `json:"library"`
			} `json:"groups"`
		} `json:"access"`
	}
	if _, err := z.getJSON("/keys/current", nil, &k); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && (ae.code == 403 || ae.code == 404 || strings.Contains(ae.msg, "拒绝")) {
			return errBad("Zotero 不认这个密钥，请确认复制完整，且没有在 Zotero 网站上删除")
		}
		return err
	}
	if k.UserID == 0 {
		return errBad("Zotero 不认这个密钥，请确认复制完整")
	}
	groups := "none"
	if g, ok := k.Access.Groups["all"]; ok && g.Library {
		groups = "all"
	} else if len(k.Access.Groups) > 0 {
		groups = "some"
	}
	if !k.Access.User.Library && groups == "none" {
		return errBad("这个密钥没有读取文库的权限：请在 zotero.org/settings/keys 编辑密钥，勾选“Allow library access”")
	}
	zc := &ZoteroCloud{UserID: k.UserID, Username: k.Username, KeyEnc: a.enc(in.Key), Library: k.Access.User.Library, Files: k.Access.User.Files,
		Write: k.Access.User.Write, Groups: groups, SavedAt: now()}
	err := a.store.Update(func(db *DB) error {
		u := db.User(me.ID)
		if u == nil {
			return errNotFound("用户不存在")
		}
		u.Zotero = zc
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, a.zoteroCloudView(me))
	return nil
}

func (a *App) hZoteroCloudDelete(w http.ResponseWriter, r *http.Request, me *Me) error {
	err := a.store.Update(func(db *DB) error {
		if u := db.User(me.ID); u != nil {
			u.Zotero = nil
		}
		return nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	return nil
}

func (a *App) hZoteroLibraries(w http.ResponseWriter, r *http.Request, me *Me) error {
	z, err := a.zoteroConn(r, me, r.URL.Query().Get("src"))
	if err != nil {
		return err
	}
	pre, _ := z.prefix("user")
	libs := []map[string]string{{"id": "user", "name": "我的文库"}}
	var gs []struct {
		ID   int `json:"id"`
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if _, err := z.getJSON(pre+"/groups", url.Values{"limit": {"100"}}, &gs); err != nil && z.local {
		return err // 本机 Zotero 没开或没开启接口；云端读不到群组时仍可使用个人文库
	}
	for _, g := range gs {
		if g.ID > 0 {
			libs = append(libs, map[string]string{"id": "group:" + itoa(g.ID), "name": "群组：" + g.Data.Name})
		}
	}
	writeJSON(w, 200, libs)
	return nil
}

func (a *App) hZoteroCollections(w http.ResponseWriter, r *http.Request, me *Me) error {
	q := r.URL.Query()
	z, err := a.zoteroConn(r, me, q.Get("src"))
	if err != nil {
		return err
	}
	pre, err := z.prefix(q.Get("lib"))
	if err != nil {
		return err
	}
	type col struct {
		Key  string `json:"key"`
		Meta struct {
			NumItems int `json:"numItems"`
		} `json:"meta"`
		Data struct {
			Name   string `json:"name"`
			Parent any    `json:"parentCollection"`
		} `json:"data"`
	}
	var all []col
	for start := 0; start < 2000; start += 100 {
		var page []col
		total, err := z.getJSON(pre+"/collections", url.Values{"limit": {"100"}, "start": {itoa(start)}}, &page)
		if err != nil {
			return err
		}
		all = append(all, page...)
		if len(page) < 100 || (total > 0 && len(all) >= total) {
			break
		}
	}
	// 按层级排好顺序，前端缩进显示
	children := map[string][]col{}
	for _, c := range all {
		p, _ := c.Data.Parent.(string)
		children[p] = append(children[p], c)
	}
	out := []map[string]any{}
	var walk func(parent string, depth int)
	seen := map[string]bool{}
	walk = func(parent string, depth int) {
		cs := children[parent]
		sortByName(cs, func(c col) string { return c.Data.Name })
		for _, c := range cs {
			if seen[c.Key] || depth > 20 {
				continue
			}
			seen[c.Key] = true
			out = append(out, map[string]any{"key": c.Key, "name": c.Data.Name, "depth": depth, "items": c.Meta.NumItems})
			walk(c.Key, depth+1)
		}
	}
	walk("", 0)
	writeJSON(w, 200, out)
	return nil
}

func sortByName[T any](xs []T, name func(T) string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && strings.ToLower(name(xs[j])) < strings.ToLower(name(xs[j-1])); j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// importedIn 返回某个资料库（项目或个人）里已有的 Zotero 条目编号、DOI 和题名。
func importedIn(db *DB, me *Me, pid string) (map[string]bool, map[string]bool, map[string]bool) {
	keys, dois, titles := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, m := range db.Materials {
		if m.DeletedAt != nil || m.ProjectID != pid || (pid == "" && m.OwnerID != me.ID) {
			continue
		}
		if m.ZoteroKey != "" {
			keys[m.ZoteroKey] = true
		}
		if m.DOI != "" {
			dois[strings.ToLower(m.DOI)] = true
		}
		if t := normTitle(m.Title); len([]rune(t)) >= 6 {
			titles[t] = true
		}
	}
	return keys, dois, titles
}

func (a *App) hZoteroItems(w http.ResponseWriter, r *http.Request, me *Me) error {
	q := r.URL.Query()
	pid := q.Get("project_id")
	if err := a.checkPaperProject(me, pid, false); err != nil {
		return err
	}
	z, err := a.zoteroConn(r, me, q.Get("src"))
	if err != nil {
		return err
	}
	pre, err := z.prefix(q.Get("lib"))
	if err != nil {
		return err
	}
	path := pre + "/items/top"
	if c := q.Get("collection"); c != "" {
		if !reZKey.MatchString(c) {
			return errBad("分类参数无效")
		}
		path = pre + "/collections/" + c + "/items/top"
	}
	start, _ := strconv.Atoi(q.Get("start"))
	if start < 0 {
		start = 0
	}
	v := url.Values{"limit": {"50"}, "start": {itoa(start)}, "sort": {"dateModified"}, "direction": {"desc"}}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		v.Set("q", s)
		v.Set("qmode", "titleCreatorYear")
	}
	var items []zItem
	total, err := z.getJSON(path, v, &items)
	if err != nil {
		return err
	}
	var keys, dois, titles map[string]bool
	a.store.View(func(db *DB) { keys, dois, titles = importedIn(db, me, pid) })
	out := []map[string]any{}
	for _, it := range items {
		t := str(it.Data["itemType"])
		isPDF := t == "attachment" && str(it.Data["contentType"]) == "application/pdf"
		if t == "note" || t == "annotation" || (t == "attachment" && !isPDF) {
			continue
		}
		p := zToPaper(it)
		var tags []string
		for _, x := range list(it.Data["tags"]) {
			if s := str(obj(x)["tag"]); s != "" && len(tags) < 6 {
				tags = append(tags, s)
			}
		}
		out = append(out, map[string]any{"paper": p, "zid": it.zid(), "zkey": it.Key, "item_type": t, "is_pdf": isPDF,
			"num_children": it.Meta.NumChildren, "tags": tags,
			"imported": keys[it.zid()] || (p.DOI != "" && dois[strings.ToLower(p.DOI)]) || titles[normTitle(p.Title)]})
	}
	if total == 0 {
		total = start + len(items)
	}
	writeJSON(w, 200, map[string]any{"items": out, "total": total, "start": start, "source": z.name()})
	return nil
}

// zPickPDF 找到条目的 PDF 附件（条目本身就是 PDF 时直接用）。
func zPickPDF(z zConn, pre, key string) (string, error) {
	var it zItem
	if _, err := z.getJSON(pre+"/items/"+key, nil, &it); err != nil {
		return "", err
	}
	usable := func(d map[string]any) bool {
		if str(d["itemType"]) != "attachment" || str(d["contentType"]) != "application/pdf" {
			return false
		}
		lm := str(d["linkMode"])
		if lm == "linked_url" {
			return false
		}
		return z.local || lm == "imported_file" || lm == "imported_url"
	}
	if usable(it.Data) {
		return it.Key, nil
	}
	var kids []zItem
	if _, err := z.getJSON(pre+"/items/"+key+"/children", url.Values{"limit": {"100"}}, &kids); err != nil {
		return "", err
	}
	for _, c := range kids {
		if usable(c.Data) {
			return c.Key, nil
		}
	}
	if !z.local {
		for _, c := range kids {
			if str(c.Data["contentType"]) == "application/pdf" && str(c.Data["linkMode"]) == "linked_file" {
				return "", errNotFound("这条文献的 PDF 是“链接到文件”的附件，只保存在添加它的电脑上，Zotero 云端没有。可以在那台电脑上用“本机 Zotero”导入")
			}
		}
	}
	return "", errNotFound("这条文献在 Zotero 里没有 PDF 附件")
}

// fileURLToPath 把 file:///C:/Users/... 转成本机路径。
func fileURLToPath(s string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "file" {
		return "", errBad("本机 Zotero 返回的附件位置无法识别")
	}
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		p = "//" + u.Host + p // 网络共享路径
	} else if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p), nil
}

func (a *App) hZoteroPDF(w http.ResponseWriter, r *http.Request, me *Me) error {
	q := r.URL.Query()
	z, err := a.zoteroConn(r, me, q.Get("src"))
	if err != nil {
		return err
	}
	pre, err := z.prefix(q.Get("lib"))
	if err != nil {
		return err
	}
	key := q.Get("key")
	if !reZKey.MatchString(key) {
		return errBad("条目参数无效")
	}
	att, err := zPickPDF(z, pre, key)
	if err != nil {
		return err
	}
	var rd io.Reader
	if z.local {
		resp, err := z.request("GET", pre+"/items/"+att+"/file/view/url", nil, nil, nil)
		if err != nil {
			return err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		path, err := fileURLToPath(string(b))
		if err != nil {
			return err
		}
		if !strings.EqualFold(filepath.Ext(path), ".pdf") {
			return errBad("附件不是 PDF 文件")
		}
		f, err := os.Open(path)
		if err != nil {
			return errNotFound("找不到附件文件（可能已被移动或删除）：" + filepath.Base(path))
		}
		defer f.Close()
		if st, err := f.Stat(); err != nil || st.IsDir() || st.Size() > maxFileBytes {
			return errBad("附件文件超过 50 MB 或无法读取")
		}
		rd = f
	} else {
		resp, err := z.request("GET", pre+"/items/"+att+"/file", nil, nil, nil)
		if err != nil {
			return err
		}
		if loc := resp.Header.Get("Location"); resp.StatusCode/100 == 3 && loc != "" {
			resp.Body.Close()
			// 文件存储服务器：不带 Zotero 密钥
			ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", loc, nil)
			resp, err = pdfClient.Do(req)
			if err != nil {
				return errBad("从 Zotero 云端下载附件失败：" + shortErr(err))
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				return errBad("从 Zotero 云端下载附件失败（状态 " + itoa(resp.StatusCode) + "）")
			}
		}
		defer resp.Body.Close()
		rd = resp.Body
	}
	head := make([]byte, 5)
	if n, _ := io.ReadFull(rd, head); n < 5 || string(head) != "%PDF-" {
		return errBad("附件不是有效的 PDF 文件")
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(head)
	io.Copy(w, io.LimitReader(rd, maxFileBytes))
	return nil
}

func (a *App) hZoteroLog(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		ProjectID  string `json:"project_id"`
		Src        string `json:"src"`
		Library    string `json:"library"`
		Collection string `json:"collection"`
		Query      string `json:"query"`
		Count      int    `json:"count"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := a.checkPaperProject(me, in.ProjectID, true); err != nil {
		return err
	}
	src := map[string]string{"local": "本机 Zotero", "cloud": "Zotero 云端"}[in.Src]
	if src == "" {
		return errBad("来源无效")
	}
	where := clipRunes(in.Library, 60)
	if in.Collection != "" {
		where += " / " + clipRunes(in.Collection, 60)
	}
	var f []string
	f = append(f, src)
	if s := strings.TrimSpace(in.Query); s != "" {
		f = append(f, "筛选："+clipRunes(s, 60))
	}
	id := a.addSearchLog(me, in.ProjectID, "zotero", where, strings.Join(f, "，"), max(in.Count, 0))
	writeJSON(w, 200, map[string]any{"log_id": id})
	return nil
}

func clipRunes(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) > n {
		return string(rs[:n]) + "…"
	}
	return string(rs)
}

// hZoteroSave 把检索结果存进 Zotero：本机用 Zotero 浏览器插件接口（存到 Zotero 当前选中的分类），云端用 Web API。
func (a *App) hZoteroSave(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		Src        string  `json:"src"`
		Lib        string  `json:"lib"`
		Collection string  `json:"collection"`
		Papers     []Paper `json:"papers"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if len(in.Papers) == 0 {
		return errBad("请先选择文献")
	}
	if len(in.Papers) > 50 {
		return errBad("一次最多保存 50 条")
	}
	for _, p := range in.Papers {
		if strings.TrimSpace(p.Title) == "" {
			return errBad("有文献缺少题名")
		}
	}
	z, err := a.zoteroConn(r, me, in.Src)
	if err != nil {
		return err
	}
	if z.local {
		items := make([]map[string]any, 0, len(in.Papers))
		for _, p := range in.Papers {
			items = append(items, paperToZotero(p, "", true))
		}
		body, _ := json.Marshal(map[string]any{"items": items, "uri": "", "sessionID": "kyws-" + newID()})
		req, _ := http.NewRequest("POST", zoteroLocalBase+"/connector/saveItems", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Zotero-Connector-API-Version", "3")
		resp, err := zLocalClient.Do(req)
		if err != nil {
			return z.connErr(err)
		}
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		if resp.StatusCode != 201 && resp.StatusCode != 200 {
			if resp.StatusCode == 500 && strings.Contains(string(msg), "read-only") {
				return errBad("Zotero 当前选中的文库是只读的，请在 Zotero 中选中自己的文库或可编辑的分类后重试")
			}
			return errBad("本机 Zotero 没有接受保存（状态 " + itoa(resp.StatusCode) + "）。请确认 Zotero 已打开")
		}
		writeJSON(w, 200, map[string]any{"saved": len(items), "failed": []string{}, "where": "Zotero 中当前选中的分类"})
		return nil
	}
	var zc ZoteroCloud
	a.store.View(func(db *DB) {
		if u := db.User(me.ID); u != nil && u.Zotero != nil {
			zc = *u.Zotero
		}
	})
	if in.Lib == "" {
		in.Lib = "user"
	}
	if in.Lib == "user" && !zc.Write {
		return errBad("你的 Zotero 密钥没有写入权限：请在 zotero.org/settings/keys 编辑密钥，勾选“Allow write access”，再到“设置 → 我的 Zotero”重新保存密钥")
	}
	pre, err := z.prefix(in.Lib)
	if err != nil {
		return err
	}
	if in.Collection != "" && !reZKey.MatchString(in.Collection) {
		return errBad("分类参数无效")
	}
	items := make([]map[string]any, 0, len(in.Papers))
	for _, p := range in.Papers {
		items = append(items, paperToZotero(p, in.Collection, false))
	}
	body, _ := json.Marshal(items)
	resp, err := z.request("POST", pre+"/items", nil, body, map[string]string{"Content-Type": "application/json", "Zotero-Write-Token": randHex(16)})
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) && strings.Contains(ae.msg, "拒绝") {
			return errBad("Zotero 云端拒绝写入：你的密钥没有这个文库的写入权限")
		}
		return err
	}
	defer resp.Body.Close()
	var res struct {
		Success map[string]string `json:"success"`
		Failed  map[string]struct {
			Message string `json:"message"`
		} `json:"failed"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&res)
	failed := []string{}
	for i, f := range res.Failed {
		n, _ := strconv.Atoi(i)
		t := ""
		if n >= 0 && n < len(in.Papers) {
			t = in.Papers[n].Title
		}
		failed = append(failed, clipRunes(t, 40)+"："+f.Message)
	}
	writeJSON(w, 200, map[string]any{"saved": len(res.Success), "failed": failed, "where": "Zotero 云端（打开 Zotero 同步后可见）"})
	return nil
}
