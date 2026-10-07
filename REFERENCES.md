# CanDo 可为 · 参考文献与来源

这份清单列出 CanDo 可为依据、借鉴、使用过的全部外部来源，共 38 条。写申报书、结题报告或论文时，可以直接从这里引用。

- 核对日期：2026-10-02。除特别注明的条目外，每一条都打开了原始页面核对过题名、作者、日期和许可证。
- 著录格式：GB/T 7714—2025 顺序编码制 [4]。
- 分成五类，使用方式不同，引用时不要混在一起：
  - **一、直接依据**：程序里的规则就是照这些文件写的。
  - **二、借鉴思路**：读过别人的开源项目，按思路重新编写，没有照搬原文。
  - **三、随程序分发**：别人的代码，原样打包在安装包里。
  - **四、调用的服务**：程序运行时联网访问，不随程序分发。
  - **五、背景文献**：设计思路对应的公开研究和政策。CanDo 不是从这些文献推导出来的，列在这里是为了说明“为什么这样设计”有据可查。

## 需要先处理的问题：两项国家标准今年已经换了新版

核对时发现，程序依据的两项国家标准都已被 2025 年的新版代替。程序里的规则和界面文字写的还是旧版。

| 标准 | 程序里写的版本 | 现行版本 | 新版实施日期 | 旧版状态 |
|---|---|---|---|---|
| 学位论文编写规则 | GB/T 7713.1—2006 [1] | GB/T 7713.1—2025 [2] | 2026-02-01 | 2026-02-01 废止 |
| 参考文献著录规则 | GB/T 7714—2015 [3] | GB/T 7714—2025 [4] | 2026-07-01 | 被新版代替 |

对程序的影响（待办，还没有改）：

- 程序、界面和说明文件里共有 41 行提到“GB/T 7714”，其中写明“—2015”的要改成新版；“毕业论文”类型的依据写的是 GB/T 7713.1—2006。
- GB/T 7714—2025 调整了文献类型标识：新增预印本 `PP`、数据集 `DS`；`EB` 的含义改为“网站、网页”；“专著”改称“图书”；“数字对象唯一标识符”改称“永久标识符”；网页要著录“创建/发布或修改日期”和“引用日期” [4]。格式检查里识别类型标识的规则（`doccheck.go` 的 `reGBType`）还不认识 `PP` 和 `DS`。
- GB/T 7713.1—2025 相对 2006 版改了哪些条款，还没有逐条比对。比对之前，不要对外说“符合 GB/T 7713.1—2025”。

## 一、直接依据的标准和规范

程序里的论文结构、格式要求和检查规则来自下面这些文件（`writing.go`、`doccheck.go`、`docxgen.go`、`writing_ai.go`）。

[1] 全国信息与文献标准化技术委员会. 学位论文编写规则: GB/T 7713.1—2006[S/OL]. (2006-12-05)[2026-10-02]. https://std.samr.gov.cn/gb/search/gbDetailed?id=71F772D7C5F6D3A7E05397BE0A0AB82A.
　用于：“本科毕业论文”类型的组成顺序、章节编号、图表编号、关键词个数。已于 2026-02-01 废止，被 [2] 代替。

[2] 全国信息与文献标准化技术委员会. 信息与文献 编写规则 第1部分：学位论文: GB/T 7713.1—2025[S/OL]. (2025-08-01)[2026-10-02]. https://std.samr.gov.cn/gb/search/gbDetailed?id=3B46A026CC84469CE06397BE0A0AEEB8.
　现行版本，2026-02-01 实施。程序还没有按它更新。

[3] 全国信息与文献标准化技术委员会. 信息与文献 参考文献著录规则: GB/T 7714—2015[S]. 2015.
　用于：参考文献的顺序编码、作者超过 3 位写“等”、文献类型标识、著录符号；检索结果一键生成引用；格式检查里的参考文献规则。已被 [4] 代替。本条的发布信息没有在线核对。

