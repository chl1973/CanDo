package main

// Office 文件（参考了同学的 Python 文件助手，用 Go 标准库重新实现）：
//   - 读取 Word（带段落编号和样式）、Excel（各工作表）、PowerPoint（各页文字）
//   - 按段落编号修改 / 插入 / 删除 Word 段落，保留原段落格式和其余内容（图片、表格、页眉页脚不动）
//   - 用 Markdown 生成排版规范的 Word（标题、列表、三线表、加粗斜体、指定字体字号颜色、对齐）
//   - 分析模板 Word 的版式（纸张、页边距、正文和各级标题的字体字号）
//   - 列出 / 提取 Word 里的图片

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	wNS          = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	mathNS       = "http://schemas.openxmlformats.org/officeDocument/2006/math"
	aNS          = "http://schemas.openxmlformats.org/drawingml/2006/main"
	maxOfficeZip = 60 << 20
)

type zipPart struct {
	f    *zip.File
	name string
}

func openZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errBad("文件已损坏，或不是 Office 2007 以后的格式（.doc/.xls/.ppt 旧格式请先另存为 .docx/.xlsx/.pptx）")
	}
	return zr, nil
}

func zipRead(zr *zip.Reader, name string) []byte {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close()
			b, _ := io.ReadAll(io.LimitReader(rc, maxOfficeZip))
			return b
		}
	}
	return nil
}

// ---------------- Word：解析段落 ----------------

type docxPara struct {
	Start, End int
	Text       string
	StyleID    string
	Obj        string // 图片 / 公式 / 对象（不能用纯文字替换）
}

type docxTable struct {
	After int // 表格前面有几个段落（表格位于第 After-1 段之后）
	Rows  int
	Cols  int
	First string
}

type docxDoc struct {
	data   []byte
	zr     *zip.Reader
	xml    string
	paras  []docxPara
	tables []docxTable
	styles map[string]string // styleId → 样式名
	images []string
}

func parseDocxDoc(data []byte) (*docxDoc, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	x := zipRead(zr, "word/document.xml")
	if x == nil {
		return nil, errBad("不是 Word 文档（缺少 word/document.xml）")
	}
	d := &docxDoc{data: data, zr: zr, xml: string(x), styles: map[string]string{}}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "word/media/") && !strings.HasSuffix(f.Name, "/") {
			d.images = append(d.images, f.Name)
		}
	}
	d.parseStyles(zipRead(zr, "word/styles.xml"))
	if err := d.parseBody(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *docxDoc) parseStyles(b []byte) {
	if b == nil {
		return
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	id := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Space == wNS {
			switch se.Name.Local {
			case "style":
				id = xattr(se, "styleId")
			case "name":
				if id != "" {
					d.styles[id] = xattr(se, "val")
				}
			}
		}
	}
}

