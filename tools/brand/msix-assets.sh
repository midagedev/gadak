#!/usr/bin/env bash
# Regenerates the Microsoft Store (MSIX) logo set from the brand source.
#
# The Windows runner that packs the .msix has no image tool we can rely on,
# so these PNGs are generated here and committed, the same way the phone
# icons are (tools/brand/mobile-icons.sh). AppxManifest.xml names the three
# base files; Windows picks the .scale-200 sibling on high-DPI displays.
#
# Usage: bash tools/brand/msix-assets.sh   (or: make brand)
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
src="$repo/docs/media/logo.png"
out="$repo/desktop/msix/Assets"

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

[ -f "$src" ] || { echo "missing brand source: $src (run: node tools/brand/render.mjs)" >&2; exit 1; }

resize() { # resize <px> <dest>
	if command -v sips >/dev/null 2>&1; then
		sips -z "$1" "$1" "$src" --out "$2" >/dev/null
	elif command -v magick >/dev/null 2>&1; then
		magick "$src" -resize "${1}x${1}" "$2"
	else
		echo "msix-assets: need sips (macOS) or ImageMagick" >&2
		exit 69
	fi
}

mkdir -p "$out"
resize 150 "$out/Square150x150Logo.png"
resize 300 "$out/Square150x150Logo.scale-200.png"
resize 44 "$out/Square44x44Logo.png"
resize 88 "$out/Square44x44Logo.scale-200.png"
resize 50 "$out/StoreLogo.png"
resize 100 "$out/StoreLogo.scale-200.png"

cat > "$out/SOURCE.sha256" <<EOF
# The brand source these logos were generated from.
# Regenerate: make brand   (tools/brand/msix-assets.sh)
# Verify:     bash tools/check-brand-icons.sh
docs/media/logo.png  $(sha256 "$src")
EOF

echo "msix assets: regenerated from docs/media/logo.png"
