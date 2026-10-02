# CanDo 可为 · 接口清单

前端（`web/` 里的网页）和后端（Go 程序）之间只通过下面这些接口来往：前端不含后端逻辑，后端不生成网页。所以只要接口不变，后端内部怎么改都不用动界面，界面怎么改也不用动后端。

## 约定

- **地址**都以 `/api/` 开头。数据用 JSON；上传文件用 multipart 表单。
- **方法**：GET 读取（不改数据）；POST 新建或执行一个动作；PUT 整体保存；PATCH 只改部分字段；DELETE 删除。浏览器直接打开的链接（下载、预览 PDF）只能是 GET。
- **登录**：登录后用 Cookie 保持会话（也接受 `Authorization: Bearer <令牌>`）。没登录时返回 401。标“免登录”的接口除外。
- **安全标记**：GET 以外的请求都要带请求头 `X-KY: 1`，否则返回 403。这是为了防止别的网站冒充用户发请求。
- **出错**：返回 `{"detail": "给人看的中文说明"}`。状态码：400 参数不对；401 没登录；403 不允许；404 不存在或无权访问（两种情况故意不区分）；500 程序出错。
- **可后台运行**：请求头带 `X-KY-Async: 1` 时立即返回 `202 {"job_id": "…", "status": "queued"}`，处理在后台继续；用 `GET /api/jobs/{id}` 取结果。不带这个请求头时照常等结果返回。
- **仅本机**：只能在运行 CanDo 的电脑上调用，手机和局域网里的其他设备会收到 403。
- **手机访问开关**：“设置 → 手机与同学访问”关闭时，除本机以外的所有请求都返回 403。

## 怎么维护

- 接口在 `app.go` 的 `apiMux` 里登记，**每行末尾写一句说明**（`// 说明`），没写说明时测试不通过。
- 改了接口后运行 `UPDATE_API_DOC=1 go test -run TestAPIDoc .`，重新生成下面的表。GitHub 自动测试会检查这张表是不是最新的。
- `TestFrontendAPICalls` 会检查 `web/` 里用到的每个接口后端都有，方法和路径都对得上。
- 表里的“请求”一列是从代码里自动找出来的 JSON 字段、查询参数和表单字段，个别写法可能找不全，以处理函数为准；“返回”只区分 JSON 和文件，具体字段看“代码”一列里的处理函数。
- 改接口时尽量只加不减：加字段没问题；删字段、改字段含义时，要同时改前端，并在 PR 里写明。

<!-- 接口表开始：以下由 go test 自动生成，不要手改 -->

共 170 个接口（GET 65，POST 73，PUT 11，PATCH 6，DELETE 15）。

