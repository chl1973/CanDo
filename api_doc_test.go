package main

// 接口清单与前后端一致性检查：
//   - TestAPIDoc：按 app.go 里 apiMux 的登记（每行末尾的注释是说明）生成 docs/API.md 的接口表；
//     文档过期时测试不通过。改了接口后运行：UPDATE_API_DOC=1 go test -run TestAPIDoc .
//   - TestFrontendAPICalls：web/ 里用到的每个接口，后端都必须有（方法和路径都对得上）。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	apiDocPath  = "docs/API.md"
	apiDocBegin = "<!-- 接口表开始：以下由 go test 自动生成，不要手改 -->"
	apiDocEnd   = "<!-- 接口表结束 -->"
)

type apiRoute struct {
	Method, Path, Desc string
	Handler, File      string
	Login, LocalOnly   bool
	BgName             string
	Body, Query, Form  []string
	Upload, Download   bool
}

// 接口表的分组：按路径 /api/ 后的第一段归类，顺序即文档中的顺序。
var apiGroups = []struct {
	Title    string
	Prefixes []string
}{
	{"账号、设置与系统", []string{"health", "setup", "login", "logout", "me", "users", "settings", "lan-info", "quit", "local"}},
	{"管理后台", []string{"admin"}},
	{"首页", []string{"home"}},
	{"项目与流程模板", []string{"projects", "templates"}},
	{"经验库", []string{"library"}},
	{"资料库与 OCR", []string{"materials", "chunks", "ocr"}},
	{"资料问答与对比", []string{"ask", "compare", "answers"}},
	{"引用核验", []string{"citechecks"}},
	{"AI 读文献", []string{"read"}},
	{"文献检索", []string{"papers", "scholar"}},
	{"Zotero", []string{"zotero"}},
	{"深度调研", []string{"research"}},
	{"写论文", []string{"writing"}},
	{"论文契约", []string{"contracts"}},
	{"后台任务", []string{"jobs"}},
	{"模型与用量", []string{"models", "usage"}},
	{"联网搜索", []string{"websearch"}},
	{"LaTeX 与手绘转图", []string{"latex", "sketch"}},
	{"本机智能体", []string{"agent"}},
}

func apiGroupOf(path string) string {
	seg := strings.TrimPrefix(path, "/api/")
	if i := strings.IndexAny(seg, "/."); i >= 0 {
		seg = seg[:i]
	}
	for _, g := range apiGroups {
		for _, p := range g.Prefixes {
			if p == seg {
				return g.Title
			}
		}
	}
	return ""
}

// parseAPIRoutes 读 Go 源码：apiMux 里的每条登记，以及对应处理函数收什么、返回什么。
func parseAPIRoutes(t *testing.T) []*apiRoute {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	handlers := map[string]*ast.FuncDecl{}
	fileOf := map[string]string{}
	structs := map[string]*ast.StructType{}
	var mux *ast.FuncDecl
	var muxFile *ast.File
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					if ts, ok := sp.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							structs[ts.Name.Name] = st
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil || d.Body == nil || len(d.Recv.List) != 1 {
					continue
				}
				if star, ok := d.Recv.List[0].Type.(*ast.StarExpr); !ok || typeName(star.X) != "App" {
					continue
				}
				handlers[d.Name.Name] = d
				fileOf[d.Name.Name] = name
				if d.Name.Name == "apiMux" {
					mux, muxFile = d, f
				}
			}
		}
	}
	if mux == nil {
		t.Fatal("没有找到 apiMux（接口登记）")
	}
	comments := map[int]string{}
	for _, cg := range muxFile.Comments {
		for _, c := range cg.List {
			comments[fset.Position(c.Slash).Line] = strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		}
	}
	var routes []*apiRoute
	ast.Inspect(mux.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "HandleFunc" {
			return true
		}
		pat := strLit(call.Args[0])
		method, path, _ := strings.Cut(pat, " ")
		line := fset.Position(call.Pos()).Line
		r := &apiRoute{Method: method, Path: path, Desc: comments[line]}
		if !unwrapAPIHandler(call.Args[1], r) {
			t.Fatalf("app.go:%d %s：不认识处理函数的包装写法，请在 api_doc_test.go 的 unwrapAPIHandler 里补上", line, pat)
		}
		if fd := handlers[r.Handler]; fd != nil {
			r.File = fileOf[r.Handler]
			analyzeAPIHandler(fd, r, structs)
		} else if r.Handler != "static" {
			t.Fatalf("app.go:%d %s：找不到处理函数 %s", line, pat, r.Handler)
		}
		routes = append(routes, r)
		return false
	})
	return routes
}

