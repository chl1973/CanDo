package main

// 学科技能库：按学科和用途分类的内置技能，以及从 GitHub 导入 SKILL.md。
// 内置技能是按思路重新编写的，不是照搬原文；每个技能注明思路参考的开源项目和它的许可证。
// 其中 academic-research-skills（CC BY-NC 4.0，禁止商用）只借鉴了“提交前诚信闸门”的思路，没有使用其文字。

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	creditKDense  = "思路参考 K-Dense-AI/scientific-agent-skills（MIT 许可）"
	creditNature  = "思路参考 Yuan1z0825/nature-skills（Apache-2.0 许可）"
	creditZh      = "思路参考 zLanqing/codex-claude-academic-skills（MIT 许可）"
	creditAuto    = "思路参考 leo-lilinxiao/codex-autoresearch（MIT 许可）"
	creditARS     = "思路参考 Imbad0202/academic-research-skills 的“诚信闸门”（CC BY-NC 4.0，仅借鉴思路，未使用原文）"
	skillCatOrder = "通用,写作,投稿,文献,生物医药化学,数据与图表,诚信,实验" // 前端按此顺序分组
)

var librarySkills = []AgentSkill{
	// ---------- 写作 ----------
	{ID: "l-zh-writing", Category: "写作", Name: "中文学术写作规范", Credit: creditZh,
		Description: "按中文学术论文的表达规范起草或修改段落：不编造、量化表述、术语保留英文",
		Body: `适用：写或改中文论文的任何一节。
1. 先 read_file 读用户的草稿和相关资料；不清楚写哪一节、给谁看（课程论文、大创、期刊、毕业论文）时先问。
2. 表达规则：
   - 默认用中文；公式、变量、方法名、软件命令、英文文献题名保留英文。
   - 不编造数据、文献、DOI、作者、实验结果；没有依据的说法删掉或标注“需要补充依据”。
   - 少用“显著”“先进”“极大地”等空泛词，改成可衡量的描述（比什么方法、在什么数据上、提高了多少）。
   - 一段只讲一件事：第一句给结论，后面给依据，最后说明意义或局限。
   - 引用他人观点必须标注参考文献；直接引用加引号。
3. 修改用 edit_file 小步提交，每处说明改了什么、为什么；不要整篇重写。
4. 结束时列出仍需作者补充的地方（数据、引用、图表）。`},
	{ID: "l-polish", Category: "写作", Name: "英文论文润色（Nature 风格）", Credit: creditNature,
		Description: "把英文稿改成简洁、准确、主动语态为主的期刊表达，逐句给出修改理由",
		Body: `1. read_file 读要润色的段落（一次一节，不要整篇）。
2. 规则：主动语态优先；一句一个主要意思；删去冗词（it is worth noting that / in order to）；用准确动词代替名词化（perform an analysis → analyse）；数字与单位之间空格（5 mm）；千位用逗号（1,000）；缩写首次出现写全称。
3. 不改变作者的科学含义和数据；不确定原意时在旁边提问，不要猜。
4. 用 edit_file 提交修改，reason 里写修改理由；最后总结常见问题，方便作者以后自己避免。`},
	{ID: "l-review", Category: "写作", Name: "模拟审稿", Credit: creditNature,
		Description: "像期刊审稿人一样读稿，给出结构化的审稿意见（主要问题、次要问题、是否支持结论）",
		Body: `1. read_file 通读稿件（摘要、引言、方法、结果、讨论、图表说明）。
2. 按以下结构写审稿意见（不要修改文件）：
   - 一段话概括研究问题、方法和主要结论；
   - 主要问题：结论是否被数据支持、对照是否充分、统计方法是否恰当、样本量、可重复性；
   - 次要问题：表述、图表、引用、格式；
   - 给作者的具体修改建议，每条对应原文位置。
3. 只依据稿件内容，指出问题要具体到段落或图表；不要编造稿件中没有的内容。`},
	{ID: "l-rebuttal", Category: "写作", Name: "回复审稿意见", Credit: creditNature,
		Description: "逐条整理审稿意见，起草礼貌、具体的回复信，并标出需要改稿的位置",
		Body: `1. read_file 读审稿意见和稿件；把意见拆成编号的条目。
2. 每条按“意见原文 → 回复 → 稿件中的修改（页码/行号或章节）”写；同意的说明怎么改了，不同意的给出依据和文献，语气礼貌。
3. 需要补实验或补数据的，写成“待作者补充”，不要替作者编造结果。
4. 用 write_file 生成回复信草稿（例如 response_letter.md），并列出需要作者决定的条目。`},
	// ---------- 投稿 ----------
	{ID: "l-availability", Category: "投稿", Name: "数据与代码可用性声明", Credit: creditNature,
		Description: "根据项目实际情况起草 Data availability / Code availability 声明",
		Body: `1. 问清楚（或从文件中确认）：数据类型、数据在哪里（公开数据库及登录号、仓库 DOI、附件、需申请）、有无隐私或伦理限制、代码在哪里、用了哪些软件和版本。
2. 按期刊常见写法起草，例如：“The data that support the findings of this study are available in [仓库] with the identifier [DOI/登录号].” 有限制的写明原因和获取方式。
3. 不要编造仓库地址、DOI 或登录号；缺的信息用【待补充】标出。
4. 提醒：Nature 系列要求单独的 Data availability 声明，某些数据类型必须存到公共数据库。`},
	{ID: "l-stats", Category: "投稿", Name: "统计报告检查", Credit: creditNature,
		Description: "检查论文中统计结果的报告是否完整：样本量、检验方法、统计量、P 值、效应量、误差线含义",
		Body: `1. read_file 读结果部分和图表说明。
2. 逐条检查：每个比较是否写明 n（生物学重复还是技术重复）、检验方法（t 检验、ANOVA 等）及是否单双侧、统计量和自由度、精确 P 值、效应量和置信区间、多重比较校正；误差线是 SD、SEM 还是 CI 是否写明。
3. 列出缺失或不一致之处（位置 + 问题 + 建议写法）；不要改动数据。`},
	// ---------- 文献 ----------
	{ID: "l-pubmed", Category: "文献", Name: "PubMed 文献检索", Credit: creditKDense,
		Description: "用 PubMed 公开接口检索生物医学文献，读取摘要并整理（适合生物、医学、药学）",
		Body: `1. 把问题转成英文检索式（可用 MeSH 词，例如 "breast neoplasms"[MeSH] AND immunotherapy）。
2. fetch_url 检索：https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi?db=pubmed&retmode=json&retmax=10&sort=relevance&term=检索式（空格写成 +）
3. 从结果的 idlist 取 PMID，fetch_url 读摘要：https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi?db=pubmed&rettype=abstract&retmode=text&id=PMID1,PMID2
4. 整理成表：PMID、题名、年份、研究类型、主要结论（只依据摘要）。需要收入资料库时，用 search_papers 按题名检索后 add_paper。
5. 不要编造 PMID 或结论。`},
	// ---------- 生物医药化学 ----------
	{ID: "l-pubchem", Category: "生物医药化学", Name: "化合物信息查询（PubChem）", Credit: creditKDense,
		Description: "按名称查询化合物的分子式、分子量、SMILES、IUPAC 名等",
		Body: `1. fetch_url：https://pubchem.ncbi.nlm.nih.gov/rest/pug/compound/name/化合物英文名/property/MolecularFormula,MolecularWeight,IUPACName,CanonicalSMILES,InChIKey/JSON
2. 名称查不到时，换同义词或 CAS 号再试；中文名先翻译成英文。
3. 汇报结果并附上来源链接 https://pubchem.ncbi.nlm.nih.gov/compound/CID。
4. 数值以查询结果为准，不要凭记忆补充。`},
	{ID: "l-uniprot", Category: "生物医药化学", Name: "蛋白质信息查询（UniProt）", Credit: creditKDense,
		Description: "按基因名和物种查询蛋白质的登录号、名称、长度和功能注释",
		Body: `1. fetch_url：https://rest.uniprot.org/uniprotkb/search?query=gene_exact:基因名+AND+organism_id:9606+AND+reviewed:true&fields=accession,protein_name,gene_names,organism_name,length,cc_function&format=tsv
   （9606 = 人；10090 = 小鼠；10116 = 大鼠；其他物种先问用户或查物种编号）
2. 汇报登录号、蛋白名、长度和功能注释要点，附链接 https://www.uniprot.org/uniprotkb/登录号。
3. 需要结构时提示用户查看 PDB 或 AlphaFold 数据库；不要编造数据。`},
	{ID: "l-bioinfo", Category: "生物医药化学", Name: "生信数据分析（Python）", Credit: creditKDense,
		Description: "用 Python（pandas、scanpy、biopython 等）分析授权文件夹里的测序或表达数据",
		Body: `1. read_file 查看数据文件的前几十行（或用 run_command 运行 python -c 打印形状和列名），确认格式（表达矩阵、FASTA、h5ad 等）。
2. 先向用户说明分析步骤（质控 → 标准化 → 降维/差异分析 → 作图）和要用的库；缺少库时告诉用户 pip install 什么，不要自己安装。
3. 用 write_file 写脚本，结果和图片写到新文件；run_command 运行（会请用户确认）。
4. 汇报关键数字和图片路径，说明参数选择的理由和局限；不要修改原始数据。`},
	// ---------- 数据与图表 ----------
	{ID: "l-figure", Category: "数据与图表", Name: "期刊级科研绘图", Credit: creditNature,
		Description: "用 Python（matplotlib）画符合期刊要求的图：尺寸、字号、配色、误差线说明",
		Body: `1. read_file 看数据；问清楚图要放在哪（单栏约 8.9 cm、双栏约 18.3 cm 宽）和要表达的比较。
2. 规则：字体 Arial/Helvetica，最终尺寸下字号 5–7 pt；线宽 0.5–1 pt；配色对色盲友好（避免红绿对比）；坐标轴写单位；误差线含义（SD/SEM/CI）写进图注；能显示原始数据点就显示；导出 PDF/SVG（矢量）和 300 dpi 以上 PNG。
3. write_file 写脚本，run_command 运行；汇报生成的文件，并给出图注（figure legend）草稿。`},
	// ---------- 诚信 ----------
	{ID: "l-integrity", Category: "诚信", Name: "提交前诚信检查", Credit: creditARS,
		Description: "提交论文或阶段成果前逐项检查：引用是否真实、说法是否有依据、数据与图表是否一致",
		Body: `这是提交前的“闸门”：任何一项不通过都要告诉用户，由用户决定是否继续，不要替用户放行。
1. 引用真实性：列出参考文献，每条用 search_papers 按题名核对是否存在，题名、作者、年份是否一致。
2. 说法与出处：抽查正文中带引用的句子，用 search_library 找到资料原文，判断原文是否真的支持这句话。
3. 数据一致：正文中提到的数字与表格、图注是否一致；方法中的样本量与结果是否一致。
4. AI 使用：提醒用户按学校或期刊要求声明使用了 AI 工具的环节。
5. 输出检查报告：每项“通过 / 需要处理”，需要处理的写清位置和原因。不修改文件，除非用户要求。`},
	// ---------- 实验 ----------
	{ID: "l-autoloop", Category: "实验", Name: "自动改进循环", Credit: creditAuto,
		Description: "对有明确数值指标的任务（程序运行时间、模型误差、编译时间等）反复“改一处 → 测量 → 保留或撤销”",
		Body: `适用：有一条能输出一个数字的测量命令（例如 python eval.py 最后一行打印误差），且目标明确（例如误差 < 0.05）。
1. 先和用户确认：测量命令、指标越大越好还是越小越好、目标值、最多尝试几轮（默认 5 轮）、允许改哪些文件。
2. 运行一次测量，记下基线。
3. 每轮只做一处有针对性的修改（edit_file，reason 写清假设），用户应用后运行测量：
   - 变好：保留，记录；
   - 没变好或出错：请用户在“待确认”里撤销这处修改，记录原因。
4. 每轮用一两句话汇报：改了什么、指标从多少到多少、保留还是撤销。
5. 达到目标或用完轮数就停，最后给出汇总表。不要改测量脚本本身来“提高”指标。`},
	{ID: "l-labnote", Category: "实验", Name: "实验记录整理", Credit: creditNature,
		Description: "把零散的实验笔记整理成规范的实验记录（目的、材料、步骤、结果、问题）",
		Body: `1. read_file 读用户的笔记、数据文件名和照片说明。
2. 按模板整理：日期、实验目的、材料与试剂（批号、浓度）、仪器与参数、步骤、原始数据位置、结果、异常与处理、下一步。
3. 笔记里没有的信息标【待补充】，不要编造。
4. 用 write_file 保存为新文件（例如 实验记录_日期.md），不覆盖原笔记。`},
}