[4] 国家市场监督管理总局, 国家标准化管理委员会. 信息与文献 参考文献著录规则: GB/T 7714—2025[S/OL]. (2025-12-02)[2026-10-02]. https://rem.cueb.edu.cn/docs/2026-04/ab13582039564d9090a722f17cfcbf2e.pdf.
　现行版本，2026-07-01 实施。本清单按它著录。链接是一所高校转载的标准全文，正式文本以国家标准全文公开系统为准。

[5] 国家市场监督管理总局, 国家标准化管理委员会. 学术论文编写规则: GB/T 7713.2—2022[S/OL]. (2022-12-30)[2026-10-02]. https://lib.tsinghua.edu.cn/wj/GBT7713_2-2022.pdf.
　现行，2023-07-01 实施。程序的“中文期刊投稿”类型目前只写了“国内期刊常见要求”，还没有按这项标准写规则，建议补上。

[6] 中国工业与应用数学学会. 全国大学生数学建模竞赛论文格式规范（2026年修订稿）[EB/OL]. (2026-03-03)[2026-10-02]. https://www.mcm.edu.cn/html_cn/node/4cd596519c9eb9fbd866398f6df0caa3.html.
　用于：“数学建模国赛”类型：正文不超过 30 页、页边距至少 2.5 厘米、摘要专用页、不得出现身份信息、附录要有全部源程序。已和程序里的数值逐项对上。

[7] Nature. Formatting guide[EB/OL]. [2026-10-02]. https://www.nature.com/nature/for-authors/formatting-guide.
　用于：“Nature（Article）”类型：题目不超过 75 个字符、摘要约 200 词、正文 2500–4300 词、Methods 不超过 3000 词、正文参考文献不超过 50 条、图注少于 300 词、必需的声明。已和程序里的数值逐项对上。

[8] Nature Communications. Article[EB/OL]. [2026-10-02]. https://www.nature.com/ncomms/submit/article.
　用于：“Nature Communications”类型：题目不超过 15 词、摘要不超过 200 词且不含引用、正文约 5000 词、参考文献不超过 70 条、图表不超过 10 个、图注不超过 350 词。已和程序里的数值逐项对上。

[9] Nature. How to construct a Nature summary paragraph[EB/OL]. [2026-10-02]. https://www.nature.com/documents/nature-summary-paragraph.pdf.
　用于：AI 起草英文摘要时的论证顺序（领域介绍 → 具体背景 → 要解决的问题 → 主要发现 → 与已有认识的比较 → 更广的意义）。

[10] Robertson S, Zaragoza H. The probabilistic relevance framework: BM25 and beyond[J]. Foundations and Trends in Information Retrieval, 2009, 3(4): 333-389. DOI:10.1561/1500000019.
　用于：论文库检索用的 BM25 算法（`textproc.go`，参数 k1 = 1.5、b = 0.75）。

## 二、借鉴思路的开源项目

内置技能和章节写作规则读过下面这些项目，然后按思路重新编写，没有复制原文（`skills_lib.go`、`writing_ai.go`）。每个内置技能在界面上都标了思路来源和许可证。

[11] Yuan Y. nature-skills[CP/OL]. [2026-10-02]. https://github.com/Yuan1z0825/nature-skills.
　许可证 Apache-2.0。借鉴：`nature-writing` 的工作流（先定位论文类型、章节和语言，再加载对应规则，先规划后成文，证据不足时留空并列出缺什么）和写作规则（摘要的论证顺序、引言的漏斗结构、结果逐步升级、讨论的顺序）；内置技能“英文论文润色”“模拟审稿”“回复审稿意见”“数据与代码可用性声明”“统计报告检查”“期刊级科研绘图”“实验记录整理”。

[12] K-Dense AI. scientific-agent-skills[CP/OL]. [2026-10-02]. https://github.com/K-Dense-AI/scientific-agent-skills.
　许可证 MIT。借鉴：内置技能“PubMed 文献检索”“化合物信息查询”“蛋白质信息查询”“生信数据分析”。

