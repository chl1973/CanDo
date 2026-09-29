import SwiftUI

/// “连接电脑”页面：输入或扫码得到电脑地址，检查后保存。
struct SetupView: View {
    @EnvironmentObject var state: AppState
    @State private var address = ""
    @State private var status = ""
    @State private var isError = false
    @State private var busy = false
    @State private var scanning = false

    private let blue = Color(red: 0x1E / 255, green: 0x2A / 255, blue: 0xB0 / 255)

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                HStack(spacing: 12) {
                    Image("AppIconImage").resizable().frame(width: 56, height: 56).cornerRadius(12)
                        .accessibilityHidden(true)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("CanDo").font(.title2).bold()
                        Text("连接老师电脑上的 CanDo 可为").font(.subheadline).foregroundColor(.secondary)
                    }
                }
                .padding(.top, 24)

                if state.server != nil {
                    Button("← 返回（不更换电脑）") { state.showSetup = false }
                        .font(.subheadline)
                }

                VStack(alignment: .leading, spacing: 10) {
                    Text("电脑地址").font(.headline)
                    TextField("例如 192.168.1.5:18765", text: $address)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.go)
                        .onSubmit { connect() }
                        .padding(12)
                        .background(Color.white)
                        .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color(white: 0.88)))
                    HStack {
                        Button(action: { scanning = true }) {
                            Label("扫描二维码", systemImage: "qrcode.viewfinder").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.bordered)
                        Button(action: connect) {
                            if busy { ProgressView().frame(maxWidth: .infinity) } else { Text("连接").frame(maxWidth: .infinity) }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(blue)
                        .disabled(busy || address.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                    if !status.isEmpty {
                        Text(status).font(.footnote).foregroundColor(isError ? .red : .secondary)
                    }
                }
                .padding(16)
                .background(Color.white)
                .cornerRadius(12)

                VStack(alignment: .leading, spacing: 8) {
                    Text("地址在哪里找？").font(.headline)
                    Text("在老师电脑的工作台里：设置 → 手机与同学访问，勾选“允许手机和同学访问”，页面上会显示访问地址和二维码②。")
                    Text("手机和电脑需要连同一个 Wi-Fi。校园网禁止设备互访时，可以让电脑连接手机热点。")
                    Text("所有资料都保存在老师电脑上，手机只记住电脑地址和登录状态。")
                }
                .font(.footnote)
                .foregroundColor(.secondary)
                .padding(16)
                .background(Color.white)
                .cornerRadius(12)
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 32)
        }
        .onAppear {
            if let s = state.server { address = AppState.strip(s) }
        }
        .sheet(isPresented: $scanning) {
            QRScannerSheet { code in
                scanning = false
                address = AppState.strip(code)
                connect()
            }
        }
    }

    private func connect() {
        guard let base = AppState.normalize(address) else {
            status = "请输入电脑地址，例如 192.168.1.5:18765"
            isError = true
            return
        }
        busy = true
        isError = false
        status = "正在连接 \(AppState.strip(base)) …"
        Task {
            let err = await AppState.check(base)
            busy = false
            if let err {
                status = err
                isError = true
            } else {
                status = ""
                state.save(server: base)
            }
        }
    }
}
