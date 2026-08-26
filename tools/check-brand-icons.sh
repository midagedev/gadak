#!/usr/bin/env bash
# Fails when the phone app's icons no longer derive from the brand source.
#
# Two ways they drift, one check each:
#   1. docs/media/logo.png changes and nobody re-runs `make brand` — the
#      desktop picks the new mark up on its next build (it resizes the logo
#      at build time) while the phone keeps shipping the old one.
#   2. The iOS set exists twice in the tree — icons/ios/ and the Xcode asset
#      catalog — and `tauri icon` writes only the catalog once gen/apple is
#      present, so the other copy silently stays behind.
#
# Usage: bash tools/check-brand-icons.sh
set -uo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$repo/docs/media/logo.png"
stamp="$repo/mobile/src-tauri/icons/SOURCE.sha256"
ios="$repo/mobile/src-tauri/icons/ios"
appiconset="$repo/mobile/src-tauri/gen/apple/Assets.xcassets/AppIcon.appiconset"

fail=0
note() { echo "brand-icons: $*" >&2; fail=1; }

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

if [ ! -f "$src" ]; then
	note "missing brand source docs/media/logo.png"
elif [ ! -f "$stamp" ]; then
	note "missing $stamp — the phone icons record no source"
else
	want=$(sha256 "$src")
	got=$(awk '$1 == "docs/media/logo.png" { print $2 }' "$stamp")
	if [ "$want" != "$got" ]; then
		note "phone icons were generated from a different logo.png"
		note "  logo.png now: $want"
		note "  icons stamped: ${got:-<unreadable>}"
	fi
fi

if [ -d "$appiconset" ]; then
	for f in "$appiconset"/*.png; do
		mirror="$ios/$(basename "$f")"
		if [ ! -f "$mirror" ]; then
			note "$(basename "$f") is in the asset catalog but not icons/ios/"
		elif ! cmp -s "$f" "$mirror"; then
			note "$(basename "$f") differs between icons/ios/ and the asset catalog"
		fi
	done
fi

if [ "$fail" -ne 0 ]; then
	echo "brand-icons: fix with  make brand  (regenerates from docs/media/logo.png and restamps)" >&2
	exit 1
fi

echo "brand-icons: phone icons match docs/media/logo.png"
