package main

// 生成论文模板：Word（.docx，只用标准库拼出 OOXML）和 LaTeX（.tex）。
// 模板里是规范的样式（标题层级、正文、图题表题、参考文献）和每一节的写作提示（灰色文字，写完删掉）。

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
)

func xmlText(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

var cnNum = []string{"一", "二", "三", "四", "五", "六", "七", "八", "九", "十", "十一", "十二", "十三", "十四", "十五"}

// 不参与章节编号的部分
var unnumbered = map[string]bool{"题目": true, "题名": true, "项目名称": true, "摘要": true, "关键词": true, "Abstract": true, "目录": true, "参考文献": true, "附录": true, "致谢": true,
	"Title": true, "Summary paragraph": true, "References": true, "Figure legends": true, "Data availability": true, "Code availability": true, "Author contributions": true, "Competing interests": true}

type docBuilder struct {
	b  strings.Builder
	en bool
}

func (d *docBuilder) para(style, text string, opts ...string) {
	d.b.WriteString("<w:p><w:pPr>")
	if style != "" {
		d.b.WriteString(`<w:pStyle w:val="` + style + `"/>`)
	}
	for _, o := range opts {
		switch o {
		case "center":
			d.b.WriteString(`<w:jc w:val="center"/>`)
		case "break":
			d.b.WriteString(`<w:pageBreakBefore/>`)
		}
	}
	d.b.WriteString("</w:pPr>")
	if text != "" {
		d.b.WriteString(`<w:r><w:t xml:space="preserve">` + xmlText(text) + `</w:t></w:r>`)
	}
	d.b.WriteString("</w:p>")
}

// label + text in one paragraph (e.g. 关键词：)
func (d *docBuilder) labelPara(label, text string) {
	d.b.WriteString(`<w:p><w:pPr><w:pStyle w:val="NoIndent"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">` + xmlText(label) + `</w:t></w:r>`)
	d.b.WriteString(`<w:r><w:rPr><w:color w:val="808080"/></w:rPr><w:t xml:space="preserve">` + xmlText(text) + `</w:t></w:r></w:p>`)
}

func (d *docBuilder) hint(text string) { d.para("Hint", text) }

// 三线表示例
func (d *docBuilder) table(rows [][]string) {
	d.b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:jc w:val="center"/><w:tblBorders>` +
		`<w:top w:val="single" w:sz="12" w:space="0" w:color="000000"/><w:bottom w:val="single" w:sz="12" w:space="0" w:color="000000"/>` +
		`</w:tblBorders><w:tblLook w:val="0000"/></w:tblPr><w:tblGrid>`)
	for range rows[0] {
		d.b.WriteString(`<w:gridCol w:w="2400"/>`)
	}
	d.b.WriteString(`</w:tblGrid>`)
	for i, r := range rows {
		d.b.WriteString("<w:tr>")
		for _, c := range r {
			d.b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="2400" w:type="dxa"/>`)
			if i == 0 {
				d.b.WriteString(`<w:tcBorders><w:bottom w:val="single" w:sz="6" w:space="0" w:color="000000"/></w:tcBorders>`)
			}
			d.b.WriteString(`</w:tcPr><w:p><w:pPr><w:pStyle w:val="TableText"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(c) + `</w:t></w:r></w:p></w:tc>`)
		}
		d.b.WriteString("</w:tr>")
	}
	d.b.WriteString("</w:tbl>")
}

func (d *docBuilder) figureBox() {
	// 用一个带边框的空段落代表图的位置
	d.b.WriteString(`<w:p><w:pPr><w:pStyle w:val="NoIndent"/><w:jc w:val="center"/><w:pBdr><w:top w:val="dashed" w:sz="6" w:space="8" w:color="999999"/><w:bottom w:val="dashed" w:sz="6" w:space="8" w:color="999999"/></w:pBdr></w:pPr>`)
	t := "（在这里插入图片：插入 → 图片）"
	if d.en {
		t = "(Insert figure here)"
	}
	d.b.WriteString(`<w:r><w:rPr><w:color w:val="999999"/></w:rPr><w:t>` + xmlText(t) + `</w:t></w:r></w:p>`)
}

func sectionHint(s WSection, en bool) string {
	var parts []string
	if en {
		parts = append(parts, "What to write: "+s.What)
		if s.Len != "" {
			parts = append(parts, "Length: "+s.Len)
		}
	} else {
		parts = append(parts, "【写什么】"+s.What)
		if s.Len != "" {
			parts = append(parts, "【篇幅】"+s.Len)
		}
		for _, t := range s.Tips {
			parts = append(parts, "【注意】"+t)
		}
	}
	return strings.Join(parts, "  ")
}

func isResultsSection(n string) bool {
	for _, k := range []string{"结果", "模型的建立与求解", "正文", "Results", "Main text", "研究内容与方法"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

func docxTemplate(p *WProfile) ([]byte, error) {
	en := p.Limits.Lang == "en"
	d := &docBuilder{en: en}
	d.b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`)
	if en {
		d.hint("Template for " + p.Name + ". Grey text is guidance — delete it when you write. Basis: " + p.Basis + ". " + p.Note + ".")
		d.para("Title", "Title of the paper", "center")
		d.para("NoIndent", "Author One¹, Author Two² & Author Three¹*", "center")
	} else {
		d.hint("这是“" + p.Name + "”模板。灰色文字是写作提示，写的时候删掉。依据：" + p.Basis + "。" + p.Note + "。")
		d.para("Title", "论文题目", "center")
		if p.Limits.NoIdentity {
			d.hint("注意：全文任何地方都不能出现学校、队员姓名和赛区信息。")
		}
	}
	n := 0
	fig, tbl := false, false
	for _, s := range p.Sections {
		switch s.Name {
		case "题目", "题名", "项目名称", "Title":
			continue
		case "关键词":
			d.labelPara("关键词：", "关键词1；关键词2；关键词3（"+s.What+"）")
			if p.Key == "mcm" {
				n = 0 // 摘要页之后另起一页
			}
			continue
		case "目录":
			d.para("Heading1", "目  录", "center")
			d.hint("写完后在 Word 中：引用 → 目录 → 自动目录，自动生成；改动后右键目录 → 更新域。")
			continue
		}
		style := "Heading1"
		title := s.Name
		opts := []string{}
		if unnumbered[s.Name] || strings.HasPrefix(s.Name, "摘要") {
			if !en {
				opts = append(opts, "center")
			}
		} else {
			n++
			switch {
			case en:
			case p.Key == "mcm":
				title = cnNum[(n-1)%len(cnNum)] + "、" + s.Name
			default:
				title = itoa(n) + " " + s.Name
			}
		}
		if p.Key == "mcm" && n == 1 && !unnumbered[s.Name] {
			opts = append(opts, "break")
		}
		if (s.Name == "参考文献" || s.Name == "References") && p.Key == "thesis" {
			opts = append(opts, "break")
		}
		d.para(style, title, opts...)
		d.hint(sectionHint(s, en))
		switch {
		case s.Name == "摘要" || strings.HasPrefix(s.Name, "Summary") || s.Name == "Abstract" && en:
			if en {
				d.para("", "Write the abstract here.")
			} else {
				d.para("", "在这里写摘要。")
			}
		case s.Name == "参考文献" || s.Name == "References":
			if p.Limits.RefStyle == "nature" {
				d.para("Ref", "1. Surname, A. B., Surname, C. D. & Surname, E. Title of the article. J. Abbrev. 12, 345–356 (2024).")
				d.para("Ref", "2. Surname, A. B. et al. Title of the article. J. Abbrev. 8, 100–110 (2023).")
				d.hint("Numbered in order of first citation; list up to five authors, otherwise first author et al.; journal names abbreviated and italic (check the journal guide).")
			} else {
				d.para("Ref", "[1] 作者1, 作者2, 作者3, 等. 文章题名[J]. 刊名, 年, 卷(期): 起始页-结束页.")
				d.para("Ref", "[2] 作者. 书名[M]. 版本（第1版不写）. 出版地: 出版者, 出版年: 引用页码.")
				d.para("Ref", "[3] 作者. 论文题名[D]. 保存地: 学位授予单位, 年.")
				d.para("Ref", "[4] 作者. 题名[EB/OL]. (发布日期)[引用日期]. 网址.")
				d.hint("以上是 GB/T 7714—2015 的格式示例，请换成真实文献。可以在工作台“论文库”里一键复制每篇文献的 GB/T 7714 引用。")
			}
		case s.Name == "附录" && p.Limits.NeedCode:
			d.para("", "附录1 支撑材料文件列表")
			d.para("", "附录2 源程序（完整、可运行）")
		case isResultsSection(s.Name) && !fig:
			fig = true
			d.figureBox()
			if en {
				d.para("Caption", "Fig. 1 | Figure title. Legend text (fewer than 300 words).")
			} else {
				d.para("Caption", "图 1  图题（图题放在图的下方）")
			}
			if !tbl {
				tbl = true
				if en {
					d.para("Caption", "Table 1 | Table title")
					d.table([][]string{{"Group", "n", "Value (mean ± s.d.)"}, {"A", "10", "1.23 ± 0.10"}, {"B", "10", "2.34 ± 0.12"}})
				} else {
					d.para("Caption", "表 1  表题（表题放在表的上方，推荐三线表）")
					d.table([][]string{{"组别", "样本量", "结果"}, {"A", "10", "1.23"}, {"B", "10", "2.34"}})
				}
				d.para("", "")
			}
		default:
			d.para("", "")
		}
	}
	if en {
		for _, st := range p.Limits.Statements {
			found := false
			for _, s := range p.Sections {
				if strings.EqualFold(s.Name, st) {
					found = true
				}
			}
			if !found {
				d.para("Heading1", st)
				d.para("", "")
			}
		}
	}
	// 页面：A4，页边距 2.5 cm，页脚居中页码
	d.b.WriteString(`<w:sectPr><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1418" w:right="1418" w:bottom="1418" w:left="1418" w:header="851" w:footer="851" w:gutter="0"/></w:sectPr>`)
	d.b.WriteString(`</w:body></w:document>`)
	return packDocx(d.b.String(), en, p.Name)
}

