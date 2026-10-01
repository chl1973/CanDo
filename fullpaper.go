package main

// 全文组装（1.15）：把已经起草、确认过的各节草稿拼成一篇完整的论文，导出 Word / LaTeX / PDF。
//
// 和其他写作功能一样遵守“模型提议、规则把关、人来确认”：
//   · 拼装不调用模型：题目和关键词由作者自己填，正文逐字来自各节草稿，参考文献按全文首次引用的顺序统一编号；
//   · 由代码检查：必需的章节都有草稿；各节都已起草并做过“导出前确认”；按同一份契约、同一个版本写；
//     契约章节地图里的每一节都写了；每条核心主张的依据在全文中至少被引用一次；契约承认的局限有交代；
//     承认因果未证实时不用因果措辞；还剩多少【需补充】和没有依据的句子；参考文献条数和信息是否完整；
//     再用“格式检查”同一套规则检查生成的 Word；
//   · 作者确认：有“错误”时不能导出；“提醒”要在导出前逐项勾选确认；拼装内容变了（换了草稿、改了题目、
//     某一节重新起草或重新确认）就要重新确认。导出的文件开头带“AI 辅助”提示。

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const fullPaperMax = 30 // 每人保留的全文数

type WPart struct {
	Section string `json:"section"`
	DraftID string `json:"draft_id"`
	Skip    bool   `json:"skip,omitempty"` // 非必需的章节：作者选择不写
}

type paperConfirm struct {
	At  time.Time `json:"at"`
	Sig string    `json:"sig"`
}