## 账号、设置与系统

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/health` | 检查程序是否在运行，返回版本号 | 免登录 | — | JSON | `api_system.go` hHealth |
| POST | `/api/setup` | 首次设置：创建管理员账号（只能在本机完成） | 免登录 | JSON：`org_name` string、`OrgName` string、`username` string、`name` string、`password` string | JSON | `api_system.go` hSetup |
| POST | `/api/login` | 登录 | 免登录 | JSON：`username` string、`password` string | JSON | `api_system.go` hLogin |
| POST | `/api/logout` | 退出登录 | 登录 | — | JSON | `api_system.go` hLogout |
| GET | `/api/me` | 当前登录的用户 | 登录 | — | JSON | `api_system.go` hMe |
| POST | `/api/me/password` | 修改自己的密码 | 登录 | JSON：`old` string、`new` string | JSON | `api_system.go` hChangePassword |
| PATCH | `/api/me` | 修改自己的姓名和用户名 | 登录 | JSON：`name` string、`username` string | JSON | `api_system.go` hPatchMe |
| GET | `/api/users` | 成员列表 | 登录 | — | JSON | `api_system.go` hListUsers |
| POST | `/api/users` | 添加成员（管理员） | 登录 | JSON：`username` string、`name` string、`role` string、`password` string | JSON | `api_system.go` hCreateUser |
| PATCH | `/api/users/{id}` | 修改成员（管理员） | 登录 | JSON：`name` string、`role` string、`disabled` bool、`password` string | JSON | `api_system.go` hUpdateUser |
| GET | `/api/settings` | 读取全组设置 | 登录 | — | JSON | `api_system.go` hGetSettings |
| PUT | `/api/settings` | 保存全组设置（管理员） | 登录 | JSON：`org_name` string、`llm_base_url` string、`llm_model` string、`llm_key` string、`llm_protocol` string、`llm_name` string、`llm_vision` bool、`llm_strong_model` string、`llm_price_in` float64、`llm_price_out` float64、`llm_strong_price_in` float64、`llm_strong_price_out` float64、`team_budget` float64、`clear_key` bool、`lan_enabled` bool、`online_check` bool | JSON | `api_system.go` hPutSettings |
| POST | `/api/settings/test-llm` | 测试团队模型能否连通（管理员） | 登录 | — | JSON | `api_system.go` hTestLLM |
| GET | `/api/lan-info` | 手机访问的地址和开关状态 | 登录 | — | JSON | `api_system.go` hLANInfo |
| POST | `/api/quit` | 退出程序（管理员，只能在本机） | 登录 | — | JSON | `api_system.go` hQuit |
| POST | `/api/local/control` | 本机控制（“忘记管理员密码”工具使用，需要令牌） | 免登录 | JSON：`token` string、`action` string | JSON | `admin.go` hLocalControl |

## 管理后台

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/admin/overview` | 后台总览（管理员） | 登录 | — | JSON | `admin.go` hAdminOverview |
| GET | `/api/admin/backup` | 下载完整备份 zip（管理员） | 登录 | — | 文件 | `admin.go` hAdminBackup |
| POST | `/api/admin/open-data-dir` | 在电脑上打开数据文件夹（管理员，只能在本机） | 登录 | — | JSON | `admin.go` hAdminOpenDir |

## 首页

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/home` | 首页总览（数字、待办、最近论文和草稿） | 登录 | — | JSON | `home.go` hHome |

## 项目与流程模板

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/templates` | 流程模板列表 | 登录 | — | JSON | `api_projects.go` hListTemplates |
| POST | `/api/templates` | 把项目流程另存为模板 | 登录 | JSON：`project_id` string、`name` string、`desc` string | JSON | `api_projects.go` hCreateTemplate |
| DELETE | `/api/templates/{key}` | 删除模板 | 登录 | — | JSON | `api_projects.go` hDeleteTemplate |
| GET | `/api/projects` | 项目列表 | 登录 | — | JSON | `api_projects.go` hListProjects |
| POST | `/api/projects` | 新建项目 | 登录 | JSON：`name` string、`desc` string、`template_key` string、`from_project_id` string、`members` []int、`advisors` []int | JSON | `api_projects.go` hCreateProject |
| GET | `/api/projects/{id}` | 项目详情 | 登录 | — | JSON | `api_projects.go` hGetProject |
| PATCH | `/api/projects/{id}` | 修改项目（名称、成员、指定模型） | 登录 | JSON：`name` string、`desc` string、`members` []int、`advisors` []int、`model_profile_id` string | JSON | `api_projects.go` hUpdateProject |
| DELETE | `/api/projects/{id}` | 删除项目 | 登录 | — | JSON | `api_projects.go` hDeleteProject |
| GET | `/api/projects/{id}/activity` | 项目动态 | 登录 | — | JSON | `api_projects.go` hActivity |
| POST | `/api/projects/{id}/archive` | 归档项目（写复盘） | 登录 | JSON：`good` string、`pitfalls` string、`advice` string、`share` bool | JSON | `api_projects.go` hArchive |
| POST | `/api/projects/{id}/unarchive` | 取消归档 | 登录 | — | JSON | `api_projects.go` hUnarchive |
| POST | `/api/projects/{id}/stages` | 添加阶段（老师） | 登录 | JSON：`name` string、`goal` string、`guide` string、`checklist` []string、`after` int | JSON | `api_projects.go` hAddStage |
| PATCH | `/api/projects/{id}/stages/{sid}` | 修改阶段 | 登录 | JSON：`name` string、`goal` string、`guide` string、`due` string、`checklist` []string | JSON | `api_projects.go` hEditStage |
| DELETE | `/api/projects/{id}/stages/{sid}` | 删除阶段 | 登录 | — | JSON | `api_projects.go` hDeleteStage |
| POST | `/api/projects/{id}/stages/{sid}/move` | 调整阶段顺序 | 登录 | JSON：`dir` int | JSON | `api_projects.go` hMoveStage |
| POST | `/api/projects/{id}/stages/{sid}/check` | 勾选清单项 | 登录 | JSON：`index` int、`done` bool | JSON | `api_projects.go` hCheckItem |
| POST | `/api/projects/{id}/stages/{sid}/start` | 开始阶段 | 登录 | — | JSON | `api_projects.go` hStartStage |
| POST | `/api/projects/{id}/stages/{sid}/submit` | 提交审核 | 登录 | JSON：`content` string、`material_ids` []string、`request_review` bool | JSON | `api_projects.go` hSubmit |
| POST | `/api/projects/{id}/stages/{sid}/review` | 老师审核（通过 / 退回） | 登录 | JSON：`decision` string、`comment` string | JSON | `api_projects.go` hReview |