func xattr(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// parseBody 找出正文里（不在表格中的）每个段落在 document.xml 中的位置
func (d *docxDoc) parseBody() error {
	dec := xml.NewDecoder(strings.NewReader(d.xml))
	depth, bodyDepth, pDepth, tblDepth := 0, -1, -1, -1
	var cur *docxPara
	var tb *docxTable
	inT, inRow, rowN := false, 0, 0
	var tText strings.Builder
	for {
		off := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errBad("Word 文件内容格式有误，无法解析：" + err.Error())
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			w := t.Name.Space == wNS
			if w && t.Name.Local == "body" {
				bodyDepth = depth
			}
			if bodyDepth > 0 && depth == bodyDepth+1 && w {
				switch t.Name.Local {
				case "p":
					cur, pDepth = &docxPara{Start: off}, depth
				case "tbl":
					tb, tblDepth, rowN = &docxTable{After: len(d.paras)}, depth, 0
				}
			}
			if tb != nil && w {
				switch {
				case t.Name.Local == "tr" && depth == tblDepth+1:
					tb.Rows++
					rowN++
					inRow = rowN
				case t.Name.Local == "tc" && inRow == 1:
					tb.Cols++
					if tText.Len() > 0 {
						tText.WriteString(" | ")
					}
				case t.Name.Local == "t" && inRow == 1:
					inT = true
				}
			}
			if cur != nil {
				switch {
				case w && t.Name.Local == "t":
					inT = true
				case w && t.Name.Local == "tab":
					cur.Text += "\t"
				case w && (t.Name.Local == "br" || t.Name.Local == "cr"):
					cur.Text += "\n"
				case w && t.Name.Local == "pStyle":
					cur.StyleID = xattr(t, "val")
				case w && (t.Name.Local == "drawing" || t.Name.Local == "pict"):
					cur.Obj = "图片"
				case w && t.Name.Local == "object":
					cur.Obj = "嵌入对象"
				case t.Name.Space == mathNS && (t.Name.Local == "oMath" || t.Name.Local == "oMathPara"):
					if cur.Obj == "" {
						cur.Obj = "公式"
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inT = false
			}
			if cur != nil && depth == pDepth {
				cur.End = int(dec.InputOffset())
				d.paras = append(d.paras, *cur)
				cur = nil
			}
			if tb != nil && t.Name.Local == "tr" && depth == tblDepth+1 {
				inRow = 0
			}
			if tb != nil && depth == tblDepth {
				tb.First = clipRunes(tText.String(), 120)
				d.tables = append(d.tables, *tb)
				tb, tText = nil, strings.Builder{}
			}
			depth--
		case xml.CharData:
			if inT {
				if cur != nil {
					cur.Text += string(t)
				} else if tb != nil {
					tText.Write(t)
				}
			}
		}
	}
	return nil
}

func (d *docxDoc) styleName(id string) string {
	if id == "" {
		return "正文"
	}
	n := d.styles[id]
	if n == "" {
		n = id
	}
	l := strings.ToLower(n)
	switch {
	case strings.HasPrefix(l, "heading "):
		return "标题 " + strings.TrimPrefix(l, "heading ")
	case l == "caption":
		return "题注"
	case l == "normal":
		return "正文"
	case l == "title":
		return "文档标题"
	case l == "subtitle":
		return "副标题"
	case strings.HasPrefix(l, "list"):
		return "列表"
	case l == "no indent":
		return "正文（不缩进）"
	case l == "table text":
		return "表格文字"
	case l == "bibliography":
		return "参考文献"
	case l == "writing hint":
		return "提示"
	case l == "toc heading":
		return "目录标题"
	case strings.HasPrefix(l, "toc "):
		return "目录 " + strings.TrimPrefix(l, "toc ")
	}
	return n
}

// outline 给模型看：带编号的段落列表（编号从 0 开始，修改时用这个编号）
func (d *docxDoc) outline(start, max, limit int) string {
	var b strings.Builder
	b.WriteString("Word 文档共 " + itoa(len(d.paras)) + " 个段落、" + itoa(len(d.tables)) + " 个表格、" + itoa(len(d.images)) + " 张图片。段落编号从 0 开始，修改时用 docx_edit 按编号操作：\n")
	ti := 0
	shown := 0
	for i, p := range d.paras {
		for ti < len(d.tables) && d.tables[ti].After <= i {
			if i >= start {
				tt := d.tables[ti]
				b.WriteString("    ┆（表格 " + itoa(ti+1) + "：" + itoa(tt.Rows) + " 行 × " + itoa(tt.Cols) + " 列；第一行：" + tt.First + "）\n")
			}
			ti++
		}
		if i < start {
			continue
		}
		if shown >= max || b.Len() > limit {
			b.WriteString("……（还有 " + itoa(len(d.paras)-i) + " 段；用 start_line 指定从第几段开始读）\n")
			return b.String()
		}
		text := strings.TrimSpace(p.Text)
		mark := ""
		if p.Obj != "" {
			mark = "［含" + p.Obj + "］"
		}
		if text == "" && mark == "" {
			b.WriteString("[" + itoa(i) + "]（空段落）\n")
		} else {
			b.WriteString("[" + itoa(i) + "] (" + d.styleName(p.StyleID) + ") " + mark + text + "\n")
		}
		shown++
	}
	for ; ti < len(d.tables); ti++ {
		tt := d.tables[ti]
		b.WriteString("    ┆（表格 " + itoa(ti+1) + "：" + itoa(tt.Rows) + " 行 × " + itoa(tt.Cols) + " 列；第一行：" + tt.First + "）\n")
	}
	return b.String()
}

// ---------------- Word：按段落修改 ----------------

type docxEdit struct {
	Op    string `json:"op"` // replace / insert_after / delete
	Index int    `json:"index"`
	Text  string `json:"text"`
}

var (
	reOpenP    = regexp.MustCompile(`^<w:p(?:\s[^>]*)?>`)
	rePPr      = regexp.MustCompile(`(?s)<w:pPr>.*?</w:pPr>|<w:pPr/>`)
	reFirstRun = regexp.MustCompile(`(?s)<w:r(?:\s[^>]*)?>.*?</w:r>`)
	reRPr      = regexp.MustCompile(`(?s)<w:rPr>.*?</w:rPr>`)
	reParaIDs  = regexp.MustCompile(`\s+w14:(?:paraId|textId)="[^"]*"`)
)

// buildPara 用参考段落的段落格式和第一段文字的字符格式，生成一个新段落
func buildPara(ref, text string, fresh bool) string {
	open := reOpenP.FindString(ref)
	if open == "" || strings.HasSuffix(open, "/>") {
		open = "<w:p>"
	}
	if fresh {
		open = reParaIDs.ReplaceAllString(open, "")
	}
	pPr := rePPr.FindString(ref)
	rPr := ""
	if run := reFirstRun.FindString(ref); run != "" {
		rPr = reRPr.FindString(run)
	}
	var b strings.Builder
	b.WriteString(open + pPr)
	if text != "" {
		b.WriteString("<w:r>" + rPr)
		for i, line := range strings.Split(text, "\n") {
			if i > 0 {
				b.WriteString("<w:br/>")
			}
			for j, seg := range strings.Split(line, "\t") {
				if j > 0 {
					b.WriteString("<w:tab/>")
				}
				if seg != "" {
					b.WriteString(`<w:t xml:space="preserve">` + xmlText(seg) + `</w:t>`)
				}
			}
		}
		b.WriteString("</w:r>")
	}
	b.WriteString("</w:p>")
	return b.String()
}

// applyEdits 返回修改后的 docx 和给用户看的改动说明
func (d *docxDoc) applyEdits(edits []docxEdit) ([]byte, []diffLine, error) {
	if !strings.Contains(d.xml, `xmlns:w="`+wNS+`"`) {
		return nil, nil, errBad("这个 Word 文件的内部写法不常见，为避免损坏文件，不做修改。可以用 Word 打开另存一份再试")
	}
	if len(edits) == 0 {
		return nil, nil, errBad("没有要做的修改")
	}
	if len(edits) > 60 {
		return nil, nil, errBad("一次最多改 60 处")
	}
	n := len(d.paras)
	type slot struct {
		replace *string
		del     bool
		inserts []string
	}
	slots := map[int]*slot{}
	get := func(i int) *slot {
		if slots[i] == nil {
			slots[i] = &slot{}
		}
		return slots[i]
	}
	var diff []diffLine
	for _, e := range edits {
		e.Text = strings.ReplaceAll(e.Text, "\r\n", "\n")
		switch e.Op {
		case "replace", "delete":
			if e.Index < 0 || e.Index >= n {
				return nil, nil, errBad("段落编号 " + itoa(e.Index) + " 不存在（共 " + itoa(n) + " 段，编号 0–" + itoa(n-1) + "）。请先 read_file 看段落编号")
			}
			p := d.paras[e.Index]
			if p.Obj != "" {
				return nil, nil, errBad("第 " + itoa(e.Index) + " 段含" + p.Obj + "，改写会把" + p.Obj + "弄丢。可以在它前后插入新段落，或者请用户在 Word 里手动改")
			}
			s := get(e.Index)
			if s.replace != nil || s.del {
				return nil, nil, errBad("第 " + itoa(e.Index) + " 段被改了两次，请合并成一次")
			}
			if e.Op == "delete" {
				s.del = true
				diff = append(diff, diffLine{Op: "-", Text: "[" + itoa(e.Index) + "] " + clipRunes(p.Text, 400), Old: e.Index + 1})
			} else {
				if strings.TrimSpace(e.Text) == "" {
					return nil, nil, errBad("替换的文字是空的；要删除段落请用 op=delete")
				}
				t := e.Text
				s.replace = &t
				diff = append(diff, diffLine{Op: "-", Text: "[" + itoa(e.Index) + "] " + clipRunes(p.Text, 400), Old: e.Index + 1},
					diffLine{Op: "+", Text: "[" + itoa(e.Index) + "] " + clipRunes(t, 400), New: e.Index + 1})
			}
		case "insert_after":
			if e.Index < -1 || e.Index >= n {
				return nil, nil, errBad("段落编号 " + itoa(e.Index) + " 不存在（插到最前面用 -1）")
			}
			if strings.TrimSpace(e.Text) == "" {
				return nil, nil, errBad("插入的文字是空的")
			}
			get(e.Index).inserts = append(get(e.Index).inserts, e.Text)
			diff = append(diff, diffLine{Op: "+", Text: "（插在第 " + itoa(e.Index) + " 段后）" + clipRunes(e.Text, 400), New: e.Index + 2})
		default:
			return nil, nil, errBad("op 只能是 replace、insert_after 或 delete")
		}
	}
	idx := make([]int, 0, len(slots))
	for i := range slots {
		idx = append(idx, i)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(idx)))
	x := d.xml
	for _, i := range idx {
		s := slots[i]
		if i == -1 {
			// 插到第一段前面：沿用第一段的格式
			if n == 0 {
				return nil, nil, errBad("文档里没有段落，无法确定插入位置")
			}
			ref := d.xml[d.paras[0].Start:d.paras[0].End]
			var add strings.Builder
			for _, t := range s.inserts {
				add.WriteString(buildPara(ref, t, true))
			}
			x = x[:d.paras[0].Start] + add.String() + x[d.paras[0].Start:]
			continue
		}
		p := d.paras[i]
		ref := d.xml[p.Start:p.End]
		var rep strings.Builder
		switch {
		case s.del:
		case s.replace != nil:
			rep.WriteString(buildPara(ref, *s.replace, false))
		default:
			rep.WriteString(ref)
		}
		for _, t := range s.inserts {
			rep.WriteString(buildPara(ref, t, true))
		}
		x = x[:p.Start] + rep.String() + x[p.End:]
	}
	// 自检：修改后的 XML 必须还能解析
	if err := xml.NewDecoder(strings.NewReader(x)).Decode(new(struct{ XMLName xml.Name })); err != nil {
		return nil, nil, errBad("修改后的文档自检失败，没有提交：" + err.Error())
	}
	out, err := rezip(d.zr, map[string][]byte{"word/document.xml": []byte(x)})
	if err != nil {
		return nil, nil, err
	}
	if _, err := parseDocxDoc(out); err != nil {
		return nil, nil, errBad("修改后的文档自检失败，没有提交")
	}
	return out, diff, nil
}

// rezip 复制原压缩包，只替换指定的部件（其余部件原样复制，不重新压缩）
func rezip(zr *zip.Reader, replace map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		if nb, ok := replace[f.Name]; ok {
			h := f.FileHeader
			h.Method = zip.Deflate
			w, err := zw.CreateHeader(&h)
			if err != nil {
				return nil, errBad("写入失败：" + err.Error())
			}
			w.Write(nb)
			continue
		}
		if err := zw.Copy(f); err != nil {
			return nil, errBad("写入失败：" + err.Error())
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------- Word：模板格式分析 ----------------

var cnSizes = []struct {
	half int
	name string
}{{84, "初号"}, {72, "小初"}, {52, "一号"}, {48, "小一"}, {44, "二号"}, {36, "小二"}, {32, "三号"}, {30, "小三"}, {28, "四号"}, {24, "小四"}, {21, "五号"}, {18, "小五"}, {15, "六号"}}

func sizeName(half int) string {
	if half <= 0 {
		return "默认字号"
	}
	pt := strconv.FormatFloat(float64(half)/2, 'f', -1, 64) + " 磅"
	for _, s := range cnSizes {
		if s.half == half {
			return s.name + "（" + pt + "）"
		}
	}
	return pt
}

func docxFormatSummary(data []byte) (string, error) {
	di, err := parseDocx(data)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	cm := func(tw int) string { return strconv.FormatFloat(float64(tw)/567, 'f', 2, 64) + " cm" }
	if di.PageW > 0 {
		paper := ""
		if di.PageW == 11906 && di.PageH == 16838 {
			paper = "A4，"
		}
		b.WriteString("纸张：" + paper + cm(di.PageW) + " × " + cm(di.PageH) + "\n")
		b.WriteString("页边距：上 " + cm(di.Margins[0]) + "，下 " + cm(di.Margins[2]) + "，左 " + cm(di.Margins[3]) + "，右 " + cm(di.Margins[1]) + "\n")
	}
	type fs struct {
		font string
		size int
	}
	body := map[fs]int{}
	heads := map[int]map[fs]int{}
	headEx := map[int]string{}
	headStyle := map[int]string{}
	var tables, imgs, caps int
	for _, bl := range di.Blocks {
		switch bl.Kind {
		case "tbl":
			tables++
			continue
		case "img":
			imgs++
			continue
		}
		if bl.Cap {
			caps++
			continue
		}
		k := fs{bl.Font, bl.Size}
		if bl.Head > 0 {
			if heads[bl.Head] == nil {
				heads[bl.Head] = map[fs]int{}
			}
			heads[bl.Head][k]++
			if headEx[bl.Head] == "" {
				headEx[bl.Head] = clipRunes(bl.Text, 30)
				headStyle[bl.Head] = bl.Style
			}
		} else if utf8.RuneCountInString(bl.Text) >= 20 && !bl.Title {
			body[k]++
		}
	}
	top := func(m map[fs]int) fs {
		var best fs
		n := -1
		for k, v := range m {
			if v > n {
				best, n = k, v
			}
		}
		return best
	}
	desc := func(k fs) string {
		f := k.font
		if f == "" {
			f = "默认字体"
		}
		return f + " " + sizeName(k.size)
	}
	if len(body) > 0 {
		b.WriteString("正文：" + desc(top(body)) + "\n")
	}
	for lv := 1; lv <= 4; lv++ {
		if heads[lv] == nil {
			continue
		}
		b.WriteString(itoa(lv) + " 级标题：" + desc(top(heads[lv])) + "；样式“" + headStyle[lv] + "”；例如“" + headEx[lv] + "”\n")
	}
	if len(heads) == 0 {
		b.WriteString("没有使用标题样式（标题只是加粗的普通段落），生成目录和导航会比较麻烦\n")
	}
	b.WriteString("表格 " + itoa(tables) + " 个，图片 " + itoa(imgs) + " 张，题注 " + itoa(caps) + " 个；")
	if di.HasTOC {
		b.WriteString("有自动目录。\n")
	} else {
		b.WriteString("没有自动目录。\n")
	}
	b.WriteString("\n结构（前 40 个标题）：\n")
	n := 0
	for _, bl := range di.Blocks {
		if bl.Head > 0 && n < 40 {
			b.WriteString(strings.Repeat("  ", bl.Head-1) + "- " + clipRunes(bl.Text, 40) + "\n")
			n++
		}
	}
	return b.String(), nil
}

// ---------------- Excel ----------------

func xlsxText(data []byte, limit int) (string, error) {
	zr, err := openZip(data)
	if err != nil {
		return "", err
	}
	wb := zipRead(zr, "xl/workbook.xml")
	if wb == nil {
		return "", errBad("不是 Excel 文件（缺少 xl/workbook.xml）")
	}
	// 共享字符串
	var shared []string
	if b := zipRead(zr, "xl/sharedStrings.xml"); b != nil {
		dec := xml.NewDecoder(bytes.NewReader(b))
		var cur strings.Builder
		in, inT, skip := false, false, 0
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "si":
					in = true
					cur.Reset()
				case "t":
					inT = true
				case "rPh":
					skip++
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "si":
					in = false
					shared = append(shared, cur.String())
				case "t":
					inT = false
				case "rPh":
					skip--
				}
			case xml.CharData:
				if in && inT && skip == 0 {
					cur.Write(t)
				}
			}
		}
	}
	// 工作表名称和文件
	rels := map[string]string{}
	if b := zipRead(zr, "xl/_rels/workbook.xml.rels"); b != nil {
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Relationship" {
				t := xattr(se, "Target")
				if strings.HasPrefix(t, "/") {
					t = strings.TrimPrefix(t, "/")
				} else {
					t = path.Join("xl", t)
				}
				rels[xattr(se, "Id")] = t
			}
		}
	}
	type sheet struct{ name, file string }
	var sheets []sheet
	dec := xml.NewDecoder(bytes.NewReader(wb))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sheet" {
			id := ""
			for _, a := range se.Attr {
				if a.Name.Local == "id" {
					id = a.Value
				}
			}
			sheets = append(sheets, sheet{xattr(se, "name"), rels[id]})
		}
	}
	var b strings.Builder
	b.WriteString("Excel 工作簿，共 " + itoa(len(sheets)) + " 个工作表（日期可能显示为数字；公式显示计算结果）：\n")
	for _, sh := range sheets {
		if b.Len() > limit {
			b.WriteString("……（内容太长，后面的工作表省略）\n")
			break
		}
		x := zipRead(zr, sh.file)
		if x == nil {
			continue
		}
		b.WriteString("\n=== 工作表：" + sh.name + " ===\n")
		rows := xlsxRows(x, shared)
		for i, r := range rows {
			if b.Len() > limit {
				b.WriteString("……（还有 " + itoa(len(rows)-i) + " 行）\n")
				break
			}
			b.WriteString(r + "\n")
		}
	}
	return b.String(), nil
}