// isMethodExpr 判断是不是 r.Method。
func isMethodExpr(e ast.Expr) bool {
	s, ok := e.(*ast.SelectorExpr)
	return ok && s.Sel.Name == "Method" && typeName(s.X) == "r"
}

func typeName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func strLit(e ast.Expr) string {
	if bl, ok := e.(*ast.BasicLit); ok && bl.Kind == token.STRING {
		s, _ := strconv.Unquote(bl.Value)
		return s
	}
	return ""
}

// unwrapAPIHandler 拆开 a.auth(a.localOnly(a.bg("名称", a.hXxx))) 这类包装。
func unwrapAPIHandler(e ast.Expr, r *apiRoute) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		r.Handler = x.Sel.Name
		return true
	case *ast.CallExpr:
		sel, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		switch sel.Sel.Name {
		case "auth":
			r.Login = true
			return unwrapAPIHandler(x.Args[0], r)
		case "open":
			return unwrapAPIHandler(x.Args[0], r)
		case "localOnly":
			r.LocalOnly = true
			return unwrapAPIHandler(x.Args[0], r)
		case "bg":
			r.BgName = strLit(x.Args[0])
			return unwrapAPIHandler(x.Args[1], r)
		}
	}
	return false
}

// analyzeAPIHandler 从处理函数里找：readJSON 读的字段、查询参数、表单字段、是否上传 / 返回文件。
func analyzeAPIHandler(fd *ast.FuncDecl, r *apiRoute, structs map[string]*ast.StructType) {
	varTypes := map[string]ast.Expr{}
	queryVars := map[string]bool{}
	seen := map[string]bool{}
	add := func(list *[]string, s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			*list = append(*list, s)
		}
	}
	isQueryCall := func(e ast.Expr) bool {
		c, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		return ok && s.Sel.Name == "Query"
	}
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SwitchStmt: // switch r.Method { case "POST": … }：只看本方法的分支
			if !isMethodExpr(x.Tag) {
				return true
			}
			var def *ast.CaseClause
			matched := false
			for _, st := range x.Body.List {
				cc := st.(*ast.CaseClause)
				if cc.List == nil {
					def = cc
				}
				for _, v := range cc.List {
					if strLit(v) == r.Method {
						matched = true
						for _, s := range cc.Body {
							ast.Inspect(s, visit)
						}
					}
				}
			}
			if !matched && def != nil {
				for _, s := range def.Body {
					ast.Inspect(s, visit)
				}
			}
			return false
		case *ast.IfStmt: // if r.Method == "PUT" { … } else { … }
			be, ok := x.Cond.(*ast.BinaryExpr)
			if !ok || (be.Op != token.EQL && be.Op != token.NEQ) || !isMethodExpr(be.X) {
				return true
			}
			if x.Init != nil {
				ast.Inspect(x.Init, visit)
			}
			if (strLit(be.Y) == r.Method) == (be.Op == token.EQL) {
				ast.Inspect(x.Body, visit)
			} else if x.Else != nil {
				ast.Inspect(x.Else, visit)
			}
			return false
		case *ast.ValueSpec:
			for _, id := range x.Names {
				if x.Type != nil {
					varTypes[id.Name] = x.Type
				}
			}
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 && isQueryCall(x.Rhs[0]) {
				for _, l := range x.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						queryVars[id.Name] = true
					}
				}
			}
		case *ast.CallExpr:
			switch fn := x.Fun.(type) {
			case *ast.Ident:
				if fn.Name == "readJSON" && len(x.Args) == 2 {
					if u, ok := x.Args[1].(*ast.UnaryExpr); ok {
						if id, ok := u.X.(*ast.Ident); ok {
							for _, f := range jsonFields(varTypes[id.Name], structs) {
								add(&r.Body, f)
							}
						}
					}
				}
			case *ast.SelectorExpr:
				switch fn.Sel.Name {
				case "Get":
					if id, ok := fn.X.(*ast.Ident); (ok && queryVars[id.Name]) || isQueryCall(fn.X) {
						if len(x.Args) == 1 {
							add(&r.Query, strLit(x.Args[0]))
						}
					}
				case "FormValue":
					add(&r.Form, strLit(x.Args[0]))
				case "FormFile", "ParseMultipartForm", "MultipartReader":
					r.Upload = true
				case "ServeContent", "ServeFile":
					r.Download = true
				case "Set":
					if len(x.Args) == 2 && strLit(x.Args[0]) == "Content-Disposition" {
						r.Download = true
					}
				case "Write":
					if typeName(fn.X) == "w" {
						r.Download = true
					}
				case "Copy":
					if len(x.Args) == 2 && typeName(x.Args[0]) == "w" {
						r.Download = true
					}
				}
			}
		}
		return true
	}
	ast.Inspect(fd.Body, visit)
	if r.Method == "GET" {
		// GET 请求里的 FormValue 读的就是查询参数
		r.Query = append(r.Query, r.Form...)
		r.Form = nil
	}
}