type WPaper struct {
	ID         string        `json:"id"`
	OwnerID    int           `json:"owner_id"`
	Profile    string        `json:"profile"`
	Lang       string        `json:"lang"`
	Title      string        `json:"title"`
	Keywords   []string      `json:"keywords"`
	ContractID string        `json:"contract_id,omitempty"`
	Parts      []WPart       `json:"parts"`
	Confirm    *paperConfirm `json:"confirm,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// ---------------- 拼装 ----------------

type asmSent struct {
	Text string `json:"text"`
	Nums []int  `json:"nums,omitempty"`
	Gap  bool   `json:"gap,omitempty"`
	Flag bool   `json:"flag,omitempty"`
}

type asmSection struct {
	Name     string      `json:"name"`
	Abstract bool        `json:"abstract,omitempty"`
	DraftID  string      `json:"draft_id"`
	Paras    [][]asmSent `json:"paras"`
}

type assembled struct {
	p        *WProfile
	paper    *WPaper
	contract *PaperContract
	drafts   map[string]*WDraft
	Secs     []asmSection
	Refs     []string // 不带编号
	refMats  []string
	Checks   []checkItem
	Format   []checkItem
	Gaps     int
	Flags    int
	Sig      string
}

func isAbstractSection(n string) bool {
	n = strings.TrimSpace(n)
	return n == "摘要" || n == "Abstract" || strings.HasPrefix(n, "Summary")
}

// markCite 把引用编号放在句末标点之前：……结果[1,2]。
func markCite(t string, nums []int, open, close string) string {
	if len(nums) == 0 {
		return t
	}
	var parts []string
	for _, n := range nums {
		parts = append(parts, itoa(n))
	}
	mark := open + strings.Join(parts, ",") + close
	if r, size := utf8.DecodeLastRuneInString(t); strings.ContainsRune("。.；;！!？?", r) {
		return t[:len(t)-size] + mark + string(r)
	}
	return t + mark
}

func (asm *assembled) add(level, cat, msg, where, fix string) {
	asm.Checks = append(asm.Checks, checkItem{Level: level, Cat: cat, Msg: msg, Where: where, Fix: fix})
}

// fullSections 一篇全文里可以由草稿组成的章节（去掉题目、关键词、参考文献、附录等）
func fullSections(p *WProfile) []WSection {
	var out []WSection
	for _, s := range p.Sections {
		if !SKIPSection(s.Name) {
			out = append(out, s)
		}
	}
	return out
}

// normalizeParts 按论文类型的章节顺序整理（作者传来的顺序和多余的章节都不采用）
func normalizeParts(p *WProfile, in []WPart) []WPart {
	have := map[string]WPart{}
	for _, x := range in {
		have[x.Section] = x
	}
	var out []WPart
	for _, s := range fullSections(p) {
		x := have[s.Name]
		x.Section = s.Name
		if s.Must {
			x.Skip = false
		}
		if x.Skip {
			x.DraftID = ""
		}
		out = append(out, x)
	}
	return out
}

func (a *App) assemble(paper *WPaper) *assembled {
	p := profileByKey(paper.Profile)
	if p == nil {
		p = &writingProfiles[0]
	}
	en := paper.Lang == "en"
	asm := &assembled{p: p, paper: paper, drafts: map[string]*WDraft{}}
	matTitle := map[string]*Material{}
	a.store.View(func(db *DB) {
		for _, x := range paper.Parts {
			if x.DraftID == "" {
				continue
			}
			for _, d := range db.Drafts {
				if d.ID == x.DraftID && d.OwnerID == paper.OwnerID {
					cp := *d
					asm.drafts[d.ID] = &cp
				}
			}
		}
		if paper.ContractID != "" {
			for _, c := range db.Contracts {
				if c.ID == paper.ContractID && c.OwnerID == paper.OwnerID {
					cp := *c
					asm.contract = &cp
				}
			}
		}
		for _, d := range asm.drafts {
			for _, f := range d.Frags {
				if _, ok := matTitle[f.MaterialID]; !ok {
					matTitle[f.MaterialID] = db.Material(f.MaterialID)
				}
			}
		}
	})

	// ---- 题目、关键词 ----
	title := strings.TrimSpace(paper.Title)
	if title == "" {
		asm.add("error", "题目", "还没有填论文题目", "", "在上方填写题目（题目由你自己定，AI 不代拟）")
	} else if n := utf8.RuneCountInString(title); p.Limits.TitleChars > 0 && n > p.Limits.TitleChars {
		asm.add("warn", "题目", "题目有 "+itoa(n)+" 个字符，超过建议的 "+itoa(p.Limits.TitleChars)+" 个", title, "精简题目，去掉“浅谈”“研究”这类空词")
	} else if w := len(reWordsEn.FindAllString(title, -1)); p.Limits.TitleWords > 0 && w > p.Limits.TitleWords {
		asm.add("warn", "题目", "题目有 "+itoa(w)+" 个词，超过 "+itoa(p.Limits.TitleWords)+" 个", title, "精简题目")
	}
	needKw := false
	for _, s := range p.Sections {
		if s.Name == "关键词" && s.Must {
			needKw = true
		}
	}
	if needKw || p.Limits.KwMin > 0 {
		k := len(paper.Keywords)
		switch {
		case k == 0 && needKw:
			asm.add("error", "关键词", "还没有填关键词", "", "在上方填写 "+itoa(max(p.Limits.KwMin, 3))+" 个左右关键词")
		case p.Limits.KwMin > 0 && k > 0 && k < p.Limits.KwMin:
			asm.add("warn", "关键词", "只有 "+itoa(k)+" 个关键词，建议至少 "+itoa(p.Limits.KwMin)+" 个", strings.Join(paper.Keywords, "；"), "")
		case p.Limits.KwMax > 0 && k > p.Limits.KwMax:
			asm.add("warn", "关键词", itoa(k)+" 个关键词，超过 "+itoa(p.Limits.KwMax)+" 个", strings.Join(paper.Keywords, "；"), "")
		}
	}

	// ---- 契约 ----
	ct := asm.contract
	if paper.ContractID != "" {
		switch {
		case ct == nil:
			asm.add("error", "论文契约", "找不到这篇全文对应的论文契约（可能已删除）", "", "重新选择契约，或改成不使用契约")
		case ct.Status != "confirmed":
			asm.add("warn", "论文契约", "论文契约「"+ct.Title+"」已解锁修改，还没有重新确认", "", "到“论文契约”里确认后再导出，这样各节都能对照最新的契约")
		}
	}

	// ---- 各节 ----
	used := map[string]string{}
	fragMat := map[string]map[string]string{} // draftID → fragID → materialID
	refNum := map[string]int{}
	for _, part := range paper.Parts {
		sec := WSection{Name: part.Section}
		for _, s := range p.Sections {
			if s.Name == part.Section {
				sec = s
			}
		}
		if part.DraftID == "" {
			switch {
			case sec.Must:
				asm.add("error", "章节", "“"+sec.Name+"”是必需的章节，还没有选草稿", "", "到“AI 起草”里起草这一节，再回来选上")
			case ct != nil && contractHasSection(ct, sec.Name):
				asm.add("warn", "章节", "契约的章节地图里有“"+sec.Name+"”，全文里没有这一节", "", "起草这一节，或者修改契约")
			}
			continue
		}
		d := asm.drafts[part.DraftID]
		if d == nil {
			asm.add("error", "章节", "“"+sec.Name+"”选的草稿已被删除", "", "重新选一份草稿")
			continue
		}
		if prev, dup := used[d.ID]; dup {
			asm.add("error", "章节", "同一份草稿同时用在“"+prev+"”和“"+sec.Name+"”", "", "每一节选各自的草稿")
			continue
		}
		used[d.ID] = sec.Name
		if d.Stage != "draft" {
			asm.add("error", "章节", "“"+sec.Name+"”选的还是论证骨架，没有起草成文", "", "到“AI 起草”里点“按骨架起草”")
			continue
		}
		if d.Profile != paper.Profile || (d.Lang == "en") != en || !sameSection(d.Section, sec.Name) {
			asm.add("error", "章节", "“"+sec.Name+"”选的草稿是别的论文类型、语言或章节（"+d.Section+"）", "", "重新选择")
			continue
		}
		if d.Confirm == nil {
			asm.add("error", "导出前确认", "“"+sec.Name+"”还没有做导出前确认", "", "到“AI 起草”里打开这份草稿，点“下载 Word”或“复制正文”，逐项确认")
		}
		if ct != nil {
			switch {
			case d.ContractID != ct.ID:
				asm.add("warn", "论文契约", "“"+sec.Name+"”不是按契约「"+ct.Title+"」写的，没有对照这份契约检查过", "", "按契约重新起草这一节，或者确认内容没有超出契约")
			case d.ContractVer < ct.Version:
				asm.add("warn", "论文契约", "“"+sec.Name+"”是按契约 v"+itoa(d.ContractVer)+" 写的，契约已经更新到 v"+itoa(ct.Version), "", "按新契约重新起草，或重新做“对照契约检查”")
			}
		}
		for _, it := range d.Drift {
			if it.Level == "error" {
				asm.add("warn", "对照契约检查", "“"+sec.Name+"”的对照契约检查里还有严重问题："+it.Cat, it.Where, it.Fix)
			}
		}
		fm := map[string]string{}
		for _, f := range d.Frags {
			fm[f.ID] = f.MaterialID
		}
		fragMat[d.ID] = fm
		as := asmSection{Name: sec.Name, Abstract: isAbstractSection(sec.Name), DraftID: d.ID}
		for _, ps := range d.Paragraphs {
			var para []asmSent
			for _, s := range ps {
				st := asmSent{Text: strings.TrimSpace(s.Text), Gap: s.Kind == "gap", Flag: s.Kind != "gap" && s.Flag != ""}
				for _, c := range s.Cites {
					mid := fm[c]
					if mid == "" {
						continue
					}
					if refNum[mid] == 0 {
						asm.refMats = append(asm.refMats, mid)
						refNum[mid] = len(asm.refMats)
					}
					if !contains(st.Nums, refNum[mid]) {
						st.Nums = append(st.Nums, refNum[mid])
					}
				}
				sort.Ints(st.Nums)
				if st.Gap {
					asm.Gaps++
				}
				if st.Flag {
					asm.Flags++
				}
				if st.Text != "" {
					para = append(para, st)
				}
			}
			if len(para) > 0 {
				as.Paras = append(as.Paras, para)
			}
		}
		asm.Secs = append(asm.Secs, as)
	}

	// ---- 对照契约（全文层面） ----
	if ct != nil {
		asm.contractChecks(ct)
	}

	// ---- 待补充、无依据 ----
	if asm.Gaps > 0 {
		asm.add("warn", "需补充", "全文还有 "+itoa(asm.Gaps)+" 处【需补充】", "", "补全后再交稿；可以在导出的 Word 里搜索“需补充”")
	}
	if asm.Flags > 0 {
		asm.add("warn", "依据", "全文有 "+itoa(asm.Flags)+" 句没有依据的陈述", "", "补充出处或删改；在“AI 起草”里打开各节可以看到标黄的句子")
	}

	// ---- 参考文献 ----
	style := p.Limits.RefStyle
	missingInfo, deleted := 0, 0
	for _, mid := range asm.refMats {
		m := matTitle[mid]
		if m == nil {
			deleted++
			asm.Refs = append(asm.Refs, "（论文已从论文库删除，请补全这条文献）")
			continue
		}
		r := materialRef(m, style)
		if strings.Contains(r, "待补充") {
			missingInfo++
		}
		asm.Refs = append(asm.Refs, r)
	}
	if deleted > 0 {
		asm.add("warn", "参考文献", itoa(deleted)+" 条参考文献对应的论文已从论文库删除", "", "手动补全这几条")
	}
	if missingInfo > 0 {
		asm.add("warn", "参考文献", itoa(missingInfo)+" 条参考文献的作者、年份或刊名信息不全（标了“待补充”）", "", "在论文库里补全题录，或导出后手动补全；可以用“引用核验”检查文献是否真实存在")
	}
	if p.Limits.RefsMax > 0 && len(asm.Refs) > p.Limits.RefsMax {
		asm.add("warn", "参考文献", "参考文献 "+itoa(len(asm.Refs))+" 条，超过 "+itoa(p.Limits.RefsMax)+" 条", "", "删去次要的引用")
	}
	if len(asm.Refs) == 0 && len(asm.Secs) > 0 {
		asm.add("warn", "参考文献", "全文没有引用任何文献", "", "引言和讨论通常需要引用前人工作")
	}

	// ---- 要作者自己补的部分 ----
	var own []string
	for _, s := range p.Sections {
		if s.Must && SKIPSection(s.Name) && !map[string]bool{"题目": true, "题名": true, "Title": true, "关键词": true, "参考文献": true, "References": true, "目录": true}[s.Name] {
			own = append(own, s.Name)
		}
	}
	if len(own) > 0 {
		asm.add("info", "自己补充", "这些部分 AI 不起草，导出后请自己补上："+strings.Join(own, "、"), "", "")
	}

	// ---- 格式检查（与“格式检查”页同一套规则，检查生成的 Word） ----
	if len(asm.Secs) > 0 {
		if data, err := asm.docx(); err == nil {
			if di, err := parseDocx(data); err == nil {
				items, _ := runDocCheck(di, p)
				for _, it := range items {
					if it.Level == "error" || it.Level == "warn" {
						it.Level = "warn" // 格式规则是按常见写法推断的，作为提醒，不拦导出
						asm.Format = append(asm.Format, it)
					}
				}
			}
		}
	}

	if !asm.hasErrors() && len(asm.Checks) == 0 {
		asm.add("ok", "全文", "没有发现问题", "", "")
	}
	asm.Sig = asm.signature()
	return asm
}

func contractHasSection(c *PaperContract, name string) bool {
	for _, s := range c.Sections {
		if sameSection(s.Name, name) {
			return true
		}
	}
	return false
}

// contractChecks 全文层面对照契约：核心主张的依据、局限、因果措辞（章节地图在逐节检查时已经对照）
func (asm *assembled) contractChecks(ct *PaperContract) {
	// 核心主张：契约给出的依据（N 你的材料 / F 论文原文）在全文中至少被引用一次。
	// 按内容对应（同一条材料原文、同一段论文原文），不靠编号，契约改版或补充检索后也不会对错。
	type evKey struct{ note, chunk string }
	ctEv := map[string]evKey{}
	for _, n := range ct.Notes {
		ctEv[n.ID] = evKey{note: strings.TrimSpace(n.Text)}
	}
	for _, f := range ct.Frags {
		ctEv[f.ID] = evKey{chunk: f.ChunkID}
	}
	cited := map[evKey]bool{}
	for _, s := range asm.Secs {
		d := asm.drafts[s.DraftID]
		if d == nil {
			continue
		}
		noteText, fragChunk := map[string]string{}, map[string]string{}
		for _, n := range d.Notes {
			noteText[n.ID] = strings.TrimSpace(n.Text)
		}
		for _, f := range d.Frags {
			fragChunk[f.ID] = f.ChunkID
		}
		for _, ps := range d.Paragraphs {
			for _, st := range ps {
				for _, c := range st.Cites {
					if t, ok := noteText[c]; ok {
						cited[evKey{note: t}] = true
					}
					if ch, ok := fragChunk[c]; ok && ch != "" {
						cited[evKey{chunk: ch}] = true
					}
				}
			}
		}
	}
	for _, k := range ct.Claims {
		if len(k.Evidence) == 0 {
			asm.add("warn", "核心主张", k.ID+" 在契约里没有依据", clipRunes(k.Text, 80), "补充数据或文献后修改契约，或在论文里把它写成有待验证的推测")
			continue
		}
		ok := false
		for _, id := range k.Evidence {
			if ev, found := ctEv[id]; found && cited[ev] {
				ok = true
			}
		}
		if !ok {
			asm.add("warn", "核心主张", k.ID+" 的依据（"+strings.Join(k.Evidence, "、")+"）在全文中一次也没有被引用，这条主张可能没有写进论文", clipRunes(k.Text, 80), "在结果或讨论里写出这条主张及其依据")
		}
	}

	// 承认的局限要有交代
	if lims := contractLimits(ct); len(lims) > 0 {
		said := false
		for _, s := range asm.Secs {
			if !isLimitSection(s.Name) {
				continue
			}
			var all strings.Builder
			for _, ps := range s.Paras {
				for _, st := range ps {
					all.WriteString(st.Text)
				}
			}
			t := strings.ToLower(all.String())
			if strings.Contains(t, "局限") || strings.Contains(t, "不足") || strings.Contains(t, "limitation") {
				said = true
			}
		}
		if !said {
			asm.add("warn", "局限", "契约里承认了 "+itoa(len(lims))+" 条局限，全文的讨论、结论里都没有交代", clipRunes(strings.Join(lims, "；"), 160), "在讨论或结论里如实说明局限")
		}
	}

	// 契约承认因果未证实：全文不能用因果措辞
	causalLimited := false
	for _, x := range ct.Attacks {
		if (x.Kind == "causality" || x.Kind == "reverse") && (x.Status == "limitation" || x.Status == "open") {
			causalLimited = true
		}
	}
	if causalLimited {
		n := 0
		for _, s := range asm.Secs {
			for _, ps := range s.Paras {
				for _, st := range ps {
					if m := reCausal.FindString(st.Text); m != "" {
						n++
						if n <= 8 {
							asm.add("warn", "说法超出证据", "契约里承认因果关系还没有被证实，“"+s.Name+"”这句用了因果说法“"+m+"”", clipRunes(st.Text, 80), "改成相关性的表述，例如“与……相关”")
						}
					}
				}
			}
		}
		if n > 8 {
			asm.add("warn", "说法超出证据", "另外还有 "+itoa(n-8)+" 句用了因果说法", "", "")
		}
	}
}

func (asm *assembled) hasErrors() bool {
	for _, it := range asm.Checks {
		if it.Level == "error" {
			return true
		}
	}
	return false
}

func (asm *assembled) warnCount() int {
	n := 0
	for _, it := range append(append([]checkItem{}, asm.Checks...), asm.Format...) {
		if it.Level == "warn" {
			n++
		}
	}
	return n
}

// signature 拼装内容的指纹：题目、关键词、契约版本、各节草稿（含其起草、检查、确认的时间）。任何一项变了都要重新确认
func (asm *assembled) signature() string {
	h := sha256.New()
	pp := asm.paper
	h.Write([]byte(pp.Profile + "\x00" + pp.Lang + "\x00" + strings.TrimSpace(pp.Title) + "\x00" + strings.Join(pp.Keywords, "\x01") + "\x00"))
	if asm.contract != nil {
		h.Write([]byte(asm.contract.ID + itoa(asm.contract.Version) + asm.contract.Status))
	}
	for _, x := range pp.Parts {
		h.Write([]byte("\x02" + x.Section + "\x00" + x.DraftID))
		if d := asm.drafts[x.DraftID]; d != nil {
			h.Write([]byte(d.UpdatedAt.UTC().Format(time.RFC3339Nano)))
			if d.Confirm != nil {
				h.Write([]byte(d.Confirm.At.UTC().Format(time.RFC3339Nano)))
			}
			if d.DriftAt != nil {
				h.Write([]byte(d.DriftAt.UTC().Format(time.RFC3339Nano)))
			}
		}
	}
	for _, r := range asm.Refs {
		h.Write([]byte("\x03" + r))
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func (asm *assembled) needs() []string {
	need := []string{"facts", "ai"}
	if asm.warnCount() > 0 {
		need = append(need, "issues")
	}
	return need
}

func (asm *assembled) confirmed() bool {
	c := asm.paper.Confirm
	return c != nil && c.Sig == asm.Sig
}

// ---------------- 生成文件 ----------------

func (asm *assembled) en() bool { return asm.paper.Lang == "en" }

func (asm *assembled) keywordLine() (string, string) {
	if asm.en() {
		return "Keywords: ", strings.Join(asm.paper.Keywords, "; ")
	}
	return "关键词：", strings.Join(asm.paper.Keywords, "；")
}

func (asm *assembled) sectionTitle(s asmSection, n int) string {
	switch {
	case asm.en() || s.Abstract || unnumbered[s.Name]:
		return s.Name
	case asm.p.Key == "mcm":
		return cnNum[(n-1)%len(cnNum)] + "、" + s.Name
	default:
		return itoa(n) + " " + s.Name
	}
}

func (asm *assembled) docx() ([]byte, error) {
	en := asm.en()
	d := &docBuilder{en: en}
	d.b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`)
	if en {
		d.hint("AI-assisted draft assembled by CanDo from section drafts based on the author's own ideas, results and selected papers. Check every statement and reference, fill in the [to be added] parts, add the parts AI does not write (e.g. appendices, statements), and disclose AI assistance as required. Delete this note before submission.")
	} else {
		d.hint("AI 辅助起草的全文：由 CanDo 可为把各节起草稿拼装而成，内容依据作者自己的想法、结果和所选论文。请逐句核对事实和引用，补全【需补充】，补上 AI 不起草的部分（如附录、成果清单），并按学校、竞赛或期刊要求声明 AI 的使用。定稿前删除本提示。")
	}
	title := strings.TrimSpace(asm.paper.Title)
	if title == "" {
		title = map[bool]string{true: "Title", false: "题目"}[en]
	}
	d.para("Title", title, "center")
	kwDone := len(asm.paper.Keywords) == 0
	kw := func() {
		if kwDone {
			return
		}
		kwDone = true
		label, text := asm.keywordLine()
		d.b.WriteString(`<w:p><w:pPr><w:pStyle w:val="NoIndent"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">` + xmlText(label) + `</w:t></w:r><w:r><w:t xml:space="preserve">` + xmlText(text) + `</w:t></w:r></w:p>`)
	}
	n := 0
	for _, s := range asm.Secs {
		if !s.Abstract {
			kw() // 没有摘要时关键词放在题目下面
		}
		opts := []string{}
		if s.Abstract && !en {
			opts = append(opts, "center")
		}
		if !s.Abstract && !unnumbered[s.Name] {
			n++
			if asm.p.Key == "mcm" && n == 1 {
				opts = append(opts, "break")
			}
		}
		d.para("Heading1", asm.sectionTitle(s, n), opts...)
		for _, ps := range s.Paras {
			var b strings.Builder
			for i, st := range ps {
				if en && i > 0 {
					b.WriteString(" ")
				}
				b.WriteString(markCite(st.Text, st.Nums, "[", "]"))
			}
			d.para("", b.String())
		}
		if s.Abstract {
			kw()
		}
	}
	kw()
	if len(asm.Refs) > 0 {
		opts := []string{}
		if asm.p.Key == "thesis" {
			opts = append(opts, "break")
		}
		d.para("Heading1", map[bool]string{true: "References", false: "参考文献"}[en], opts...)
		for i, r := range asm.Refs {
			d.para("Ref", "["+itoa(i+1)+"] "+r)
		}
	}
	d.b.WriteString(`<w:sectPr><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1418" w:right="1418" w:bottom="1418" w:left="1418" w:header="851" w:footer="851" w:gutter="0"/></w:sectPr></w:body></w:document>`)
	return packDocx(d.b.String(), en, title)
}

