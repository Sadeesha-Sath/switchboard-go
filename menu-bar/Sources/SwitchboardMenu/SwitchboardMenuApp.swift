import SwiftUI
import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // Task 9 replaces DebugNotifier with SystemNotifier and moves the
        // authorization request here. For now nothing to do.
    }
}

/// Temporary notifier so the app compiles before Task 9 lands.
struct DebugNotifier: Notifying {
    func post(title: String, body: String) {
        NSLog("switchboard-menubar: %@ — %@", title, body)
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
        _model = StateObject(wrappedValue: StatusModel(api: client, notifier: DebugNotifier(), interval: 30))
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