// jsonFields 列出请求结构的 JSON 字段：`名称` 类型。
func jsonFields(t ast.Expr, structs map[string]*ast.StructType) []string {
	switch x := t.(type) {
	case *ast.Ident:
		if st, ok := structs[x.Name]; ok {
			return jsonFields(st, structs)
		}
	case *ast.StructType:
		var out []string
		for _, f := range x.Fields.List {
			name := ""
			if f.Tag != nil {
				tag, _ := strconv.Unquote(f.Tag.Value)
				name, _, _ = strings.Cut(reflect.StructTag(tag).Get("json"), ",")
			}
			if name == "-" {
				continue
			}
			names := []string{name}
			if name == "" {
				names = nil
				for _, id := range f.Names {
					if id.IsExported() { // 没写 json 标签时按字段名匹配（不分大小写），文档里写小写
						names = append(names, strings.ToLower(id.Name))
					}
				}
				if len(f.Names) == 0 { // 嵌入的结构：展开它的字段
					out = append(out, jsonFields(f.Type, structs)...)
				}
			}
			for _, n := range names {
				out = append(out, "`"+n+"` "+simpleType(f.Type))
			}
		}
		return out
	case *ast.MapType:
		return []string{"键值对 " + simpleType(x)}
	}
	if t == nil {
		return nil
	}
	return []string{simpleType(t)}
}

func simpleType(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return simpleType(x.X)
	case *ast.ArrayType:
		return "[]" + simpleType(x.Elt)
	case *ast.MapType:
		return "map[" + simpleType(x.Key) + "]" + simpleType(x.Value)
	case *ast.StructType:
		return "对象"
	case *ast.InterfaceType:
		return "any"
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return simpleType(x.X) + "." + x.Sel.Name
	}
	return "?"
}

