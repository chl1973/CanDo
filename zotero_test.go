package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const zGoodKey = "GOODKEY1234567890ABCDEFG"
const zWriteKey = "WRITEKEY234567890ABCDEFG"

type zMock struct {
	mu        sync.Mutex
	connector []map[string]any
	webPosts  [][]map[string]any
	s3Headers http.Header
}

func mockZotero(t *testing.T) (*httptest.Server, *zMock) {
	st := &zMock{}
	dir := t.TempDir()
	pdf := filepath.Join(dir, "我的 论文.pdf")
	os.WriteFile(pdf, []byte("%PDF-1.4 zotero local"), 0o600)
	lib := func(typ string, id int) map[string]any {
		return map[string]any{"type": typ, "id": id, "name": "me"}
	}
	item := func(key, typ string, l map[string]any, children int, data map[string]any) map[string]any {
		data["key"], data["itemType"] = key, typ
		return map[string]any{"key": key, "library": l, "meta": map[string]any{"numChildren": children}, "data": data}
	}
	items := func(l map[string]any) []map[string]any {
		return []map[string]any{
			item("ITEM0001", "journalArticle", l, 1, map[string]any{"title": "Deep learning for rivers", "date": "2020-05-01",
				"creators":         []any{map[string]any{"creatorType": "author", "lastName": "Smith", "firstName": "John A."}, map[string]any{"creatorType": "author", "lastName": "张", "firstName": "三"}},
				"publicationTitle": "Water Research", "volume": "12", "issue": "3", "pages": "1–10", "DOI": "https://doi.org/10.1/ABC", "abstractNote": "We study rivers.",
				"tags": []any{map[string]any{"tag": "河流"}}}),
			item("ITEM0002", "thesis", l, 0, map[string]any{"title": "河流治理研究", "date": "2019", "creators": []any{map[string]any{"creatorType": "author", "name": "王五"}}, "university": "河海大学"}),
			item("NOTE0001", "note", l, 0, map[string]any{"note": "<p>x</p>"}),
			item("ATT00009", "attachment", l, 0, map[string]any{"title": "scan.pdf", "contentType": "application/pdf", "linkMode": "linked_file"}),
			item("ITEM0004", "journalArticle", l, 1, map[string]any{"title": "Linked only", "extra": "DOI: 10.9/xyz"}),
		}
	}
	children := map[string][]map[string]any{
		"ITEM0001": {item("ATT00001", "attachment", lib("user", 123), 0, map[string]any{"contentType": "application/pdf", "linkMode": "imported_file", "filename": "a.pdf"})},
		"ITEM0004": {item("ATT00004", "attachment", lib("user", 123), 0, map[string]any{"contentType": "application/pdf", "linkMode": "linked_file"})},
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		send := func(v any, total int) {
			if total >= 0 {
				w.Header().Set("Total-Results", itoa(total))
			}
			json.NewEncoder(w).Encode(v)
		}
		if p == "/connector/saveItems" {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			st.mu.Lock()
			st.connector = append(st.connector, b)
			st.mu.Unlock()
			w.WriteHeader(201)
			return
		}
		if p == "/s3/a.pdf" {
			st.mu.Lock()
			st.s3Headers = r.Header.Clone()
			st.mu.Unlock()
			w.Write([]byte("%PDF-1.4 zotero cloud"))
			return
		}
		local := strings.HasPrefix(p, "/api/")
		if local {
			p = strings.TrimPrefix(p, "/api")
			if !strings.HasPrefix(p, "/users/0") && !strings.HasPrefix(p, "/groups/") {
				http.Error(w, "No endpoint found", 404)
				return
			}
		} else {
			k := r.Header.Get("Zotero-API-Key")
			if p == "/keys/current" {
				switch k {
				case zGoodKey, zWriteKey:
					send(map[string]any{"key": k, "userID": 123, "username": "zhang",
						"access": map[string]any{"user": map[string]any{"library": true, "files": true, "write": k == zWriteKey}, "groups": map[string]any{"all": map[string]any{"library": true}}}}, -1)
				case "READONLYKEY1234567890ABC":
					send(map[string]any{"key": k, "userID": 124, "access": map[string]any{"user": map[string]any{"library": false}}}, -1)
				default:
					http.Error(w, "Forbidden", 403)
				}
				return
			}
			if k != zGoodKey && k != zWriteKey {
				http.Error(w, "Forbidden", 403)
				return
			}
			if !strings.HasPrefix(p, "/users/123") && !strings.HasPrefix(p, "/groups/") {
				http.Error(w, "Forbidden", 403)
				return
			}
		}
		up := "/users/0"
		if !local {
			up = "/users/123"
		}
		l := lib("user", 123)
		if strings.HasPrefix(p, "/groups/456") {
			up, l = "/groups/456", lib("group", 456)
		}
		rest := strings.TrimPrefix(p, up)
		switch {
		case rest == "/groups":
			send([]any{map[string]any{"id": 456, "data": map[string]any{"name": "河流课题组"}}}, 1)
		case rest == "/collections":
			send([]any{
				map[string]any{"key": "COLL0002", "meta": map[string]any{"numItems": 1}, "data": map[string]any{"name": "子类", "parentCollection": "COLL0001"}},
				map[string]any{"key": "COLL0001", "meta": map[string]any{"numItems": 3}, "data": map[string]any{"name": "课题A", "parentCollection": false}},
			}, 2)
		case rest == "/items/top":
			its := items(l)
			if q := r.URL.Query().Get("q"); q != "" {
				var f []map[string]any
				for _, it := range its {
					if strings.Contains(it["data"].(map[string]any)["title"].(string), q) {
						f = append(f, it)
					}
				}
				its = f
			}
			send(its, len(its))
		case rest == "/collections/COLL0001/items/top":
			send(items(l)[:1], 1)
		case rest == "/items" && r.Method == "POST":
			var b []map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			st.mu.Lock()
			st.webPosts = append(st.webPosts, b)
			st.mu.Unlock()
			res := map[string]any{"success": map[string]any{"0": "NEWKEY01"}, "failed": map[string]any{}}
			if len(b) > 1 {
				res["failed"] = map[string]any{"1": map[string]any{"code": 400, "message": "字段无效"}}
			}
			send(res, -1)
		case strings.HasPrefix(rest, "/items/"):
			parts := strings.Split(strings.TrimPrefix(rest, "/items/"), "/")
			key := parts[0]
			if len(parts) == 1 {
				for _, it := range items(l) {
					if it["key"] == key {
						send(it, -1)
						return
					}
				}
				for _, cs := range children {
					for _, c := range cs {
						if c["key"] == key {
							send(c, -1)
							return
						}
					}
				}
				http.Error(w, "Not found", 404)
			} else if parts[1] == "children" {
				send(children[key], len(children[key]))
			} else if local && strings.Join(parts[1:], "/") == "file/view/url" {
				switch key {
				case "ATT00001":
					w.Write([]byte("file://" + filepath.ToSlash(pdf)))
				case "ATT00009":
					w.Write([]byte("file:///etc/passwd"))
				default:
					http.Error(w, "Not found", 404)
				}
			} else if !local && parts[1] == "file" && key == "ATT00001" {
				http.Redirect(w, r, srv.URL+"/s3/a.pdf", 302)
			} else {
				http.Error(w, "Not found", 404)
			}
		default:
			http.Error(w, "Not found", 404)
		}
	}))
	t.Cleanup(srv.Close)
	zoteroLocalBase, zoteroWebBase = srv.URL, srv.URL
	allowPrivateFetch = true
	t.Cleanup(func() {
		allowPrivateFetch = false
		zoteroLocalBase, zoteroWebBase = "http://127.0.0.1:23119", "https://api.zotero.org"
	})
	return srv, st
}

