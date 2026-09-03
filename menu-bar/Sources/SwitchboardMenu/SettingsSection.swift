import SwiftUI
import ServiceManagement

struct SettingsSection: View {
    @EnvironmentObject var model: StatusModel
    @AppStorage("sb_menubar_interval") private var intervalMinutes = 0.5 // minutes; 0.5 = 30s
    @State private var baseURL: String = ""
    @State private var apiKey: String = ""
    @State private var savedFlash = false
    @State private var loginItemEnabled = SMAppService.mainApp.status == .enabled
    @State private var isHoveringSave = false
    @State private var isHoveringClear = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("SETTINGS")
                .font(.system(size: 11, weight: .regular))
                .tracking(1.3)
                .foregroundColor(SBTheme.accent400)
                .padding(.bottom, 12)

            VStack(alignment: .leading, spacing: 11) {
                // Proxy URL field
                VStack(alignment: .leading, spacing: 5) {
                    Text("PROXY URL")
                        .font(.system(size: 10.5, weight: .regular))
                        .tracking(0.6)
                        .foregroundColor(SBTheme.neutral500)

                    TextField("http://127.0.0.1:8495", text: $baseURL)
                        .textFieldStyle(.plain)
                        .font(.system(size: 12.5))
                        .monospacedDigit()
                        .foregroundColor(SBTheme.neutral200)
                        .padding(.horizontal, 9)
                        .padding(.vertical, 7)
                        .background(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(SBTheme.borderSubtle, lineWidth: 1)
                        )
                }

                // Proxy Key field
                VStack(alignment: .leading, spacing: 5) {
                    Text("PROXY KEY")
                        .font(.system(size: 10.5, weight: .regular))
                        .tracking(0.6)
                        .foregroundColor(SBTheme.neutral500)

                    SecureField("server.proxy_api_key", text: $apiKey)
                        .textFieldStyle(.plain)
                        .font(.system(size: 12.5))
                        .foregroundColor(SBTheme.neutral200)
                        .padding(.horizontal, 9)
                        .padding(.vertical, 7)
                        .background(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(SBTheme.borderSubtle, lineWidth: 1)
                        )
                }

                // Poll Interval row
                HStack(alignment: .center) {
                    Text("POLL INTERVAL")
                        .font(.system(size: 10.5, weight: .regular))
                        .tracking(0.6)
                        .foregroundColor(SBTheme.neutral500)

                    Spacer()

                    HStack(spacing: 0) {
                        pollSegment("15s", minutes: 0.25, hasDivider: true)
                        pollSegment("30s", minutes: 0.5, hasDivider: true)
                        pollSegment("60s", minutes: 1.0, hasDivider: false)
                    }
                    .background(Color.clear)
                    .overlay(
                        RoundedRectangle(cornerRadius: 3)
                            .stroke(SBTheme.borderSubtle, lineWidth: 1)
                    )
                    .clipShape(RoundedRectangle(cornerRadius: 3))
                }

                // Launch at Login row
                HStack(alignment: .center) {
                    Text("Launch at login")
                        .font(.system(size: 12.5))
                        .foregroundColor(SBTheme.neutral300)

                    Spacer()

                    Button {
                        toggleLoginItem()
                    } label: {
                        ZStack(alignment: loginItemEnabled ? .trailing : .leading) {
                            RoundedRectangle(cornerRadius: 10)
                                .fill(loginItemEnabled ? SBTheme.accentToggleBg : Color.white.opacity(0.04))
                                .overlay(
                                    RoundedRectangle(cornerRadius: 10)
                                        .stroke(loginItemEnabled ? SBTheme.accent500 : SBTheme.borderSubtle, lineWidth: 1)
                                )
                                .frame(width: 38, height: 20)

                            Circle()
                                .fill(loginItemEnabled ? SBTheme.accent400 : SBTheme.neutral600)
                                .frame(width: 14, height: 14)
                                .padding(.horizontal, 3)
                        }
                    }
                    .buttonStyle(.plain)
                    .animation(.easeInOut(duration: 0.15), value: loginItemEnabled)
                }
            }

            Rectangle()
                .fill(SBTheme.borderMuted)
                .frame(height: 1)
                .padding(.top, 13)
                .padding(.bottom, 11)

            // Settings footer buttons
            HStack(spacing: 8) {
                Button {
                    saveSettings()
                } label: {
                    Text("Save")
                        .font(.sbHeading(size: 12.5, weight: .semibold))
                        .foregroundColor(SBTheme.accent300)
                        .padding(.horizontal, 13)
                        .padding(.vertical, 6)
                        .background(isHoveringSave ? SBTheme.accentHover : Color.clear)
                        .overlay(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(isHoveringSave ? SBTheme.accent400 : SBTheme.accent600, lineWidth: 1)
                        )
                        .cornerRadius(3)
                }
                .buttonStyle(.plain)
                .onHover { isHoveringSave = $0 }

                Button {
                    clearKey()
                } label: {
                    Text("Clear key")
                        .font(.sbHeading(size: 12.5, weight: .semibold))
                        .foregroundColor(isHoveringClear ? SBTheme.neutral200 : SBTheme.neutral400)
                        .padding(.horizontal, 13)
                        .padding(.vertical, 6)
                        .background(isHoveringClear ? Color.white.opacity(0.07) : Color.clear)
                        .overlay(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(SBTheme.borderSubtle, lineWidth: 1)
                        )
                        .cornerRadius(3)
                }
                .buttonStyle(.plain)
                .onHover { isHoveringClear = $0 }

                if savedFlash {
                    Text("Saved")
                        .font(.system(size: 11))
                        .foregroundColor(SBTheme.accent300)
                        .transition(.opacity)
                }

                Spacer()

                Text("key source: \(KeychainStore.load() != nil ? "Keychain" : "config.yaml")")
                    .font(.system(size: 10.5))
                    .monospacedDigit()
                    .foregroundColor(SBTheme.neutral600)
            }
        }
        .onAppear {
            if let client = model.api as? APIClient {
                baseURL = client.baseURL.absoluteString
            }
            apiKey = KeychainStore.load() ?? (model.api as? APIClient)?.apiKey ?? ""
            loginItemEnabled = SMAppService.mainApp.status == .enabled
        }
    }

    private func pollSegment(_ text: String, minutes: Double, hasDivider: Bool) -> some View {
        let isSelected = abs(intervalMinutes - minutes) < 0.01
        return Button {
            intervalMinutes = minutes
            model.pollInterval = minutes * 60
        } label: {
            HStack(spacing: 0) {
                Text(text)
                    .font(.system(size: 12))
                    .monospacedDigit()
                    .foregroundColor(isSelected ? SBTheme.accent300 : SBTheme.neutral400)
                    .padding(.horizontal, 13)
                    .padding(.vertical, 5)
                    .background(isSelected ? SBTheme.accentHover : Color.clear)

                if hasDivider {
                    Rectangle()
                        .fill(SBTheme.borderSubtle)
                        .frame(width: 1, height: 24)
                }
            }
        }
        .buttonStyle(.plain)
    }

    private func toggleLoginItem() {
        let target = !loginItemEnabled
        do {
            if target {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
            loginItemEnabled = SMAppService.mainApp.status == .enabled
        } catch {
            loginItemEnabled = SMAppService.mainApp.status == .enabled
        }
    }

    private func saveSettings() {
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
        withAnimation {
            savedFlash = true
        }
        Task {
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            withAnimation {
                savedFlash = false
            }
            await model.refresh(force: true)
        }
    }

    private func clearKey() {
        KeychainStore.delete()
        apiKey = ""
    }
}
