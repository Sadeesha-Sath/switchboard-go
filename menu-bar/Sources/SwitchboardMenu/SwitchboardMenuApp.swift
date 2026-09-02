import SwiftUI
import AppKit

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let model: StatusModel

    override init() {
        let env = ProcessInfo.processInfo.environment
        let discovered = ConfigLocator.locate(env: env)
        let baseURL = discovered?.baseURL ?? URL(string: "http://127.0.0.1:8495")!
        let key = KeychainStore.load() ?? discovered?.proxyAPIKey ?? ""
        let storedInterval = UserDefaults.standard.double(forKey: "sb_menubar_interval")
        let interval: TimeInterval = storedInterval > 0 ? storedInterval * 60 : 30
        model = StatusModel(api: APIClient(baseURL: baseURL, apiKey: key),
                            notifier: SystemNotifier.shared, interval: interval)
        super.init()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        SystemNotifier.shared.requestAuthorization()
        model.startPolling()
    }

    func applicationWillTerminate(_ notification: Notification) {
        model.stopPolling()
    }
}

@main
struct SwitchboardMenuApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            PopoverContentView()
                .environmentObject(delegate.model)
        } label: {
            StatusLabelView(model: delegate.model)
        }
        .menuBarExtraStyle(.window)
    }
}
