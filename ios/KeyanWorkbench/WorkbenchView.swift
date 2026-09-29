import SwiftUI
import WebKit
import SafariServices

/// 工作台页面：用 WKWebView 打开电脑上的工作台。
/// - 登录状态保存在默认的网站数据存储里，下次打开不用重新登录；
/// - 网页可以通过 window.KyApp 调用 App（与安卓 App 相同：server()、version()、openExternal()、changeServer()）；
/// - 下载的文件（PDF、Word 模板、导出的表格等）会弹出系统“分享 / 存储到文件”；
/// - 外部网站（DOI、知网等）在 App 内的 Safari 视图中打开；
/// - 连不上电脑时显示提示页，可以重试或更换电脑。
struct WorkbenchView: View {
    let server: String
    @EnvironmentObject var state: AppState
    @StateObject private var model = WebModel()

    var body: some View {
        ZStack(alignment: .top) {
            WebViewRepresentable(server: server, model: model, onChangeServer: { state.showSetup = true })
                .ignoresSafeArea(edges: .bottom)
            if model.loading {
                ProgressView(value: model.progress)
                    .progressViewStyle(.linear)
                    .tint(Color(red: 0x1E / 255, green: 0x2A / 255, blue: 0xB0 / 255))
            }
            if let err = model.error {
                ErrorOverlay(message: err, server: server, retry: { model.reload() }, change: { state.showSetup = true })
            }
        }
        .sheet(item: $model.shareItem) { item in
            ShareSheet(items: [item.url])
        }
        .sheet(item: $model.safariURL) { item in
            SafariView(url: item.url).ignoresSafeArea()
        }
    }
}

struct IdentURL: Identifiable {
    let id = UUID()
    let url: URL
}

@MainActor
final class WebModel: ObservableObject {
    @Published var loading = false
    @Published var progress = 0.0
    @Published var error: String?
    @Published var shareItem: IdentURL?
    @Published var safariURL: IdentURL?
    weak var webView: WKWebView?
    var server = ""
    var downloads: [ObjectIdentifier: URL] = [:]
    private var progressObs: NSKeyValueObservation?

    func attach(_ wv: WKWebView) {
        webView = wv
        progressObs = wv.observe(\.estimatedProgress, options: [.new]) { [weak self] wv, _ in
            let p = wv.estimatedProgress
            Task { @MainActor in self?.progress = p }
        }
    }

    func reload() {
        error = nil
        guard let wv = webView else { return }
        if let u = wv.url, u.absoluteString.hasPrefix(server) {
            wv.reload()
        } else if let u = URL(string: server + "/") {
            wv.load(URLRequest(url: u))
        }
    }

    func isOurs(_ url: URL) -> Bool {
        guard let s = URL(string: server) else { return false }
        return url.host == s.host && (url.port ?? 80) == (s.port ?? 80)
    }

    func openExternal(_ url: URL) {
        guard let scheme = url.scheme?.lowercased(), scheme == "http" || scheme == "https" else { return }
        safariURL = IdentURL(url: url)
    }
}