var reKwSplit = regexp.MustCompile(`[;；,，、\n]+`)

var reTexGap = regexp.MustCompile(`【[^】]*】|\[[Tt]o be added[^\]]*\]`)

func (asm *assembled) tex() string {
	en := asm.en()
	var b strings.Builder
	c := func(s string) { b.WriteString("% " + s + "\n") }
	c("CanDo 可为 · " + asm.p.Name + " · AI 辅助起草的全文（由各节起草稿拼装）")
	c("请逐句核对事实和引用，补全【需补充】，补上 AI 不起草的部分，并按要求声明 AI 的使用。")
	if en {
		b.WriteString("\\documentclass[12pt]{article}\n\\usepackage[a4paper,margin=2.5cm]{geometry}\n\\usepackage{graphicx,booktabs,amsmath,xcolor}\n\\usepackage{setspace}\\onehalfspacing\n")
	} else {
		c("编译：请用 XeLaTeX（TeXstudio：选项 → 设置 → 构建 → 默认编译器选 XeLaTeX）。")
		b.WriteString("\\documentclass[UTF8,zihao=-4]{ctexart}\n\\usepackage[a4paper,margin=2.5cm]{geometry}\n\\usepackage{graphicx,booktabs,amsmath,xcolor}\n\\usepackage{setspace}\\onehalfspacing\n")
		if asm.p.Limits.ChapterNumber {
			b.WriteString("\\numberwithin{equation}{section}\\numberwithin{figure}{section}\\numberwithin{table}{section}\n")
		}
		if asm.p.Key == "mcm" {
			b.WriteString("\\ctexset{section={format=\\zihao{4}\\heiti\\centering, number=\\chinese{section}, aftername=、}}\n")
		}
	}
	c("【需补充】的地方显示为橙色，补全后删掉 \\gap{…}")
	b.WriteString("\\newcommand{\\gap}[1]{\\textcolor{orange}{#1}}\n\n")
	title := strings.TrimSpace(asm.paper.Title)
	b.WriteString("\\title{" + texEscape(title) + "}\n")
	if asm.p.Limits.NoIdentity {
		b.WriteString("\\author{}\n")
	} else if en {
		b.WriteString("\\author{Author}\n")
	} else {
		b.WriteString("\\author{作者}\n")
	}
	b.WriteString("\\date{}\n\n\\begin{document}\n\\maketitle\n\n")
	kwDone := len(asm.paper.Keywords) == 0
	kw := func() {
		if kwDone {
			return
		}
		kwDone = true
		label, text := asm.keywordLine()
		b.WriteString("\\noindent\\textbf{" + texEscape(strings.TrimSpace(label)) + "}" + texEscape(text) + "\n\n")
	}
	para := func(ps []asmSent) {
		for i, st := range ps {
			if en && i > 0 {
				b.WriteString(" ")
			}
			t := texEscape(st.Text)
			if st.Gap {
				t = reTexGap.ReplaceAllStringFunc(t, func(m string) string { return "\\gap{" + m + "}" })
			}
			var keys []int
			keys = append(keys, st.Nums...)
			if len(keys) > 0 {
				var parts []string
				for _, k := range keys {
					parts = append(parts, "r"+itoa(k))
				}
				cite := "~\\cite{" + strings.Join(parts, ",") + "}"
				if r, size := utf8.DecodeLastRuneInString(t); strings.ContainsRune("。.；;！!？?", r) {
					t = t[:len(t)-size] + cite + string(r)
				} else {
					t += cite
				}
			}
			b.WriteString(t)
		}
		b.WriteString("\n\n")
	}
	first := true
	for _, s := range asm.Secs {
		if s.Abstract {
			b.WriteString("\\begin{abstract}\n")
			for _, ps := range s.Paras {
				para(ps)
			}
			b.WriteString("\\end{abstract}\n\n")
			kw()
			if asm.p.Key == "mcm" {
				b.WriteString("\\newpage\n\n")
			}
			continue
		}
		kw()
		if asm.p.Key == "thesis" && first {
			b.WriteString("\\tableofcontents\n\\newpage\n\n")
		}
		first = false
		if unnumbered[s.Name] {
			b.WriteString("\\section*{" + texEscape(s.Name) + "}\n\\addcontentsline{toc}{section}{" + texEscape(s.Name) + "}\n")
		} else {
			b.WriteString("\\section{" + texEscape(s.Name) + "}\n")
		}
		for _, ps := range s.Paras {
			para(ps)
		}
	}
	kw()
	if len(asm.Refs) > 0 {
		if !en {
			b.WriteString("\\renewcommand{\\refname}{参考文献}\n")
		}
		b.WriteString("\\begin{thebibliography}{99}\n")
		for i, r := range asm.Refs {
			b.WriteString("\\bibitem{r" + itoa(i+1) + "} " + texEscape(r) + "\n")
		}
		b.WriteString("\\end{thebibliography}\n\n")
	}
	b.WriteString("\\end{document}\n")
	return b.String()
}

