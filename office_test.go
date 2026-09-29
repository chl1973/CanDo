package main

import (
	"os"
	"strings"
	"testing"
)

func TestOfficeRead(t *testing.T) {
	b, _ := os.ReadFile("testdata/office/report.docx")
	d, err := parseDocxDoc(b)
	if err != nil {
		t.Fatal(err)
	}
	out := d.outline(0, 400, 1<<20)
	for _, want := range []string{"[0] (标题 1) 短视频与大学生注意力", "[1] (正文) 摘要：本文研究", "表格 1：2 行 × 3 列；第一行：组别 | 人数 | 专注时长", "［含图片］", "(题注) 图 1 实验流程", "1 张图片"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Word 大纲缺少 %q：\n%s", want, out)
		}
	}
	xb, _ := os.ReadFile("testdata/office/data.xlsx")
	x, err := xlsxText(xb, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"=== 工作表：数据 ===", "1│姓名\t分数\t通过", "2│张三\t88\tTRUE", "3│李四\t\tFALSE", "=== 工作表：说明 ===", "第二个表"} {
		if !strings.Contains(x, want) {
			t.Fatalf("Excel 缺少 %q：\n%s", want, x)
		}
	}
	pb, _ := os.ReadFile("testdata/office/slides.pptx")
	p, err := pptxText(pb, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, "--- 第 1 页 ---\n开题报告\n研究问题\n研究方法") || !strings.Contains(p, "第二页：结果") {
		t.Fatalf("PPT 文字不对：\n%s", p)
	}
	f, err := docxFormatSummary(b)
	if err != nil || !strings.Contains(f, "1 级标题") || !strings.Contains(f, "图片 1 张") {
		t.Fatalf("格式分析不对：%v\n%s", err, f)
	}
}

func TestDocxEdit(t *testing.T) {
	b, _ := os.ReadFile("testdata/office/report.docx")
	d, _ := parseDocxDoc(b)
	img := -1
	for i, p := range d.paras {
		if p.Obj != "" {
			img = i
		}
	}
	if _, _, err := d.applyEdits([]docxEdit{{Op: "replace", Index: img, Text: "x"}}); err == nil || !strings.Contains(err.Error(), "图片") {
		t.Fatal("含图片的段落不能用文字替换")
	}
	if _, _, err := d.applyEdits([]docxEdit{{Op: "replace", Index: 99, Text: "x"}}); err == nil {
		t.Fatal("段落编号越界应报错")
	}
	out, diff, err := d.applyEdits([]docxEdit{
		{Op: "replace", Index: 1, Text: "摘要：本文研究短视频<使用>时长 & 注意力。"},
		{Op: "insert_after", Index: 3, Text: "新增的一段。\n第二行"},
		{Op: "delete", Index: 4},
		{Op: "insert_after", Index: -1, Text: "最前面"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 5 {
		t.Fatalf("改动说明不对：%v", diff)
	}
	d2, err := parseDocxDoc(out)
	if err != nil {
		t.Fatal(err)
	}
	texts := []string{}
	for _, p := range d2.paras {
		texts = append(texts, p.Text)
	}
	j := strings.Join(texts, "|")
	if !strings.HasPrefix(j, "最前面|短视频与大学生注意力|摘要：本文研究短视频<使用>时长 & 注意力。|一、引言|短视频平台依靠算法推荐延长使用时间。|新增的一段。\n第二行|") || strings.Contains(j, "第二段") {
		t.Fatalf("修改结果不对：%s", j)
	}
	if len(d2.images) != 1 || len(d2.tables) != 1 {
		t.Fatal("图片和表格应保持不变")
	}
	// 保留格式：替换后的段落沿用第一段文字的加粗格式
	if !strings.Contains(d2.xml[d2.paras[2].Start:d2.paras[2].End], "<w:b/>") {
		t.Fatalf("替换后应保留原来的文字格式：%s", d2.xml[d2.paras[2].Start:d2.paras[2].End])
	}
	if p := os.Getenv("DOCX_OUT"); p != "" {
		os.WriteFile(p, out, 0o644)
	}
}

func TestMarkdownDocx(t *testing.T) {
	md := "# 实验报告\n\n<h1 style=\"text-align: center;\">居中标题</h1>\n这是**加粗**和*斜体*，还有<span style=\"font-family: 楷体; color: red; font-size: 小四\">红色楷体</span>。\n\n- 第一条\n- 第二条\n1. 步骤一\n\n| 组别 | 人数 |\n|---|---|\n| 干预组 | 32 |\n\n<p style=\"text-align: right;\">2026 年 9 月</p>\n\\newpage\n## 附录\n```\ncode line\n```"
	out, n, err := markdownDocx(md, "实验报告", false)
	if err != nil {
		t.Fatal(err)
	}
	d, err := parseDocxDoc(out)
	if err != nil {
		t.Fatal(err)
	}
	x := d.xml
	for _, want := range []string{`<w:pStyle w:val="Heading1"/>`, `<w:jc w:val="center"/>`, `<w:jc w:val="right"/>`, `w:eastAsia="楷体"`, `<w:color w:val="FF0000"/>`, `<w:sz w:val="24"/>`, "<w:i/>", `<w:br w:type="page"/>`, "干预组", "• 第一条", "Consolas"} {
		if !strings.Contains(x, want) {
			t.Fatalf("生成的 Word 缺少 %q", want)
		}
	}
	if n < 8 || len(d.tables) != 1 {
		t.Fatalf("段落或表格数量不对：%d %d", n, len(d.tables))
	}
	if strings.Contains(x, "<span") || strings.Contains(x, "**") {
		t.Fatal("不应残留标记")
	}
}
