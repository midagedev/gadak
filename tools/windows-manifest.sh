#!/usr/bin/env bash
# Regenerate (or --check) the rsrc .syso pair that embeds
# desktop/windows-app.manifest into gadak-desktop.exe (GDK-1407).
#
# The manifest is the fix for blur when the app crosses a monitor of a
# different scale factor: a GUI process without one is system-DPI-aware,
# so Windows bitmap-stretches the window. rsrc packs the manifest as a COFF
# object; the _windows_<arch> filename suffix is what the go tool matches,
# so darwin and linux builds never link it and each windows arch links its
# own. desktop/build-windows.ps1 byte-checks the built exe for the
# PerMonitorV2 value — that gate asks "is a manifest inside the exe", this
# script's --check asks "is *this* manifest file the one inside".
#
# Usage:
#   bash tools/windows-manifest.sh            regenerate both .syso files
#   bash tools/windows-manifest.sh --check    fail if committed != regenerated
#
# --check exists for the same reason tools/check-brand-icons.sh does: the
# committed artifact is one edit away from the source it was made from.
# Editing the manifest without regenerating leaves every build carrying the
# old resource while the ps1 byte gate keeps passing. rsrc's output is
# deterministic (measured: two runs, same bytes), so the comparison is
# meaningful. Needs network for `go run` on a cold module cache.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="$ROOT/desktop/windows-app.manifest"
RSRC_PIN="github.com/akavel/rsrc@v0.10.2"

fail() { echo "windows-manifest: $*" >&2; exit 1; }

[[ -f "$MANIFEST" ]] || fail "missing $MANIFEST"
command -v go >/dev/null || fail "go is not on PATH"

check=
if [[ "${1:-}" == "--check" ]]; then
  check=1
  shift
fi
[[ $# -eq 0 ]] || fail "usage: tools/windows-manifest.sh [--check]"

for arch in amd64 arm64; do
  out="$ROOT/desktop/rsrc_windows_${arch}.syso"
  tmp="$(mktemp "${TMPDIR:-/tmp}/gadak-rsrc-${arch}.XXXXXX")"
  if ! go run "$RSRC_PIN" -manifest "$MANIFEST" -arch "$arch" -o "$tmp"; then
    rm -f "$tmp"
    fail "rsrc failed for $arch"
  fi
  if [[ -n "$check" ]]; then
    [[ -f "$out" ]] || { rm -f "$tmp"; fail "$out is missing — run: bash tools/windows-manifest.sh"; }
    cmp -s "$tmp" "$out" || { rm -f "$tmp"; fail "$out does not match desktop/windows-app.manifest — run: bash tools/windows-manifest.sh and commit the pair"; }
    echo "ok: rsrc_windows_${arch}.syso matches desktop/windows-app.manifest"
  else
    mv "$tmp" "$out"
    echo "wrote $out"
  fi
  rm -f "$tmp"
done
