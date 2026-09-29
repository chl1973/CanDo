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

func (a *App) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", a.open(a.hHealth))
	m.HandleFunc("POST /api/setup", a.open(a.hSetup))
	m.HandleFunc("POST /api/login", a.open(a.hLogin))
	m.HandleFunc("POST /api/logout", a.auth(a.hLogout))
	m.HandleFunc("GET /api/me", a.auth(a.hMe))
	m.HandleFunc("POST /api/me/password", a.auth(a.hChangePassword))
	m.HandleFunc("PATCH /api/me", a.auth(a.hPatchMe))
	m.HandleFunc("GET /api/users", a.auth(a.hListUsers))
	m.HandleFunc("POST /api/users", a.auth(a.hCreateUser))
	m.HandleFunc("PATCH /api/users/{id}", a.auth(a.hUpdateUser))
	m.HandleFunc("GET /api/settings", a.auth(a.hGetSettings))
	m.HandleFunc("PUT /api/settings", a.auth(a.hPutSettings))
	m.HandleFunc("POST /api/settings/test-llm", a.auth(a.hTestLLM))
	m.HandleFunc("GET /api/lan-info", a.auth(a.hLANInfo))
	m.HandleFunc("POST /api/quit", a.auth(a.hQuit))
	m.HandleFunc("POST /api/local/control", a.open(a.hLocalControl))
	m.HandleFunc("GET /api/admin/overview", a.auth(a.hAdminOverview))
	m.HandleFunc("GET /api/admin/backup", a.auth(a.hAdminBackup))
	m.HandleFunc("POST /api/admin/open-data-dir", a.auth(a.hAdminOpenDir))

	m.HandleFunc("GET /api/models", a.auth(a.hListModels))
	m.HandleFunc("POST /api/models", a.auth(a.hCreateModel))
	m.HandleFunc("POST /api/models/team/check", a.auth(a.hCheckTeamModel))
	m.HandleFunc("PATCH /api/models/{id}", a.auth(a.hUpdateModel))
	m.HandleFunc("DELETE /api/models/{id}", a.auth(a.hDeleteModel))
	m.HandleFunc("POST /api/models/{id}/check", a.auth(a.hCheckModel))
	m.HandleFunc("POST /api/models/{id}/activate", a.auth(a.hActivateModel))
	m.HandleFunc("GET /api/models/{id}/export", a.auth(a.hExportModel))

	m.HandleFunc("POST /api/papers/search", a.auth(a.hPaperSearch))
	m.HandleFunc("POST /api/papers/keywords", a.auth(a.hPaperKeywords))
	m.HandleFunc("POST /api/papers/import", a.auth(a.hPaperImport))
	m.HandleFunc("GET /api/papers/fetch-pdf", a.auth(a.hPaperFetchPDF))
	m.HandleFunc("POST /api/papers/save-record", a.auth(a.hPaperSaveRecord))
	m.HandleFunc("GET /api/papers/logs", a.auth(a.hPaperLogs))
	m.HandleFunc("GET /api/scholar/status", a.auth(a.hScholarStatus))
	m.HandleFunc("PUT /api/scholar/key", a.auth(a.hScholarKey))
	m.HandleFunc("GET /api/zotero/status", a.auth(a.hZoteroStatus))
	m.HandleFunc("PUT /api/zotero/cloud", a.auth(a.hZoteroCloudSave))
	m.HandleFunc("DELETE /api/zotero/cloud", a.auth(a.hZoteroCloudDelete))
	m.HandleFunc("GET /api/zotero/libraries", a.auth(a.hZoteroLibraries))
	m.HandleFunc("GET /api/zotero/collections", a.auth(a.hZoteroCollections))
	m.HandleFunc("GET /api/zotero/items", a.auth(a.hZoteroItems))
	m.HandleFunc("GET /api/zotero/pdf", a.auth(a.hZoteroPDF))
	m.HandleFunc("POST /api/zotero/log", a.auth(a.hZoteroLog))
	m.HandleFunc("POST /api/zotero/save", a.auth(a.hZoteroSave))
	m.HandleFunc("POST /api/latex/from-image", a.auth(a.hLatexFromImage))
	m.HandleFunc("POST /api/read/brief", a.auth(a.bg("AI 速读", a.hBrief)))
	m.HandleFunc("POST /api/read/concept", a.auth(a.bg("概念讲解", a.hConcept)))
	m.HandleFunc("GET /api/read/cards", a.auth(a.hReadCards))
	m.HandleFunc("GET /api/usage", a.auth(a.hUsage))
	m.HandleFunc("POST /api/models/probe", a.auth(a.hProbeModels))
	m.HandleFunc("POST /api/latex/check", a.auth(a.hLatexCheck))
	m.HandleFunc("GET /api/latex/engine", a.auth(a.hTeXStatus))
	m.HandleFunc("POST /api/latex/tectonic", a.auth(a.localOnly(a.bg("下载 LaTeX 编译器", a.hTectonicInstall))))
	m.HandleFunc("POST /api/sketch", a.auth(a.hSketch))
	m.HandleFunc("POST /api/sketch/refine", a.auth(a.hSketchRefine))
	m.HandleFunc("POST /api/latex/compile", a.auth(a.localOnly(a.hTeXCompile)))
	m.HandleFunc("GET /api/latex/pdf/{id}", a.auth(a.localOnly(a.hTeXPDF)))
	m.HandleFunc("GET /api/agent/status", a.auth(a.hAgentStatus))
	m.HandleFunc("PUT /api/agent/folders", a.auth(a.localOnly(a.hAgentFolders)))
	m.HandleFunc("GET /api/agent/sessions", a.auth(a.localOnly(a.hAgentList)))
	m.HandleFunc("POST /api/agent/sessions", a.auth(a.localOnly(a.hAgentNew)))
	m.HandleFunc("GET /api/agent/sessions/{id}", a.auth(a.localOnly(a.hAgentGet)))
	m.HandleFunc("GET /api/agent/sessions/{id}/images/{n}", a.auth(a.localOnly(a.hAgentImage)))
	m.HandleFunc("POST /api/agent/sessions/{id}/messages", a.auth(a.localOnly(a.hAgentSend)))
	m.HandleFunc("POST /api/agent/sessions/{id}/stop", a.auth(a.localOnly(a.hAgentStop)))
	m.HandleFunc("POST /api/agent/sessions/{id}/changes/{cid}/{action}", a.auth(a.localOnly(a.hAgentChange)))
	m.HandleFunc("GET /api/agent/logs", a.auth(a.localOnly(a.hAgentLogs)))
	m.HandleFunc("POST /api/agent/sessions/{id}/approve", a.auth(a.localOnly(a.hAgentApprove)))
	m.HandleFunc("GET /api/agent/memory", a.auth(a.localOnly(a.hAgentMemory)))
	m.HandleFunc("PUT /api/agent/memory", a.auth(a.localOnly(a.hAgentMemory)))
	m.HandleFunc("GET /api/agent/skills", a.auth(a.localOnly(a.hSkills)))
	m.HandleFunc("POST /api/agent/skills", a.auth(a.localOnly(a.hSkills)))
	m.HandleFunc("DELETE /api/agent/skills", a.auth(a.localOnly(a.hSkills)))
	m.HandleFunc("POST /api/agent/skills/import", a.auth(a.localOnly(a.hSkillImport)))
	m.HandleFunc("GET /api/agent/policy", a.auth(a.localOnly(a.hAgentPolicy)))
	m.HandleFunc("PUT /api/agent/policy", a.auth(a.localOnly(a.hAgentPolicy)))
	m.HandleFunc("GET /api/agent/tasks", a.auth(a.localOnly(a.hAgentTasks)))
	m.HandleFunc("POST /api/agent/tasks", a.auth(a.localOnly(a.hAgentTasks)))
	m.HandleFunc("DELETE /api/agent/tasks", a.auth(a.localOnly(a.hAgentTasks)))
	m.HandleFunc("POST /api/agent/tasks/{id}/run", a.auth(a.localOnly(a.hAgentTaskRun)))

	m.HandleFunc("GET /api/templates", a.auth(a.hListTemplates))
	m.HandleFunc("POST /api/templates", a.auth(a.hCreateTemplate))
	m.HandleFunc("DELETE /api/templates/{key}", a.auth(a.hDeleteTemplate))

	m.HandleFunc("GET /api/home", a.auth(a.hHome))
	m.HandleFunc("GET /api/projects", a.auth(a.hListProjects))
	m.HandleFunc("POST /api/projects", a.auth(a.hCreateProject))
	m.HandleFunc("GET /api/projects/{id}", a.auth(a.hGetProject))
	m.HandleFunc("PATCH /api/projects/{id}", a.auth(a.hUpdateProject))
	m.HandleFunc("DELETE /api/projects/{id}", a.auth(a.hDeleteProject))
	m.HandleFunc("GET /api/projects/{id}/activity", a.auth(a.hActivity))
	m.HandleFunc("POST /api/projects/{id}/archive", a.auth(a.hArchive))
	m.HandleFunc("POST /api/projects/{id}/unarchive", a.auth(a.hUnarchive))
	m.HandleFunc("POST /api/projects/{id}/stages", a.auth(a.hAddStage))
	m.HandleFunc("PATCH /api/projects/{id}/stages/{sid}", a.auth(a.hEditStage))
	m.HandleFunc("DELETE /api/projects/{id}/stages/{sid}", a.auth(a.hDeleteStage))
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/move", a.auth(a.hMoveStage))
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/check", a.auth(a.hCheckItem))
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/start", a.auth(a.hStartStage))
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/submit", a.auth(a.hSubmit))
	m.HandleFunc("POST /api/projects/{id}/stages/{sid}/review", a.auth(a.hReview))

	m.HandleFunc("GET /api/library", a.auth(a.hLibrary))
	m.HandleFunc("GET /api/library/{id}", a.auth(a.hLibraryItem))

	m.HandleFunc("GET /api/materials", a.auth(a.hListMaterials))
	m.HandleFunc("POST /api/materials", a.auth(a.hUpload))
	m.HandleFunc("GET /api/materials/{id}", a.auth(a.hGetMaterial))
	m.HandleFunc("PATCH /api/materials/{id}", a.auth(a.hPatchMaterial))
	m.HandleFunc("DELETE /api/materials/{id}", a.auth(a.hDeleteMaterial))
	m.HandleFunc("GET /api/materials/{id}/chunks", a.auth(a.hMaterialChunks))
	m.HandleFunc("GET /api/materials/{id}/file", a.auth(a.hMaterialFile))
	m.HandleFunc("POST /api/ocr/page", a.auth(a.hOCRPage))
	m.HandleFunc("GET /api/writing/profiles", a.auth(a.hWritingProfiles))
	m.HandleFunc("POST /api/writing/outline", a.auth(a.bg("AI 列提纲", a.hWritingOutline)))
	m.HandleFunc("GET /api/writing/template", a.auth(a.hWritingTemplate))
	m.HandleFunc("POST /api/writing/check", a.auth(a.bg("格式检查", a.hWritingCheck)))
	m.HandleFunc("GET /api/jobs", a.auth(a.hJobs))
	m.HandleFunc("GET /api/jobs/{id}", a.auth(a.hJob))
	m.HandleFunc("DELETE /api/jobs/{id}", a.auth(a.hJobDelete))
	m.HandleFunc("POST /api/writing/plan", a.auth(a.bg("生成论证骨架", a.hWritingPlan)))
	m.HandleFunc("POST /api/writing/draft", a.auth(a.bg("AI 起草", a.hWritingDraft)))
	m.HandleFunc("GET /api/writing/drafts", a.auth(a.hWritingDrafts))
	m.HandleFunc("DELETE /api/writing/drafts", a.auth(a.hWritingDrafts))
	m.HandleFunc("POST /api/contracts/plan", a.auth(a.bg("生成论文契约", a.hContractPlan)))
	m.HandleFunc("GET /api/contracts", a.auth(a.hContracts))
	m.HandleFunc("DELETE /api/contracts", a.auth(a.hContracts))
	m.HandleFunc("GET /api/contracts.md", a.auth(a.hContractMarkdown))
	m.HandleFunc("PUT /api/contracts/{id}", a.auth(a.hContractUpdate))
	m.HandleFunc("POST /api/contracts/{id}/attack", a.auth(a.bg("审稿人攻击", a.hContractAttack)))
	m.HandleFunc("POST /api/contracts/{id}/rebut", a.auth(a.bg("判定回应", a.hContractRebut)))
	m.HandleFunc("POST /api/contracts/{id}/resolve", a.auth(a.hContractResolve))
	m.HandleFunc("POST /api/contracts/{id}/confirm", a.auth(a.hContractConfirm))
	m.HandleFunc("POST /api/contracts/{id}/unlock", a.auth(a.hContractUnlock))
	m.HandleFunc("GET /api/contracts/reviewers", a.auth(a.hContractReviewers))
	m.HandleFunc("POST /api/contracts/{id}/review-request", a.auth(a.hContractReviewRequest))
	m.HandleFunc("POST /api/contracts/{id}/review", a.auth(a.hContractReview))
	m.HandleFunc("GET /api/websearch/status", a.auth(a.hWebSearchStatus))
	m.HandleFunc("PUT /api/websearch/key", a.auth(a.hWebSearchKey))
	m.HandleFunc("POST /api/writing/drift", a.auth(a.bg("偏离检查", a.hWritingDrift)))
	m.HandleFunc("POST /api/writing/confirm", a.auth(a.hWritingConfirm))
	m.HandleFunc("GET /api/writing/draft.docx", a.auth(a.hWritingDraftDocx))
	m.HandleFunc("GET /api/writing/draft.txt", a.auth(a.hWritingDraftText))
	m.HandleFunc("GET /api/research", a.auth(a.hResearchList))
	m.HandleFunc("POST /api/research", a.auth(a.hResearchStart))
	m.HandleFunc("GET /api/research/{id}", a.auth(a.hResearchGet))
	m.HandleFunc("POST /api/research/{id}/stop", a.auth(a.hResearchStop))
	m.HandleFunc("DELETE /api/research/{id}", a.auth(a.hResearchDelete))
	m.HandleFunc("POST /api/materials/{id}/ocr", a.auth(a.hOCRSave))
	m.HandleFunc("GET /api/chunks/{id}", a.auth(a.hGetChunk))

	m.HandleFunc("POST /api/ask", a.auth(a.bg("资料问答", a.hAsk)))
	m.HandleFunc("POST /api/compare", a.auth(a.bg("资料对比", a.hCompare)))
	m.HandleFunc("GET /api/answers", a.auth(a.hListAnswers))
	m.HandleFunc("GET /api/answers/{id}", a.auth(a.hGetAnswer))
	m.HandleFunc("DELETE /api/answers/{id}", a.auth(a.hDeleteAnswer))

	m.HandleFunc("POST /api/citechecks", a.auth(a.hCreateCiteCheck))
	m.HandleFunc("GET /api/citechecks", a.auth(a.hListCiteChecks))
	m.HandleFunc("GET /api/citechecks/{id}", a.auth(a.hGetCiteCheck))
	m.HandleFunc("PUT /api/citechecks/{id}/mapping", a.auth(a.hCiteMapping))
	m.HandleFunc("POST /api/citechecks/{id}/run", a.auth(a.hRunCiteCheck))
	m.HandleFunc("DELETE /api/citechecks/{id}", a.auth(a.hDeleteCiteCheck))

	m.HandleFunc("GET /", a.static)
	return a.guard(m)
}

func now() time.Time { return time.Now() }
