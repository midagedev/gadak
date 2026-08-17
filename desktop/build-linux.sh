#!/usr/bin/env bash
# Build a Linux AppDir (and optionally an AppImage) from the desktop module.
#
# Prerequisites: `npm run build` at the repo root (the app embeds dist/app),
# a Linux host, CGO, GTK4 + WebKitGTK 6.0 development packages, pkg-config,
# a C compiler, and one of magick/convert/ffmpeg to resize the icon.
# `--appimage` also needs `appimagetool` on PATH. No wails3 CLI: plain
# `go build`, same contract as desktop/build-app.sh.
#
# Usage: desktop/build-linux.sh [--appimage]
#
# Exit 64 = usage / unknown argument
#      69 = a required tool is missing
#       1 = dist/app missing, not Linux, CGO_ENABLED=0, or a build step failed
#       0 = AppDir (and AppImage if --appimage) written
#
# The AppImage is the only desktop artifact this script may emit. Do not
# emit a zip — v0.14.0 apps match gadak-desktop-*-<arch>.zip and would
# self-swap (same rule as desktop/build-app.sh --dmg).
set -euo pipefail

usage() {
  echo "usage: desktop/build-linux.sh [--appimage]" >&2
  exit 64
}

need() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "build-linux: missing ${name}" >&2
    exit 69
  fi
}

want_appimage=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --appimage)
      want_appimage=1
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "build-linux: unknown argument: $1" >&2
      usage
      ;;
  esac
done

repo="$(cd "$(dirname "$0")/.." && pwd)"
out="$repo/desktop/build"
appdir="$out/Gadak.AppDir"

# Same formula as desktop/build-app.sh — eval that file's assignment so the
# two packs cannot drift. A shared helper is not in this round's file list.
version_line="$(grep -E '^version=' "$repo/desktop/build-app.sh" || true)"
if [[ -z "$version_line" ]]; then
  echo "build-linux: cannot find version= in desktop/build-app.sh" >&2
  exit 1
fi
# shellcheck disable=SC2294
eval "$version_line"
if [[ -z "${version:-}" ]]; then
  echo "build-linux: version stamp from desktop/build-app.sh was empty" >&2
  exit 1
fi

# AppImage / uname convention (x86_64, aarch64), not the macOS dmg labels
# (amd64, arm64). Filename: Gadak-<ver>-<this>.AppImage.
host_arch="$(uname -m)"
case "$host_arch" in
  x86_64|amd64) file_arch=x86_64 ;;
  aarch64|arm64) file_arch=aarch64 ;;
  *)
    echo "build-linux: unsupported architecture: ${host_arch}" >&2
    exit 1
    ;;
esac

need go
need pkg-config

icon_tool=""
for candidate in magick convert ffmpeg; do
  if command -v "$candidate" >/dev/null 2>&1; then
    icon_tool="$candidate"
    break
  fi
done
if [[ -z "$icon_tool" ]]; then
  echo "build-linux: missing magick, convert, or ffmpeg" >&2
  exit 69
fi

if [[ "$want_appimage" -eq 1 ]]; then
  need appimagetool
fi

src="$repo/docs/media/logo.png"
if [[ ! -f "$src" ]]; then
  echo "build-linux: missing icon source ${src}" >&2
  exit 1
fi

test -f "$repo/dist/app/index.html" || {
  echo "dist/app missing — run \`npm run build\` at the repo root first" >&2
  exit 1
}

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "build-linux: this runner is $(uname -s) ($(uname -m)); compiling the wails v3 Linux host needs Linux, CGO, GTK4, and WebKitGTK 6.0" >&2
  exit 1
fi

if [[ "${CGO_ENABLED:-1}" == "0" ]]; then
  echo "build-linux: CGO_ENABLED=0 cannot compile wails v3 on Linux" >&2
  exit 1
fi
export CGO_ENABLED=1

need "${CC:-cc}"

if ! pkg-config --exists gtk4; then
  echo "build-linux: missing pkg-config module gtk4" >&2
  exit 69
fi
if ! pkg-config --exists webkitgtk-6.0; then
  echo "build-linux: missing pkg-config module webkitgtk-6.0" >&2
  exit 69
fi

resize_png() {
  local size="$1" dest="$2"
  case "$icon_tool" in
    magick) magick "$src" -resize "${size}x${size}" "$dest" ;;
    convert) convert "$src" -resize "${size}x${size}" "$dest" ;;
    ffmpeg) ffmpeg -nostdin -hide_banner -loglevel error -y -i "$src" -vf "scale=${size}:${size}" "$dest" ;;
    *)
      echo "build-linux: internal: unknown icon tool ${icon_tool}" >&2
      exit 1
      ;;
  esac
}

rm -rf "$appdir"
mkdir -p \
  "$appdir/usr/bin" \
  "$appdir/usr/share/applications" \
  "$appdir/usr/share/icons/hicolor/256x256/apps" \
  "$appdir/usr/share/icons/hicolor/512x512/apps"

# wails v3 default Linux stack is GTK4 + WebKitGTK 6.0 (pkg-config gtk4
# webkitgtk-6.0). Do not pass -tags gtk3 — that is the webkit2gtk 4.1
# legacy path, scheduled for removal in v3.1.
#
# appVersion is what the sidebar banner compares (desktop/main.go copies it
# onto server.Version). Same ldflags as desktop/build-app.sh.
(cd "$repo/desktop" && go build -tags desktop,production -trimpath \
  -ldflags "-s -w -X main.appVersion=${version#v}" \
  -o "$appdir/usr/bin/gadak-desktop" .)

# CLI for agent wiring, same stamp as the standalone goreleaser binary.
(cd "$repo" && CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=${version#v}" \
  -o "$appdir/usr/bin/gadak" ./cmd/gadak)

chmod 0755 "$appdir/usr/bin/gadak-desktop" "$appdir/usr/bin/gadak"

resize_png 256 "$appdir/gadak.png"
resize_png 256 "$appdir/usr/share/icons/hicolor/256x256/apps/gadak.png"
resize_png 512 "$appdir/usr/share/icons/hicolor/512x512/apps/gadak.png"
cp "$appdir/gadak.png" "$appdir/.DirIcon"

# MimeType declares gadak://. Registering the handler is GDK-207, not this
# script — no xdg-mime / update-desktop-database call here.
cat > "$appdir/gadak.desktop" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Gadak
Exec=gadak-desktop %u
Icon=gadak
Terminal=false
Categories=Utility;
MimeType=x-scheme-handler/gadak;
EOF
cp "$appdir/gadak.desktop" "$appdir/usr/share/applications/gadak.desktop"

cat > "$appdir/AppRun" <<'EOF'
#!/bin/sh
self="$(readlink -f "$0")"
here="$(dirname "$self")"
export PATH="$here/usr/bin:${PATH:-}"
exec "$here/usr/bin/gadak-desktop" "$@"
EOF
chmod 0755 "$appdir/AppRun"

echo "built $appdir ($version, $file_arch)"

if [[ "$want_appimage" -eq 1 ]]; then
  img="$out/Gadak-${version#v}-${file_arch}.AppImage"
  rm -f "$img"
  ARCH="$file_arch" appimagetool --no-appstream "$appdir" "$img"
  chmod 0755 "$img"
  echo "built $img"
fi