func renderAPITable(routes []*apiRoute) string {
	byGroup := map[string][]*apiRoute{}
	count := map[string]int{}
	total := 0
	for _, r := range routes {
		if !strings.HasPrefix(r.Path, "/api/") {
			continue
		}
		byGroup[apiGroupOf(r.Path)] = append(byGroup[apiGroupOf(r.Path)], r)
		count[r.Method]++
		total++
	}
	var b strings.Builder
	b.WriteString(apiDocBegin + "\n\n")
	var methods []string
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if count[m] > 0 {
			methods = append(methods, m+" "+strconv.Itoa(count[m]))
		}
	}
	b.WriteString("共 " + strconv.Itoa(total) + " 个接口（" + strings.Join(methods, "，") + "）。\n")
	titles := []string{}
	for _, g := range apiGroups {
		titles = append(titles, g.Title)
	}
	titles = append(titles, "") // 未分组的放最后
	for _, title := range titles {
		list := byGroup[title]
		if len(list) == 0 {
			continue
		}
		if title == "" {
			title = "其他（请在 api_doc_test.go 的 apiGroups 里归类）"
		}
		b.WriteString("\n## " + title + "\n\n")
		b.WriteString("| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |\n|---|---|---|---|---|---|---|\n")
		for _, r := range list {
			var need []string
			if r.Login {
				need = append(need, "登录")
			} else {
				need = append(need, "免登录")
			}
			if r.LocalOnly {
				need = append(need, "仅本机")
			}
			if r.BgName != "" {
				need = append(need, "可后台运行")
			}
			var req []string
			if len(r.Query) > 0 {
				req = append(req, "查询："+joinCode(r.Query))
			}
			if r.Upload {
				req = append(req, "上传文件（multipart）")
			}
			if len(r.Form) > 0 {
				req = append(req, "表单："+joinCode(r.Form))
			}
			if len(r.Body) > 0 {
				req = append(req, "JSON："+strings.Join(r.Body, "、"))
			}
			if len(req) == 0 {
				req = []string{"—"}
			}
			resp := "JSON"
			if r.Download {
				resp = "文件"
			}
			b.WriteString("| " + r.Method + " | `" + r.Path + "` | " + r.Desc + " | " + strings.Join(need, "、") + " | " +
				strings.Join(req, "<br>") + " | " + resp + " | `" + r.File + "` " + r.Handler + " |\n")
		}
	}
	b.WriteString("\n" + apiDocEnd + "\n")
	return b.String()
}

func joinCode(list []string) string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = "`" + s + "`"
	}
	return strings.Join(out, "、")
}