## 经验库

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/library` | 经验库（已归档并共享的项目） | 登录 | — | JSON | `api_projects.go` hLibrary |
| GET | `/api/library/{id}` | 经验库中的一个项目 | 登录 | — | JSON | `api_projects.go` hLibraryItem |

## 资料库与 OCR

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/materials` | 资料列表（项目 / 个人 / 全组共享） | 登录 | 查询：`project_id`、`scope` | JSON | `api_materials.go` hListMaterials |
| POST | `/api/materials` | 上传资料 | 登录 | 上传文件（multipart）<br>表单：`project_id`、`title`、`pages`、`author`、`source_date`、`doi`、`source_url`、`zotero_key`、`search_log_id` | JSON | `api_materials.go` hUpload |
| GET | `/api/materials/{id}` | 资料详情 | 登录 | — | JSON | `api_materials.go` hGetMaterial |
| PATCH | `/api/materials/{id}` | 修改资料信息、共享到全组 | 登录 | JSON：`author` string、`source_date` string、`shared` bool | JSON | `api_materials.go` hPatchMaterial |
| DELETE | `/api/materials/{id}` | 删除资料 | 登录 | 查询：`delete_answers` | JSON | `api_materials.go` hDeleteMaterial |
| GET | `/api/materials/{id}/chunks` | 资料的全部片段 | 登录 | — | JSON | `api_materials.go` hMaterialChunks |
| GET | `/api/materials/{id}/file` | 下载资料原文件 | 登录 | — | 文件 | `api_materials.go` hMaterialFile |
| POST | `/api/ocr/page` | 识别扫描件一页的文字 | 登录 | JSON：`image` string、`project_id` string、`lang` string | JSON | `ocr.go` hOCRPage |
| POST | `/api/materials/{id}/ocr` | 保存 OCR 识别出的页 | 登录 | JSON：`total_pages` int、`pages` []PDFPage、`model` string | JSON | `ocr.go` hOCRSave |
| GET | `/api/chunks/{id}` | 读取一个片段（点开引用时用） | 登录 | — | JSON | `api_materials.go` hGetChunk |

## 资料问答与对比

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| POST | `/api/ask` | 资料问答 | 登录、可后台运行 | JSON：`material_ids` []string、`question` string、`project_id` string、`effort` string | JSON | `qa.go` hAsk |
| POST | `/api/compare` | 资料对比 | 登录、可后台运行 | JSON：`material_a` string、`material_b` string、`question` string、`project_id` string、`effort` string | JSON | `qa.go` hCompare |
| GET | `/api/answers` | 问答记录 | 登录 | 查询：`project_id` | JSON | `qa.go` hListAnswers |
| GET | `/api/answers/{id}` | 读取一条问答 | 登录 | — | JSON | `qa.go` hGetAnswer |
| DELETE | `/api/answers/{id}` | 删除问答 | 登录 | — | JSON | `qa.go` hDeleteAnswer |

## 引用核验

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| POST | `/api/citechecks` | 新建引用核验 | 登录 | JSON：`title` string、`text` string、`project_id` string | JSON | `citecheck.go` hCreateCiteCheck |
| GET | `/api/citechecks` | 引用核验列表 | 登录 | 查询：`project_id` | JSON | `citecheck.go` hListCiteChecks |
| GET | `/api/citechecks/{id}` | 引用核验报告 | 登录 | — | JSON | `citecheck.go` hGetCiteCheck |
| PUT | `/api/citechecks/{id}/mapping` | 手动关联参考文献与资料 | 登录 | JSON：键值对 map[string]string | JSON | `citecheck.go` hCiteMapping |
| POST | `/api/citechecks/{id}/run` | 运行引用核验 | 登录 | — | JSON | `citecheck.go` hRunCiteCheck |
| DELETE | `/api/citechecks/{id}` | 删除引用核验 | 登录 | — | JSON | `citecheck.go` hDeleteCiteCheck |