// ---------------- 接口 ----------------

func (a *App) getPaper(me *Me, id string) (*WPaper, error) {
	var p *WPaper
	a.store.View(func(db *DB) {
		for _, x := range db.Papers {
			if x.ID == id && x.OwnerID == me.ID {
				cp := *x
				cp.Parts = append([]WPart{}, x.Parts...)
				cp.Keywords = append([]string{}, x.Keywords...)
				p = &cp
			}
		}
	})
	if p == nil {
		return nil, errNotFound("全文不存在")
	}
	return p, nil
}

func (a *App) savePaper(p *WPaper) error {
	return a.store.Update(func(db *DB) error {
		for i, x := range db.Papers {
			if x.ID == p.ID {
				db.Papers[i] = p
				return nil
			}
		}
		n := 0
		for _, x := range db.Papers {
			if x.OwnerID == p.OwnerID {
				n++
			}
		}
		if n >= fullPaperMax {
			return errBad("最多保留 " + itoa(fullPaperMax) + " 篇全文，请先删除不用的")
		}
		db.Papers = append(db.Papers, p)
		return nil
	})
}

// candidates 每一节可以选的草稿：同一论文类型、语言、章节、已起草；按契约的排前面
func (a *App) candidates(me *Me, paper *WPaper) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	a.store.View(func(db *DB) {
		var ds []*WDraft
		for _, d := range db.Drafts {
			if d.OwnerID == me.ID && d.Profile == paper.Profile && (d.Lang == "en") == (paper.Lang == "en") && d.Stage == "draft" {
				ds = append(ds, d)
			}
		}
		sort.SliceStable(ds, func(i, j int) bool {
			ci, cj := paper.ContractID != "" && ds[i].ContractID == paper.ContractID, paper.ContractID != "" && ds[j].ContractID == paper.ContractID
			if ci != cj {
				return ci
			}
			return ds[i].UpdatedAt.After(ds[j].UpdatedAt)
		})
		for _, s := range fullSections(profileByKey(paper.Profile)) {
			for _, d := range ds {
				if !sameSection(d.Section, s.Name) {
					continue
				}
				label := clipRunes(firstLineOf(d.Idea), 40)
				if label == "" {
					label = "（按契约起草）"
				}
				ct := ""
				if d.ContractID != "" {
					ct = "v" + itoa(d.ContractVer)
					if d.ContractID != paper.ContractID {
						ct = "其他契约"
					}
				}
				out[s.Name] = append(out[s.Name], map[string]any{"id": d.ID, "label": label, "updated_at": d.UpdatedAt, "confirmed": d.Confirm != nil, "contract": ct})
			}
		}
	})
	return out
}

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func (a *App) paperView(me *Me, paper *WPaper) map[string]any {
	asm := a.assemble(paper)
	p := asm.p
	cands := a.candidates(me, paper)
	var parts []map[string]any
	for _, x := range paper.Parts {
		sec := WSection{Name: x.Section}
		for _, s := range p.Sections {
			if s.Name == x.Section {
				sec = s
			}
		}
		v := map[string]any{"section": x.Section, "must": sec.Must, "what": sec.What, "draft_id": x.DraftID, "skip": x.Skip, "candidates": nonNilMA(cands[x.Section])}
		if d := asm.drafts[x.DraftID]; d != nil {
			g, f, dr := draftCounts(d)
			v["draft"] = map[string]any{"id": d.ID, "confirmed": d.Confirm != nil, "gaps": g, "flags": f, "drift": dr, "contract_ver": d.ContractVer, "updated_at": d.UpdatedAt, "stage": d.Stage}
		}
		parts = append(parts, v)
	}
	var ct any
	if asm.contract != nil {
		ct = map[string]any{"id": asm.contract.ID, "title": asm.contract.Title, "version": asm.contract.Version, "status": asm.contract.Status}
	}
	units := 0
	for _, s := range asm.Secs {
		for _, ps := range s.Paras {
			for _, st := range ps {
				units += countUnits(st.Text, paper.Lang == "en")
			}
		}
	}
	return map[string]any{
		"paper": paper, "profile": map[string]any{"key": p.Key, "name": p.Name, "lang": p.Limits.Lang, "kw_min": p.Limits.KwMin, "kw_max": p.Limits.KwMax},
		"parts": parts, "contract": ct, "checks": asm.Checks, "format": nonNilC(asm.Format),
		"sections": nonNilA(asm.Secs), "refs": nonNilS(asm.Refs),
		"stats":      map[string]any{"sections": len(asm.Secs), "refs": len(asm.Refs), "units": units, "unit": map[bool]string{true: "words", false: "字"}[paper.Lang == "en"], "gaps": asm.Gaps, "flags": asm.Flags},
		"errors":     asm.hasErrors(),
		"need":       asm.needs(),
		"confirmed":  asm.confirmed(),
		"can_export": !asm.hasErrors() && asm.confirmed(),
		"warn_count": asm.warnCount(),
		"tex_engine": findTeX(false).Found,
	}
}

