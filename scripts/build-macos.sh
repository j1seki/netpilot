#!/usr/bin/env bash
# Builds NetPilot macOS app bundle & DMG containing both NetPilot and official net-merge binary
set -euo pipefail

VERSION="${1:-0.1.0}"
VERSION="${VERSION#v}"
mkdir -p "${2:-dist}"
OUT_DIR="$(cd "${2:-dist}" && pwd -P)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

go run scripts/bundle-net-merge.go darwin arm64

echo "=== Building single-binary NetPilot macOS Universal Binary (netpilot = GUI + net-merge CLI + tray) ==="
mkdir -p "$work/bin" "$work/dmg"

for arch in amd64 arm64; do
  CGO_ENABLED=1 GOARCH=$arch go build -trimpath -ldflags "-s -w -X netpilot/internal/branding.AppVersion=$VERSION" -o "$work/bin/netpilot-$arch" ./cmd/netpilot
done
lipo -create -output "$work/bin/netpilot" "$work/bin/netpilot-amd64" "$work/bin/netpilot-arm64"

echo "=== Creating macOS App Bundle ==="
APP="$work/dmg/NetPilot.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$APP/Contents/Helpers"

cp "$work/bin/netpilot" "$APP/Contents/MacOS/netpilot"
chmod +x "$APP/Contents/MacOS/netpilot"

# Generate or copy icon
if [ -f packaging/icons/NetPilot.icns ]; then
  cp packaging/icons/NetPilot.icns "$APP/Contents/Resources/AppIcon.icns"
fi

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>netpilot</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>com.netpilot.desktop</string>
    <key>CFBundleName</key>
    <string>NetPilot</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleVersion</key>
    <string>$VERSION</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

codesign --force --deep --sign - "$APP"

echo "=== Creating DMG and Universal Tarball ==="
# 1. DMG installer
install -m 755 'packaging/Open NetPilot.command' "$work/dmg/Open NetPilot.command"
ln -s /Applications "$work/dmg/Applications"
cp LICENSE THIRD-PARTY-NOTICES.md "$work/dmg/"

if command -v hdiutil >/dev/null 2>&1; then
  hdiutil create -volname "NetPilot" -srcfolder "$work/dmg" -ov -format UDZO "$OUT_DIR/NetPilot-$VERSION-macOS-universal.dmg"
elif command -v create-dmg >/dev/null 2>&1; then
  create-dmg --volname "NetPilot" "$OUT_DIR/NetPilot-$VERSION-macOS-universal.dmg" "$work/dmg"
else
  echo "hdiutil not found (not macOS runner?); creating tar.gz instead"
fi

# 2. Universal Tarball
tar -czf "$OUT_DIR/netpilot-$VERSION-macos-universal.tar.gz" -C "$work/dmg" .

echo "=== Done building macOS artifacts ==="
ls -lh "$OUT_DIR"
