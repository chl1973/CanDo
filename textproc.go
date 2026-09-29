package main

// 文本切片与检索。
// - Markdown/TXT 由服务端解析；PDF 由浏览器端 pdf.js 逐页提取文字后提交，服务端负责切片与质量判定。
// - 检索：BM25，中文按“二字组 + 单字”切分，不依赖分词库。

import (
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxChunkRunes = 600
const minPageRunes = 10

type Piece struct {
	Text      string
	ParaIndex int
	PageIndex int
	PageLabel string
}

type PDFPage struct {
	PageIndex int    `json:"page_index"`
	PageLabel string `json:"page_label"`
	Text      string `json:"text"`
}

var reBlank = regexp.MustCompile(`\n[ \t\x{3000}]*\n`)

func paragraphs(text string) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var out []string
	for _, p := range reBlank.Split(text, -1) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitLong(s string) []string {
	rs := []rune(s)
	if len(rs) <= maxChunkRunes {
		return []string{s}
	}
	var out []string
	var buf []rune
	for i, r := range rs {
		buf = append(buf, r)
		end := strings.ContainsRune("。！？!?；;", r) || (r == '.' && (i+1 == len(rs) || unicode.IsSpace(rs[i+1])))
		if (end && len(buf) >= maxChunkRunes/2) || len(buf) >= maxChunkRunes {
			out = append(out, strings.TrimSpace(string(buf)))
			buf = nil
		}
	}
	if len(strings.TrimSpace(string(buf))) > 0 {
		out = append(out, strings.TrimSpace(string(buf)))
	}
	return out
}

// DecodeText 支持 UTF-8（含 BOM）；GBK 文本请先另存为 UTF-8。
func DecodeText(b []byte) (string, error) {
	b = []byte(strings.TrimPrefix(string(b), "\uFEFF"))
	if !utf8.Valid(b) {
		return "", errors.New("文件不是 UTF-8 编码。请用记事本“另存为”并选择 UTF-8 编码后再上传")
	}
	return string(b), nil
}

func ParseText(b []byte) (pieces []Piece, status, errMsg string) {
	text, err := DecodeText(b)
	if err != nil {
		return nil, "failed", err.Error()
	}
	n := 0
	for _, p := range paragraphs(text) {
		n++
		for _, part := range splitLong(p) {
			pieces = append(pieces, Piece{Text: part, ParaIndex: n})
		}
	}
	if len(pieces) == 0 {
		return nil, "failed", "文件中没有文字内容"
	}
	return pieces, "ready", ""
}

func isCJK(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }

// mergeLines 把 PDF 碎行合并成段落：短行且无句末标点视为标题。
func mergeLines(lines []string) []string {
	var out []string
	buf := ""
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			if buf != "" {
				out = append(out, buf)
				buf = ""
			}
			continue
		}
		rs := []rune(ln)
		last := rs[len(rs)-1]
		if buf == "" && len(rs) <= 20 && !strings.ContainsRune("。.！？!?，,；;：:", last) {
			out = append(out, ln)
			continue
		}
		if buf == "" {
			buf = ln
		} else {
			br := []rune(buf)
			if isCJK(br[len(br)-1]) || isCJK(rs[0]) {
				buf += ln
			} else {
				buf += " " + ln
			}
		}
		if len([]rune(buf)) >= 300 || strings.ContainsRune("。！？!?", last) {
			out = append(out, buf)
			buf = ""
		}
	}
	if buf != "" {
		out = append(out, buf)
	}
	return out
}

