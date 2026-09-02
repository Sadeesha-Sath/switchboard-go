// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SwitchboardMenu",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SwitchboardMenu", targets: ["SwitchboardMenu"])
    ],
    targets: [
        .executableTarget(name: "SwitchboardMenu"),
        .testTarget(name: "SwitchboardMenuTests", dependencies: ["SwitchboardMenu"]),
    ]
)
