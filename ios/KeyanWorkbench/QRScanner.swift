import SwiftUI
import AVFoundation

/// 扫描电脑上“手机与同学访问”里的二维码②，得到电脑地址。
struct QRScannerSheet: View {
    let onCode: (String) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var denied = false

    var body: some View {
        NavigationView {
            ZStack {
                Color.black.ignoresSafeArea()
                if denied {
                    VStack(spacing: 12) {
                        Text("没有相机权限").font(.headline).foregroundColor(.white)
                        Text("请在 设置 → CanDo 里打开“相机”，或者直接手动输入电脑地址。")
                            .font(.subheadline).foregroundColor(.gray).multilineTextAlignment(.center)
                        Button("打开设置") {
                            if let u = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(u) }
                        }
                    }
                    .padding(24)
                } else {
                    QRCameraView(onCode: { code in
                        if let n = AppState.normalize(code) { onCode(n) }
                    }, onDenied: { denied = true })
                    .ignoresSafeArea()
                    VStack {
                        Spacer()
                        Text("对准电脑上“手机与同学访问”里的二维码②")
                            .font(.subheadline).foregroundColor(.white)
                            .padding(10).background(Color.black.opacity(0.5)).cornerRadius(8)
                            .padding(.bottom, 40)
                    }
                }
            }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
            }
        }
    }
}

struct QRCameraView: UIViewControllerRepresentable {
    let onCode: (String) -> Void
    let onDenied: () -> Void

    func makeUIViewController(context: Context) -> ScannerController {
        let vc = ScannerController()
        vc.onCode = onCode
        vc.onDenied = onDenied
        return vc
    }

    func updateUIViewController(_ vc: ScannerController, context: Context) {}

    final class ScannerController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
        var onCode: ((String) -> Void)?
        var onDenied: (() -> Void)?
        private let session = AVCaptureSession()
        private var preview: AVCaptureVideoPreviewLayer?
        private var done = false

        override func viewDidLoad() {
            super.viewDidLoad()
            view.backgroundColor = .black
            switch AVCaptureDevice.authorizationStatus(for: .video) {
            case .authorized:
                setup()
            case .notDetermined:
                AVCaptureDevice.requestAccess(for: .video) { ok in
                    DispatchQueue.main.async { ok ? self.setup() : self.onDenied?() }
                }
            default:
                onDenied?()
            }
        }

        private func setup() {
            guard let device = AVCaptureDevice.default(for: .video),
                  let input = try? AVCaptureDeviceInput(device: device),
                  session.canAddInput(input) else { onDenied?(); return }
            session.addInput(input)
            let output = AVCaptureMetadataOutput()
            guard session.canAddOutput(output) else { return }
            session.addOutput(output)
            output.setMetadataObjectsDelegate(self, queue: .main)
            output.metadataObjectTypes = [.qr]
            let layer = AVCaptureVideoPreviewLayer(session: session)
            layer.videoGravity = .resizeAspectFill
            layer.frame = view.bounds
            view.layer.addSublayer(layer)
            preview = layer
            DispatchQueue.global(qos: .userInitiated).async { self.session.startRunning() }
        }

        override func viewDidLayoutSubviews() {
            super.viewDidLayoutSubviews()
            preview?.frame = view.bounds
        }

        override func viewWillDisappear(_ animated: Bool) {
            super.viewWillDisappear(animated)
            if session.isRunning { DispatchQueue.global().async { self.session.stopRunning() } }
        }

        func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput metadataObjects: [AVMetadataObject], from connection: AVCaptureConnection) {
            guard !done, let obj = metadataObjects.first as? AVMetadataMachineReadableCodeObject, let s = obj.stringValue,
                  s.hasPrefix("http://") || s.hasPrefix("https://") else { return }
            done = true
            UINotificationFeedbackGenerator().notificationOccurred(.success)
            onCode?(s)
        }
    }
}