func colIndex(ref string) int {
	n := 0
	for _, c := range ref {
		if c >= 'A' && c <= 'Z' {
			n = n*26 + int(c-'A'+1)
		} else {
			break
		}
	}
	return n - 1
}

func xlsxRows(x []byte, shared []string) []string {
	dec := xml.NewDecoder(bytes.NewReader(x))
	var out []string
	var cells []string
	rowNo := ""
	var cType, cRef string
	var val strings.Builder
	inV, inT := false, false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				cells, rowNo = nil, xattr(t, "r")
			case "c":
				cType, cRef = xattr(t, "t"), xattr(t, "r")
				val.Reset()
			case "v":
				inV = true
			case "t":
				inT = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
			case "t":
				inT = false
			case "c":
				v := val.String()
				switch cType {
				case "s":
					if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				case "b":
					v = map[string]string{"1": "TRUE", "0": "FALSE"}[v]
				}
				ci := colIndex(cRef)
				if ci < 0 {
					ci = len(cells)
				}
				for len(cells) < ci {
					cells = append(cells, "")
				}
				if ci < 200 {
					cells = append(cells[:ci], strings.ReplaceAll(v, "\n", " "))
				}
			case "row":
				for len(cells) > 0 && cells[len(cells)-1] == "" {
					cells = cells[:len(cells)-1]
				}
				if len(cells) > 0 {
					out = append(out, rowNo+"│"+strings.Join(cells, "\t"))
				}
			}
		case xml.CharData:
			if inV || inT {
				val.Write(t)
			}
		}
	}
	return out
}

