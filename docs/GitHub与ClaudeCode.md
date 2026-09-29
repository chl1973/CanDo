# 用 GitHub + Claude Code 继续开发

## 第一次：把代码放到 GitHub
1. 在 github.com 新建仓库，建议名字 `cando`，**选 Private（私有）**。不要勾选“添加 README / .gitignore”（仓库里已经有了）。
2. 解压 `CanDo_GitHub仓库_v1.14.0.zip`，得到 `cando` 文件夹（里面已经是一个 git 仓库，带第一次提交）。
3. 在这个文件夹里打开终端，运行（把地址换成你自己的仓库）：
   ```bash
   git remote add origin https://github.com/你的用户名/cando.git
   git push -u origin main
   ```
   也可以直接让 Claude Code 帮你做这一步。
4. 推上去以后，GitHub 的 “Actions” 页会自动跑一次测试（`.github/workflows/test.yml`），绿色对勾表示通过。

## 用 Claude Code 开发
- 在 `cando` 文件夹里启动 Claude Code，它会自动读 `CLAUDE.md`（项目说明、约定、发版清单）。
- 建议每做一件事开一个分支、提一个 PR，GitHub 自动测试通过后再合并。
- 发新版本：按 `CLAUDE.md` 里“发新版本的清单”改版本号、跑测试、`./build.sh`。打包需要 Linux 或 WSL（Go、NSIS、zip）。

## 安卓签名密钥（重要）
- `android/release.keystore` **没有放进仓库**（`.gitignore` 已排除），它在之前的“开发包”zip 里。
- 请把它私下备份（网盘 / U 盘），打包安卓 App 时放回 `android/release.keystore`。丢了它，以后的 App 就不能覆盖升级，手机上只能卸载重装。
- 不要把它上传到 GitHub，哪怕是私有仓库也不建议。

## 不在仓库里的东西
- `dist/`（打包结果）、`devdata/`（本地测试数据）、`tools/e2e/out/`（截图）、`brand/png/`（可用 `brand/render.py` 重新生成）。
