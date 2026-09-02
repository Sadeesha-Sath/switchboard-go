import XCTest
@testable import SwitchboardMenu

final class ConfigLocatorTests: XCTestCase {
    private let sampleYAML = """
    server:
      listen_addr: "127.0.0.1:8495"
      proxy_api_key: "test-proxy-key-123"

    upstream:
      base_url: "https://opencode.ai/zen/go/v1"
      api_keys:
        - key: "sk-upstream-1"
          priority: 1
          weight: 1
    """

    func testParseExtractsServerSection() {
        let cfg = ConfigLocator.parse(yaml: sampleYAML)
        XCTAssertNotNil(cfg)
        XCTAssertEqual(cfg?.baseURL.absoluteString, "http://127.0.0.1:8495")
        XCTAssertEqual(cfg?.proxyAPIKey, "test-proxy-key-123")
    }

    func testParseRejectsConfigWithoutProxyKey() {
        let yaml = "server:\n  listen_addr: \"127.0.0.1:8495\"\n"
        XCTAssertNil(ConfigLocator.parse(yaml: yaml))
    }

    func testParseRejectsInvalidYAML() {
        XCTAssertNil(ConfigLocator.parse(yaml: "::::not yaml::::"))
    }

    func testNormalize() {
        XCTAssertEqual(ConfigLocator.normalize(":8495").absoluteString, "http://127.0.0.1:8495")
        XCTAssertEqual(ConfigLocator.normalize("0.0.0.0:8495").absoluteString, "http://127.0.0.1:8495")
        XCTAssertEqual(ConfigLocator.normalize("127.0.0.1:8495").absoluteString, "http://127.0.0.1:8495")
    }

    func testEnvOverrides() {
        var cfg = ConfigLocator.parse(yaml: sampleYAML)
        cfg = ConfigLocator.applyEnvOverrides(cfg, env: ["PROXY_API_KEY": "env-key", "LISTEN_ADDR": "127.0.0.1:9000"])
        XCTAssertEqual(cfg?.proxyAPIKey, "env-key")
        XCTAssertEqual(cfg?.baseURL.absoluteString, "http://127.0.0.1:9000")
    }

    func testEmptyEnvDoesNotOverride() {
        var cfg = ConfigLocator.parse(yaml: sampleYAML)
        cfg = ConfigLocator.applyEnvOverrides(cfg, env: [:])
        XCTAssertEqual(cfg?.proxyAPIKey, "test-proxy-key-123")
    }

    func testLocateUsesExplicitEnvPath() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("myconfig.yaml")
        try sampleYAML.write(to: file, atomically: true, encoding: .utf8)
        defer { try? FileManager.default.removeItem(at: dir) }

        let cfg = ConfigLocator.locate(cwd: dir.path, home: dir.path, env: ["SWITCHBOARD_GO_CONFIG": file.path])
        XCTAssertEqual(cfg?.proxyAPIKey, "test-proxy-key-123")
        XCTAssertEqual(cfg?.configSource, file.path)
    }

    func testLocateFallsBackToCWDConfig() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("config.yaml")
        try sampleYAML.write(to: file, atomically: true, encoding: .utf8)
        defer { try? FileManager.default.removeItem(at: dir) }

        let cfg = ConfigLocator.locate(cwd: dir.path, home: dir.path, env: [:])
        XCTAssertEqual(cfg?.configSource, file.path)
    }

    func testLocateReturnsNilWhenNothingExists() {
        let empty = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true).path
        let cfg = ConfigLocator.locate(cwd: empty, home: empty, env: [:])
        XCTAssertNil(cfg)
    }
}
