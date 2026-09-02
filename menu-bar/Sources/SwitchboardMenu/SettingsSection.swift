import SwiftUI
import ServiceManagement

struct SettingsSection: View {
    @EnvironmentObject var model: StatusModel
    @AppStorage("sb_menubar_interval") private var intervalMinutes = 0.5 // minutes; 0.5 = 30s
    @State private var baseURL: String = ""
    @State private var apiKey: String = ""
    @State private var savedFlash = false
    @State private var loginItemEnabled = SMAppService.mainApp.status == .enabled

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Divider()
            Text("Settings").font(.subheadline).bold()

            LabeledContent("Proxy URL") {
                TextField("http://127.0.0.1:8495", text: $baseURL)
                    .textFieldStyle(.roundedBorder)
            }
            LabeledContent("Proxy key") {
                SecureField("server.proxy_api_key", text: $apiKey)
                    .textFieldStyle(.roundedBorder)
            }

            LabeledContent("Poll interval") {
                Picker("", selection: $intervalMinutes) {
                    Text("15s").tag(0.25)
                    Text("30s").tag(0.5)
                    Text("60s").tag(1.0)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .onChange(of: intervalMinutes) { newValue in
                    model.pollInterval = newValue * 60
                }
            }

            Toggle("Launch at login", isOn: $loginItemEnabled)
                .onChange(of: loginItemEnabled) { enabled in
                do {
                    if enabled {
                        try SMAppService.mainApp.register()
                    } else {
                        try SMAppService.mainApp.unregister()
                    }
                } catch {
                    loginItemEnabled = SMAppService.mainApp.status == .enabled
                }
            }

            HStack {
                Button("Save") {
                    var trimmed = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
                    while trimmed.hasSuffix("/") { trimmed.removeLast() }
                    let url = URL(string: trimmed) ?? URL(string: "http://127.0.0.1:8495")!
                    let effectiveKey = apiKey.trimmingCharacters(in: .whitespacesAndNewlines)
                    if !effectiveKey.isEmpty {
                        model.updateConnection(baseURL: url, apiKey: effectiveKey)
                        KeychainStore.save(effectiveKey)
                    } else {
                        (model.api as? APIClient)?.baseURL = url
                    }
                    savedFlash = true
                    Task {
                        try? await Task.sleep(nanoseconds: 1_500_000_000)
                        savedFlash = false
                        await model.refresh(force: true)
                    }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)

                Button("Clear key") {
                    KeychainStore.delete()
                    apiKey = ""
                }
                .controlSize(.small)

                if savedFlash {
                    Text("Saved").font(.caption).foregroundColor(.green)
                }
                Spacer()
            }

            if let client = model.api as? APIClient {
                Text("Connected to \(client.baseURL.absoluteString) · key source: \(KeychainStore.load() != nil ? "Keychain" : "config.yaml")")
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }
        }
        .onAppear {
            if let client = model.api as? APIClient {
                baseURL = client.baseURL.absoluteString
            }
            apiKey = KeychainStore.load() ?? (model.api as? APIClient)?.apiKey ?? ""
        }
    }
}
