import Foundation
import SwiftUI

/// CanDo iOS App：连接老师电脑上运行的“CanDo 可为”，在 App 内使用全部功能。
/// 数据全部保存在老师电脑上，手机只保存“电脑地址”和登录状态（WebView 的 Cookie）。
enum AppInfo {
    static let version = "1.15.0"
    static let defaultPort = 18765
}

@MainActor
final class AppState: ObservableObject {
    /// 当前连接的电脑地址，例如 http://192.168.1.5:18765（nil 表示还没设置）
    @Published var server: String?
    /// 显示“连接电脑”页面（首次使用，或用户在设置里点“更换电脑”）
    @Published var showSetup: Bool
    /// WebView 需要重新创建时加一（换了电脑地址）
    @Published var webGeneration = 0

    private let key = "server"

    init() {
        let s = UserDefaults.standard.string(forKey: key)
        server = s
        showSetup = (s == nil)
    }

    func save(server s: String) {
        UserDefaults.standard.set(s, forKey: key)
        server = s
        showSetup = false
        webGeneration += 1
    }

    /// 地址去掉 http:// 前缀，用于显示
    static func strip(_ s: String) -> String {
        s.replacingOccurrences(of: "^https?://", with: "", options: .regularExpression)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }

    /// 输入 “192.168.1.5” / “192.168.1.5:18765” / “http://192.168.1.5:18765/任意路径” 都可以
    static func normalize(_ input: String) -> String? {
        var s = input.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty { return nil }
        if !s.hasPrefix("http://") && !s.hasPrefix("https://") { s = "http://" + s }
        guard let u = URL(string: s), let host = u.host, !host.isEmpty else { return nil }
        let scheme = u.scheme ?? "http"
        let port = u.port ?? (scheme == "https" ? 443 : AppInfo.defaultPort)
        let defaultPort = (scheme == "https" && port == 443) || (scheme == "http" && port == 80)
        return defaultPort ? "\(scheme)://\(host)" : "\(scheme)://\(host):\(port)"
    }

    /// 检查地址是否为 CanDo 可为；返回 nil 表示成功，否则返回给用户看的错误说明
    static func check(_ base: String) async -> String? {
        guard let url = URL(string: base + "/api/health") else { return "地址格式不对" }
        var req = URLRequest(url: url)
        req.timeoutInterval = 6
        req.cachePolicy = .reloadIgnoringLocalCacheData
        do {
            let (data, resp) = try await URLSession.shared.data(for: req)
            let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
            if code == 403 {
                return "已连上电脑，但老师还没有开启“手机与同学访问”。请在电脑上：设置 → 手机与同学访问 → 勾选“允许手机和同学访问”。"
            }
            if code != 200 { return "连接失败（状态 \(code)）。请确认地址是否正确。" }
            let body = String(data: data, encoding: .utf8) ?? ""
            if !body.contains("\"app\":\"kyws\"") { return "这个地址不是 CanDo 可为，请检查地址。" }
            return nil
        } catch let e as URLError where e.code == .timedOut {
            return "连接超时。请确认手机和电脑连的是同一个 Wi-Fi，且电脑上的工作台已打开。"
        } catch {
            return "连接不上 \(strip(base))。请确认电脑已开机、工作台已打开，且手机和电脑在同一个 Wi-Fi 下。第一次连接时如果弹出“查找并连接本地网络上的设备”，请点“好”（也可以在 设置 → 隐私与安全性 → 本地网络 里打开）。"
        }
    }
}