## AI 读文献

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| POST | `/api/read/brief` | AI 速读卡 | 登录、可后台运行 | JSON：`material_id` string、`effort` string、`refresh` bool | JSON | `read.go` hBrief |
| POST | `/api/read/concept` | 概念讲解 | 登录、可后台运行 | JSON：`material_id` string、`term` string、`context` string、`effort` string、`refresh` bool | JSON | `read.go` hConcept |
| GET | `/api/read/cards` | 一份资料已保存的速读卡和概念讲解 | 登录 | 查询：`material_id` | JSON | `read.go` hReadCards |

## 文献检索

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| POST | `/api/papers/search` | 文献检索（OpenAlex，额度用完时改用 Crossref） | 登录 | JSON：`query` string、`year_from` int、`year_to` int、`oa_only` bool、`sort` string、`page` int、`project_id` string | JSON | `papers.go` hPaperSearch |
| POST | `/api/papers/keywords` | AI 根据研究问题拆检索词 | 登录 | JSON：`question` string、`project_id` string | JSON | `papers.go` hPaperKeywords |
| POST | `/api/papers/import` | 导入题录文件（RIS、EndNote、NoteExpress 等） | 登录 | 上传文件（multipart）<br>表单：`project_id` | JSON | `papers.go` hPaperImport |
| GET | `/api/papers/fetch-pdf` | 下载开放获取论文的全文 PDF | 登录 | 查询：`url` | 文件 | `papers.go` hPaperFetchPDF |
| POST | `/api/papers/save-record` | 没有全文时收入题录和摘要 | 登录 | JSON：`paper` Paper、`project_id` string、`search_log_id` string、`zotero_key` string | JSON | `papers.go` hPaperSaveRecord |
| GET | `/api/papers/logs` | 检索记录 | 登录 | 查询：`project_id` | JSON | `papers.go` hPaperLogs |
| GET | `/api/scholar/status` | OpenAlex 密钥和今日剩余额度 | 登录 | — | JSON | `scholar.go` hScholarStatus |
| PUT | `/api/scholar/key` | 保存 OpenAlex 密钥（个人或团队） | 登录 | JSON：`scope` string、`key` string、`contact_email` string | JSON | `scholar.go` hScholarKey |

## Zotero

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/zotero/status` | Zotero 连接状态（本机 / 云端） | 登录 | — | JSON | `zotero.go` hZoteroStatus |
| PUT | `/api/zotero/cloud` | 保存 Zotero 云端密钥 | 登录 | JSON：`key` string | JSON | `zotero.go` hZoteroCloudSave |
| DELETE | `/api/zotero/cloud` | 断开 Zotero 云端 | 登录 | — | JSON | `zotero.go` hZoteroCloudDelete |
| GET | `/api/zotero/libraries` | Zotero 文库列表 | 登录 | 查询：`src` | JSON | `zotero.go` hZoteroLibraries |
| GET | `/api/zotero/collections` | Zotero 分类列表 | 登录 | 查询：`src`、`lib` | JSON | `zotero.go` hZoteroCollections |
| GET | `/api/zotero/items` | Zotero 条目（分页、筛选） | 登录 | 查询：`project_id`、`src`、`lib`、`collection`、`start`、`q` | JSON | `zotero.go` hZoteroItems |
| GET | `/api/zotero/pdf` | 读取 Zotero 条目的 PDF 附件 | 登录 | 查询：`src`、`lib`、`key` | 文件 | `zotero.go` hZoteroPDF |
| POST | `/api/zotero/log` | 记录一次 Zotero 导入 | 登录 | JSON：`project_id` string、`src` string、`library` string、`collection` string、`query` string、`count` int | JSON | `zotero.go` hZoteroLog |
| POST | `/api/zotero/save` | 把文献存进 Zotero | 登录 | JSON：`src` string、`lib` string、`collection` string、`papers` []Paper | JSON | `zotero.go` hZoteroSave |

## 深度调研

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/research` | 深度调研列表 | 登录 | 查询：`project_id` | JSON | `research.go` hResearchList |
| POST | `/api/research` | 开始深度调研 | 登录 | JSON：`question` string、`mode` string、`columns` []string、`project_id` string、`year_from` int、`max_papers` int、`max_rounds` int、`queries` []string | JSON | `research.go` hResearchStart |
| GET | `/api/research/{id}` | 调研进度和报告 | 登录 | — | JSON | `research.go` hResearchGet |
| POST | `/api/research/{id}/stop` | 停止调研 | 登录 | — | JSON | `research.go` hResearchStop |
| DELETE | `/api/research/{id}` | 删除调研 | 登录 | — | JSON | `research.go` hResearchDelete |

