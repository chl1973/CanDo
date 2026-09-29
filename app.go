package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

const AppVersion = "1.14.0"

type App struct {
	store        *Store
	quit         chan struct{}
	port         int
	controlToken string
	secret       []byte
	agentMu      sync.Mutex
	agents       map[string]*agentSession
}

type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return e.msg }

func errBad(msg string) error       { return &apiError{400, msg} }
func errForbidden(msg string) error { return &apiError{403, msg} }
func errNotFound(msg string) error  { return &apiError{404, msg} }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSON(w, ae.code, map[string]string{"detail": ae.msg})
		return
	}
	writeJSON(w, 500, map[string]string{"detail": "服务器内部错误：" + err.Error()})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	if err := dec.Decode(v); err != nil {
		return errBad("请求格式错误")
	}
	return nil
}

type handler func(w http.ResponseWriter, r *http.Request, me *Me) error

// auth 包装需要登录的接口。
func (a *App) auth(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := a.currentUser(r)
		if me == nil {
			writeJSON(w, 401, map[string]string{"detail": "未登录或登录已失效"})
			return
		}
		if err := h(w, r, me); err != nil {
			writeErr(w, err)
		}
	}
}

func (a *App) open(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeErr(w, err)
		}
	}
}

func (a *App) settings() Settings {
	var s Settings
	a.store.View(func(db *DB) { s = db.Settings })
	return s
}