// packDocx 把正文 XML 和统一的样式、页脚、关系文件打包成 .docx
func packDocx(docXML string, en bool, title string) ([]byte, error) {
	east, latin, head := "宋体", "Times New Roman", "黑体"
	if en {
		east, head = "Times New Roman", "Times New Roman"
	}
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="` + latin + `" w:hAnsi="` + latin + `" w:eastAsia="` + east + `" w:cs="` + latin + `"/><w:sz w:val="24"/><w:szCs w:val="24"/><w:lang w:val="en-US" w:eastAsia="zh-CN"/></w:rPr></w:rPrDefault>` +
		`<w:pPrDefault><w:pPr><w:spacing w:line="360" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		style("Normal", "Normal", "", firstIndent(!en), "", true) +
		style("Title", "Title", "Normal", `<w:jc w:val="center"/><w:spacing w:before="240" w:after="240"/><w:ind w:firstLineChars="0" w:firstLine="0"/>`, fontRun(head, 32, true), false) +
		style("Heading1", "heading 1", "Normal", `<w:keepNext/><w:spacing w:before="240" w:after="120"/><w:ind w:firstLineChars="0" w:firstLine="0"/><w:outlineLvl w:val="0"/>`, fontRun(head, 28, true), false) +
		style("Heading2", "heading 2", "Normal", `<w:keepNext/><w:spacing w:before="120" w:after="60"/><w:ind w:firstLineChars="0" w:firstLine="0"/><w:outlineLvl w:val="1"/>`, fontRun(head, 24, true), false) +
		style("Heading3", "heading 3", "Normal", `<w:keepNext/><w:ind w:firstLineChars="0" w:firstLine="0"/><w:outlineLvl w:val="2"/>`, fontRun(head, 24, false), false) +
		style("Caption", "caption", "Normal", `<w:jc w:val="center"/><w:ind w:firstLineChars="0" w:firstLine="0"/><w:spacing w:before="60" w:after="120"/>`, `<w:rPr><w:sz w:val="21"/><w:szCs w:val="21"/></w:rPr>`, false) +
		style("NoIndent", "No Indent", "Normal", `<w:ind w:firstLineChars="0" w:firstLine="0"/>`, "", false) +
		style("TableText", "Table Text", "Normal", `<w:jc w:val="center"/><w:spacing w:line="240" w:lineRule="auto"/><w:ind w:firstLineChars="0" w:firstLine="0"/>`, `<w:rPr><w:sz w:val="21"/><w:szCs w:val="21"/></w:rPr>`, false) +
		style("Ref", "Bibliography", "Normal", `<w:ind w:left="420" w:hanging="420" w:firstLineChars="0"/>`, `<w:rPr><w:sz w:val="21"/><w:szCs w:val="21"/></w:rPr>`, false) +
		style("Hint", "Writing Hint", "Normal", `<w:ind w:firstLineChars="0" w:firstLine="0"/><w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/>`, `<w:rPr><w:i/><w:color w:val="808080"/><w:sz w:val="21"/><w:szCs w:val="21"/></w:rPr>`, false) +
		`</w:styles>`
	footer := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/><w:ind w:firstLineChars="0" w:firstLine="0"/></w:pPr>` +
		`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>1</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p></w:ftr>`
	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`},
		{"docProps/core.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + xmlText(title) + `</dc:title><dc:creator></dc:creator></cp:coreProperties>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/></Relationships>`},
		{"word/document.xml", docXML},
		{"word/styles.xml", styles},
		{"word/footer1.xml", footer},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		fw, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		fw.Write([]byte(f.body))
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func firstIndent(on bool) string {
	if on {
		return `<w:ind w:firstLineChars="200" w:firstLine="480"/><w:jc w:val="both"/>`
	}
	return `<w:jc w:val="both"/>`
}

func fontRun(font string, sz int, bold bool) string {
	b := ""
	if bold {
		b = "<w:b/><w:bCs/>"
	}
	return `<w:rPr><w:rFonts w:ascii="` + font + `" w:hAnsi="` + font + `" w:eastAsia="` + font + `"/>` + b + `<w:sz w:val="` + itoa(sz) + `"/><w:szCs w:val="` + itoa(sz) + `"/></w:rPr>`
}

func style(id, name, basedOn, pPr, rPr string, def bool) string {
	s := `<w:style w:type="paragraph" w:styleId="` + id + `"`
	if def {
		s += ` w:default="1"`
	}
	s += `><w:name w:val="` + name + `"/>`
	if basedOn != "" {
		s += `<w:basedOn w:val="` + basedOn + `"/><w:qFormat/>`
	} else {
		s += `<w:qFormat/>`
	}
	if pPr != "" {
		s += "<w:pPr>" + pPr + "</w:pPr>"
	}
	return s + rPr + `</w:style>`
}

// ---------------- LaTeX 模板 ----------------

func texEscape(s string) string {
	r := strings.NewReplacer(`\`, `\textbackslash{}`, "%", `\%`, "&", `\&`, "#", `\#`, "_", `\_`, "$", `\$`, "{", `\{`, "}", `\}`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`)
	return r.Replace(s)
}

func texTemplate(p *WProfile) string {
	en := p.Limits.Lang == "en"
	var b strings.Builder
	c := func(s string) { b.WriteString("% " + s + "\n") }
	c(p.Name + " 模板（CanDo 可为生成）")
	c("依据：" + p.Basis + "。" + p.Note + "。")
	c("编译：中文请用 XeLaTeX（TeXstudio：选项 → 设置 → 构建 → 默认编译器选 XeLaTeX）。以 % 开头的行是写作提示，不会出现在 PDF 里。")
	if en {
		b.WriteString("\\documentclass[12pt]{article}\n\\usepackage[a4paper,margin=2.5cm]{geometry}\n\\usepackage{graphicx,booktabs,amsmath,siunitx}\n\\usepackage{setspace}\\onehalfspacing\n\n")
		b.WriteString("\\title{Title of the paper}\n\\author{Author One\\textsuperscript{1}, Author Two\\textsuperscript{2}}\n\\date{}\n\n\\begin{document}\n\\maketitle\n\n")
	} else {
		b.WriteString("\\documentclass[UTF8,zihao=-4]{ctexart}\n\\usepackage[a4paper,margin=2.5cm]{geometry}\n\\usepackage{graphicx,booktabs,amsmath}\n\\usepackage{setspace}\\onehalfspacing\n")
		if p.Limits.ChapterNumber {
			c("学位论文按章编号：图、表、公式编号带章号（如 图 1-1）")
			b.WriteString("\\numberwithin{equation}{section}\\numberwithin{figure}{section}\\numberwithin{table}{section}\n")
		}
		if p.Key == "mcm" {
			c("数学建模国赛：全文不能出现学校、队员姓名和赛区信息；页码从摘要页开始，页脚居中")
			b.WriteString("\\ctexset{section={format=\\zihao{4}\\heiti\\centering, number=\\chinese{section}, aftername=、}}\n")
		}
		b.WriteString("\n\\title{论文题目}\n")
		if p.Limits.NoIdentity {
			b.WriteString("\\author{}\n")
		} else {
			b.WriteString("\\author{作者}\n")
		}
		b.WriteString("\\date{}\n\n\\begin{document}\n\\maketitle\n\n")
	}
	fig := false
	for _, s := range p.Sections {
		switch s.Name {
		case "题目", "题名", "项目名称", "Title":
			continue
		case "关键词":
			if !en {
				b.WriteString("\\noindent\\textbf{关键词：}关键词1；关键词2；关键词3\n")
				c(s.What)
				b.WriteString("\n")
				if p.Key == "mcm" {
					b.WriteString("\\newpage\n\n")
				}
			}
			continue
		case "目录":
			b.WriteString("\\tableofcontents\n\\newpage\n\n")
			continue
		}
		hint := strings.ReplaceAll(sectionHint(s, en), "\n", " ")
		switch {
		case s.Name == "摘要" || strings.HasPrefix(s.Name, "Summary") || (s.Name == "Abstract" && en):
			b.WriteString("\\begin{abstract}\n")
			c(hint)
			if en {
				b.WriteString("Write the abstract here.\n\\end{abstract}\n\n")
			} else {
				b.WriteString("在这里写摘要。\n\\end{abstract}\n\n")
			}
			continue
		case s.Name == "Abstract":
			b.WriteString("\\section*{Abstract}\n")
			c(hint)
			b.WriteString("Write the English abstract here.\n\n\\noindent\\textbf{Key words:} keyword1; keyword2; keyword3\n\n")
			continue
		case s.Name == "参考文献" || s.Name == "References":
			c(hint)
			if en {
				c("Numbered in order of first citation. You can use BibTeX with \\bibliographystyle{naturemag} if available.")
				b.WriteString("\\begin{thebibliography}{99}\n\\bibitem{ref1} Surname, A. B., Surname, C. D. \\& Surname, E. Title of the article. \\textit{J. Abbrev.} \\textbf{12}, 345--356 (2024).\n\\end{thebibliography}\n\n")
			} else {
				c("推荐用 BibTeX + gbt7714 宏包自动生成 GB/T 7714 格式：\\usepackage{gbt7714} 并 \\bibliographystyle{gbt7714-numerical}，\\bibliography{refs}。下面是手写格式示例。")
				b.WriteString("\\begin{thebibliography}{99}\n\\bibitem{ref1} 作者1, 作者2, 作者3, 等. 文章题名[J]. 刊名, 年, 卷(期): 起始页-结束页.\n\\bibitem{ref2} 作者. 书名[M]. 出版地: 出版者, 出版年.\n\\end{thebibliography}\n\n")
			}
			continue
		}
		if unnumbered[s.Name] {
			b.WriteString("\\section*{" + texEscape(s.Name) + "}\n")
			if s.Name != "Figure legends" {
				b.WriteString("\\addcontentsline{toc}{section}{" + texEscape(s.Name) + "}\n")
			}
		} else {
			b.WriteString("\\section{" + texEscape(s.Name) + "}\n")
		}
		c(hint)
		if s.Name == "附录" && p.Limits.NeedCode {
			c("附录1 支撑材料文件列表；附录2 全部完整、可运行的源程序（可用 listings 宏包排版代码）")
		}
		if isResultsSection(s.Name) && !fig {
			fig = true
			if en {
				b.WriteString("As shown in Fig.~\\ref{fig:1} and Table~\\ref{tab:1}, ...\n\n")
				b.WriteString("\\begin{figure}[htbp]\n  \\centering\n  % \\includegraphics[width=0.8\\linewidth]{figure1.pdf}\n  \\fbox{\\parbox{0.6\\linewidth}{\\centering Figure placeholder}}\n  \\caption{Figure title. Legend.}\\label{fig:1}\n\\end{figure}\n\n")
				b.WriteString("\\begin{table}[htbp]\n  \\centering\n  \\caption{Table title}\\label{tab:1}\n  \\begin{tabular}{ccc}\n    \\toprule\n    Group & $n$ & Value \\\\\n    \\midrule\n    A & 10 & 1.23 \\\\\n    B & 10 & 2.34 \\\\\n    \\bottomrule\n  \\end{tabular}\n\\end{table}\n\n")
			} else {
				b.WriteString("如图~\\ref{fig:1} 和表~\\ref{tab:1} 所示，……\n\n")
				b.WriteString("\\begin{figure}[htbp]\n  \\centering\n  % \\includegraphics[width=0.8\\linewidth]{figure1.pdf}\n  \\fbox{\\parbox{0.6\\linewidth}{\\centering 在这里放图}}\n  \\caption{图题（图题在图的下方）}\\label{fig:1}\n\\end{figure}\n\n")
				b.WriteString("\\begin{table}[htbp]\n  \\centering\n  \\caption{表题（表题在表的上方，推荐三线表）}\\label{tab:1}\n  \\begin{tabular}{ccc}\n    \\toprule\n    组别 & 样本量 & 结果 \\\\\n    \\midrule\n    A & 10 & 1.23 \\\\\n    B & 10 & 2.34 \\\\\n    \\bottomrule\n  \\end{tabular}\n\\end{table}\n\n")
			}
		} else {
			b.WriteString("\n")
		}
	}
	if en {
		for _, st := range p.Limits.Statements {
			found := false
			for _, s := range p.Sections {
				if strings.EqualFold(s.Name, st) {
					found = true
				}
			}
			if !found {
				b.WriteString("\\section*{" + st + "}\n\n")
			}
		}
	}
	b.WriteString("\\end{document}\n")
	return b.String()
}

// simpleDocx 生成一份普通文档（AI 起草稿导出用）：标题、提示、正文段落、参考文献
func simpleDocx(title string, paras, refs []string, en bool, refStyle string) ([]byte, error) {
	d := &docBuilder{en: en}
	d.b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`)
	if en {
		d.hint("AI-assisted draft generated from the author's own ideas, results and selected papers. Check every statement and reference, fill in the [to be added] parts, and disclose AI assistance as required by your institution or journal. Delete this note before submission.")
	} else {
		d.hint("AI 辅助起草稿：内容依据作者提供的想法、结果和所选论文生成。请逐句核对事实和引用，补全【需补充】的地方，并按学校或期刊要求声明 AI 的使用。定稿前删除本提示。")
	}
	d.para("Heading1", title)
	for _, t := range paras {
		d.para("", t)
	}
	if len(refs) > 0 {
		if en {
			d.para("Heading1", "References")
		} else {
			d.para("Heading1", "参考文献")
		}
		for _, r := range refs {
			d.para("Ref", r)
		}
	}
	d.b.WriteString(`<w:sectPr><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1418" w:right="1418" w:bottom="1418" w:left="1418" w:header="851" w:footer="851" w:gutter="0"/></w:sectPr></w:body></w:document>`)
	return packDocx(d.b.String(), en, title)
}