// ---------------- PowerPoint ----------------

var reSlideNo = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

func pptxText(data []byte, limit int) (string, error) {
	zr, err := openZip(data)
	if err != nil {
		return "", err
	}
	type sl struct {
		n    int
		name string
	}
	var slides []sl
	for _, f := range zr.File {
		if m := reSlideNo.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, sl{n, f.Name})
		}
	}
	if len(slides) == 0 {
		return "", errBad("不是 PowerPoint 文件，或里面没有幻灯片")
	}
	// 按演示文稿中的顺序排列（读不到时按文件编号）
	order := map[string]int{}
	if pres := zipRead(zr, "ppt/presentation.xml"); pres != nil {
		rels := map[string]string{}
		if rb := zipRead(zr, "ppt/_rels/presentation.xml.rels"); rb != nil {
			dec := xml.NewDecoder(bytes.NewReader(rb))
			for {
				tok, err := dec.Token()
				if err != nil {
					break
				}
				if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Relationship" {
					rels[xattr(se, "Id")] = path.Join("ppt", xattr(se, "Target"))
				}
			}
		}
		dec := xml.NewDecoder(bytes.NewReader(pres))
		i := 0
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sldId" {
				for _, a := range se.Attr {
					if a.Name.Local == "id" && a.Name.Space != "" {
						order[rels[a.Value]] = i
						i++
					}
				}
			}
		}
	}
	sort.Slice(slides, func(i, j int) bool {
		oi, ok1 := order[slides[i].name]
		oj, ok2 := order[slides[j].name]
		if ok1 && ok2 {
			return oi < oj
		}
		return slides[i].n < slides[j].n
	})
	var b strings.Builder
	b.WriteString("PowerPoint 演示文稿，共 " + itoa(len(slides)) + " 页：\n")
	for i, s := range slides {
		if b.Len() > limit {
			b.WriteString("……（后面 " + itoa(len(slides)-i) + " 页省略）\n")
			break
		}
		b.WriteString("\n--- 第 " + itoa(i+1) + " 页 ---\n")
		x := zipRead(zr, s.name)
		dec := xml.NewDecoder(bytes.NewReader(x))
		var para strings.Builder
		inT := false
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Space == aNS && t.Name.Local == "t" {
					inT = true
				}
			case xml.EndElement:
				if t.Name.Space == aNS && t.Name.Local == "t" {
					inT = false
				}
				if t.Name.Space == aNS && t.Name.Local == "p" {
					if s := strings.TrimSpace(para.String()); s != "" {
						b.WriteString(s + "\n")
					}
					para.Reset()
				}
			case xml.CharData:
				if inT {
					para.Write(t)
				}
			}
		}
	}
	return b.String(), nil
}