// guard：局域网访问开关、防跨站请求、安全响应头。
func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; worker-src 'self' blob:; frame-src 'self' blob:; object-src 'self' blob:; connect-src 'self'")
		if !isLoopback(r) && !a.settings().LANEnabled {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(403)
			w.Write([]byte(`<meta charset="utf-8"><meta name="viewport" content="width=device-width"><div style="font:16px sans-serif;padding:24px;line-height:1.7"><h3>手机/他人访问尚未开启</h3>请在运行工作台的电脑上打开“设置 → 手机与同学访问”，开启后再扫码。</div>`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("X-KY") != "1" {
			writeJSON(w, 403, map[string]string{"detail": "请求被拒绝（缺少安全标记）"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

var mimeByExt = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8", ".json": "application/json", ".webmanifest": "application/manifest+json",
	".png": "image/png", ".svg": "image/svg+xml", ".ico": "image/x-icon", ".bcmap": "application/octet-stream",
	".pfb": "application/octet-stream", ".ttf": "font/ttf", ".woff2": "font/woff2", ".woff": "font/woff", ".apk": "application/vnd.android.package-archive",
}

func (a *App) static(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" || p == "." {
		p = "index.html"
	}
	sub, _ := fs.Sub(webFS, "web")
	b, err := fs.ReadFile(sub, p)
	if err != nil {
		// 前端使用 # 路由，其他路径一律 404
		http.NotFound(w, r)
		return
	}
	ct := mimeByExt[path.Ext(p)]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if path.Ext(p) == ".apk" {
		w.Header().Set("Content-Disposition", `attachment; filename="keyan-workbench.apk"`)
	}
	// 程序自身的页面、脚本、样式每次都重新验证，避免升级后浏览器仍使用旧版本；第三方库可以缓存
	if !strings.HasPrefix(p, "lib/") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.Write(b)
}

func (a *App) Routes() http.Handler { return a.guard(a.apiMux()) }

// apiMux 登记全部接口。每行末尾的注释是接口说明，docs/API.md 由 api_doc_test.go 据此自动生成。
func (a *App) apiMux() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", a.open(a.hHealth))                     // 检查程序是否在运行，返回版本号
	m.HandleFunc("POST /api/setup", a.open(a.hSetup))                      // 首次设置：创建管理员账号（只能在本机完成）
	m.HandleFunc("POST /api/login", a.open(a.hLogin))                      // 登录
	m.HandleFunc("POST /api/logout", a.auth(a.hLogout))                    // 退出登录
	m.HandleFunc("GET /api/me", a.auth(a.hMe))                             // 当前登录的用户
	m.HandleFunc("POST /api/me/password", a.auth(a.hChangePassword))       // 修改自己的密码
	m.HandleFunc("PATCH /api/me", a.auth(a.hPatchMe))                      // 修改自己的姓名和用户名
	m.HandleFunc("GET /api/users", a.auth(a.hListUsers))                   // 成员列表
	m.HandleFunc("POST /api/users", a.auth(a.hCreateUser))                 // 添加成员（管理员）
	m.HandleFunc("PATCH /api/users/{id}", a.auth(a.hUpdateUser))           // 修改成员（管理员）
	m.HandleFunc("GET /api/settings", a.auth(a.hGetSettings))              // 读取全组设置
	m.HandleFunc("PUT /api/settings", a.auth(a.hPutSettings))              // 保存全组设置（管理员）
	m.HandleFunc("POST /api/settings/test-llm", a.auth(a.hTestLLM))        // 测试团队模型能否连通（管理员）
	m.HandleFunc("GET /api/lan-info", a.auth(a.hLANInfo))                  // 手机访问的地址和开关状态
	m.HandleFunc("POST /api/quit", a.auth(a.hQuit))                        // 退出程序（管理员，只能在本机）
	m.HandleFunc("POST /api/local/control", a.open(a.hLocalControl))       // 本机控制（“忘记管理员密码”工具使用，需要令牌）
	m.HandleFunc("GET /api/admin/overview", a.auth(a.hAdminOverview))      // 后台总览（管理员）
	m.HandleFunc("GET /api/admin/backup", a.auth(a.hAdminBackup))          // 下载完整备份 zip（管理员）
	m.HandleFunc("POST /api/admin/open-data-dir", a.auth(a.hAdminOpenDir)) // 在电脑上打开数据文件夹（管理员，只能在本机）

	m.HandleFunc("GET /api/models", a.auth(a.hListModels))                   // 我的模型配置卡和团队模型
	m.HandleFunc("POST /api/models", a.auth(a.hCreateModel))                 // 新增模型配置卡
	m.HandleFunc("POST /api/models/team/check", a.auth(a.hCheckTeamModel))   // 自检团队默认模型（管理员）
	m.HandleFunc("PATCH /api/models/{id}", a.auth(a.hUpdateModel))           // 修改模型配置卡
	m.HandleFunc("DELETE /api/models/{id}", a.auth(a.hDeleteModel))          // 删除模型配置卡
	m.HandleFunc("POST /api/models/{id}/check", a.auth(a.hCheckModel))       // 模型兼容性自检
	m.HandleFunc("POST /api/models/{id}/activate", a.auth(a.hActivateModel)) // 启用或停用模型配置卡
	m.HandleFunc("GET /api/models/{id}/export", a.auth(a.hExportModel))      // 导出配置卡（不含密钥）

	m.HandleFunc("POST /api/papers/search", a.auth(a.hPaperSearch))                                           // 文献检索（OpenAlex，额度用完时改用 Crossref）
	m.HandleFunc("POST /api/papers/keywords", a.auth(a.hPaperKeywords))                                       // AI 根据研究问题拆检索词
	m.HandleFunc("POST /api/papers/import", a.auth(a.hPaperImport))                                           // 导入题录文件（RIS、EndNote、NoteExpress 等）
	m.HandleFunc("GET /api/papers/fetch-pdf", a.auth(a.hPaperFetchPDF))                                       // 下载开放获取论文的全文 PDF
	m.HandleFunc("POST /api/papers/save-record", a.auth(a.hPaperSaveRecord))                                  // 没有全文时收入题录和摘要
	m.HandleFunc("GET /api/papers/logs", a.auth(a.hPaperLogs))                                                // 检索记录
	m.HandleFunc("GET /api/scholar/status", a.auth(a.hScholarStatus))                                         // OpenAlex 密钥和今日剩余额度
	m.HandleFunc("PUT /api/scholar/key", a.auth(a.hScholarKey))                                               // 保存 OpenAlex 密钥（个人或团队）
	m.HandleFunc("GET /api/zotero/status", a.auth(a.hZoteroStatus))                                           // Zotero 连接状态（本机 / 云端）
	m.HandleFunc("PUT /api/zotero/cloud", a.auth(a.hZoteroCloudSave))                                         // 保存 Zotero 云端密钥
	m.HandleFunc("DELETE /api/zotero/cloud", a.auth(a.hZoteroCloudDelete))                                    // 断开 Zotero 云端
	m.HandleFunc("GET /api/zotero/libraries", a.auth(a.hZoteroLibraries))                                     // Zotero 文库列表
	m.HandleFunc("GET /api/zotero/collections", a.auth(a.hZoteroCollections))                                 // Zotero 分类列表
	m.HandleFunc("GET /api/zotero/items", a.auth(a.hZoteroItems))                                             // Zotero 条目（分页、筛选）
	m.HandleFunc("GET /api/zotero/pdf", a.auth(a.hZoteroPDF))                                                 // 读取 Zotero 条目的 PDF 附件
	m.HandleFunc("POST /api/zotero/log", a.auth(a.hZoteroLog))                                                // 记录一次 Zotero 导入
	m.HandleFunc("POST /api/zotero/save", a.auth(a.hZoteroSave))                                              // 把文献存进 Zotero
	m.HandleFunc("POST /api/latex/from-image", a.auth(a.hLatexFromImage))                                     // 图片转 LaTeX（公式、表格、段落、手写、批注）
	m.HandleFunc("POST /api/read/brief", a.auth(a.bg("AI 速读", a.hBrief)))                                     // AI 速读卡
	m.HandleFunc("POST /api/read/concept", a.auth(a.bg("概念讲解", a.hConcept)))                                  // 概念讲解
	m.HandleFunc("GET /api/read/cards", a.auth(a.hReadCards))                                                 // 一份资料已保存的速读卡和概念讲解
	m.HandleFunc("GET /api/usage", a.auth(a.hUsage))                                                          // 模型用量和花费（本人 / 全组）
	m.HandleFunc("POST /api/models/probe", a.auth(a.hProbeModels))                                            // 用接口地址和密钥查询可用模型（接入向导）
	m.HandleFunc("POST /api/latex/check", a.auth(a.hLatexCheck))                                              // LaTeX 静态检查
	m.HandleFunc("GET /api/latex/engine", a.auth(a.hTeXStatus))                                               // 检测本机的 LaTeX 编译器
	m.HandleFunc("POST /api/latex/tectonic", a.auth(a.localOnly(a.bg("下载 LaTeX 编译器", a.hTectonicInstall))))   // 一键下载便携 LaTeX（Tectonic）
	m.HandleFunc("POST /api/sketch", a.auth(a.hSketch))                                                       // 手绘草图转 TikZ 图
	m.HandleFunc("POST /api/sketch/refine", a.auth(a.hSketchRefine))                                          // 按一句话调整已有的图
	m.HandleFunc("POST /api/latex/compile", a.auth(a.localOnly(a.hTeXCompile)))                               // 编译 LaTeX 生成 PDF
	m.HandleFunc("GET /api/latex/pdf/{id}", a.auth(a.localOnly(a.hTeXPDF)))                                   // 取回编译好的 PDF
	m.HandleFunc("GET /api/agent/status", a.auth(a.hAgentStatus))                                             // 智能体是否可用（模型、授权文件夹）
	m.HandleFunc("PUT /api/agent/folders", a.auth(a.localOnly(a.hAgentFolders)))                              // 设置授权文件夹
	m.HandleFunc("GET /api/agent/sessions", a.auth(a.localOnly(a.hAgentList)))                                // 智能体对话列表
	m.HandleFunc("POST /api/agent/sessions", a.auth(a.localOnly(a.hAgentNew)))                                // 新建智能体对话
	m.HandleFunc("GET /api/agent/sessions/{id}", a.auth(a.localOnly(a.hAgentGet)))                            // 读取一次对话（消息、待确认的修改）
	m.HandleFunc("GET /api/agent/sessions/{id}/images/{n}", a.auth(a.localOnly(a.hAgentImage)))               // 对话里的第 n 张图片
	m.HandleFunc("POST /api/agent/sessions/{id}/messages", a.auth(a.localOnly(a.hAgentSend)))                 // 给智能体发消息
	m.HandleFunc("POST /api/agent/sessions/{id}/stop", a.auth(a.localOnly(a.hAgentStop)))                     // 停止智能体
	m.HandleFunc("POST /api/agent/sessions/{id}/changes/{cid}/{action}", a.auth(a.localOnly(a.hAgentChange))) // 处理一条待确认的修改（应用 / 放弃 / 撤销）
	m.HandleFunc("GET /api/agent/logs", a.auth(a.localOnly(a.hAgentLogs)))                                    // 智能体操作记录
	m.HandleFunc("POST /api/agent/sessions/{id}/approve", a.auth(a.localOnly(a.hAgentApprove)))               // 回应运行命令的确认（允许一次 / 始终允许 / 拒绝）
	m.HandleFunc("GET /api/agent/memory", a.auth(a.localOnly(a.hAgentMemory)))                                // 读取智能体记忆
	m.HandleFunc("PUT /api/agent/memory", a.auth(a.localOnly(a.hAgentMemory)))                                // 保存智能体记忆
	m.HandleFunc("GET /api/agent/skills", a.auth(a.localOnly(a.hSkills)))                                     // 技能列表
	m.HandleFunc("POST /api/agent/skills", a.auth(a.localOnly(a.hSkills)))                                    // 新建或修改技能
	m.HandleFunc("DELETE /api/agent/skills", a.auth(a.localOnly(a.hSkills)))                                  // 删除技能（?id=）
	m.HandleFunc("POST /api/agent/skills/import", a.auth(a.localOnly(a.hSkillImport)))                        // 从网址导入 SKILL.md
	m.HandleFunc("GET /api/agent/policy", a.auth(a.localOnly(a.hAgentPolicy)))                                // 读取智能体权限设置
	m.HandleFunc("PUT /api/agent/policy", a.auth(a.localOnly(a.hAgentPolicy)))                                // 保存智能体权限设置
	m.HandleFunc("GET /api/agent/tasks", a.auth(a.localOnly(a.hAgentTasks)))                                  // 定时任务列表
	m.HandleFunc("POST /api/agent/tasks", a.auth(a.localOnly(a.hAgentTasks)))                                 // 新建或修改定时任务
	m.HandleFunc("DELETE /api/agent/tasks", a.auth(a.localOnly(a.hAgentTasks)))                               // 删除定时任务（?id=）
	m.HandleFunc("POST /api/agent/tasks/{id}/run", a.auth(a.localOnly(a.hAgentTaskRun)))                      // 立即运行定时任务

	m.HandleFunc("GET /api/templates", a.auth(a.hListTemplates))           // 流程模板列表
	m.HandleFunc("POST /api/templates", a.auth(a.hCreateTemplate))         // 把项目流程另存为模板
	m.HandleFunc("DELETE /api/templates/{key}", a.auth(a.hDeleteTemplate)) // 删除模板

	m.HandleFunc("GET /api/home", a.auth(a.hHome))                                    // 首页总览（数字、待办、最近论文和草稿）
	m.HandleFunc("GET /api/projects", a.auth(a.hListProjects))                        // 项目列表
	m.HandleFunc("POST /api/projects", a.auth(a.hCreateProject))                      // 新建项目
	m.HandleFunc("GET /api/projects/{id}", a.auth(a.hGetProject))                     // 项目详情
	m.HandleFunc("PATCH /api/projects/{id}", a.auth(a.hUpdateProject))                // 修改项目（名称、成员、指定模型）
	m.HandleFunc("DELETE /api/projects/{id}", a.auth(a.hDeleteProject))               // 删除项目
	m.HandleFunc("GET /api/projects/{id}/activity", a.auth(a.hActivity))              // 项目动态
	m.HandleFunc("POST /api/projects/{id}/archive", a.auth(a.hArchive))               // 归档项目（写复盘）
	m.HandleFunc("POST /api/projects/{id}/unarchive", a.auth(a.hUnarchive))           // 取消归档
	m.HandleFunc("POST /api/projects/{id}/stages", a.auth(a.hAddStage))               // 添加阶段（老师）
	m.HandleFunc("PATCH /api/projects/{id}/stages/{sid}", a.auth(a.hEditStage))       // 修改阶段
	m.HandleFunc("DELETE /api/projects/{id}/stages/{sid}", a.auth(a.hDeleteStage))    // 删除阶段
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/move", a.auth(a.hMoveStage))   // 调整阶段顺序
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/check", a.auth(a.hCheckItem))  // 勾选清单项
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/start", a.auth(a.hStartStage)) // 开始阶段
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/submit", a.auth(a.hSubmit))    // 提交审核
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/review", a.auth(a.hReview))    // 老师审核（通过 / 退回）

	m.HandleFunc("GET /api/library", a.auth(a.hLibrary))          // 经验库（已归档并共享的项目）
	m.HandleFunc("GET /api/library/{id}", a.auth(a.hLibraryItem)) // 经验库中的一个项目

	m.HandleFunc("GET /api/materials", a.auth(a.hListMaterials))                              // 资料列表（项目 / 个人 / 全组共享）
	m.HandleFunc("POST /api/materials", a.auth(a.hUpload))                                    // 上传资料
	m.HandleFunc("GET /api/materials/{id}", a.auth(a.hGetMaterial))                           // 资料详情
	m.HandleFunc("PATCH /api/materials/{id}", a.auth(a.hPatchMaterial))                       // 修改资料信息、共享到全组
	m.HandleFunc("DELETE /api/materials/{id}", a.auth(a.hDeleteMaterial))                     // 删除资料
	m.HandleFunc("GET /api/materials/{id}/chunks", a.auth(a.hMaterialChunks))                 // 资料的全部片段
	m.HandleFunc("GET /api/materials/{id}/file", a.auth(a.hMaterialFile))                     // 下载资料原文件
	m.HandleFunc("POST /api/ocr/page", a.auth(a.hOCRPage))                                    // 识别扫描件一页的文字
	m.HandleFunc("GET /api/writing/profiles", a.auth(a.hWritingProfiles))                     // 论文类型（结构、写法、自查清单）
	m.HandleFunc("POST /api/writing/outline", a.auth(a.bg("AI 列提纲", a.hWritingOutline)))      // AI 带列提纲（不代写）
	m.HandleFunc("GET /api/writing/template", a.auth(a.hWritingTemplate))                     // 下载论文模板（Word / LaTeX）
	m.HandleFunc("POST /api/writing/check", a.auth(a.bg("格式检查", a.hWritingCheck)))            // 格式检查
	m.HandleFunc("GET /api/jobs", a.auth(a.hJobs))                                            // 我的后台任务
	m.HandleFunc("GET /api/jobs/{id}", a.auth(a.hJob))                                        // 查询一个后台任务（完成后带结果）
	m.HandleFunc("DELETE /api/jobs/{id}", a.auth(a.hJobDelete))                               // 移除后台任务
	m.HandleFunc("POST /api/writing/plan", a.auth(a.bg("生成论证骨架", a.hWritingPlan)))            // 生成论证骨架
	m.HandleFunc("POST /api/writing/draft", a.auth(a.bg("AI 起草", a.hWritingDraft)))           // AI 起草
	m.HandleFunc("GET /api/writing/drafts", a.auth(a.hWritingDrafts))                         // 草稿列表；带 ?id= 时读取一份
	m.HandleFunc("DELETE /api/writing/drafts", a.auth(a.hWritingDrafts))                      // 删除草稿（?id=）
	m.HandleFunc("POST /api/contracts/plan", a.auth(a.bg("生成论文契约", a.hContractPlan)))         // 生成论文契约
	m.HandleFunc("GET /api/contracts", a.auth(a.hContracts))                                  // 契约列表；带 ?id= 时读取一份
	m.HandleFunc("DELETE /api/contracts", a.auth(a.hContracts))                               // 删除契约（?id=）
	m.HandleFunc("GET /api/contracts.md", a.auth(a.hContractMarkdown))                        // 导出契约文档（Markdown）
	m.HandleFunc("PUT /api/contracts/{id}", a.auth(a.hContractUpdate))                        // 修改契约
	m.HandleFunc("POST /api/contracts/{id}/attack", a.auth(a.bg("审稿人攻击", a.hContractAttack))) // 审稿人攻击（提出质疑）
	m.HandleFunc("POST /api/contracts/{id}/rebut", a.auth(a.bg("判定回应", a.hContractRebut)))    // 回应质疑（服务端判定是否让步）
	m.HandleFunc("POST /api/contracts/{id}/resolve", a.auth(a.hContractResolve))              // 把质疑写进局限、保留为待解决或撤销
	m.HandleFunc("POST /api/contracts/{id}/confirm", a.auth(a.hContractConfirm))              // 确认契约（确认后锁定）
	m.HandleFunc("POST /api/contracts/{id}/unlock", a.auth(a.hContractUnlock))                // 解锁契约
	m.HandleFunc("GET /api/contracts/reviewers", a.auth(a.hContractReviewers))                // 可以请来审阅的老师
	m.HandleFunc("POST /api/contracts/{id}/review-request", a.auth(a.hContractReviewRequest)) // 请老师审阅契约
	m.HandleFunc("POST /api/contracts/{id}/review", a.auth(a.hContractReview))                // 老师审阅契约（通过 / 退回 / 评论）
	m.HandleFunc("GET /api/websearch/status", a.auth(a.hWebSearchStatus))                     // 联网搜索的密钥状态
	m.HandleFunc("PUT /api/websearch/key", a.auth(a.hWebSearchKey))                           // 保存联网搜索密钥（博查 / Tavily）
	m.HandleFunc("POST /api/writing/drift", a.auth(a.bg("偏离检查", a.hWritingDrift)))            // 对照契约检查草稿（偏离检查）
	m.HandleFunc("POST /api/writing/confirm", a.auth(a.hWritingConfirm))                      // 导出前人工确认
	m.HandleFunc("GET /api/writing/draft.docx", a.auth(a.hWritingDraftDocx))                  // 把草稿导出为 Word
	m.HandleFunc("GET /api/writing/draft.txt", a.auth(a.hWritingDraftText))                   // 草稿的纯文本（标题、段落、参考文献，复制用）
	m.HandleFunc("GET /api/research", a.auth(a.hResearchList))                                // 深度调研列表
	m.HandleFunc("POST /api/research", a.auth(a.hResearchStart))                              // 开始深度调研
	m.HandleFunc("GET /api/research/{id}", a.auth(a.hResearchGet))                            // 调研进度和报告
	m.HandleFunc("POST /api/research/{id}/stop", a.auth(a.hResearchStop))                     // 停止调研
	m.HandleFunc("DELETE /api/research/{id}", a.auth(a.hResearchDelete))                      // 删除调研
	m.HandleFunc("POST /api/materials/{id}/ocr", a.auth(a.hOCRSave))                          // 保存 OCR 识别出的页
	m.HandleFunc("GET /api/chunks/{id}", a.auth(a.hGetChunk))                                 // 读取一个片段（点开引用时用）

	m.HandleFunc("POST /api/ask", a.auth(a.bg("资料问答", a.hAsk)))         // 资料问答
	m.HandleFunc("POST /api/compare", a.auth(a.bg("资料对比", a.hCompare))) // 资料对比
	m.HandleFunc("GET /api/answers", a.auth(a.hListAnswers))            // 问答记录
	m.HandleFunc("GET /api/answers/{id}", a.auth(a.hGetAnswer))         // 读取一条问答
	m.HandleFunc("DELETE /api/answers/{id}", a.auth(a.hDeleteAnswer))   // 删除问答

	m.HandleFunc("POST /api/citechecks", a.auth(a.hCreateCiteCheck))         // 新建引用核验
	m.HandleFunc("GET /api/citechecks", a.auth(a.hListCiteChecks))           // 引用核验列表
	m.HandleFunc("GET /api/citechecks/{id}", a.auth(a.hGetCiteCheck))        // 引用核验报告
	m.HandleFunc("PUT /api/citechecks/{id}/mapping", a.auth(a.hCiteMapping)) // 手动关联参考文献与资料
	m.HandleFunc("POST /api/citechecks/{id}/run", a.auth(a.hRunCiteCheck))   // 运行引用核验
	m.HandleFunc("DELETE /api/citechecks/{id}", a.auth(a.hDeleteCiteCheck))  // 删除引用核验

	m.HandleFunc("GET /", a.static) // 网页界面（web/ 文件夹里的静态文件）
	return m
}

func now() time.Time { return time.Now() }