func init() {
	for i := range builtinSkills {
		builtinSkills[i].BuiltIn = true
	}
	for i := range librarySkills {
		librarySkills[i].BuiltIn = true
	}
}

// ---------------- 从 GitHub 导入 ----------------

var reGHBlob = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/(blob|tree)/([^/]+)/(.+)$`)

// skillRawURL 把 GitHub 页面地址换成原始文件地址。
func skillRawURL(u string) (string, error) {
	u = strings.TrimSpace(u)
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "https" {
		return "", errBad("请填写以 https:// 开头的网址")
	}
	switch pu.Host {
	case "raw.githubusercontent.com":
		return u, nil
	case "github.com":
		m := reGHBlob.FindStringSubmatch(strings.TrimSuffix(u, "/"))
		if m == nil {
			return "", errBad("请打开具体技能的文件夹或 SKILL.md 文件，再复制浏览器地址栏的网址（例如 https://github.com/作者/仓库/tree/main/skills/技能名）")
		}
		p := m[5]
		if m[3] == "tree" || !strings.HasSuffix(strings.ToLower(p), ".md") {
			p = strings.TrimSuffix(p, "/") + "/SKILL.md"
		}
		return "https://raw.githubusercontent.com/" + m[1] + "/" + m[2] + "/" + m[4] + "/" + p, nil
	}
	return "", errBad("目前只支持从 GitHub 导入")
}

// ---------------- 下载 SKILL.md：GitHub 连不上时换备用线路 ----------------

type skillSource struct{ URL, Accept string }

var reRawGH = regexp.MustCompile(`^https://raw\.githubusercontent\.com/([^/]+)/([^/]+)/([^/]+)/(.+)$`)