// ---------------- Markdown → Word ----------------

var (
	reMdInline = regexp.MustCompile(`(?s)\*\*(.+?)\*\*|<span\s+style\s*=\s*["']([^"']*)["']\s*>(.*?)</span>|<b>(.*?)</b>|<strong>(.*?)</strong>|<i>(.*?)</i>|<em>(.*?)</em>|<u>(.*?)</u>|\*([^*\s][^*]*?)\*`)
	reMdBlock  = regexp.MustCompile(`(?is)^<(h[1-6]|p)(?:\s+style\s*=\s*["']([^"']*)["'])?\s*>(.*)</(?:h[1-6]|p)>$`)
	reMdHead   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reMdBullet = regexp.MustCompile(`^\s*[-*+•]\s+(.*)$`)
	reMdNum    = regexp.MustCompile(`^\s*(\d+[.、)])\s*(.*)$`)
	reMdTblSep = regexp.MustCompile(`^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$`)
	reColorHex = regexp.MustCompile(`^#?([0-9a-fA-F]{6})$`)
)

var cssColors = map[string]string{"red": "FF0000", "blue": "0000FF", "green": "008000", "black": "000000", "gray": "808080", "grey": "808080", "orange": "FF8C00", "purple": "800080", "红色": "FF0000", "蓝色": "0000FF", "绿色": "008000", "黑色": "000000", "灰色": "808080"}

