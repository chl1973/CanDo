package main

// 能力中心（1.16）：把“智能体能做什么”摆到明面上。
//   - 内置能力：智能体自带的工具，按用途分组，每项带一句示例；缺什么（模型、授权文件夹、LaTeX、搜索密钥……）如实标出。
//   - 外部工具：合作者用 Python 等写的本机工具服务（extools.go），每个工具显示成一张卡片。
//   - 开发包：接口约定和 Python 示例打成 zip，直接发给合作者。
// 这里只负责“展示和入口”，不改变智能体的权限：调用、确认、写入仍然走原来的流程。

import (
	"archive/zip"
	"bytes"
	"embed"
	"net/http"
	"net/url"
	"strings"
)

//go:embed docs/外部工具接口.md docs/cando-tools.openapi.json tools/extool/example_server.py tools/extool/check_contract.py
var extKitFS embed.FS

type capItem struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Desc    string `json:"desc"`
	Example string `json:"example"`        // 点“用它”时填进智能体输入框
	Ready   bool   `json:"ready"`          // 现在能不能用
	Need    string `json:"need,omitempty"` // 不能用时：还缺什么
	Link    string `json:"link,omitempty"` // 去哪里设置："#/settings" 或 "agent:folders"
	Safe    string `json:"safe,omitempty"` // 安全说明（一句）
}

type capGroup struct {
	Name  string    `json:"name"`
	Icon  string    `json:"icon"`
	Items []capItem `json:"items"`
}

// capState 影响内置能力是否可用的几项状态
type capState struct {
	Model, Folders, Writable, TeX, WebKey, Vision bool
	Commands, Open, Web                           string
}

