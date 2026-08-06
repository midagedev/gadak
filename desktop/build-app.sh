#!/usr/bin/env bash
# Build Scry.app (and optionally a .dmg) from the desktop module.
#
# Prerequisites: `npm run build` at the repo root (the app embeds dist/app),
# Xcode command line tools. Run from anywhere; paths are repo-relative.
#
# Usage: desktop/build-app.sh [--dmg]
#
# Optional signing / notarization (off by default for local development):
#   SCRY_SIGN_IDENTITY          codesign identity (e.g. "Developer ID Application: …")
#   SCRY_NOTARY_KEY             path to App Store Connect API key (.p8)
#   SCRY_NOTARY_KEY_ID          key id
#   SCRY_NOTARY_ISSUER_ID       issuer id
# Notarization runs only when --dmg and all three SCRY_NOTARY_* vars are set.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
out="$repo/desktop/build"
app="$out/Scry.app"
version="$(cd "$repo" && git describe --tags --always 2>/dev/null || echo 0.0.0-dev)"
# Single-arch for now (macos-14 runners are arm64). File name carries the arch
# so a future universal build can ship alongside without colliding.
arch="$(uname -m)"
case "$arch" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=amd64 ;;
esac

test -f "$repo/dist/app/index.html" || {
  echo "dist/app missing — run \`npm run build\` at the repo root first" >&2
  exit 1
}

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/bin"

# UTType lives in UniformTypeIdentifiers on current SDKs; wails v2 does not
# link it by itself.
(cd "$repo/desktop" && CGO_LDFLAGS="-framework UniformTypeIdentifiers" \
  go build -tags desktop,production -trimpath \
  -ldflags "-s -w" \
  -o "$app/Contents/MacOS/scry-desktop" .)

# CLI for agent wiring (scry mcp install, scry sql, …) without a separate brew
# install. Built from the root module so dist/app stays embedded.
(cd "$repo" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" \
  -o "$app/Contents/Resources/bin/scry" ./cmd/scry)

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

sign_if_configured() {
  if [[ -z "${SCRY_SIGN_IDENTITY:-}" ]]; then
    echo "SCRY_SIGN_IDENTITY unset — leaving bundle unsigned (local-dev default)"
    return 0
  fi
  local id="$SCRY_SIGN_IDENTITY"
  echo "signing with identity: $id"
  # Nested binaries first, then the bundle (Apple prefers no --deep).
  # Hardened runtime; no entitlements file — Go needs none for this app today.
  codesign --force --options runtime --timestamp -s "$id" \
    "$app/Contents/MacOS/scry-desktop"
  codesign --force --options runtime --timestamp -s "$id" \
    "$app/Contents/Resources/bin/scry"
  codesign --force --options runtime --timestamp -s "$id" "$app"
  codesign --verify --strict --verbose=2 "$app"
  echo "signed $app"
}

sign_if_configured

echo "built $app ($version, $arch)"

dmg=""
if [[ "${1:-}" == "--dmg" ]]; then
  dmg="$out/Scry-${version#v}-${arch}.dmg"
  rm -f "$dmg"
  stage="$out/dmg-stage"
  rm -rf "$stage" && mkdir -p "$stage"
  cp -R "$app" "$stage/"
  ln -s /Applications "$stage/Applications"
  hdiutil create -volname Scry -srcfolder "$stage" -ov -format UDZO "$dmg" >/dev/null
  rm -rf "$stage"

  if [[ -n "${SCRY_SIGN_IDENTITY:-}" ]]; then
    codesign --force --timestamp -s "$SCRY_SIGN_IDENTITY" "$dmg"
    codesign --verify --verbose=2 "$dmg"
    echo "signed $dmg"
  fi

  if [[ -n "${SCRY_NOTARY_KEY:-}" && -n "${SCRY_NOTARY_KEY_ID:-}" && -n "${SCRY_NOTARY_ISSUER_ID:-}" ]]; then
    echo "submitting $dmg for notarization"
    xcrun notarytool submit "$dmg" \
      --key "$SCRY_NOTARY_KEY" \
      --key-id "$SCRY_NOTARY_KEY_ID" \
      --issuer "$SCRY_NOTARY_ISSUER_ID" \
      --wait
    xcrun stapler staple "$dmg"
    xcrun stapler staple "$app"
    echo "notarized and stapled $dmg"
  elif [[ -n "${SCRY_NOTARY_KEY:-}${SCRY_NOTARY_KEY_ID:-}${SCRY_NOTARY_ISSUER_ID:-}" ]]; then
    echo "partial SCRY_NOTARY_* set — need KEY (path), KEY_ID, and ISSUER_ID; skipping notarization" >&2
  fi

  echo "built $dmg"
fi