[13] zLanqing. codex-claude-academic-skills[CP/OL]. [2026-10-02]. https://github.com/zLanqing/codex-claude-academic-skills.
　许可证 MIT。借鉴：内置技能“中文学术写作规范”。

[14] Li L. codex-autoresearch[CP/OL]. [2026-10-02]. https://github.com/leo-lilinxiao/codex-autoresearch.
　许可证 MIT。借鉴：内置技能“自动改进循环”（改一处、验证、留下或丢弃、再来一轮）。

[15] Wu C. academic-research-skills[CP/OL]. [2026-10-02]. https://github.com/Imbad0202/academic-research-skills.
　许可证 CC BY-NC 4.0，禁止商用。只借鉴了“提交前诚信闸门”这个想法，写成内置技能“提交前诚信检查”，没有使用它的任何文字。以后也不要使用它的原文。

[16] Anthropic. Agent Skills[CP/OL]. [2026-10-02]. https://github.com/anthropics/skills.
　用于：技能文件的格式（`SKILL.md`，开头写 `name` 和 `description`）。CanDo 可以从网址导入这种格式的技能，只导入文字说明，不下载、不执行脚本。规范见 https://agentskills.io/specification 。

## 三、随程序分发的第三方软件

这些代码原样放在 `web/lib/` 里，随安装包一起分发，许可证文字保留在文件头或同目录的 LICENSE 中。

[17] Mozilla Foundation. PDF.js: 4.10.38[CP/OL]. [2026-10-02]. https://github.com/mozilla/pdf.js.
　许可证 Apache-2.0。用于：在浏览器里逐页读取 PDF 的文字和渲染页面（上传论文、OCR 前的渲染、PDF 预览）。

[18] Khan Academy. KaTeX: 0.16.22[CP/OL]. [2026-10-02]. https://github.com/KaTeX/KaTeX.
　许可证 MIT。用于：在网页上显示数学公式。

[19] Arase K. QR Code Generator for JavaScript[CP/OL]. [2026-10-02]. https://github.com/kazuhikoarase/qrcode-generator.
　许可证 MIT。用于：生成手机扫码连接电脑的二维码。“QR Code”是 DENSO WAVE 的注册商标。

后端只用 Go 语言标准库，没有其他第三方依赖（见 `go.mod`）。

## 四、调用的外部数据和服务

程序运行时按用户的操作联网访问，不随程序分发。接口地址取自程序代码；除 [20]、[27] 外，本节条目的说明页没有逐一在线核对。

[20] Priem J, Piwowar H, Orr R. OpenAlex: a fully-open index of scholarly works, authors, venues, institutions, and concepts[PP/OL]. arXiv:2205.01833, 2022[2026-10-02]. https://arxiv.org/abs/2205.01833.
　用于：文献检索、深度调研、引用核验第一关（检查参考文献是否存在，只发送题名）。接口 https://api.openalex.org 。

[21] Crossref. Crossref REST API[EB/OL]. [2026-10-02]. https://api.crossref.org.
　用于：按 DOI 补全题录（`scholar.go`）。

[22] Corporation for Digital Scholarship. Zotero[CP/OL]. [2026-10-02]. https://www.zotero.org.
　用于：从本机 Zotero（`http://127.0.0.1:23119/api/`，只读）或 Zotero 云端（`https://api.zotero.org`）导入文献（`zotero.go`）。

[23] National Center for Biotechnology Information. Entrez Programming Utilities[EB/OL]. [2026-10-02]. https://eutils.ncbi.nlm.nih.gov.
　用于：内置技能“PubMed 文献检索”指引智能体访问的接口。同类的还有 PubChem（https://pubchem.ncbi.nlm.nih.gov）和 UniProt（https://rest.uniprot.org）。

[24] 博查. 博查搜索开放平台[EB/OL]. [2026-10-02]. https://open.bochaai.com.
　用于：本机智能体的联网搜索（国内）。需要用户自己填密钥。