func cssStyle(s string) map[string]string {
	out := map[string]string{}
	for _, it := range strings.Split(s, ";") {
		if k, v, ok := strings.Cut(it, ":"); ok {
			out[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return out
}

// cssRPr 把 CSS 样式转成 Word 字符格式
func cssRPr(st map[string]string, bold, italic, under bool) string {
	var b strings.Builder
	if f := st["font-family"]; f != "" {
		f = xmlText(strings.Split(f, ",")[0])
		b.WriteString(`<w:rFonts w:ascii="` + f + `" w:hAnsi="` + f + `" w:eastAsia="` + f + `"/>`)
	}
	if bold || st["font-weight"] == "bold" || st["font-weight"] == "700" {
		b.WriteString("<w:b/><w:bCs/>")
	}
	if italic || st["font-style"] == "italic" {
		b.WriteString("<w:i/>")
	}
	if c := strings.ToLower(st["color"]); c != "" {
		if hex, ok := cssColors[c]; ok {
			b.WriteString(`<w:color w:val="` + hex + `"/>`)
		} else if m := reColorHex.FindStringSubmatch(c); m != nil {
			b.WriteString(`<w:color w:val="` + strings.ToUpper(m[1]) + `"/>`)
		}
	}
	if sz := st["font-size"]; sz != "" {
		half := 0
		for _, s := range cnSizes {
			if strings.Contains(sz, s.name) {
				half = s.half
			}
		}
		if half == 0 {
			v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(sz), "pt"), "px"))
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 200 {
				if strings.HasSuffix(strings.ToLower(sz), "px") {
					f *= 0.75
				}
				half = int(f*2 + 0.5)
			}
		}
		if half > 0 {
			b.WriteString(`<w:sz w:val="` + itoa(half) + `"/><w:szCs w:val="` + itoa(half) + `"/>`)
		}
	}
	if under || strings.Contains(st["text-decoration"], "underline") {
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return "<w:rPr>" + b.String() + "</w:rPr>"
}

func mdRuns(s string) string {
	var b strings.Builder
	run := func(text, rPr string) {
		if text == "" {
			return
		}
		b.WriteString("<w:r>" + rPr + `<w:t xml:space="preserve">` + xmlText(stripTags(text)) + "</w:t></w:r>")
	}
	last := 0
	for _, m := range reMdInline.FindAllStringSubmatchIndex(s, -1) {
		run(s[last:m[0]], "")
		g := func(i int) string {
			if m[2*i] < 0 {
				return ""
			}
			return s[m[2*i]:m[2*i+1]]
		}
		switch {
		case m[2] >= 0:
			run(g(1), cssRPr(nil, true, false, false))
		case m[4] >= 0:
			run(g(3), cssRPr(cssStyle(g(2)), false, false, false))
		case m[8] >= 0 || m[10] >= 0:
			run(g(4)+g(5), cssRPr(nil, true, false, false))
		case m[12] >= 0 || m[14] >= 0:
			run(g(6)+g(7), cssRPr(nil, false, true, false))
		case m[16] >= 0:
			run(g(8), cssRPr(nil, false, false, true))
		case m[18] >= 0:
			run(g(9), cssRPr(nil, false, true, false))
		}
		last = m[1]
	}
	run(s[last:], "")
	return b.String()
}

var reTag = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)

