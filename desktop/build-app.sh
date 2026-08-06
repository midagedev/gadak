#!/usr/bin/env bash
# Build Scry.app (and optionally a .dmg) from the desktop module.
#
# Prerequisites: `npm run build` at the repo root (the app embeds dist/app),
# Xcode command line tools. Run from anywhere; paths are repo-relative.
#
# Usage: desktop/build-app.sh [--dmg]
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
out="$repo/desktop/build"
app="$out/Scry.app"
version="$(cd "$repo" && git describe --tags --always 2>/dev/null || echo 0.0.0-dev)"

test -f "$repo/dist/app/index.html" || {
  echo "dist/app missing — run \`npm run build\` at the repo root first" >&2
  exit 1
}

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

# UTType lives in UniformTypeIdentifiers on current SDKs; wails v2 does not
# link it by itself.
(cd "$repo/desktop" && CGO_LDFLAGS="-framework UniformTypeIdentifiers" \
  go build -tags desktop,production -trimpath \
  -ldflags "-s -w" \
  -o "$app/Contents/MacOS/scry-desktop" .)

# Icon: the 1024px logo becomes the full iconset.
iconset="$out/scry.iconset"
rm -rf "$iconset" && mkdir -p "$iconset"
src="$repo/docs/media/logo.png"
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" "$src" --out "$iconset/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  sips -z "$d" "$d" "$src" --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/scry.icns"

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Scry</string>
	<key>CFBundleDisplayName</key><string>Scry</string>
	<key>CFBundleIdentifier</key><string>com.midagedev.scry</string>
	<key>CFBundleExecutable</key><string>scry-desktop</string>
	<key>CFBundleIconFile</key><string>scry</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>${version#v}</string>
	<key>CFBundleVersion</key><string>${version#v}</string>
	<key>LSMinimumSystemVersion</key><string>12.0</string>
	<key>NSHighResolutionCapable</key><true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key><true/>
</dict>
</plist>
PLIST

echo "built $app ($version)"

if [[ "${1:-}" == "--dmg" ]]; then
  dmg="$out/Scry-${version#v}.dmg"
  rm -f "$dmg"
  stage="$out/dmg-stage"
  rm -rf "$stage" && mkdir -p "$stage"
  cp -R "$app" "$stage/"
  ln -s /Applications "$stage/Applications"
  hdiutil create -volname Scry -srcfolder "$stage" -ov -format UDZO "$dmg" >/dev/null
  rm -rf "$stage"
  echo "built $dmg"
fi