// skillSources 同一个文件的几条下载线路，按顺序试。
// raw.githubusercontent.com 在国内经常连不上（浏览器能打开 github.com 也不代表它能连上），
// 所以后面跟着两条 jsDelivr 镜像（内容可能比 GitHub 晚几个小时）和 GitHub 的接口。测试时可以替换。
var skillSources = func(raw string) []skillSource {
	out := []skillSource{{URL: raw}}
	if m := reRawGH.FindStringSubmatch(raw); m != nil {
		owner, repo, ref, p := m[1], m[2], m[3], m[4]
		out = append(out,
			skillSource{URL: "https://cdn.jsdelivr.net/gh/" + owner + "/" + repo + "@" + ref + "/" + p},
			skillSource{URL: "https://fastly.jsdelivr.net/gh/" + owner + "/" + repo + "@" + ref + "/" + p},
			skillSource{URL: "https://api.github.com/repos/" + owner + "/" + repo + "/contents/" + p + "?ref=" + url.QueryEscape(ref), Accept: "application/vnd.github.raw+json"},
		)
	}
	return out
}

var skillTryTimeout = 12 * time.Second // 每条线路最多等多久

const skillManualHint = "可以在浏览器里打开这个技能的 SKILL.md，点右上角的下载按钮存到电脑，再点“导入 SKILL.md 文件”"

