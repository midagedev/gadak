#!/usr/bin/env bash
# Regenerates the phone app's icon set from the brand source.
#
# The desktop app builds its .icns from docs/media/logo.png
# (desktop/build-app.sh), and that logo is itself rendered by
# tools/brand/render.mjs. The phone app was the one surface outside that
# chain — it shipped the Tauri scaffold default from the day the skeleton
# landed. This script puts it inside the chain.
#
# Two tracked copies of the iOS set exist and `tauri icon` writes only one:
# with gen/apple present it fills the asset catalog and leaves
# icons/ios/ untouched. icons/ios/ is what a future `tauri ios init` would
# copy back in, so it is mirrored here rather than left to rot.
#
# Usage: bash tools/brand/mobile-icons.sh   (or: make brand)
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
src="$repo/docs/media/logo.png"
icons="$repo/mobile/src-tauri/icons"
appiconset="$repo/mobile/src-tauri/gen/apple/Assets.xcassets/AppIcon.appiconset"

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

[ -f "$src" ] || { echo "missing brand source: $src (run: node tools/brand/render.mjs)" >&2; exit 1; }

(cd "$repo/mobile" && npx tauri icon "$src")

# tauri wrote the iOS set into the asset catalog; mirror it back to the
# checked-in source copy so the two never disagree.
if [ -d "$appiconset" ]; then
	cp "$appiconset"/*.png "$icons/ios/"
fi

cat > "$icons/SOURCE.sha256" <<EOF
# The brand source these icons were generated from.
# Regenerate: make brand   (tools/brand/mobile-icons.sh)
# Verify:     bash tools/check-brand-icons.sh
docs/media/logo.png  $(sha256 "$src")
EOF

echo "mobile icons: regenerated from docs/media/logo.png"
