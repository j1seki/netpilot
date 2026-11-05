#!/usr/bin/env bash
# Builds NetPilot macOS app bundle & DMG containing both NetPilot and official net-merge binary
set -euo pipefail

VERSION="${1:-0.1.0}"
VERSION="${VERSION#v}"
OUT_DIR="$(realpath "${2:-dist}")"
mkdir -p "$OUT_DIR"

echo "=== Building NetPilot macOS Universal Binary ==="
mkdir -p /tmp/np-mac-bin
for arch in amd64 arm64; do
  CGO_ENABLED=1 GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /tmp/np-mac-bin/netpilot-$arch ./cmd/netpilot
done
lipo -create -output /tmp/np-mac-bin/netpilot /tmp/np-mac-bin/netpilot-amd64 /tmp/np-mac-bin/netpilot-arm64
codesign --force --sign - /tmp/np-mac-bin/netpilot 2>/dev/null || true

echo "=== Pulling latest official net-merge binary from net-merge/net-merge ==="
mkdir -p /tmp/nm-mac-dl
gh release download --repo net-merge/net-merge --pattern 'net-merge-macos-universal.tar.gz' --dir /tmp/nm-mac-dl --clobber
tar -xzf /tmp/nm-mac-dl/net-merge-macos-universal.tar.gz -C /tmp/nm-mac-dl/

echo "=== Creating macOS App Bundle ==="
APP="NetPilot.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$APP/Contents/Helpers"

cp /tmp/np-mac-bin/netpilot "$APP/Contents/MacOS/netpilot"
cp /tmp/nm-mac-dl/net-merge "$APP/Contents/Helpers/net-merge"
chmod +x "$APP/Contents/MacOS/netpilot" "$APP/Contents/Helpers/net-merge"

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

codesign --force --deep --sign - "$APP" 2>/dev/null || true

echo "=== Creating DMG and Universal Tarball ==="
# 1. DMG installer
mkdir -p /tmp/dmg-root
cp -r "$APP" /tmp/dmg-root/
ln -s /Applications /tmp/dmg-root/Applications
cp LICENSE THIRD-PARTY-NOTICES.md /tmp/dmg-root/ 2>/dev/null || true

if command -v hdiutil >/dev/null 2>&1; then
  hdiutil create -volname "NetPilot" -srcfolder /tmp/dmg-root -ov -format UDZO "$OUT_DIR/NetPilot-$VERSION-macOS-universal.dmg"
elif command -v create-dmg >/dev/null 2>&1; then
  create-dmg --volname "NetPilot" "$OUT_DIR/NetPilot-$VERSION-macOS-universal.dmg" /tmp/dmg-root
else
  echo "hdiutil not found (not macOS runner?); creating tar.gz instead"
fi

# 2. Universal Tarball
tar -czf "$OUT_DIR/netpilot-$VERSION-macos-universal.tar.gz" -C /tmp/dmg-root .

rm -rf /tmp/np-mac-bin /tmp/nm-mac-dl /tmp/dmg-root "$APP"
echo "=== Done building macOS artifacts ==="
ls -lh "$OUT_DIR"
