#!/bin/bash
# Package SwitchboardMenu as a macOS .app bundle.
#
# Why: `swift build -c release` produces a raw Mach-O with no
# CFBundleIdentifier / Info.plist. As a bare binary the menu bar app
#   - dies with the terminal (SIGHUP) when launched with `&`
#   - shows a Dock icon (no LSUIElement)
#   - cannot use SMAppService login-item or UNUserNotificationCenter properly
#
# Usage:
#   ./package-app.sh [--install] [--open] [--no-build] [--bundle-id ID] [--icon PATH] [--no-icon]
#
#   --install     copy the .app to /Applications (required for Launch at login)
#   --open        open the bundled app after packaging
#   --no-build    skip `swift build`, just re-bundle the existing binary
#   --bundle-id   override CFBundleIdentifier (default: com.switchboard.menu)
#   --icon PATH   use a custom PNG or SVG as the .app icon source
#                 (default: ../assets/favicon.svg)
#   --no-icon     skip icon generation (Finder shows a generic icon)

set -euo pipefail

APP_NAME="SwitchboardMenu"
BUNDLE_ID="com.switchboard.menu"
INSTALL=0
OPEN_APP=0
BUILD=1
NO_ICON=0
ICON_SRC=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install) INSTALL=1; shift ;;
    --open) OPEN_APP=1; shift ;;
    --no-build) BUILD=0; shift ;;
    --bundle-id)
      BUNDLE_ID="${2:?--bundle-id requires a value}"; shift 2 ;;
    --icon)
      ICON_SRC="${2:?--icon requires a value}"; shift 2 ;;
    --no-icon) NO_ICON=1; shift ;;
    -h|--help)
      sed -n '2,22p' "$0"; exit 0 ;;
    *) echo "unknown flag: $1 (see --help)" >&2; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/.build/release/$APP_NAME"
APP_BUNDLE="$SCRIPT_DIR/$APP_NAME.app"

if [[ "$BUILD" -eq 1 ]]; then
  (cd "$SCRIPT_DIR" && swift build -c release)
fi

if [[ ! -x "$BINARY" ]]; then
  echo "error: release binary not found at $BINARY" >&2
  echo "run without --no-build first" >&2
  exit 1
fi

rm -rf "$APP_BUNDLE"
mkdir -p "$APP_BUNDLE/Contents/MacOS" "$APP_BUNDLE/Contents/Resources"

cp "$BINARY" "$APP_BUNDLE/Contents/MacOS/$APP_NAME"
chmod +x "$APP_BUNDLE/Contents/MacOS/$APP_NAME"

# App icon: generate AppIcon.icns from an image source (default:
# repo assets/favicon.svg — gold mark on the dark rounded tile) with
# rasterized first (rsvg-convert preferred, qlmanage fallback). Skipped
# with --no-icon or when the tools/source are unavailable.
ICON_NAME=""
if [[ "$NO_ICON" -eq 0 ]]; then
  if [[ -z "$ICON_SRC" ]]; then
    ICON_SRC="$SCRIPT_DIR/../assets/favicon.svg"
  fi
  if [[ ! -f "$ICON_SRC" ]]; then
    echo "warning: icon source not found at $ICON_SRC, skipping icon" >&2
  elif ! command -v sips >/dev/null 2>&1 || ! command -v iconutil >/dev/null 2>&1; then
    echo "warning: sips/iconutil not available, skipping icon" >&2
  else
    ICON_TMP="$(mktemp -d)"
    trap 'rm -rf "$ICON_TMP"' EXIT
    RENDER_SRC="$ICON_SRC"
    if [[ "$ICON_SRC" == *.svg ]]; then
      if command -v rsvg-convert >/dev/null 2>&1; then
        rsvg-convert -w 1024 -h 1024 "$ICON_SRC" -o "$ICON_TMP/source.png"
        RENDER_SRC="$ICON_TMP/source.png"
      elif command -v qlmanage >/dev/null 2>&1; then
        # qlmanage renders SVGs at their declared size, so upscale a temp
        # copy to 1024 first (viewBox keeps it sharp).
        sed 's/width="[0-9.]*" height="[0-9.]*"/width="1024" height="1024"/' "$ICON_SRC" > "$ICON_TMP/source.svg"
        mkdir -p "$ICON_TMP/ql"
        qlmanage -t -s 1024 -o "$ICON_TMP/ql" "$ICON_TMP/source.svg" >/dev/null 2>&1
        RENDER_SRC="$(find "$ICON_TMP/ql" -type f -name '*.png' | head -n 1)"
        if [[ -z "$RENDER_SRC" || ! -f "$RENDER_SRC" ]]; then
          echo "warning: could not rasterize $ICON_SRC, skipping icon" >&2
          RENDER_SRC=""
        fi
      else
        echo "warning: no SVG rasterizer (rsvg-convert/qlmanage) available, skipping icon" >&2
        RENDER_SRC=""
      fi
    fi
    if [[ -n "$RENDER_SRC" ]]; then
      ICONSET="$ICON_TMP/AppIcon.iconset"
      mkdir -p "$ICONSET"
      for spec in "16:icon_16x16.png" "32:icon_16x16@2x.png" "32:icon_32x32.png" "64:icon_32x32@2x.png" "128:icon_128x128.png" "256:icon_128x128@2x.png" "256:icon_256x256.png" "512:icon_256x256@2x.png" "512:icon_512x512.png" "1024:icon_512x512@2x.png"; do
        size="${spec%%:*}"
        name="${spec##*:}"
        sips -z "$size" "$size" "$RENDER_SRC" --out "$ICONSET/$name" >/dev/null
      done
      iconutil -c icns "$ICONSET" -o "$APP_BUNDLE/Contents/Resources/AppIcon.icns"
      ICON_NAME="AppIcon"
      echo "Icon built from $ICON_SRC"
    fi
    rm -rf "$ICON_TMP"
    trap - EXIT
  fi
fi

cat > "$APP_BUNDLE/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>$APP_NAME</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleName</key><string>$APP_NAME</string>
  <key>CFBundleVersion</key><string>1.0</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF

if [[ -n "$ICON_NAME" ]]; then
  /usr/libexec/PlistBuddy -c "Add :CFBundleIconFile string $ICON_NAME" "$APP_BUNDLE/Contents/Info.plist"
  /usr/libexec/PlistBuddy -c "Add :CFBundleIconName string $ICON_NAME" "$APP_BUNDLE/Contents/Info.plist"
fi

# Ad-hoc sign so the bundle runs locally and SMAppService accepts it.
# For distribution, replace `-` with a Developer ID identity.
codesign --force --deep --sign - "$APP_BUNDLE"

echo "Built $APP_BUNDLE"

if [[ "$INSTALL" -eq 1 ]]; then
  cp -R "$APP_BUNDLE" /Applications/
  echo "Installed to /Applications/$APP_NAME.app"
fi

if [[ "$OPEN_APP" -eq 1 ]]; then
  if [[ "$INSTALL" -eq 1 ]]; then
    open "/Applications/$APP_NAME.app"
  else
    open "$APP_BUNDLE"
  fi
fi