func ParsePDFPages(pages []PDFPage) (pieces []Piece, status, errMsg, note string) {
	var empty []int
	for _, pg := range pages {
		txt := strings.TrimSpace(pg.Text)
		if len([]rune(strings.Join(strings.Fields(txt), ""))) < minPageRunes {
			empty = append(empty, pg.PageIndex)
			continue
		}
		paras := paragraphs(txt)
		if len(paras) <= 1 {
			paras = mergeLines(strings.Split(txt, "\n"))
		}
		for i, p := range paras {
			for _, part := range splitLong(p) {
				pieces = append(pieces, Piece{Text: part, ParaIndex: i + 1, PageIndex: pg.PageIndex, PageLabel: pg.PageLabel})
			}
		}
	}
	limit := "PDF 文字提取可能丢失公式、表格结构和特殊符号，请以原文件为准。"
	if len(pieces) == 0 {
		return nil, "failed", "共 " + itoa(len(pages)) + " 页均未提取到文字，可能是扫描件或图片 PDF。可以点“OCR 识别”用 AI 识别文字。", ""
	}
	if len(empty) > 0 {
		return pieces, "partial", "", "第 " + joinInts(empty, 10) + " 页（文件页序号）未提取到文字，这些页不参与检索。" + limit
	}
	return pieces, "ready", "", limit
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func joinInts(xs []int, max int) string {
	var parts []string
	for i, x := range xs {
		if i >= max {
			return strings.Join(parts, "、") + " 等 " + itoa(len(xs))
		}
		parts = append(parts, itoa(x))
	}
	return strings.Join(parts, "、")
}

// ---------------- 检索 ----------------

var reToken = regexp.MustCompile(`[a-zA-Z]+|\d+(?:\.\d+)?|[\x{4e00}-\x{9fff}]+`)
var stopChars = "的了是在和与及或则为有这那就也都而且一个中对以"

func Tokenize(text string) []string {
	var toks []string
	for _, m := range reToken.FindAllString(strings.ToLower(text), -1) {
		r, _ := utf8.DecodeRuneInString(m)
		if isCJK(r) {
			rs := []rune(m)
			for i := 0; i+1 < len(rs); i++ {
				toks = append(toks, string(rs[i:i+2]))
			}
			for _, c := range rs {
				if !strings.ContainsRune(stopChars, c) {
					toks = append(toks, string(c))
				}
			}
		} else {
			toks = append(toks, m)
		}
	}
	return toks
}

type Hit struct {
	ChunkID    string  `json:"chunk_id"`
	MaterialID string  `json:"material_id"`
	Title      string  `json:"title"`
	Version    int     `json:"version"`
	Filename   string  `json:"filename"`
	Ftype      string  `json:"ftype"`
	PageIndex  int     `json:"page_index"`
	PageLabel  string  `json:"page_label"`
	ParaIndex  int     `json:"para_index"`
	Location   string  `json:"location"`
	Text       string  `json:"text"`
	Score      float64 `json:"score,omitempty"`
	Context    bool    `json:"context,omitempty"`
	Deleted    bool    `json:"deleted,omitempty"`
	Message    string  `json:"message,omitempty"`
	seq        int
}

func LocationText(page int, label string, para int) string {
	if page > 0 {
		s := "文件第 " + itoa(page) + " 页"
		if label != "" {
			s += "（印刷页码 " + label + "）"
		}
		return s + " 第 " + itoa(para) + " 段"
	}
	return "第 " + itoa(para) + " 段"
}

func makeHit(m *Material, c Chunk) Hit {
	return Hit{ChunkID: c.ID, MaterialID: m.ID, Title: m.Title, Version: m.Version, Filename: m.Filename, Ftype: m.Ftype,
		PageIndex: c.PageIndex, PageLabel: c.PageLabel, ParaIndex: c.ParaIndex,
		Location: chunkLocation(c), Text: c.Text, seq: c.Seq}
}

type MatChunks struct {
	M      *Material // 快照
	Chunks []Chunk
}

// Search 在给定材料（调用方已完成权限校验）中检索，并补充命中片段的前后相邻片段（适用条件常写在相邻段落）。
func Search(mats []MatChunks, query string, topK int) []Hit {
	q := map[string]int{}
	for _, t := range Tokenize(query) {
		q[t]++
	}
	if len(q) == 0 {
		return nil
	}
	type doc struct {
		mi, ci int
		tf     map[string]int
		dl     int
	}
	var docs []doc
	df := map[string]int{}
	total := 0
	for mi, mc := range mats {
		for ci, c := range mc.Chunks {
			tf := map[string]int{}
			toks := Tokenize(c.Text)
			for _, t := range toks {
				tf[t]++
			}
			for t := range tf {
				df[t]++
			}
			docs = append(docs, doc{mi, ci, tf, len(toks)})
			total += len(toks)
		}
	}
	if len(docs) == 0 {
		return nil
	}
	N := float64(len(docs))
	avg := float64(total) / N
	if avg == 0 {
		avg = 1
	}
	k1, b := 1.5, 0.75
	type scored struct {
		s float64
		d doc
	}
	var ss []scored
	for _, d := range docs {
		s := 0.0
		for t := range q {
			f := float64(d.tf[t])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (N-float64(df[t])+0.5)/(float64(df[t])+0.5))
			s += idf * f * (k1 + 1) / (f + k1*(1-b+b*float64(d.dl)/avg))
		}
		if s > 0 {
			ss = append(ss, scored{s, d})
		}
	}
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].s > ss[j].s })
	if len(ss) > topK {
		ss = ss[:topK]
	}
	chosen := map[string]bool{}
	var out []Hit
	for _, x := range ss {
		mc := mats[x.d.mi]
		h := makeHit(mc.M, mc.Chunks[x.d.ci])
		h.Score = math.Round(x.s*1000) / 1000
		chosen[h.ChunkID] = true
		out = append(out, h)
	}
	for _, x := range ss {
		mc := mats[x.d.mi]
		for _, d := range []int{-1, 1} {
			j := x.d.ci + d
			if j < 0 || j >= len(mc.Chunks) || chosen[mc.Chunks[j].ID] {
				continue
			}
			chosen[mc.Chunks[j].ID] = true
			h := makeHit(mc.M, mc.Chunks[j])
			h.Context = true
			out = append(out, h)
		}
	}
	return out
}

// Similarity 用字符二元组 Dice 系数比较两个标题（忽略标点与大小写）。
func Similarity(a, b string) float64 {
	norm := func(s string) []rune {
		var rs []rune
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				rs = append(rs, r)
			}
		}
		return rs
	}
	ra, rb := norm(a), norm(b)
	if len(ra) < 2 || len(rb) < 2 {
		if string(ra) == string(rb) && len(ra) > 0 {
			return 1
		}
		return 0
	}
	grams := func(rs []rune) map[string]int {
		m := map[string]int{}
		for i := 0; i+1 < len(rs); i++ {
			m[string(rs[i:i+2])]++
		}
		return m
	}
	ga, gb := grams(ra), grams(rb)
	inter := 0
	for g, n := range ga {
		if m := gb[g]; m > 0 {
			if m < n {
				inter += m
			} else {
				inter += n
			}
		}
	}
	return 2 * float64(inter) / float64(len(ra)-1+len(rb)-1)
}

var reNum = regexp.MustCompile(`\d+(?:\.\d+)?`)

func numbers(s string) map[string]bool {
	out := map[string]bool{}
	for _, n := range reNum.FindAllString(s, -1) {
		if strings.Contains(n, ".") {
			n = strings.TrimRight(strings.TrimRight(n, "0"), ".")
		}
		out[n] = true
	}
	return out
}

func chunkLocation(c Chunk) string {
	l := LocationText(c.PageIndex, c.PageLabel, c.ParaIndex)
	if c.OCR {
		l += "（OCR 识别）"
	}
	return l
}
