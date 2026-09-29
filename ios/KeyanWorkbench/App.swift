import SwiftUI

@main
struct KeyanWorkbenchApp: App {
    @StateObject private var state = AppState()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(state)
                // 扫描电脑上的二维码② 后，用相机打开的链接也可能被分享到 App（例如 kyws://192.168.1.5:18765）
                .onOpenURL { url in
                    let s = url.absoluteString.replacingOccurrences(of: "^kyws://", with: "http://", options: .regularExpression)
                    if let n = AppState.normalize(s) {
                        Task {
                            if await AppState.check(n) == nil { state.save(server: n) }
                        }
                    }
                }
        }
    }
}

struct RootView: View {
    @EnvironmentObject var state: AppState

    var body: some View {
        ZStack {
            Color(red: 0.96, green: 0.965, blue: 0.973).ignoresSafeArea()
            if let server = state.server, !state.showSetup {
                WorkbenchView(server: server)
                    .id(state.webGeneration)
            } else {
                SetupView()
            }
        }
    }
}