func stripTags(s string) string { return reTag.ReplaceAllString(s, "") }

// markdownDocx 用 Markdown（可夹带简单 HTML）生成 Word
func markdownDocx(md, title string, en bool) ([]byte, int, error) {
	d := &docBuilder{en: en}
	d.b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`)
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	paras := 0
	pp := func(style, jc, runs string, extra string) {
		d.b.WriteString("<w:p><w:pPr>")
		if style != "" {
			d.b.WriteString(`<w:pStyle w:val="` + style + `"/>`)
		}
		d.b.WriteString(extra)
		if jc != "" {
			d.b.WriteString(`<w:jc w:val="` + jc + `"/>`)
		}
		d.b.WriteString("</w:pPr>" + runs + "</w:p>")
		paras++
	}
	jcOf := func(st map[string]string) string {
		return map[string]string{"center": "center", "right": "right", "left": "left", "justify": "both"}[strings.ToLower(st["text-align"])]
	}
	inCode := false
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			pp("NoIndent", "", `<w:r><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="20"/></w:rPr><w:t xml:space="preserve">`+xmlText(raw)+`</w:t></w:r>`, "")
			continue
		}
		if line == "" {
			continue
		}
		if line == `\newpage` || strings.EqualFold(line, "<pagebreak>") || strings.EqualFold(line, "<pagebreak/>") {
			d.b.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
			continue
		}
		// 表格：| a | b | 下一行是 |---|---|
		if strings.HasPrefix(line, "|") && i+1 < len(lines) && reMdTblSep.MatchString(lines[i+1]) {
			var rows [][]string
			cells := func(l string) []string {
				l = strings.Trim(strings.TrimSpace(l), "|")
				var out []string
				for _, c := range strings.Split(l, "|") {
					out = append(out, stripTags(strings.ReplaceAll(strings.TrimSpace(c), "**", "")))
				}
				return out
			}
			rows = append(rows, cells(line))
			i++
			for i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "|") {
				i++
				r := cells(lines[i])
				for len(r) < len(rows[0]) {
					r = append(r, "")
				}
				rows = append(rows, r[:len(rows[0])])
			}
			d.table(rows)
			paras++
			continue
		}
		if m := reMdBlock.FindStringSubmatch(line); m != nil {
			st := cssStyle(m[2])
			tag := strings.ToLower(m[1])
			style := ""
			if tag != "p" {
				lv := int(tag[1] - '0')
				if lv > 3 {
					lv = 3
				}
				style = "Heading" + itoa(lv)
			} else if jcOf(st) != "" {
				style = "NoIndent"
			}
			pp(style, jcOf(st), mdRuns(wrapSpan(m[3], st)), "")
			continue
		}
		if m := reMdHead.FindStringSubmatch(line); m != nil {
			lv := len(m[1])
			if lv > 3 {
				lv = 3
			}
			pp("Heading"+itoa(lv), "", mdRuns(m[2]), "")
			continue
		}
		if m := reMdBullet.FindStringSubmatch(raw); m != nil {
			pp("NoIndent", "", mdRuns("• "+m[1]), `<w:ind w:left="420" w:hanging="210"/>`)
			continue
		}
		if m := reMdNum.FindStringSubmatch(raw); m != nil && utf8.RuneCountInString(m[2]) > 0 && !strings.HasPrefix(strings.TrimSpace(m[2]), "|") {
			pp("NoIndent", "", mdRuns(m[1]+" "+m[2]), `<w:ind w:left="420" w:hanging="420"/>`)
			continue
		}
		if strings.HasPrefix(line, "> ") {
			pp("Hint", "", mdRuns(strings.TrimPrefix(line, "> ")), "")
			continue
		}
		pp("", "", mdRuns(line), "")
	}
	if paras == 0 {
		return nil, 0, errBad("内容是空的")
	}
	d.b.WriteString(`<w:sectPr><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1418" w:right="1418" w:bottom="1418" w:left="1418" w:header="851" w:footer="851" w:gutter="0"/></w:sectPr></w:body></w:document>`)
	out, err := packDocx(d.b.String(), en, title)
	return out, paras, err
}

// wrapSpan 块级标签上的字体样式作用到整段文字
func wrapSpan(inner string, st map[string]string) string {
	var parts []string
	for k, v := range st {
		if k != "text-align" {
			parts = append(parts, k+":"+v)
		}
	}
	if len(parts) == 0 {
		return inner
	}
	sort.Strings(parts)
	if strings.Contains(inner, "<span") || strings.Contains(inner, "**") {
		return inner
	}
	return `<span style="` + strings.Join(parts, ";") + `">` + inner + `</span>`
}