func TestZotero(t *testing.T) {
	app, srv := newEnv(t)
	tc, s1, s2, s1ID, _ := setupTeam(t, srv)
	_, st := mockZotero(t)
	p := tc.ok("POST", "/api/projects", map[string]any{"name": "河流", "template_key": "research", "members": []int{s1ID}})
	pid := p["id"].(string)

	if s := s1.ok("GET", "/api/zotero/status", nil); s["local_allowed"] != true || s["local"].(map[string]any)["ok"] != true || s["cloud"].(map[string]any)["configured"] != false {
		t.Fatalf("状态不对 %v", s)
	}
	libs := s1.ok("GET", "/api/zotero/libraries?src=local", nil)["list"].([]any)
	if len(libs) != 2 || libs[1].(map[string]any)["id"] != "group:456" {
		t.Fatalf("文库列表不对 %v", libs)
	}
	cols := s1.ok("GET", "/api/zotero/collections?src=local&lib=user", nil)["list"].([]any)
	if len(cols) != 2 || cols[0].(map[string]any)["name"] != "课题A" || cols[1].(map[string]any)["depth"].(float64) != 1 {
		t.Fatalf("分类层级不对 %v", cols)
	}
	r := s1.ok("GET", "/api/zotero/items?src=local&lib=user&project_id="+pid, nil)
	its := r["items"].([]any)
	if len(its) != 4 {
		t.Fatalf("应过滤笔记，保留独立 PDF：%d", len(its))
	}
	i1 := its[0].(map[string]any)
	pp := i1["paper"].(map[string]any)
	if pp["gbt"] != "SMITH J A, 张三. Deep learning for rivers[J]. Water Research, 2020, 12(3): 1-10. DOI: 10.1/ABC." || i1["zid"] != "user:123:ITEM0001" || i1["imported"] != false {
		t.Fatalf("条目转换不对 %v", i1)
	}
	if g := its[1].(map[string]any)["paper"].(map[string]any)["gbt"]; g != "王五. 河流治理研究[D]. 河海大学, 2019." {
		t.Fatalf("学位论文格式不对 %v", g)
	}
	if d := its[3].(map[string]any)["paper"].(map[string]any)["doi"]; d != "10.9/xyz" {
		t.Fatalf("应从“其他”字段读出 DOI：%v", d)
	}
	if n := len(s1.ok("GET", "/api/zotero/items?src=local&lib=user&collection=COLL0001", nil)["items"].([]any)); n != 1 {
		t.Fatal("按分类浏览不对")
	}
	if code, _ := s1.do("GET", "/api/zotero/items?src=local&lib=user&collection=../x", nil); code != 400 {
		t.Fatal("非法分类参数应拒绝")
	}
	if code, _ := s2.do("GET", "/api/zotero/items?src=local&lib=user&project_id="+pid, nil); code != 404 {
		t.Fatal("非成员不能导入到项目")
	}
	// 本机 PDF
	resp, _ := s1.hc.Get(srv.URL + "/api/zotero/pdf?src=local&lib=user&key=ITEM0001")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "%PDF-1.4 zotero local" {
		t.Fatalf("应读出本机 PDF：%d %s", resp.StatusCode, b)
	}
	if code, m := s1.do("GET", "/api/zotero/pdf?src=local&lib=user&key=ATT00009", nil); code != 400 || !strings.Contains(m["detail"].(string), "不是 PDF") {
		t.Fatalf("非 PDF 路径必须拒绝 %d %v", code, m)
	}
	if code, _ := s1.do("GET", "/api/zotero/pdf?src=local&lib=user&key=ITEM0002", nil); code != 404 {
		t.Fatal("没有附件应返回 404")
	}
	// 外部设备不能用本机 Zotero
	tc.ok("PUT", "/api/settings", map[string]any{"lan_enabled": true})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/zotero/items?src=local&lib=user", nil)
	req.RemoteAddr = "192.168.1.20:5555"
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s1.hc.Jar.Cookies(mustURL(srv.URL))[0].Value})
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "云端") {
		t.Fatalf("外部设备不能读本机 Zotero：%d %s", rec.Code, rec.Body.String())
	}
	// 导入：留痕 + 去重
	logID := s1.ok("POST", "/api/zotero/log", map[string]any{"project_id": pid, "src": "local", "library": "我的文库", "collection": "课题A", "count": 2})["log_id"].(string)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "Deep learning for rivers.pdf")
	fw.Write([]byte("%PDF-1.4 zotero local"))
	for k, v := range map[string]string{"project_id": pid, "title": "Deep learning for rivers", "zotero_key": "user:123:ITEM0001", "search_log_id": logID,
		"pages": `[{"page_index":1,"text":"We study rivers with deep learning in detail."}]`} {
		mw.WriteField(k, v)
	}
	mw.Close()
	hreq, _ := http.NewRequest("POST", srv.URL+"/api/materials", &buf)
	hreq.Header.Set("Content-Type", mw.FormDataContentType())
	hreq.Header.Set("X-KY", "1")
	resp, _ = s1.hc.Do(hreq)
	resp.Body.Close()
	s1.ok("POST", "/api/papers/save-record", map[string]any{"paper": its[1].(map[string]any)["paper"], "project_id": pid, "search_log_id": logID, "zotero_key": "user:123:ITEM0002"})
	its = s1.ok("GET", "/api/zotero/items?src=local&lib=user&project_id="+pid, nil)["items"].([]any)
	if its[0].(map[string]any)["imported"] != true || its[1].(map[string]any)["imported"] != true || its[3].(map[string]any)["imported"] != false {
		t.Fatal("已导入的条目应标出")
	}
	if its := s1.ok("GET", "/api/zotero/items?src=local&lib=user", nil)["items"].([]any); its[0].(map[string]any)["imported"] != false {
		t.Fatal("个人资料库与项目资料库分开判断")
	}
	logs := tc.ok("GET", "/api/papers/logs?project_id="+pid, nil)["list"].([]any)
	l0 := logs[0].(map[string]any)
	if l0["kind"] != "zotero" || l0["query"] != "我的文库 / 课题A" || len(l0["added"].([]any)) != 2 {
		t.Fatalf("Zotero 导入应留痕 %v", l0)
	}
	// 云端
	if code, m := s1.do("GET", "/api/zotero/items?src=cloud&lib=user", nil); code != 400 || !strings.Contains(m["detail"].(string), "设置") {
		t.Fatal("未连接云端应提示")
	}
	if code, _ := s1.do("PUT", "/api/zotero/cloud", map[string]any{"key": "WRONGKEY1234567890ABCDEF"}); code != 400 {
		t.Fatal("错误密钥应拒绝")
	}
	if code, m := s1.do("PUT", "/api/zotero/cloud", map[string]any{"key": "READONLYKEY1234567890ABC"}); code != 400 || !strings.Contains(m["detail"].(string), "权限") {
		t.Fatal("没有读取权限的密钥应拒绝")
	}
	c := s1.ok("PUT", "/api/zotero/cloud", map[string]any{"key": zGoodKey})
	if c["user_id"].(float64) != 123 || c["groups"] != "all" || c["write"] != false || strings.Contains(c["key_masked"].(string), "1234567890") {
		t.Fatalf("云端连接信息不对 %v", c)
	}
	raw, _ := os.ReadFile(filepath.Join(app.store.dir, "data.json"))
	if bytes.Contains(raw, []byte(zGoodKey)) {
		t.Fatal("Zotero 密钥不能明文保存")
	}
	its = s1.ok("GET", "/api/zotero/items?src=cloud&lib=user&project_id="+pid, nil)["items"].([]any)
	if its[0].(map[string]any)["zid"] != "user:123:ITEM0001" || its[0].(map[string]any)["imported"] != true {
		t.Fatal("云端与本机是同一文库时应识别为已导入")
	}
	if n := len(s1.ok("GET", "/api/zotero/items?src=cloud&lib=group:456", nil)["items"].([]any)); n != 4 {
		t.Fatal("应能读群组文库")
	}
	resp, _ = s1.hc.Get(srv.URL + "/api/zotero/pdf?src=cloud&lib=user&key=ITEM0001")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "%PDF-1.4 zotero cloud" {
		t.Fatalf("应下载云端附件 %s", b)
	}
	if st.s3Headers.Get("Zotero-API-Key") != "" {
		t.Fatal("不能把 Zotero 密钥发给文件存储服务器")
	}
	if code, m := s1.do("GET", "/api/zotero/pdf?src=cloud&lib=user&key=ITEM0004", nil); code != 404 || !strings.Contains(m["detail"].(string), "链接到文件") {
		t.Fatalf("链接文件应说明原因 %d %v", code, m)
	}
	if code, _ := s2.do("GET", "/api/zotero/items?src=cloud&lib=user", nil); code != 400 {
		t.Fatal("每人的 Zotero 云端连接互不共用")
	}
	// 保存到 Zotero
	papers := []any{pp, its[1].(map[string]any)["paper"]}
	if code, m := s1.do("POST", "/api/zotero/save", map[string]any{"src": "cloud", "lib": "user", "papers": papers}); code != 400 || !strings.Contains(m["detail"].(string), "写入权限") {
		t.Fatal("没有写入权限应提示")
	}
	sv := s1.ok("POST", "/api/zotero/save", map[string]any{"src": "local", "papers": papers})
	if sv["saved"].(float64) != 2 || len(st.connector) != 1 {
		t.Fatal("本机保存不对")
	}
	ci := st.connector[0]["items"].([]any)
	c0, c1 := ci[0].(map[string]any), ci[1].(map[string]any)
	if c0["itemType"] != "journalArticle" || c0["DOI"] != "10.1/ABC" || c0["pages"] != "1-10" || c1["itemType"] != "thesis" || c1["university"] != "河海大学" {
		t.Fatalf("转换为 Zotero 条目不对 %v %v", c0, c1)
	}
	cr := c0["creators"].([]any)
	if cr[0].(map[string]any)["lastName"] != "Smith" || cr[1].(map[string]any)["fieldMode"].(float64) != 1 {
		t.Fatalf("作者转换不对 %v", cr)
	}
	s1.ok("PUT", "/api/zotero/cloud", map[string]any{"key": zWriteKey})
	sv = s1.ok("POST", "/api/zotero/save", map[string]any{"src": "cloud", "lib": "user", "collection": "COLL0001", "papers": papers})
	if sv["saved"].(float64) != 1 || len(sv["failed"].([]any)) != 1 || !strings.Contains(sv["failed"].([]any)[0].(string), "河流治理研究") {
		t.Fatalf("云端保存结果不对 %v", sv)
	}
	if w := st.webPosts[0][0]; w["collections"].([]any)[0] != "COLL0001" || w["creators"].([]any)[1].(map[string]any)["name"] != "张三" {
		t.Fatalf("云端条目不对 %v", w)
	}
	s1.ok("DELETE", "/api/zotero/cloud", nil)
	if s := s1.ok("GET", "/api/zotero/status", nil); s["cloud"].(map[string]any)["configured"] != false {
		t.Fatal("应能断开云端")
	}
}

func TestFileURLToPath(t *testing.T) {
	for in, want := range map[string]string{
		"file:///C:/Users/%E5%BC%A0/Zotero/storage/AB12CD34/a%20b.pdf": "C:/Users/张/Zotero/storage/AB12CD34/a b.pdf",
		"file:///home/u/Zotero/x.pdf":                                  "/home/u/Zotero/x.pdf",
		"file://server/share/x.pdf":                                    "//server/share/x.pdf",
	} {
		got, err := fileURLToPath(in)
		if err != nil || filepath.ToSlash(got) != want {
			t.Fatalf("%s → %q %v", in, got, err)
		}
	}
	if _, err := fileURLToPath("http://x/a.pdf"); err == nil {
		t.Fatal("只接受 file://")
	}
}
