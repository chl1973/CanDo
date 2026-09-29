package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 所有论文类型的模板都能生成；设置 KY_DUMP=目录 时写出文件，便于用 LibreOffice / XeLaTeX 人工验证。
func TestWritingTemplates(t *testing.T) {
	dir := os.Getenv("KY_DUMP")
	for _, p := range writingProfiles {
		b, err := docxTemplate(&p)
		if err != nil || len(b) < 1000 {
			t.Fatalf("%s docx: %v", p.Key, err)
		}
		di, err := parseDocx(b)
		if err != nil {
			t.Fatalf("%s 模板自己都读不出来：%v", p.Key, err)
		}
		if !di.StyleHdr || di.PageW != 11906 || di.Margins[0] != 1418 {
			t.Fatalf("%s 模板样式或页面设置不对 %+v", p.Key, di.Margins)
		}
		tex := texTemplate(&p)
		if !strings.Contains(tex, `\begin{document}`) || strings.Count(tex, `\begin{`) != strings.Count(tex, `\end{`) {
			t.Fatalf("%s tex 环境不配对", p.Key)
		}
		if dir != "" {
			os.WriteFile(filepath.Join(dir, p.Key+".docx"), b, 0o644)
			os.WriteFile(filepath.Join(dir, p.Key+".tex"), []byte(tex), 0o644)
		}
	}
}

const sampleThesis = `面向校园的共享单车调度研究
摘要
本文研究校园共享单车的调度问题。我们采集了 30 天的骑行数据，建立了需求预测模型和调度优化模型，求解后调度成本降低 18%，高峰期缺车率从 12% 降到 4%。结果表明该方法在校园场景下可行，可推广到其他园区。
关键词：共享单车；调度；需求预测；整数规划
1 引言
共享单车改变了校园出行方式[1]。已有研究主要关注城市场景[2,3]，对校园的潮汐特点讨论较少,本文针对这一问题展开研究。
2 数据与方法
我们采集了骑行记录[4]，模型如图 1 所示，参数见表 1。
图 1 调度模型框架
表 1 模型参数
3 结果
调度后成本降低 18%。
4 结论
本文方法有效。
参考文献
[1] 张三, 李四. 共享单车的发展[J]. 交通研究, 2020, 12(3): 1-10.
[2] Smith J, Lee K, Wang Y, Chen X. Bike sharing in cities[J]. Transport Reviews, 2019, 39(2): 100-120.
[3] 王五. 城市交通调度[M]. 北京: 科学出版社, 2018.
[4] 赵六。校园出行调查，2021。
[5] 未被引用的文献[J]. 期刊, 2022, 1(1): 1-2.
`

func TestDocCheckText(t *testing.T) {
	di := parseText(sampleThesis)
	items, stats := runDocCheck(di, profileByKey("course"))
	get := func(cat, sub string) *checkItem {
		for i := range items {
			if items[i].Cat == cat && strings.Contains(items[i].Msg+items[i].Where, sub) {
				return &items[i]
			}
		}
		return nil
	}
	if stats["refs"] != 5 {
		t.Fatalf("应识别 5 条参考文献：%v", stats["refs"])
	}
	for _, c := range [][2]string{
		{"参考文献", "[5]"},      // 未被引用
		{"参考文献", "作者超过 3 位"}, // Smith 等 4 位作者没写 et al.
		{"参考文献", "全角标点"},     // [4]
		{"参考文献", "类型标识"},     // [4]
		{"标点", "英文标点"},       // “较少,本文”
		{"关键词", "4 个"},       // ok
		{"结构", "必需的部分都有"},
	} {
		if get(c[0], c[1]) == nil {
			t.Errorf("缺少检查项 %v；结果：%+v", c, items)
		}
	}
	if get("图表", "没有被提到") != nil {
		t.Error("图 1、表 1 都被提到了")
	}
	// 国赛：身份信息、附录代码
	di = parseText("摘要\n本文……\n关键词：模型\n问题重述\n华东理工大学 队员：张三\n参考文献\n[1] 甲. 乙[J]. 丙, 2020, 1(1): 1.\n附录\n说明文字")
	items, _ = runDocCheck(di, profileByKey("mcm"))
	var hasID, hasCode bool
	for _, it := range items {
		if it.Cat == "身份信息" && it.Level == "error" && strings.Contains(it.Where, "华东理工大学") {
			hasID = true
		}
		if it.Cat == "附录" && it.Level == "warn" {
			hasCode = true
		}
	}
	if !hasID || !hasCode {
		t.Fatalf("国赛应报告身份信息和附录缺代码：%+v", items)
	}
	// “大学生”不算身份信息
	di = parseText("摘要\n研究大学生的出行。\n关键词：出行\n参考文献\n附录\nimport numpy as np\nx = np.zeros(3)\nprint(x)\nfor i in range(3):\n    y = i\n")
	items, _ = runDocCheck(di, profileByKey("mcm"))
	for _, it := range items {
		if it.Cat == "身份信息" && it.Level == "error" {
			t.Fatalf("“大学生”不应算身份信息：%v", it)
		}
		if it.Cat == "附录" && it.Level != "ok" {
			t.Fatalf("附录有代码：%v", it)
		}
	}
}

func TestDocCheckTex(t *testing.T) {
	src := `\documentclass{ctexart}
\title{测试论文}
\begin{document}
\maketitle
\begin{abstract}
本文研究了一个问题，得到了结果。
\end{abstract}
关键词：测试；论文；检查
\section{引言}
已有研究\cite{a}。如图~\ref{fig:1}。
\begin{figure}[h]
\caption{放错位置的图题}
\includegraphics{x.png}
\end{figure}
\begin{table}[h]
\begin{tabular}{cc}
a & b \\
\end{tabular}
\caption{放错位置的表题}
\end{table}
\section{结论}
结论\cite{b}。
\begin{thebibliography}{9}
\bibitem{a} 甲. 乙[J]. 丙, 2020, 1(1): 1-2.
\bibitem{b} 丁. 戊[M]. 北京: 出版社, 2019.
\end{thebibliography}
\end{document}`
	di := parseTex(src)
	items, _ := runDocCheck(di, profileByKey("course"))
	var pos bool
	for _, it := range items {
		if it.Cat == "图表" && strings.Contains(it.Msg, "题注位置") && strings.Contains(it.Where, "图题在图的上方") && strings.Contains(it.Where, "表题在表的下方") {
			pos = true
		}
		if it.Cat == "参考文献" && it.Level == "error" {
			t.Fatalf("引用应能对应：%v", it)
		}
	}
	if !pos {
		t.Fatalf("应发现题注位置错误：%+v", items)
	}
}

func TestDocCheckDocxTemplateItself(t *testing.T) {
	// 用 mcm 模板检查自己：页边距 2.5 cm 合格
	p := profileByKey("mcm")
	b, _ := docxTemplate(p)
	di, _ := parseDocx(b)
	items, _ := runDocCheck(di, p)
	for _, it := range items {
		if it.Cat == "页面" && it.Level == "error" {
			t.Fatalf("模板页边距应合格：%v", it)
		}
		if it.Cat == "图表" && strings.Contains(it.Msg, "题注位置") {
			t.Fatalf("模板的题注位置应正确：%v", it)
		}
	}
}