[25] Tavily. Tavily Search API[EB/OL]. [2026-10-02]. https://api.tavily.com.
　用于：本机智能体的联网搜索（国外）。需要用户自己填密钥。

[26] 大模型接口：任何兼容 OpenAI `/chat/completions` 的接口，以及 Anthropic `/v1/messages`（`llm.go`）。用哪家模型由用户在设置里填写，程序不内置任何模型和密钥。

[27] Tectonic Project. Tectonic[CP/OL]. [2026-10-02]. https://github.com/tectonic-typesetting/tectonic.
　许可证 MIT。用于：便携版 LaTeX 编译器，用户点“一键下载”后才从它的发布页下载，不随安装包分发（`tectonic.go`）。电脑上已装 TeX Live 或 MiKTeX 时优先用它们；中文排版用 CTeX 宏集（`ctexart` 文档类）。

## 五、设计思路的背景文献

下面这些是 CanDo 几个关键设计对应的公开研究和政策。写申报书或论文介绍“为什么这样设计”时可以引用。

**主流程由代码固定，模型只做其中的环节（工作流，而不是让模型自己决定全部步骤）**

[28] Schluntz E, Zhang B. Building effective agents[EB/OL]. (2024-12-19)[2026-10-02]. https://www.anthropic.com/engineering/building-effective-agents.
　文中把“工作流”定义为按预先写好的代码路径编排模型和工具，把“智能体”定义为由模型自己动态决定流程和工具的使用。CanDo 的写作主流程属于前者，本机智能体属于后者。

[29] Yao S, Zhao J, Yu D, et al. ReAct: synergizing reasoning and acting in language models[C/OL]//International Conference on Learning Representations (ICLR), 2023[2026-10-02]. https://arxiv.org/abs/2210.03629.
　本机智能体“想一步、调用工具、看结果、再想下一步”的循环，是这类做法的一种。

**回答和起草都要有原文依据，引用编号由后端核对**

[30] Lewis P, Perez E, Piktus A, et al. Retrieval-augmented generation for knowledge-intensive NLP tasks[C/OL]//Advances in Neural Information Processing Systems 33 (NeurIPS 2020), 2020[2026-10-02]. https://arxiv.org/abs/2005.11401.
　先检索原文片段、再让模型依据片段生成（检索增强生成）。

[31] Walters W H, Wilder E I. Fabrication and errors in the bibliographic citations generated by ChatGPT[J/OL]. Scientific Reports, 2023, 13: 14045[2026-10-02]. https://doi.org/10.1038/s41598-023-41032-5.
　该研究统计，GPT-3.5 生成的参考文献有 55% 是编造的，GPT-4 为 18%。这是 CanDo 不让模型自己写参考文献、只从论文库里已有的论文生成、并做“引用核验”的原因。

**反谄媚：是否让步由代码规则判定，不看模型愿不愿意**

[32] Sharma M, Tong M, Korbak T, et al. Towards understanding sycophancy in language models[PP/OL]. arXiv:2310.13548, 2023[2026-10-02]. https://arxiv.org/abs/2310.13548.
　该研究把“谄媚”定义为模型的回答迎合用户的看法而不是事实，并在五个主流助手上都观察到这种现象。论文契约里“回应必须答到核心并给出可核对的证据，审稿人才让步”的门槛，针对的就是这个问题。

**文件、网页、工具返回的内容只当作数据，不当作指令**

[33] Greshake K, Abdelnabi S, Mishra S, et al. Not what you've signed up for: compromising real-world LLM-integrated applications with indirect prompt injection[PP/OL]. arXiv:2302.12173, 2023[2026-10-02]. https://arxiv.org/abs/2302.12173.
　间接提示注入：藏在资料里的文字可以诱导模型执行操作。CanDo 的做法是限定授权文件夹、修改和运行命令都要用户确认、工具结果标明“这是数据”。

**外部工具通过一个窄接口接入**