## 写论文

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/writing/profiles` | 论文类型（结构、写法、自查清单） | 登录 | — | JSON | `writing.go` hWritingProfiles |
| POST | `/api/writing/outline` | AI 带列提纲（不代写） | 登录、可后台运行 | JSON：`profile` string、`topic` string、`notes` string、`project_id` string | JSON | `writing.go` hWritingOutline |
| GET | `/api/writing/template` | 下载论文模板（Word / LaTeX） | 登录 | 查询：`profile`、`format` | 文件 | `writing.go` hWritingTemplate |
| POST | `/api/writing/check` | 格式检查 | 登录、可后台运行 | 上传文件（multipart）<br>表单：`profile`、`text`、`ai`、`project_id` | JSON | `doccheck.go` hWritingCheck |
| POST | `/api/writing/plan` | 生成论证骨架 | 登录、可后台运行 | JSON：`profile` string、`section` string、`lang` string、`idea` string、`results` string、`material_ids` []string、`project_id` string、`contract_id` string | JSON | `writing_ai.go` hWritingPlan |
| POST | `/api/writing/draft` | AI 起草 | 登录、可后台运行 | JSON：`id` string、`claims` []WClaim、`length` string | JSON | `writing_ai.go` hWritingDraft |
| GET | `/api/writing/drafts` | 草稿列表；带 ?id= 时读取一份 | 登录 | 查询：`id` | JSON | `writing_ai.go` hWritingDrafts |
| DELETE | `/api/writing/drafts` | 删除草稿（?id=） | 登录 | 查询：`id` | JSON | `writing_ai.go` hWritingDrafts |
| POST | `/api/writing/drift` | 对照契约检查草稿（偏离检查） | 登录、可后台运行 | JSON：`id` string | JSON | `drift.go` hWritingDrift |
| POST | `/api/writing/confirm` | 导出前人工确认 | 登录 | JSON：`id` string、`checks` map[string]bool | JSON | `drift.go` hWritingConfirm |
| GET | `/api/writing/draft.docx` | 把草稿导出为 Word | 登录 | 查询：`id` | 文件 | `writing_ai.go` hWritingDraftDocx |
| GET | `/api/writing/draft.txt` | 草稿的纯文本（标题、段落、参考文献，复制用） | 登录 | 查询：`id` | JSON | `writing_ai.go` hWritingDraftText |
| GET | `/api/writing/full` | 全文列表；带 ?id= 时读取一篇（各节、可选草稿、规则检查、预览） | 登录 | 查询：`id` | JSON | `fullpaper.go` hFullPapers |
| POST | `/api/writing/full` | 新建全文（按论文类型或论文契约，自动选上各节草稿） | 登录 | 查询：`id`<br>JSON：`profile` string、`lang` string、`title` string、`contract_id` string | JSON | `fullpaper.go` hFullPapers |
| DELETE | `/api/writing/full` | 删除全文（?id=） | 登录 | 查询：`id` | JSON | `fullpaper.go` hFullPapers |
| PUT | `/api/writing/full/{id}` | 保存全文的题目、关键词和各节选用的草稿 | 登录 | JSON：`title` string、`keywords` []string、`parts` []WPart | JSON | `fullpaper.go` hFullPaperUpdate |
| POST | `/api/writing/full/{id}/confirm` | 全文导出前确认 | 登录 | JSON：`checks` map[string]bool | JSON | `fullpaper.go` hFullPaperConfirm |
| GET | `/api/writing/full/{id}/docx` | 导出全文 Word（须无错误并已确认） | 登录 | — | 文件 | `fullpaper.go` hFullPaperDocx |
| GET | `/api/writing/full/{id}/tex` | 导出全文 LaTeX（须无错误并已确认） | 登录 | — | 文件 | `fullpaper.go` hFullPaperTex |
| POST | `/api/writing/full/pdf` | 用本机 LaTeX 把全文编译成 PDF（仅本机） | 登录、仅本机、可后台运行 | JSON：`id` string | JSON | `fullpaper.go` hFullPaperPDF |

## 论文契约

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| POST | `/api/contracts/plan` | 生成论文契约 | 登录、可后台运行 | JSON：`profile` string、`lang` string、`title` string、`idea` string、`results` string、`material_ids` []string、`project_id` string | JSON | `contract.go` hContractPlan |
| GET | `/api/contracts` | 契约列表；带 ?id= 时读取一份 | 登录 | 查询：`id`、`review` | JSON | `contract.go` hContracts |
| DELETE | `/api/contracts` | 删除契约（?id=） | 登录 | 查询：`id`、`review` | JSON | `contract.go` hContracts |
| GET | `/api/contracts.md` | 导出契约文档（Markdown） | 登录 | 查询：`id` | 文件 | `contract.go` hContractMarkdown |
| PUT | `/api/contracts/{id}` | 修改契约 | 登录 | JSON：`title` string、`question` CQuestion、`claims` []CClaim、`sections` []CSection、`add_notes` []string | JSON | `contract.go` hContractUpdate |
| POST | `/api/contracts/{id}/attack` | 审稿人攻击（提出质疑） | 登录、可后台运行 | — | JSON | `contract.go` hContractAttack |
| POST | `/api/contracts/{id}/rebut` | 回应质疑（服务端判定是否让步） | 登录、可后台运行 | JSON：`qid` string、`text` string | JSON | `contract.go` hContractRebut |
| POST | `/api/contracts/{id}/resolve` | 把质疑写进局限、保留为待解决或撤销 | 登录 | JSON：`qid` string、`status` string、`limit` string | JSON | `contract.go` hContractResolve |
| POST | `/api/contracts/{id}/confirm` | 确认契约（确认后锁定） | 登录 | — | JSON | `contract.go` hContractConfirm |
| POST | `/api/contracts/{id}/unlock` | 解锁契约 | 登录 | — | JSON | `contract.go` hContractUnlock |
| GET | `/api/contracts/reviewers` | 可以请来审阅的老师 | 登录 | — | JSON | `contract.go` hContractReviewers |
| POST | `/api/contracts/{id}/review-request` | 请老师审阅契约 | 登录 | JSON：`reviewer` int、`note` string | JSON | `contract.go` hContractReviewRequest |
| POST | `/api/contracts/{id}/review` | 老师审阅契约（通过 / 退回 / 评论） | 登录 | JSON：`decision` string、`text` string | JSON | `contract.go` hContractReview |

## 后台任务

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/jobs` | 我的后台任务 | 登录 | — | JSON | `jobs.go` hJobs |
| GET | `/api/jobs/{id}` | 查询一个后台任务（完成后带结果） | 登录 | — | JSON | `jobs.go` hJob |
| DELETE | `/api/jobs/{id}` | 移除后台任务 | 登录 | — | JSON | `jobs.go` hJobDelete |

