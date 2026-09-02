import SwiftUI
import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        SystemNotifier.shared.requestAuthorization()
    }
}

@main
struct SwitchboardMenuApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model: StatusModel

    init() {
        let env = ProcessInfo.processInfo.environment
        let discovered = ConfigLocator.locate(env: env)
        let baseURL = discovered?.baseURL ?? URL(string: "http://127.0.0.1:8495")!
        let key = KeychainStore.load() ?? discovered?.proxyAPIKey ?? ""
        let client = APIClient(baseURL: baseURL, apiKey: key)
        _model = StateObject(wrappedValue: StatusModel(api: client, notifier: SystemNotifier.shared, interval: 30))
        // Idempotent (guarded); safe to call here — App.init runs on the main actor.
        model.startPolling()
    }

    var body: some Scene {
        MenuBarExtra {
            PopoverContentView()
                .environmentObject(model)
        } label: {
            StatusLabelView(model: model)
        }
        .menuBarExtraStyle(.window)
    }
}