// fetchSkillMD 依次试各条线路，返回文件内容（最多 200 KB）。
func fetchSkillMD(ctx context.Context, raw string) ([]byte, error) {
	notFound, lastStatus, private := false, 0, false
	for i, src := range skillSources(raw) {
		c, cancel := context.WithTimeout(ctx, skillTryTimeout)
		req, _ := http.NewRequestWithContext(c, "GET", src.URL, nil)
		req.Header.Set("User-Agent", "KeyanWorkbench/"+AppVersion)
		if src.Accept != "" {
			req.Header.Set("Accept", src.Accept)
		}
		resp, err := pdfClient.Do(req)
		if err != nil {
			cancel()
			if strings.Contains(err.Error(), "内网") {
				private = true
			}
			continue // 这条线路连不上，换下一条
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200<<10))
		resp.Body.Close()
		cancel()
		switch {
		case resp.StatusCode == 200:
			return b, nil
		case resp.StatusCode == 404 && i == 0:
			// GitHub 自己说没有这个文件：不用再试镜像
			return nil, errBad("没有找到 SKILL.md：请确认网址指向一个技能文件夹（里面有 SKILL.md）")
		case resp.StatusCode == 404:
			notFound = true
		default:
			lastStatus = resp.StatusCode
		}
	}
	switch {
	case notFound:
		return nil, errBad("下载失败：GitHub 连不上，备用线路上也没有找到这个文件。请确认网址指向一个技能文件夹（里面有 SKILL.md）；也" + skillManualHint)
	case lastStatus != 0:
		return nil, errBad("下载失败（" + itoa(lastStatus) + "）。" + skillManualHint)
	case private:
		return nil, errBad("下载失败：这台电脑把 GitHub 的地址解析成了内网地址（常见于 hosts 文件或加速器的设置）。" + skillManualHint)
	}
	return nil, errBad("下载失败：连不上 GitHub（国内网络常见，备用线路也试过了）。" + skillManualHint)
}

