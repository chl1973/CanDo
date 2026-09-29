# 科研工作台 iOS App（原生工程源码）

和安卓 App 一样：连接老师电脑上运行的“科研竞赛工作台”，在 App 里使用全部功能。资料都保存在老师电脑上，手机只记住电脑地址和登录状态。

> **不想折腾也可以不用编译**：iPhone 用 Safari 打开工作台，点“分享 → 添加到主屏幕”，就有桌面图标、全屏使用，功能相同。
> 原生 App 多出来的是：扫码连接电脑、下载文件直接弹出“存储到文件”、外部网页在 App 内打开、连不上电脑时有提示页。

## 需要准备

- 一台 Mac，安装 Xcode 15 或更新版本（App Store 免费下载）。
- 一个 Apple ID。
  - **免费 Apple ID**：可以装到自己的 iPhone 上，但 **7 天后需要用 Xcode 重新安装一次**。
  - **苹果开发者账号**（个人 ¥688/年）：可以用 TestFlight 发给课题组同学安装（最多 1 万人，每个版本 90 天有效），或上架 App Store。
- iPhone / iPad：iOS 15 或更新。

## 编译步骤

1. 安装 XcodeGen（用来生成 Xcode 工程文件）：
   ```bash
   brew install xcodegen      # 没有 Homebrew 时先装：https://brew.sh
   ```
2. 在本文件夹（ios）里生成工程并打开：
   ```bash
   cd ios
   xcodegen generate
   open KeyanWorkbench.xcodeproj
   ```
3. 在 Xcode 左侧点 `KeyanWorkbench` 工程 → `Signing & Capabilities`：
   - Team 选你的 Apple ID（没有的话点 “Add Account…” 登录）；
   - 如果提示 Bundle Identifier 已被占用，把 `com.keyan.workbench` 改成你自己的，例如 `com.你的名字.workbench`。
4. 用数据线连接 iPhone，在 Xcode 顶部选择你的手机，点 ▶ 运行。
5. 第一次运行时，iPhone 上要信任开发者：设置 → 通用 → VPN 与设备管理 → 点你的 Apple ID → 信任。
   iOS 16 以后还要打开：设置 → 隐私与安全性 → 开发者模式。

## 发给同学（需要开发者账号）

Xcode 菜单 Product → Archive → Distribute App → TestFlight & App Store → 上传。
在 App Store Connect 的 TestFlight 里添加同学的邮箱（或生成公开链接），同学安装 TestFlight 后就能装。

## 使用

1. 打开 App，输入老师电脑上显示的访问地址（设置 → 手机与同学访问），或点“扫描二维码”扫二维码②。
2. 第一次连接时，系统会问“是否允许查找并连接本地网络上的设备”，请点“好”。
3. 登录后和网页版一样使用。“设置”页里可以看到当前连接的电脑，并可以更换。

## 说明

- 工作台在局域网里用 http 访问，所以 Info.plist 里允许了明文 http（NSAllowsArbitraryLoads / NSAllowsLocalNetworking）。上架审核时如被问到，说明“App 只连接用户自己电脑上的局域网服务”即可。
- 网页通过 `window.KyApp` 与 App 通信，接口与安卓 App 相同：`server()`、`version()`、`openExternal(url)`、`changeServer()`，另有 `platform()` 返回 `"ios"`。
- 本工程在 Linux 上做过语法检查，**没有在 Mac 上实际编译和真机测试过**；如果 Xcode 报错，请把错误信息发回来修改。

## 文件

| 文件 | 作用 |
|---|---|
| `project.yml` | XcodeGen 工程描述（iOS 15+，版本号在这里改） |
| `KeyanWorkbench/App.swift` | 入口；根据是否已设置电脑地址显示连接页或工作台 |
| `KeyanWorkbench/AppState.swift` | 保存电脑地址、地址格式整理、检查地址是否为工作台 |
| `KeyanWorkbench/SetupView.swift` | “连接电脑”页面 |
| `KeyanWorkbench/QRScanner.swift` | 扫描二维码 |
| `KeyanWorkbench/WorkbenchView.swift` | 工作台网页、KyApp 接口、下载、外部链接、出错提示 |
| `KeyanWorkbench/Info.plist` | 权限说明（本地网络、相机、相册）、URL scheme `kyws://` |
| `KeyanWorkbench/Assets.xcassets` | 图标 |
