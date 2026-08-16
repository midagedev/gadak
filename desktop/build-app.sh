#!/usr/bin/env bash
# Build Gadak.app (and optionally a .dmg) from the desktop module.
#
# Prerequisites: `npm run build` at the repo root (the app embeds dist/app),
# Xcode command line tools. Run from anywhere; paths are repo-relative.
#
# Usage: desktop/build-app.sh [--dmg]
#
# Optional signing / notarization (off by default for local development):
#   GADAK_SIGN_IDENTITY          codesign identity (e.g. "Developer ID Application: …")
#   GADAK_NOTARY_KEY             path to App Store Connect API key (.p8)
#   GADAK_NOTARY_KEY_ID          key id
#   GADAK_NOTARY_ISSUER_ID       issuer id
# Notarization runs only when --dmg and all three GADAK_NOTARY_* vars are set.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
out="$repo/desktop/build"
app="$out/Gadak.app"
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

# wails v3 declares its own -framework UniformTypeIdentifiers (v2 did not, and
# needed the flag passed in here).
#
# appVersion is what the sidebar banner compares (desktop/main.go copies it
# onto server.Version). A release tag stamps a bare semver; the "dev" default
# is mapped to 0.0.0-dev and never checks.
(cd "$repo/desktop" && go build -tags desktop,production -trimpath \
  -ldflags "-s -w -X main.appVersion=${version#v}" \
  -o "$app/Contents/MacOS/gadak-desktop" .)

# CLI for agent wiring (gadak mcp install, gadak sql, …) without a separate brew
# install. Built from the root module so dist/app stays embedded. The version
# has to be stamped the way goreleaser stamps the standalone binary, or the
# bundled copy answers `gadak --version` with its compile-time default and the
# two CLIs from one release disagree about what release they are.
(cd "$repo" && CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=${version#v}" \
  -o "$app/Contents/Resources/bin/gadak" ./cmd/gadak)

# Icon: the 1024px logo becomes the full iconset.
iconset="$out/gadak.iconset"
rm -rf "$iconset" && mkdir -p "$iconset"
src="$repo/docs/media/logo.png"
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" "$src" --out "$iconset/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  sips -z "$d" "$d" "$src" --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/gadak.icns"

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Gadak</string>
	<key>CFBundleDisplayName</key><string>Gadak</string>
	<key>CFBundleIdentifier</key><string>com.midagedev.gadak</string>
	<key>CFBundleExecutable</key><string>gadak-desktop</string>
	<key>CFBundleIconFile</key><string>gadak</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>${version#v}</string>
	<key>CFBundleVersion</key><string>${version#v}</string>
	<key>LSMinimumSystemVersion</key><string>12.0</string>
	<key>NSHighResolutionCapable</key><true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key><true/>
	<!-- gadak:// — the one way to hand someone a view without a shell.
	     An agent that built a filtered list can put a link in chat; clicking
	     it launches or raises this app on that view. LaunchServices reads
	     this on install, so a scheme added here reaches a machine only after
	     the new bundle lands there.

	     Read-only by construction: the handler in main.go accepts nothing
	     but a view hash, so the worst a hostile page's gadak:// link can
	     achieve is that you look at the wrong list. Do not add a verb. -->
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key><string>com.midagedev.gadak.view</string>
			<key>CFBundleTypeRole</key><string>Viewer</string>
			<key>CFBundleURLSchemes</key>
			<array><string>gadak</string></array>
		</dict>
	</array>
</dict>
</plist>
PLIST

sign_if_configured() {
  if [[ -z "${GADAK_SIGN_IDENTITY:-}" ]]; then
    echo "GADAK_SIGN_IDENTITY unset — leaving bundle unsigned (local-dev default)"
    return 0
  fi
  local id="$GADAK_SIGN_IDENTITY"
  echo "signing with identity: $id"
  # Nested binaries first, then the bundle (Apple prefers no --deep).
  # Hardened runtime; no entitlements file — Go needs none for this app today.
  codesign --force --options runtime --timestamp -s "$id" \
    "$app/Contents/MacOS/gadak-desktop"
  codesign --force --options runtime --timestamp -s "$id" \
    "$app/Contents/Resources/bin/gadak"
  codesign --force --options runtime --timestamp -s "$id" "$app"
  codesign --verify --strict --verbose=2 "$app"
  echo "signed $app"
}

sign_if_configured

echo "built $app ($version, $arch)"

dmg=""
if [[ "${1:-}" == "--dmg" ]]; then
  dmg="$out/Gadak-${version#v}-${arch}.dmg"
  rm -f "$dmg"
  stage="$out/dmg-stage"
  rm -rf "$stage" && mkdir -p "$stage"
  cp -R "$app" "$stage/"
  ln -s /Applications "$stage/Applications"
  hdiutil create -volname Gadak -srcfolder "$stage" -ov -format UDZO "$dmg" >/dev/null
  rm -rf "$stage"

  if [[ -n "${GADAK_SIGN_IDENTITY:-}" ]]; then
    codesign --force --timestamp -s "$GADAK_SIGN_IDENTITY" "$dmg"
    codesign --verify --verbose=2 "$dmg"
    echo "signed $dmg"
  fi

  if [[ -n "${GADAK_NOTARY_KEY:-}" && -n "${GADAK_NOTARY_KEY_ID:-}" && -n "${GADAK_NOTARY_ISSUER_ID:-}" ]]; then
    echo "submitting $dmg for notarization"
    xcrun notarytool submit "$dmg" \
      --key "$GADAK_NOTARY_KEY" \
      --key-id "$GADAK_NOTARY_KEY_ID" \
      --issuer "$GADAK_NOTARY_ISSUER_ID" \
      --wait
    xcrun stapler staple "$dmg"
    xcrun stapler staple "$app"
    echo "notarized and stapled $dmg"
  elif [[ -n "${GADAK_NOTARY_KEY:-}${GADAK_NOTARY_KEY_ID:-}${GADAK_NOTARY_ISSUER_ID:-}" ]]; then
    echo "partial GADAK_NOTARY_* set — need KEY (path), KEY_ID, and ISSUER_ID; skipping notarization" >&2
  fi

  # Notify-only (2026-08-15): the dmg is the only desktop artifact. Do not
  # emit gadak-desktop-darwin-<arch>.zip — v0.14.0 apps match that name
  # exactly and would self-swap.
  echo "built $dmg"
fi