var reLicense = regexp.MustCompile(`(?mi)^\s*license\s*:\s*["']?([^"'\n]+)`)

func (a *App) hSkillImport(w http.ResponseWriter, r *http.Request, me *Me) error {
	var in struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	raw, err := skillRawURL(in.URL)
	if err != nil {
		return err
	}
	b, err := fetchSkillMD(r.Context(), raw)
	if err != nil {
		return err
	}
	if !utf8.Valid(b) {
		return errBad("文件不是文本")
	}
	text := string(b)
	name, desc, body := parseSkillMD(text)
	if name == "" {
		name = strings.TrimSuffix(pathBase(raw), ".md")
		if name == "SKILL" {
			name = pathBase(strings.TrimSuffix(raw, "/SKILL.md"))
		}
	}
	lic := ""
	if fm := reFront.FindStringSubmatch(text); fm != nil {
		if m := reLicense.FindStringSubmatch(fm[1]); m != nil {
			lic = strings.TrimSpace(m[1])
		}
	}
	name, desc = clipRunes(strings.TrimSpace(name), 40), clipRunes(strings.TrimSpace(desc), 200)
	if strings.TrimSpace(body) == "" {
		return errBad("这个 SKILL.md 没有内容")
	}
	if len(body) > 20000 {
		body = body[:20000] + "\n…（内容过长，已截断）"
	}
	if strings.Contains(body, "scripts/") {
		body += "\n\n（说明：这个技能原本附带脚本文件，工作台只导入了步骤说明，没有下载脚本。需要脚本时请到原仓库查看：" + in.URL + "）"
	}
	sk := &AgentSkill{ID: newID(), OwnerID: me.ID, Name: name, Description: desc, Body: body, Category: "导入", Source: strings.TrimSpace(in.URL), License: lic, UpdatedAt: now()}
	err = a.store.Update(func(db *DB) error {
		n := 0
		for _, s := range db.Skills {
			if s.OwnerID == me.ID {
				n++
				if s.Source == sk.Source {
					s.Name, s.Description, s.Body, s.License, s.UpdatedAt = sk.Name, sk.Description, sk.Body, sk.License, now()
					sk = s
					return nil
				}
			}
		}
		if n >= 50 {
			return errBad("每人最多 50 个技能")
		}
		db.Skills = append(db.Skills, sk)
		return nil
	})
	if err != nil {
		return err
	}
	warn := ""
	ll := strings.ToLower(lic)
	switch {
	case lic == "":
		warn = "这个技能没有写明许可证，请到原仓库查看许可证后再决定能否在团队中使用。"
	case strings.Contains(ll, "nc") || strings.Contains(ll, "noncommercial") || strings.Contains(ll, "non-commercial"):
		warn = "许可证 " + lic + " 禁止商业用途：个人学习、科研可以用，不能用于收费服务或商业产品。"
	}
	writeJSON(w, 200, map[string]any{"skill": sk, "warning": warn, "skills": a.skillsFor(me)})
	return nil
}

func pathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