## 模型与用量

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/models` | 我的模型配置卡和团队模型 | 登录 | 查询：`project_id` | JSON | `models.go` hListModels |
| POST | `/api/models` | 新增模型配置卡 | 登录 | JSON：`name` string、`protocol` string、`base_url` string、`model` string、`key` string、`temperature` float64、`timeout` int、`vision` bool、`strong_model` string、`price_in` float64、`price_out` float64、`strong_price_in` float64、`strong_price_out` float64 | JSON | `models.go` hCreateModel |
| POST | `/api/models/team/check` | 自检团队默认模型（管理员） | 登录 | — | JSON | `models.go` hCheckTeamModel |
| PATCH | `/api/models/{id}` | 修改模型配置卡 | 登录 | JSON：`name` string、`protocol` string、`base_url` string、`model` string、`key` string、`temperature` float64、`timeout` int、`vision` bool、`strong_model` string、`price_in` float64、`price_out` float64、`strong_price_in` float64、`strong_price_out` float64 | JSON | `models.go` hUpdateModel |
| DELETE | `/api/models/{id}` | 删除模型配置卡 | 登录 | — | JSON | `models.go` hDeleteModel |
| POST | `/api/models/{id}/check` | 模型兼容性自检 | 登录 | — | JSON | `models.go` hCheckModel |
| POST | `/api/models/{id}/activate` | 启用或停用模型配置卡 | 登录 | JSON：`active` bool | JSON | `models.go` hActivateModel |
| GET | `/api/models/{id}/export` | 导出配置卡（不含密钥） | 登录 | — | 文件 | `models.go` hExportModel |
| GET | `/api/usage` | 模型用量和花费（本人 / 全组） | 登录 | 查询：`scope`、`month` | JSON | `usage.go` hUsage |
| POST | `/api/models/probe` | 用接口地址和密钥查询可用模型（接入向导） | 登录 | JSON：`protocol` string、`base_url` string、`key` string | JSON | `models.go` hProbeModels |

## 联网搜索

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/websearch/status` | 联网搜索的密钥状态 | 登录 | — | JSON | `websearch.go` hWebSearchStatus |
| PUT | `/api/websearch/key` | 保存联网搜索密钥（博查 / Tavily） | 登录 | JSON：`scope` string、`provider` string、`key` string | JSON | `websearch.go` hWebSearchKey |

