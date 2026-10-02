# CanDo 可为 · 给 Claude Code 的项目说明

用户是这个项目的负责人（面向高校课题组的科研工作台），用中文交流，不是专业程序员：回复用简单中文，少用术语，改动前先说清楚要做什么。

## 这是什么
- 产品：CanDo 可为（原“科研竞赛工作台”），理念 “Everyone can do research. 人人都能做科研。”当前版本见 `app.go` 的 `AppVersion`（1.15.0）。
- 形态：Go 单文件程序（只用标准库），运行在老师的 Windows 电脑上，浏览器打开网页界面；同一 Wi-Fi 的手机通过安卓 App（`android/`）或 iPhone 主屏幕网页 / iOS 工程（`ios/`）访问。数据全部在本机：`%LOCALAPPDATA%\KeyanWorkbench\data`（目录名保持旧名，不要改，否则升级丢数据）。
- 下一阶段方向：见 `docs/平台化方案.md`（打破学术壁垒的公开交流平台）；第一步“论坛模式 / 校内联盟版”的方案见 `docs/论坛模式.md`。进展记录见 `TEST_REPORT.md` 和 `build/使用说明.txt`。

## 常用命令
```bash
go run . -data ./devdata -port 18800 -no-browser   # 本地开发，浏览器打开 http://127.0.0.1:18800
go vet ./... && go test -race ./...                # 后端测试（全部应通过，约 1 分钟）
tools/e2e/run.sh                                   # 浏览器端到端测试（模拟模型，无需 API Key），截图在 tools/e2e/out/
./build.sh                                         # 出正式版：Windows 程序 + 安装包 + 免安装版 zip + 安卓 APK（Linux / WSL）
```
- `build.sh` 依赖：Go 1.22+、NSIS（makensis）、zip；改了图标或 `build/app.rc` 时还要 mingw 的 windres；安卓打包要 `android/build_apk.sh` 顶部列出的工具（没有就沿用 `web/download/keyan-workbench.apk`）。在 Windows 上建议装 WSL（Ubuntu）运行。
- 端到端测试依赖 Python3 + `pip install playwright` + `python -m playwright install chromium`。`-tags e2e` 构建会打开 `e2e_hooks.go` 里的测试开关（模拟 OpenAlex 等），正式版不要带这个标签。

## 代码结构
- 后端：`app.go`（路由、CSP、静态文件）、`store.go`（JSON 存储，`Store.Update/View` 事务）、`auth.go`、`api_*.go`、功能文件（`contract.go` 论文契约（反谄媚让步门槛在服务端强制）、`writing_ai.go` AI 起草、`jobs.go` 后台任务、`home.go` 首页、`research.go` 深度调研、`ocr.go`、`agent2.go` 本机智能体、`office.go` / `agent_office.go` Word·Excel·PPT 读写（改 Word 只动 `word/document.xml` 里的正文段落，其余部件原样复制）、`extools.go` 外部工具服务（合作者用 Python 等写的本机工具，约定见 `docs/外部工具接口.md`，示例 `tools/extool/example_server.py`；智能体循环、提示、文件读写和确认都留在 CanDo 这边）、`websearch.go` 联网搜索（博查 / Tavily）、`tectonic.go` 便携 LaTeX、`drift.go` 偏离检查与导出前确认、`fullpaper.go` 全文组装（各节草稿拼成全文，不调用模型；规则检查 + 作者确认后导出 Word / LaTeX / PDF）……）。每个功能都有 `*_test.go`。
- 前端：`web/`（原生 ES 模块，无构建步骤，`go:embed` 打包进程序）。`core.js` 公共函数；`app.js` 路由和主要页面；其余按功能分文件。
- 品牌：`brand/`（图标 SVG 母版、`render.py` 生成全套 PNG/ico、`README.md` 色系规范）。

## 必须遵守的约定
- **CSP 禁止内联脚本**：不要写 `onclick=`。用 `data-act="名称"` / `data-change` / `data-input` / `data-submit`，在 `actions` 里注册处理函数（见 `core.js`）。
- **颜色只用 `web/style.css` 顶部的变量**（浅色 / 深色两套）。主色深群青 #1E2AB0，纸色 #F6F4EE，墨色 #14161F，深色模式主色 #7C86FF。图标用 `index.html` 里的 SVG sprite：`<svg class="ic"><use href="#i-xxx"/></svg>`。
- **接口**：前后端只通过 `/api/` 接口来往，清单见 `docs/API.md`（自动生成）。新接口在 `app.go` 的 `apiMux` 里登记，行末写一句 `// 说明`；改了接口运行 `UPDATE_API_DOC=1 go test -run TestAPIDoc .` 更新清单。`TestFrontendAPICalls` 会检查前端用到的接口后端都有。改接口尽量只加字段，不删不改含义。
- **耗时的 AI 接口**用 `a.bg("任务名", handler)` 包装，前端用 `apiBg(...)`，这样可以后台运行、离开页面不中断。
- **模型调用**走 `modelFor / callValidated`，输出必须是 JSON 并在后端校验引用 ID（不存在的出处要标出来，不能让模型编造）。
- **权限**：无权访问和不存在都返回 404；个人资料只有本人可见，共享的论文对组内只读；本机智能体只允许本机访问、只能碰授权文件夹、修改要用户确认、不读密钥类文件。
- **学术诚信**：AI 帮忙组织和检查，不替人代写；起草结果逐句标依据，导出时带“AI 辅助”声明。不要使用 CC BY-NC 许可的内容原文。
- **界面文字**：简单中文，给不懂技术的老师和学生看；中英文之间留空格；产品名写“CanDo 可为”（手机桌面写“CanDo”）。

## 发新版本的清单
1. 版本号同时改：`app.go` AppVersion、`build.sh` VERSION、`build/installer.nsi`（VERSION 和 VIProductVersion）、`build/app.rc`（4 处）、`android/AndroidManifest.xml`（versionName + versionCode 加 1）、`android/.../MainActivity.java` VERSION、`ios/project.yml`（MARKETING_VERSION + CURRENT_PROJECT_VERSION 加 1）、`ios/KeyanWorkbench/AppState.swift` AppInfo.version。
2. `go test -race ./...`、`tools/e2e/run.sh` 全部通过。
3. 在 `TEST_REPORT.md` 顶部加本版本的检查表；在 `build/使用说明.txt` 开头的功能列表加说明，并改标题版本号。
4. `./build.sh`，交付：安装包 exe、安装包压缩版 zip、免安装版 zip、安卓 APK（和 zip）、iOS 工程源码 zip、源码 zip、使用说明、测试报告。

## 注意
- 代码在 GitHub 私有仓库，推送后 `.github/workflows/test.yml` 自动跑 gofmt、go vet、`go test -race` 和 Windows 编译；做改动建议开分支、提 PR，测试通过再合并。步骤见 `docs/GitHub与ClaudeCode.md`。
- `android/release.keystore` 是安卓签名密钥：以后的 APK 必须用它签名，手机才能覆盖升级。它不在仓库里（`.gitignore` 已排除），由用户私下保管；打包前放回原位。缺少时 `build.sh` 会沿用 `web/download/` 里的旧 APK。
- 安装包的 APPID / 安装目录 / 注册表键仍为 KeyanWorkbench（兼容旧版升级），显示名为 CanDo 可为，程序文件为 CanDo.exe；安装时会清理旧的 KeyanWorkbench.exe 和旧快捷方式。
- 未在真机验证：Windows 实机、安卓和 iPhone 真机（含深色模式与新图标）；iOS 工程只做过语法检查，未在 Mac 上编译。
