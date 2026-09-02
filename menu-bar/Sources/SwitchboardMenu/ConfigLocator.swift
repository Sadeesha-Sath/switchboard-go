import Foundation
import Yams

public struct ProxyConfig: Equatable, Sendable {
    public var baseURL: URL
    public var proxyAPIKey: String
    public var configSource: String
}

public enum ConfigLocator {
    /// Mirrors the Go binary's discovery order (main.go resolveConfigPath):
    /// SWITCHBOARD_GO_CONFIG, then <cwd>/config.yaml|yml, then
    /// ~/.config/switchboard-go/config.yaml, then /etc/switchboard-go/config.yaml.
    public static func locate(
        cwd: String = FileManager.default.currentDirectoryPath,
        home: String = NSHomeDirectory(),
        env: [String: String] = ProcessInfo.processInfo.environment
    ) -> ProxyConfig? {
        var paths: [String] = []
        if let explicit = env["SWITCHBOARD_GO_CONFIG"], !explicit.isEmpty {
            paths.append(explicit)
        }
        paths.append(cwd + "/config.yaml")
        paths.append(cwd + "/config.yml")
        paths.append(home + "/.config/switchboard-go/config.yaml")
        paths.append("/etc/switchboard-go/config.yaml")

        for path in paths where FileManager.default.fileExists(atPath: path) {
            guard let yaml = try? String(contentsOfFile: path, encoding: .utf8),
                  var cfg = parse(yaml: yaml) else { continue }
            cfg = applyEnvOverrides(cfg, env: env)!
            cfg.configSource = path
            return cfg
        }
        return nil
    }

    /// Extracts server.listen_addr and server.proxy_api_key. Returns nil when
    /// the YAML is unparseable or no proxy key is configured.
    public static func parse(yaml: String) -> ProxyConfig? {
        guard let doc = try? Yams.load(yaml: yaml) as? [String: Any] else { return nil }
        let server = doc["server"] as? [String: Any] ?? [:]
        let listen = server["listen_addr"] as? String ?? ":8495"
        guard let key = server["proxy_api_key"] as? String, !key.isEmpty else { return nil }
        return ProxyConfig(baseURL: normalize(listen), proxyAPIKey: key, configSource: "")
    }

    /// ":8495" and "0.0.0.0:8495" are reachable on loopback — pin to 127.0.0.1.
    public static func normalize(_ listenAddr: String) -> URL {
        var addr = listenAddr.trimmingCharacters(in: .whitespaces)
        if addr.hasPrefix(":") {
            addr = "127.0.0.1" + addr
        } else if addr.hasPrefix("0.0.0.0:") {
            addr = "127.0.0.1" + addr.dropFirst("0.0.0.0".count)
        }
        return URL(string: "http://" + addr)!
    }

    /// Matches the Go binary's env precedence (main.go applyEnvOverrides).
    public static func applyEnvOverrides(_ cfg: ProxyConfig?, env: [String: String]) -> ProxyConfig? {
        guard var cfg else { return nil }
        if let key = env["PROXY_API_KEY"], !key.isEmpty {
            cfg.proxyAPIKey = key
        }
        if let listen = env["LISTEN_ADDR"], !listen.isEmpty {
            cfg.baseURL = normalize(listen)
        }
        return cfg
    }
}