func nonNilMA(x []map[string]any) []map[string]any {
	if x == nil {
		return []map[string]any{}
	}
	return x
}
func nonNilC(x []checkItem) []checkItem {
	if x == nil {
		return []checkItem{}
	}
	return x
}
func nonNilA(x []asmSection) []asmSection {
	if x == nil {
		return []asmSection{}
	}
	return x
}

// hFullPapers GET 列表 / ?id= 读取一篇；POST 新建；DELETE ?id= 删除
func (a *App) hFullPapers(w http.ResponseWriter, r *http.Request, me *Me) error {
	switch r.Method {
	case "POST":
		var in struct {
			Profile    string `json:"profile"`
			Lang       string `json:"lang"`
			Title      string `json:"title"`
			ContractID string `json:"contract_id"`
		}
		if err := readJSON(r, &in); err != nil {
			return err
		}
		paper := &WPaper{ID: newID(), OwnerID: me.ID, Profile: in.Profile, Lang: in.Lang, Title: clipRunes(strings.TrimSpace(in.Title), 200), Keywords: []string{}, CreatedAt: now(), UpdatedAt: now()}
		if in.ContractID != "" {
			ct, err := a.getContract(me, in.ContractID)
			if err != nil {
				return err
			}
			paper.ContractID, paper.Profile, paper.Lang = ct.ID, ct.Profile, ct.Lang
			if paper.Title == "" {
				paper.Title = clipRunes(strings.TrimSpace(ct.Title), 200)
			}
		}
		p := profileByKey(paper.Profile)
		if p == nil {
			return errBad("请选择论文类型")
		}
		if paper.Lang != "en" && paper.Lang != "zh" {
			paper.Lang = p.Limits.Lang
		}
		if paper.Lang != "en" {
			paper.Lang = "zh"
		}
		// 每一节自动选上最合适的草稿（按契约的、最近的）；作者可以再换
		paper.Parts = normalizeParts(p, nil)
		cands := a.candidates(me, paper)
		usedD := map[string]bool{}
		for i := range paper.Parts {
			for _, c := range cands[paper.Parts[i].Section] {
				id := c["id"].(string)
				if !usedD[id] {
					paper.Parts[i].DraftID = id
					usedD[id] = true
					break
				}
			}
		}
		if err := a.savePaper(paper); err != nil {
			return err
		}
		writeJSON(w, 200, a.paperView(me, paper))
		return nil
	case "DELETE":
		id := r.URL.Query().Get("id")
		if _, err := a.getPaper(me, id); err != nil {
			return err
		}
		a.store.Update(func(db *DB) error {
			for i, x := range db.Papers {
				if x.ID == id && x.OwnerID == me.ID {
					db.Papers = append(db.Papers[:i], db.Papers[i+1:]...)
					break
				}
			}
			return nil
		})
		writeJSON(w, 200, map[string]any{"ok": true})
		return nil
	}
	if id := r.URL.Query().Get("id"); id != "" {
		paper, err := a.getPaper(me, id)
		if err != nil {
			return err
		}
		writeJSON(w, 200, a.paperView(me, paper))
		return nil
	}
	out := []map[string]any{}
	var ps []*WPaper
	a.store.View(func(db *DB) {
		for _, x := range db.Papers {
			if x.OwnerID == me.ID {
				cp := *x
				ps = append(ps, &cp)
			}
		}
	})
	sort.Slice(ps, func(i, j int) bool { return ps[i].UpdatedAt.After(ps[j].UpdatedAt) })
	for _, x := range ps {
		name := x.Profile
		if p := profileByKey(x.Profile); p != nil {
			name = p.Name
		}
		n := 0
		for _, pt := range x.Parts {
			if pt.DraftID != "" {
				n++
			}
		}
		out = append(out, map[string]any{"id": x.ID, "title": x.Title, "profile": name, "sections": n, "contract": x.ContractID != "", "updated_at": x.UpdatedAt})
	}
	writeJSON(w, 200, out)
	return nil
}