func TestAPIDoc(t *testing.T) {
	routes := parseAPIRoutes(t)
	if len(routes) < 100 {
		t.Fatalf("只找到 %d 个接口，解析规则可能失效", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		key := r.Method + " " + r.Path
		if r.Desc == "" {
			t.Errorf("接口 %s 没有说明：请在 app.go 登记这一行的末尾写一句 // 说明", key)
		}
		if seen[key] {
			t.Errorf("接口 %s 登记了两次", key)
		}
		seen[key] = true
	}
	table := renderAPITable(routes)
	b, err := os.ReadFile(apiDocPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	i, j := strings.Index(doc, apiDocBegin), strings.Index(doc, apiDocEnd)
	if i < 0 || j < i {
		t.Fatalf("%s 里缺少接口表的开始 / 结束标记", apiDocPath)
	}
	updated := doc[:i] + table + doc[j+len(apiDocEnd)+1:]
	if updated == doc {
		return
	}
	if os.Getenv("UPDATE_API_DOC") == "1" {
		if err := os.WriteFile(apiDocPath, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("已更新 %s", apiDocPath)
		return
	}
	t.Errorf("%s 的接口表不是最新的。请运行：UPDATE_API_DOC=1 go test -run TestAPIDoc .", apiDocPath)
}

type frontendAPICall struct {
	file         string
	line         int
	method, path string
	prefix       bool // 路径后面还拼接了内容（如 ${path}、${query}），只核对前面这一段
}

var reAPIMethodBefore = regexp.MustCompile(`\bapi(?:Bg)?\(\s*"(GET|POST|PUT|PATCH|DELETE)"\s*,\s*$`)

// findFrontendAPICalls 找出前端代码里所有以 /api/ 开头的字符串（含模板字符串），
// 整段的 ${...} 换成占位符 x；${...} 紧跟在非 / 字符后（拼接查询参数或后半段路径）时从那里截断，
// 只核对前面一段。去掉 ? 后面的查询参数；紧跟在 api("方法", 之后的记下方法。
func findFrontendAPICalls(file, src string) []frontendAPICall {
	var out []frontendAPICall
	for off := 0; ; {
		k := strings.Index(src[off:], "/api/")
		if k < 0 {
			break
		}
		i := off + k
		off = i + 5
		if i == 0 {
			continue
		}
		q := src[i-1]
		if q != '"' && q != '\'' && q != '`' {
			continue
		}
		var path strings.Builder
		prefix := false
		j := i
		for j < len(src) && src[j] != q && src[j] != '\n' {
			if strings.HasPrefix(src[j:], "${") { // 也可能是模板字符串里 href="..." 中的插值
				depth := 0
				for j < len(src) {
					if src[j] == '{' {
						depth++
					} else if src[j] == '}' {
						depth--
						if depth == 0 {
							j++
							break
						}
					}
					j++
				}
				if strings.ContainsAny(path.String(), "?#") { // 查询参数里的插值不影响路径
					path.WriteString("x")
				} else if prefix || !strings.HasSuffix(path.String(), "/") {
					prefix = true
				} else {
					path.WriteString("x")
				}
				continue
			}
			if !prefix {
				path.WriteByte(src[j])
			}
			j++
		}
		p := path.String()
		if k := strings.IndexAny(p, "?#"); k >= 0 {
			p = p[:k]
		}
		if strings.HasSuffix(p, "/") && !prefix { // "/api/xxx/" + id 这类拼接
			p += "x"
		}
		method := ""
		from := i - 1 - 60
		if from < 0 {
			from = 0
		}
		if m := reAPIMethodBefore.FindStringSubmatch(src[from : i-1]); m != nil {
			method = m[1]
		}
		out = append(out, frontendAPICall{file, strings.Count(src[:i], "\n") + 1, method, p, prefix})
		off = j
	}
	return out
}

var reAPIPathParam = regexp.MustCompile(`\{[^}]+\}`)

func TestFrontendAPICalls(t *testing.T) {
	mux := (&App{}).apiMux()
	// 截断的路径按前缀核对：把登记的 {id} 等也换成 x 再比较
	var patterns []struct{ method, path string }
	for _, r := range parseAPIRoutes(t) {
		patterns = append(patterns, struct{ method, path string }{r.Method, reAPIPathParam.ReplaceAllString(r.Path, "x")})
	}
	var files []string
	for _, pat := range []string{"web/*.js", "web/*.html"} {
		m, _ := filepath.Glob(pat)
		files = append(files, m...)
	}
	sort.Strings(files)
	total := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range findFrontendAPICalls(f, string(b)) {
			total++
			methods := []string{c.method}
			if c.method == "" { // 方法不在字面上（如 api(method, url)、链接）：任一方法对得上即可
				methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
			}
			ok := false
			for _, m := range methods {
				if c.prefix {
					for _, p := range patterns {
						if p.method == m && strings.HasPrefix(p.path, c.path) && strings.HasPrefix(p.path, "/api/") &&
							(len(p.path) == len(c.path) || strings.ContainsRune("/.", rune(p.path[len(c.path)]))) {
							ok = true
						}
					}
					continue
				}
				req, err := http.NewRequest(m, c.path, nil)
				if err != nil {
					break
				}
				if _, pat := mux.Handler(req); pat != "" && pat != "GET /" {
					ok = true
					break
				}
			}
			if ok {
				continue
			}
			if c.prefix {
				t.Errorf("%s:%d 用到了 %s %s…，但后端没有以它开头的接口", c.file, c.line, c.method, c.path)
			} else {
				t.Errorf("%s:%d 用到了 %s %s，但后端没有这个接口", c.file, c.line, c.method, c.path)
			}
		}
	}
	if total < 150 {
		t.Fatalf("前端只找到 %d 处接口调用，提取规则可能失效", total)
	}
}

func TestFindFrontendAPICalls(t *testing.T) {
	src := "api(\"POST\", `/api/projects/${encodeURIComponent(p.id)}/stages/${sid}/check`, {x: 1});\n" +
		"api(\"DELETE\", \"/api/models/\" + id);\n" +
		"const u = `/api/materials?project_id=${enc(`a${b}`)}`;\n" +
		"x = \"no /api/ here\";\n" +
		"api(\"POST\", `/api/projects/${p.id}/stages/${sid}${path}`, body);\n" +
		"`<a href=\"/api/materials/${esc(c.id)}/file${page}\">`"
	got := findFrontendAPICalls("t.js", src)
	want := []frontendAPICall{
		{"t.js", 1, "POST", "/api/projects/x/stages/x/check", false},
		{"t.js", 2, "DELETE", "/api/models/x", false},
		{"t.js", 3, "", "/api/materials", false},
		{"t.js", 5, "POST", "/api/projects/x/stages/x", true},
		{"t.js", 6, "", "/api/materials/x/file", true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("提取结果不对：\n得到 %+v\n应为 %+v", got, want)
	}
}
