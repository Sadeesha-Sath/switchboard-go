import Foundation
import UserNotifications

final class SystemNotifier: Notifying {
    static let shared = SystemNotifier()

    /// Lazily resolved so bare-binary smoke (no bundle proxy) does not crash
    /// on `UNUserNotificationCenter.current()`. When `bundleIdentifier` is nil
    /// we are running outside an .app bundle — fall back to logging.
    private var center: UNUserNotificationCenter? {
        guard Bundle.main.bundleIdentifier != nil else { return nil }
        return UNUserNotificationCenter.current()
    }

    func requestAuthorization() {
        guard let center else { return }
        center.requestAuthorization(options: [.alert, .sound]) { _, _ in }
    }

    func post(title: String, body: String) {
        guard let center else {
            NSLog("switchboard-menubar: %@ — %@", title, body)
            return
        }
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        center.add(request)
    }
}