// hFullPaperUpdate 保存题目、关键词和各节选用的草稿
func (a *App) hFullPaperUpdate(w http.ResponseWriter, r *http.Request, me *Me) error {
	paper, err := a.getPaper(me, r.PathValue("id"))
	if err != nil {
		return err
	}
	var in struct {
		Title    *string  `json:"title"`
		Keywords []string `json:"keywords"`
		Parts    []WPart  `json:"parts"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	p := profileByKey(paper.Profile)
	if p == nil {
		return errBad("论文类型不存在")
	}
	if in.Title != nil {
		paper.Title = clipRunes(strings.TrimSpace(*in.Title), 200)
	}
	if in.Keywords != nil {
		var kws []string
		for _, k := range in.Keywords {
			for _, x := range reKwSplit.Split(k, -1) {
				if x = strings.TrimSpace(x); x != "" && len(kws) < 20 && !containsFold(kws, x) {
					kws = append(kws, clipRunes(x, 40))
				}
			}
		}
		paper.Keywords = nonNilS(kws)
	}
	if in.Parts != nil {
		mine := map[string]bool{}
		a.store.View(func(db *DB) {
			for _, d := range db.Drafts {
				if d.OwnerID == me.ID {
					mine[d.ID] = true
				}
			}
		})
		for i := range in.Parts {
			if in.Parts[i].DraftID != "" && !mine[in.Parts[i].DraftID] {
				return errNotFound("草稿不存在")
			}
		}
		paper.Parts = normalizeParts(p, in.Parts)
	}
	paper.UpdatedAt = now()
	if err := a.savePaper(paper); err != nil {
		return err
	}
	writeJSON(w, 200, a.paperView(me, paper))
	return nil
}

// hFullPaperConfirm 导出前确认（全文）
func (a *App) hFullPaperConfirm(w http.ResponseWriter, r *http.Request, me *Me) error {
	paper, err := a.getPaper(me, r.PathValue("id"))
	if err != nil {
		return err
	}
	var in struct {
		Checks map[string]bool `json:"checks"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	asm := a.assemble(paper)
	if asm.hasErrors() {
		return errBad("还有标红的问题没有解决，解决后才能确认导出")
	}
	for _, k := range asm.needs() {
		if !in.Checks[k] {
			return errBad("请逐项勾选确认后再导出")
		}
	}
	paper.Confirm = &paperConfirm{At: now(), Sig: asm.Sig}
	a.store.Update(func(db *DB) error {
		for _, x := range db.Papers {
			if x.ID == paper.ID && x.OwnerID == me.ID {
				x.Confirm = paper.Confirm
			}
		}
		return nil
	})
	writeJSON(w, 200, a.paperView(me, paper))
	return nil
}

// readyPaper 导出前的统一关卡：没有错误、确认过且确认后内容没变
func (a *App) readyPaper(me *Me, id string) (*assembled, error) {
	paper, err := a.getPaper(me, id)
	if err != nil {
		return nil, err
	}
	asm := a.assemble(paper)
	if asm.hasErrors() {
		return nil, errBad("还有标红的问题没有解决，不能导出")
	}
	if !asm.confirmed() {
		return nil, errBad("导出前请先确认；确认之后如果换了草稿、改了题目或某一节重新起草，要重新确认")
	}
	return asm, nil
}

func (asm *assembled) fileName(ext string) string {
	t := strings.TrimSpace(asm.paper.Title)
	if t == "" {
		t = "全文"
	}
	return safeName(clipRunes(t, 40)) + "_AI辅助全文" + ext
}

func (a *App) hFullPaperDocx(w http.ResponseWriter, r *http.Request, me *Me) error {
	asm, err := a.readyPaper(me, r.PathValue("id"))
	if err != nil {
		return err
	}
	b, err := asm.docx()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="paper.docx"; filename*=UTF-8''`+urlPathEscape(asm.fileName(".docx")))
	w.Write(b)
	return nil
}

func (a *App) hFullPaperTex(w http.ResponseWriter, r *http.Request, me *Me) error {
	asm, err := a.readyPaper(me, r.PathValue("id"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/x-tex; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="paper.tex"; filename*=UTF-8''`+urlPathEscape(asm.fileName(".tex")))
	w.Write([]byte(asm.tex()))
	return nil
}

// hFullPaperPDF 用本机的 LaTeX 编译成 PDF（仅本机；可以后台运行）
func (a *App) hFullPaperPDF(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	asm, err := a.readyPaper(me, in.ID)
	if err != nil {
		return err
	}
	e := findTeX(false)
	if !e.Found {
		return errBad("这台电脑还没有安装 LaTeX。可以先下载 .tex 文件，或在“AI 助手 → 本机智能体 → 文件夹与权限”里一键下载便携 LaTeX（Tectonic）")
	}
	dir, err := os.MkdirTemp(texTempBase(), "kyws-paper-")
	if err != nil {
		return errBad("无法创建临时文件夹")
	}
	defer os.RemoveAll(dir)
	src := asm.tex()
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte(src), 0o600)
	res := runTeX(e, dir, "main.tex", dir, false, 180*time.Second)
	if res.pdf != nil {
		res.PDFID = keepPDF(me.ID, asm.fileName(".pdf"), res.pdf)
	}
	writeJSON(w, 200, map[string]any{"result": res, "id": in.ID})
	return nil
}