[34] Anthropic. Model Context Protocol[EB/OL]. (2024-11-25)[2026-10-02]. https://modelcontextprotocol.io.
　把工具做成独立服务、通过“工具清单 + 调用”接给智能体的开放标准，2025 年 12 月捐赠给 Linux 基金会下的 Agentic AI Foundation。CanDo 的外部工具接口（`docs/外部工具接口.md`，协议 cando-tools/1）思路相同，字段设计与它相近，但没有使用它的代码，也不兼容它的协议。发布日期和捐赠信息引自维基百科条目 https://en.wikipedia.org/wiki/Model_Context_Protocol 。

**AI 不代写；使用 AI 要声明；内容由作者负责**

[35] Tools such as ChatGPT threaten transparent science; here are our ground rules for their use[J/OL]. Nature, 2023, 613(7945): 612[2026-10-02]. https://doi.org/10.1038/d41586-023-00191-1.
　Nature 的两条规则：大模型不能署名为作者；使用了大模型要在方法或致谢里说明。

[36] 科技部监督司. 负责任研究行为规范指引（2023）[EB/OL]. (2023-12)[2026-10-02]. https://www.research.pku.edu.cn/docs/2023-12/20231227162417283170.pdf.
　其中规定：不得使用生成式人工智能直接生成申报材料；生成的内容要明确标注并说明生成过程；不得直接使用未经核实的由生成式人工智能生成的参考文献；生成式人工智能不得列为成果共同完成人。链接是北京大学转载的全文。

[37] 魏哲哲. 规范学位授予，推动高等教育高质量发展[N/OL]. 人民日报, 2024-05-23(18)[2026-10-02]. https://paper.people.com.cn/rmrbwap/html/2024-05/23/nw.D110000renmrb_20240523_1-18.htm.
　报道《中华人民共和国学位法》：2024-04-26 由十四届全国人大常委会第九次会议通过，2025-01-01 起施行，把代写、剽窃、伪造列为学术不端行为。

## 六、致谢

- **指导老师江慧敏**：论文契约的设计来自她的建议，包括：契约要经本人确认才生效；动笔前先确认研究问题（变量、方法、边界）；写 2–3 个核心主张，每个都写明证据和“什么情况下不成立”；各部分围绕“问题、证据、结论”；反谄媚的让步门槛；契约搭好后强制跑一轮审稿人攻击（见 `TEST_REPORT.md` v1.13.0）。
- **同组同学**：v1.14 对照了同学的脚本 `AI_API_2_4(demo).py`，把其中我们没有的能力（读写 Office 文件、联网搜索、便携 LaTeX、兼容 DeepSeek 的工具调用标记）用 Go 重新实现；更早的本机文件助手参考了组员的 `AI_helper 2.0` 的思路。两处都没有沿用原脚本的代码（见 `TEST_REPORT.md`）。同学的姓名待本人同意后补上。

## 七、AI 辅助开发声明

[38] Anthropic. Claude Code[CP/OL]. [2026-10-02]. https://claude.com/claude-code.
　CanDo 的代码、测试和文档是项目负责人在 Claude（Anthropic 的大模型，通过 Claude Code 使用）的辅助下完成的：负责人提出需求、做设计上的取舍并验收；Claude 编写代码和测试、撰写文档。GitHub 上由 Claude 参与的提交都带有 `Co-Authored-By: Claude` 标记。按 [35] [36] 的要求在此说明；Claude 不署名为作者或成果完成人。

本清单也是由 Claude 起草的，各条目的核对情况见开头的说明。引用到正式材料之前，请负责人再抽查一遍，尤其是第四节没有在线核对的条目。

## 怎么维护这份清单

- 程序里新增一条规则、一个内置技能、一个第三方文件或一个联网服务时，同时在这里登记来源。
- 引入别人的代码或文字之前，先核对许可证。标了 NC（禁止商用）的内容只能借鉴想法，不能使用原文。
- 标准和投稿指南会修订。每次发新版本前，重新打开第一节的链接核对一遍，并更新“核对日期”。