## LaTeX 与手绘转图

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| POST | `/api/latex/from-image` | 图片转 LaTeX（公式、表格、段落、手写、批注） | 登录 | JSON：`image` string、`task` string、`note` string、`project_id` string | JSON | `latex.go` hLatexFromImage |
| POST | `/api/latex/check` | LaTeX 静态检查 | 登录 | JSON：`text` string、`fragment` bool | JSON | `latex.go` hLatexCheck |
| GET | `/api/latex/engine` | 检测本机的 LaTeX 编译器 | 登录 | 查询：`refresh` | JSON | `texcompile.go` hTeXStatus |
| POST | `/api/latex/tectonic` | 一键下载便携 LaTeX（Tectonic） | 登录、仅本机、可后台运行 | — | JSON | `tectonic.go` hTectonicInstall |
| POST | `/api/sketch` | 手绘草图转 TikZ 图 | 登录 | JSON：`image` string、`target` string、`note` string、`project_id` string | JSON | `sketch.go` hSketch |
| POST | `/api/sketch/refine` | 按一句话调整已有的图 | 登录 | JSON：`intent` string、`tikz` string、`instruction` string、`project_id` string | JSON | `sketch.go` hSketchRefine |
| POST | `/api/latex/compile` | 编译 LaTeX 生成 PDF | 登录、仅本机 | JSON：`source` string、`kind` string、`name` string | JSON | `texcompile.go` hTeXCompile |
| GET | `/api/latex/pdf/{id}` | 取回编译好的 PDF | 登录、仅本机 | 查询：`download` | 文件 | `texcompile.go` hTeXPDF |

## 本机智能体

