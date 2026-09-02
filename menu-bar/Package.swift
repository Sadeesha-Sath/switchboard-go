// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SwitchboardMenu",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SwitchboardMenu", targets: ["SwitchboardMenu"])
    ],
    dependencies: [
        .package(url: "https://github.com/jpsim/Yams.git", from: "5.1.0"),
    ],
    targets: [
        .executableTarget(name: "SwitchboardMenu", dependencies: ["Yams"]),
        .testTarget(name: "SwitchboardMenuTests", dependencies: ["SwitchboardMenu"]),
    ]
)