struct WebViewRepresentable: UIViewRepresentable {
    let server: String
    let model: WebModel
    let onChangeServer: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(model: model, onChangeServer: onChangeServer) }

    func makeUIView(context: Context) -> WKWebView {
        let cfg = WKWebViewConfiguration()
        cfg.websiteDataStore = .default()
        cfg.allowsInlineMediaPlayback = true
        let uc = WKUserContentController()
        // 与安卓 App 相同的 KyApp 接口（server/version 为同步返回值）
        let js = """
        window.KyApp = {
          server: function () { return \(Self.jsString(AppState.strip(server))); },
          version: function () { return \(Self.jsString(AppInfo.version)); },
          platform: function () { return "ios"; },
          openExternal: function (u) { window.webkit.messageHandlers.kyapp.postMessage({op: "open", url: String(u)}); },
          changeServer: function () { window.webkit.messageHandlers.kyapp.postMessage({op: "changeServer"}); }
        };
        """
        uc.addUserScript(WKUserScript(source: js, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        uc.add(context.coordinator, name: "kyapp")
        cfg.userContentController = uc

        let wv = WKWebView(frame: .zero, configuration: cfg)
        wv.navigationDelegate = context.coordinator
        wv.uiDelegate = context.coordinator
        wv.allowsBackForwardNavigationGestures = true
        // 下拉刷新
        let rc = UIRefreshControl()
        rc.addTarget(context.coordinator, action: #selector(Coordinator.pullRefresh(_:)), for: .valueChanged)
        wv.scrollView.refreshControl = rc

        model.server = server
        model.attach(wv)
        if let u = URL(string: server + "/") { wv.load(URLRequest(url: u)) }
        return wv
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {}

    static func dismantleUIView(_ uiView: WKWebView, coordinator: Coordinator) {
        uiView.configuration.userContentController.removeScriptMessageHandler(forName: "kyapp")
    }

    static func jsString(_ s: String) -> String {
        guard let data = try? JSONSerialization.data(withJSONObject: [s]), let arr = String(data: data, encoding: .utf8) else { return "\"\"" }
        return String(arr.dropFirst().dropLast())
    }

    @MainActor
    final class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate, WKScriptMessageHandler, WKDownloadDelegate {
        let model: WebModel
        let onChangeServer: () -> Void

        init(model: WebModel, onChangeServer: @escaping () -> Void) {
            self.model = model
            self.onChangeServer = onChangeServer
        }

        @objc func pullRefresh(_ rc: UIRefreshControl) {
            model.reload()
            rc.endRefreshing()
        }

        // MARK: 网页调用 App
        func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
            guard let body = message.body as? [String: Any], let op = body["op"] as? String else { return }
            switch op {
            case "open":
                if let s = body["url"] as? String, let u = URL(string: s) { model.openExternal(u) }
            case "changeServer":
                onChangeServer()
            default:
                break
            }
        }

        // MARK: 导航
        func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction) async -> WKNavigationActionPolicy {
            if navigationAction.shouldPerformDownload { return .download }
            guard let url = navigationAction.request.url, let scheme = url.scheme?.lowercased() else { return .allow }
            if scheme == "blob" || scheme == "data" || scheme == "about" { return .allow }
            if scheme != "http" && scheme != "https" {
                // mailto:、tel: 等交给系统
                _ = await UIApplication.shared.open(url)
                return .cancel
            }
            // 外部网站（论文原文、知网、DOI 等）用 App 内的 Safari 视图打开，不离开工作台
            if !model.isOurs(url) {
                model.openExternal(url)
                return .cancel
            }
            return .allow
        }

        func webView(_ webView: WKWebView, decidePolicyFor navigationResponse: WKNavigationResponse) async -> WKNavigationResponsePolicy {
            if let http = navigationResponse.response as? HTTPURLResponse,
               let disp = http.value(forHTTPHeaderField: "Content-Disposition"), disp.lowercased().hasPrefix("attachment") {
                return .download
            }
            return navigationResponse.canShowMIMEType ? .allow : .download
        }

        func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { download.delegate = self }
        func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) { download.delegate = self }

        func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {
            model.loading = true
            model.error = nil
        }

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            model.loading = false
        }

        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
            fail(error)
        }

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            fail(error)
        }

        private func fail(_ error: Error) {
            model.loading = false
            let ns = error as NSError
            // 取消、转为下载（WebKitErrorDomain 102）不算错误
            if ns.code == NSURLErrorCancelled || (ns.domain == "WebKitErrorDomain" && ns.code == 102) { return }
            model.error = "连接不上电脑上的工作台（\(ns.localizedDescription)）。\n请确认电脑已开机、工作台已打开，手机和电脑在同一个 Wi-Fi 下。"
        }

        // 页面进程被系统回收时自动重新加载
        func webViewWebContentProcessDidTerminate(_ webView: WKWebView) { webView.reload() }

        // MARK: 新窗口（target=_blank）
        func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
            if let url = navigationAction.request.url {
                if model.isOurs(url) {
                    webView.load(navigationAction.request) // 工作台自己的页面：在当前页面打开（可以左滑返回）
                } else {
                    model.openExternal(url)
                }
            }
            return nil
        }

        // MARK: 网页里的 alert / confirm / prompt
        func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo) async {
            await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
                let ac = UIAlertController(title: nil, message: message, preferredStyle: .alert)
                ac.addAction(UIAlertAction(title: "好", style: .default) { _ in c.resume() })
                if !present(ac, from: webView) { c.resume() }
            }
        }

        func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo) async -> Bool {
            await withCheckedContinuation { (c: CheckedContinuation<Bool, Never>) in
                let ac = UIAlertController(title: nil, message: message, preferredStyle: .alert)
                ac.addAction(UIAlertAction(title: "取消", style: .cancel) { _ in c.resume(returning: false) })
                ac.addAction(UIAlertAction(title: "确定", style: .default) { _ in c.resume(returning: true) })
                if !present(ac, from: webView) { c.resume(returning: false) }
            }
        }

        func webView(_ webView: WKWebView, runJavaScriptTextInputPanelWithPrompt prompt: String, defaultText: String?, initiatedByFrame frame: WKFrameInfo) async -> String? {
            await withCheckedContinuation { (c: CheckedContinuation<String?, Never>) in
                let ac = UIAlertController(title: nil, message: prompt, preferredStyle: .alert)
                ac.addTextField { $0.text = defaultText }
                ac.addAction(UIAlertAction(title: "取消", style: .cancel) { _ in c.resume(returning: nil) })
                ac.addAction(UIAlertAction(title: "确定", style: .default) { _ in c.resume(returning: ac.textFields?.first?.text) })
                if !present(ac, from: webView) { c.resume(returning: nil) }
            }
        }

        private func present(_ vc: UIViewController, from view: UIView) -> Bool {
            var top = view.window?.rootViewController
            while let p = top?.presentedViewController { top = p }
            guard let top else { return false }
            top.present(vc, animated: true)
            return true
        }

        // MARK: 下载：保存到临时文件夹，完成后弹出“分享 / 存储到文件”
        func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String) async -> URL? {
            let dir = FileManager.default.temporaryDirectory.appendingPathComponent("downloads", isDirectory: true)
            try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            let name = suggestedFilename.isEmpty ? "download" : suggestedFilename
            let dest = dir.appendingPathComponent(name)
            try? FileManager.default.removeItem(at: dest)
            model.downloads[ObjectIdentifier(download)] = dest
            return dest
        }

        func downloadDidFinish(_ download: WKDownload) {
            if let url = model.downloads.removeValue(forKey: ObjectIdentifier(download)) {
                model.shareItem = IdentURL(url: url)
            }
        }

        func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
            model.downloads.removeValue(forKey: ObjectIdentifier(download))
        }
    }
}

struct ErrorOverlay: View {
    let message: String
    let server: String
    let retry: () -> Void
    let change: () -> Void

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "wifi.exclamationmark").font(.system(size: 44)).foregroundColor(.secondary)
            Text("连不上工作台").font(.title3).bold()
            Text(message).font(.subheadline).foregroundColor(.secondary).multilineTextAlignment(.center)
            Text("当前电脑：\(AppState.strip(server))").font(.footnote).foregroundColor(.secondary)
            HStack {
                Button("更换电脑", action: change).buttonStyle(.bordered)
                Button("重试", action: retry).buttonStyle(.borderedProminent)
            }
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(red: 0.96, green: 0.965, blue: 0.973))
    }
}

struct ShareSheet: UIViewControllerRepresentable {
    let items: [Any]
    func makeUIViewController(context: Context) -> UIActivityViewController {
        UIActivityViewController(activityItems: items, applicationActivities: nil)
    }
    func updateUIViewController(_ vc: UIActivityViewController, context: Context) {}
}

struct SafariView: UIViewControllerRepresentable {
    let url: URL
    func makeUIViewController(context: Context) -> SFSafariViewController { SFSafariViewController(url: url) }
    func updateUIViewController(_ vc: SFSafariViewController, context: Context) {}
}