// builtinCapabilities 内置能力清单。新增智能体工具时在这里加一项（TestCapabilities 会检查示例不为空、编号不重复）。
func builtinCapabilities(st capState) []capGroup {
	const toFolders, toSettings = "agent:folders", "#/settings"
	needFolder := func(it capItem, write bool) capItem {
		switch {
		case !st.Folders:
			it.Need, it.Link = "先授权一个文件夹，智能体才看得到你的文件", toFolders
		case write && !st.Writable:
			it.Need, it.Link = "授权文件夹现在都是“只读”，要改文件请把其中一个设为“可修改”", toFolders
		}
		return it
	}
	groups := []capGroup{
		{Name: "文件", Icon: "i-file", Items: []capItem{
			needFolder(capItem{Key: "find_read", Title: "查找和阅读文件", Desc: "在授权文件夹里找文件、读内容、按文字搜索。",
				Example: "看看我的论文文件夹里有哪些 .tex 文件，各自写的是什么"}, false),
			needFolder(capItem{Key: "edit_text", Title: "修改文本和代码", Desc: "改 LaTeX、Markdown、代码等文本文件，只改需要改的地方。",
				Example: "把 intro.tex 里的“显著提高”都改成“提高”", Safe: "每处修改都要你确认，写入前备份，可以撤销"}, true),
			needFolder(capItem{Key: "organize", Title: "整理文件", Desc: "移动、重命名、复制、新建文件夹。",
				Example: "把“下载”文件夹里的 PDF 按年份整理到子文件夹", Safe: "删除的文件先放进回收区，可以恢复"}, true),
		}},
		{Name: "Word · Excel · PPT", Icon: "i-writing", Items: []capItem{
			needFolder(capItem{Key: "office_read", Title: "读 Word、Excel、PPT", Desc: "Word 按段落、Excel 按行列、PPT 按页读出来。",
				Example: "读一下 report.docx，告诉我每一节讲了什么"}, false),
			needFolder(capItem{Key: "docx_edit", Title: "按段修改 Word", Desc: "替换、插入、删除段落，保留原来的字体、排版、图片、表格和公式。",
				Example: "把 report.docx 的摘要改得更简洁，其他地方不要动", Safe: "含图片或公式的段落不会被改动"}, true),
			needFolder(capItem{Key: "make_docx", Title: "生成排好版的 Word", Desc: "把内容排成宋体小四、1.5 倍行距、黑体标题的文档，支持三线表。",
				Example: "把这次修改的内容整理成一份“修改说明.docx”，用表格列出改了哪里", Safe: "只新建，不覆盖已有文件"}, true),
			needFolder(capItem{Key: "docx_format", Title: "分析 Word 模板的格式", Desc: "看懂模板用的纸张、页边距、各级标题和正文的字体字号。",
				Example: "分析一下“模板.docx”用的字体、字号和页边距"}, false),
			needFolder(capItem{Key: "docx_images", Title: "导出 Word 里的图片", Desc: "把 Word 里的图片存成单独的文件。",
				Example: "把 report.docx 里的图片都导出到 figures 文件夹"}, true),
		}},
		{Name: "LaTeX", Icon: "i-formula", Items: []capItem{
			func() capItem {
				it := needFolder(capItem{Key: "compile_latex", Title: "编译 PDF，按报错修改", Desc: "编译 .tex 生成 PDF；出错时按行号找到问题，改好再编译。",
					Example: "编译 main.tex，有错误就帮我改好"}, false)
				if it.Need == "" && !st.TeX {
					it.Need, it.Link = "这台电脑还没有 LaTeX，可以一键下载便携版", toFolders
				}
				return it
			}(),
			needFolder(capItem{Key: "check_latex", Title: "检查括号和环境", Desc: "不用编译，先查括号、环境、公式有没有没配对的。",
				Example: "检查 main.tex 的括号、环境和公式有没有没配对的"}, false),
		}},
		{Name: "资料与网络", Icon: "i-library", Items: []capItem{
			{Key: "search_library", Title: "在论文库里找原文", Desc: "检索工作台论文库里的原文片段，回答时带出处。",
				Example: "在我的论文库里找找关于“注意力”的说法，带上出处"},
			{Key: "search_papers", Title: "检索学术文献", Desc: "在公开的学术数据库里查论文，挑中的可以把题录收进论文库。",
				Example: "查一下近三年关于钙钛矿电池稳定性的论文，挑 5 篇加入论文库"},
			func() capItem {
				it := capItem{Key: "search_web", Title: "联网搜索", Desc: "查竞赛通知、软件报错、模板写法等网上的资料，回答时附来源链接。",
					Example: "搜一下今年全国大学生数学建模竞赛的报名时间"}
				if !st.WebKey {
					it.Need, it.Link = "还没有填搜索服务的密钥（设置 → 联网搜索）", toSettings
				}
				return it
			}(),
			func() capItem {
				it := capItem{Key: "fetch_url", Title: "读取网页", Desc: "打开一个网址，读出正文。", Example: "读一下这个网页讲了什么：（把网址贴在这里）"}
				if st.Web == "off" {
					it.Need, it.Link = "“读取网页内容”在权限里被关掉了", toFolders
				}
				return it
			}(),
		}},
		{Name: "图片", Icon: "i-image", Items: []capItem{
			func() capItem {
				it := capItem{Key: "recognize_image", Title: "看图：公式、表格、截图", Desc: "看你附上的图片或文件夹里的图片：转成 LaTeX，或者回答图里的问题。",
					Example: "看看我附的这张截图报的是什么错，该怎么改"}
				if !st.Vision {
					it.Need, it.Link = "还没有能看图片的模型（设置 → 我的 AI 模型，勾选“能看图片”）", toSettings
				}
				return it
			}(),
		}},
		{Name: "电脑", Icon: "i-bolt", Items: []capItem{
			func() capItem {
				it := needFolder(capItem{Key: "run_command", Title: "运行命令和脚本", Desc: "在授权文件夹里运行命令、Python 脚本，比如画图、处理数据。",
					Example: "用 Python 画出 data.csv 第二列随时间的变化，存成 PNG", Safe: "每条命令都会先问你"}, false)
				if it.Need == "" && st.Commands == "off" {
					it.Need, it.Link = "“运行命令”在权限里被关掉了", toFolders
				}
				return it
			}(),
			func() capItem {
				it := needFolder(capItem{Key: "open", Title: "打开文件和网址", Desc: "用电脑上的默认程序打开文件或网址。", Example: "打开刚才编译好的 main.pdf"}, false)
				if it.Need == "" && st.Open == "off" {
					it.Need, it.Link = "“打开文件、网址”在权限里被关掉了", toFolders
				}
				return it
			}(),
		}},
		{Name: "记忆与技能", Icon: "i-award", Items: []capItem{
			{Key: "remember", Title: "记住你的习惯", Desc: "记住常用路径、编译方式、研究方向，下次不用再说。",
				Example: "记住：我的毕业论文在 D:\\论文\\毕业论文，用 XeLaTeX 编译", Safe: "不记密码和密钥；可以在“记忆”里查看和删除"},
			{Key: "use_skill", Title: "按技能里的步骤做事", Desc: "技能是写好的做事步骤（比如某类论文的检查清单），需要时智能体会自己读。",
				Example: "看看有哪些技能可以用来检查数学建模论文"},
		}},
	}
	for gi := range groups {
		for ii := range groups[gi].Items {
			it := &groups[gi].Items[ii]
			if !st.Model {
				it.Need, it.Link = "还没有可用的 AI 模型（设置 → 我的 AI 模型）", toSettings
			}
			it.Ready = it.Need == ""
		}
	}
	return groups
}

