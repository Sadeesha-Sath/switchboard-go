import Foundation
import Security

/// Stores the manually-entered proxy key. The auto-discovered key from
/// config.yaml is NOT persisted here — it is re-read each launch so config
/// changes are picked up (Keychain wins over config when both exist).
public enum KeychainStore {
    private static let service = "switchboard-go-menu-bar"
    private static let account = "proxy-api-key"

    private static var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true,
        ]
    }

    public static func save(_ key: String) {
        let data = Data(key.utf8)
        let status = SecItemCopyMatching(baseQuery as CFDictionary, nil)
        if status == errSecSuccess {
            SecItemUpdate(baseQuery as CFDictionary,
                          [kSecValueData as String: data] as CFDictionary)
        } else {
            var add = baseQuery
            add[kSecValueData as String] = data
            SecItemAdd(add as CFDictionary, nil)
        }
    }

    public static func load() -> String? {
        var item: CFTypeRef?
        var query = baseQuery
        query[kSecReturnData as String] = true
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data,
              let value = String(data: data, encoding: .utf8), !value.isEmpty else {
            return nil
        }
        return value
    }

    public static func delete() {
        SecItemDelete(baseQuery as CFDictionary)
    }
}