| 方法 | 路径 | 说明 | 要求 | 请求 | 返回 | 代码 |
|---|---|---|---|---|---|---|
| GET | `/api/agent/status` | 智能体是否可用（模型、授权文件夹） | 登录 | 查询：`project_id` | JSON | `assistant.go` hAgentStatus |
| PUT | `/api/agent/folders` | 设置授权文件夹 | 登录、仅本机 | JSON：`folders` []AgentFolder | JSON | `assistant.go` hAgentFolders |
| GET | `/api/agent/sessions` | 智能体对话列表 | 登录、仅本机 | — | JSON | `assistant.go` hAgentList |
| POST | `/api/agent/sessions` | 新建智能体对话 | 登录、仅本机 | JSON：`project_id` string | JSON | `assistant.go` hAgentNew |
| GET | `/api/agent/sessions/{id}` | 读取一次对话（消息、待确认的修改） | 登录、仅本机 | 查询：`since` | JSON | `assistant.go` hAgentGet |
| GET | `/api/agent/sessions/{id}/images/{n}` | 对话里的第 n 张图片 | 登录、仅本机 | — | 文件 | `assistant.go` hAgentImage |
| POST | `/api/agent/sessions/{id}/messages` | 给智能体发消息 | 登录、仅本机 | JSON：`text` string、`images` []string | JSON | `assistant.go` hAgentSend |
| POST | `/api/agent/sessions/{id}/stop` | 停止智能体 | 登录、仅本机 | — | JSON | `assistant.go` hAgentStop |
| POST | `/api/agent/sessions/{id}/changes/{cid}/{action}` | 处理一条待确认的修改（应用 / 放弃 / 撤销） | 登录、仅本机 | — | JSON | `assistant.go` hAgentChange |
| GET | `/api/agent/logs` | 智能体操作记录 | 登录、仅本机 | — | JSON | `assistant.go` hAgentLogs |
| POST | `/api/agent/sessions/{id}/approve` | 回应运行命令的确认（允许一次 / 始终允许 / 拒绝） | 登录、仅本机 | JSON：`id` string、`decision` string | JSON | `agent2.go` hAgentApprove |
| GET | `/api/agent/memory` | 读取智能体记忆 | 登录、仅本机 | — | JSON | `agent2.go` hAgentMemory |
| PUT | `/api/agent/memory` | 保存智能体记忆 | 登录、仅本机 | JSON：`memory` string | JSON | `agent2.go` hAgentMemory |
| GET | `/api/agent/skills` | 技能列表 | 登录、仅本机 | — | JSON | `agent2.go` hSkills |
| POST | `/api/agent/skills` | 新建或修改技能 | 登录、仅本机 | JSON：`id` string、`name` string、`description` string、`body` string、`skill_md` string | JSON | `agent2.go` hSkills |
| DELETE | `/api/agent/skills` | 删除技能（?id=） | 登录、仅本机 | 查询：`id` | JSON | `agent2.go` hSkills |
| POST | `/api/agent/skills/import` | 从网址导入 SKILL.md | 登录、仅本机 | JSON：`url` string | JSON | `skills_lib.go` hSkillImport |
| GET | `/api/agent/policy` | 读取智能体权限设置 | 登录、仅本机 | — | JSON | `agent2.go` hAgentPolicy |
| PUT | `/api/agent/policy` | 保存智能体权限设置 | 登录、仅本机 | JSON：`commands` string、`open` string、`web` string、`allow_cmds` []string | JSON | `agent2.go` hAgentPolicy |
| GET | `/api/agent/tasks` | 定时任务列表 | 登录、仅本机 | — | JSON | `agent2.go` hAgentTasks |
| POST | `/api/agent/tasks` | 新建或修改定时任务 | 登录、仅本机 | JSON：`id` string、`owner_id` int、`name` string、`prompt` string、`project_id` string、`kind` string、`time` string、`weekday` int、`every` int、`once_at` time.Time、`enabled` bool、`next_run` time.Time、`last_run` time.Time、`runs` []TaskRun、`created_at` time.Time | JSON | `agent2.go` hAgentTasks |
| DELETE | `/api/agent/tasks` | 删除定时任务（?id=） | 登录、仅本机 | 查询：`id` | JSON | `agent2.go` hAgentTasks |
| POST | `/api/agent/tasks/{id}/run` | 立即运行定时任务 | 登录、仅本机 | — | JSON | `agent2.go` hAgentTaskRun |
| GET | `/api/agent/extools` | 外部工具服务：已接入的服务、连接状态和发现的工具（?refresh=1 重新检查） | 登录、仅本机 | 查询：`refresh` | JSON | `extools.go` hAgentExt |
| PUT | `/api/agent/extools` | 保存接入的外部工具服务（只能是本机地址） | 登录、仅本机 | 查询：`refresh`<br>JSON：`services` []ExtService | JSON | `extools.go` hAgentExt |
| GET | `/api/agent/capabilities` | 能力中心：内置能力（是否可用、还缺什么、示例）和已接入的外部工具（?refresh=1 重新检查） | 登录 | 查询：`refresh` | JSON | `capabilities.go` hAgentCapabilities |
| GET | `/api/agent/extools/kit` | 下载给合作者的外部工具开发包（接口约定 + Python 示例，zip） | 登录 | — | 文件 | `capabilities.go` hAgentExtKit |

<!-- 接口表结束 -->