func (a *App) capStateFor(me *Me) capState {
	st := capState{Model: a.resolveModel(me, "").Configured(), Vision: a.resolveVision(me, "").Configured(), TeX: findTeX(false).Found}
	for _, f := range a.agentFolders(me) {
		st.Folders = true
		st.Writable = st.Writable || f.Write
	}
	_, key, _ := a.webSearchKey(me)
	st.WebKey = key != ""
	pol := a.agentPolicy(me)
	st.Commands, st.Open, st.Web = pol.Commands, pol.Open, pol.Web
	return st
}

// hAgentCapabilities 能力中心：内置能力（含是否可用）+ 已接入的外部工具
func (a *App) hAgentCapabilities(w http.ResponseWriter, r *http.Request, me *Me) error {
	local := isLoopback(r)
	groups := builtinCapabilities(a.capStateFor(me))
	total, ready := 0, 0
	for _, g := range groups {
		for _, it := range g.Items {
			total++
			if it.Ready {
				ready++
			}
		}
	}
	services := []map[string]any{}
	extTools := 0
	if local { // 外部工具服务只在运行工作台的电脑上显示和设置
		services = a.extView(me, r.URL.Query().Get("refresh") == "1")
		for _, s := range services {
			if s["enabled"] == true && s["ok"] == true {
				if ts, ok := s["tools"].([]map[string]any); ok {
					extTools += len(ts)
				}
			}
		}
	}
	writeJSON(w, 200, map[string]any{"local": local, "groups": groups, "services": services, "protocol": extProtocol,
		"counts": map[string]int{"builtin": total, "ready": ready, "external": extTools, "services": len(services)}})
	return nil
}

const extKitReadme = `CanDo 可为 · 外部工具开发包
================================

这个包是给合作开发者的：用任何语言（例如 Python）写工具，接进 CanDo 可为，交给智能体使用。
不需要看 CanDo 的代码，也不需要安装别的东西。

里面有什么
  · 外部工具接口.md      接口约定（协议 cando-tools/1）：只有两个地址，GET /tools 和 POST /tools/{name}
  · example_server.py    Python 示例，只用标准库。照着在里面加 @tool 函数就行
  · cando-tools.openapi.json   同一份约定的机器可读版（OpenAPI 3.0）。给程序和 AI 编程工具看，可以导入 Swagger、Postman 或用来生成代码
  · check_contract.py    按契约检查你的服务：python check_contract.py http://127.0.0.1:8765

三步跑起来
  1. 运行：python example_server.py        （默认 http://127.0.0.1:8765）
  2. 在 CanDo 可为里打开「AI 助手 → 能力中心」，点“接入工具服务”，
     短名填 fig，地址填 http://127.0.0.1:8765
  3. 能力中心里会出现 3 张工具卡片。点“用它”，或者直接对智能体说“把 3、5、2 画成柱状图”

加自己的工具
  在 example_server.py 里写一个函数，上面加 @tool(...)，重启服务。
  CanDo 会自动发现新工具（最多 20 秒），不用改 CanDo 的任何东西。
  建议给每个工具写一句 example（用户会说的话），它会显示在能力中心的卡片上。

分工
  CanDo 负责指挥、用户的文件、确认和撤销；工具服务只做好分配给它的那一件事。
  工具服务收不到对话和用户资料，也碰不到用户的磁盘：文件由 CanDo 读出后发来，
  生成的文件交回 CanDo，用户确认后才保存。
`

// hAgentExtKit 下载给合作者的开发包（接口约定 + Python 示例）
func (a *App) hAgentExtKit(w http.ResponseWriter, r *http.Request, me *Me) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	if err := add("CanDo外部工具开发包/先看这里.txt", []byte(strings.ReplaceAll(extKitReadme, "\n", "\r\n"))); err != nil {
		return err
	}
	for src, dst := range map[string]string{"docs/外部工具接口.md": "外部工具接口.md", "docs/cando-tools.openapi.json": "cando-tools.openapi.json",
		"tools/extool/example_server.py": "example_server.py", "tools/extool/check_contract.py": "check_contract.py"} {
		b, err := extKitFS.ReadFile(src)
		if err != nil {
			return err
		}
		if err := add("CanDo外部工具开发包/"+dst, b); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"cando-tools-kit.zip\"; filename*=UTF-8''"+url.PathEscape("CanDo外部工具开发包.zip"))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
	return nil
}
