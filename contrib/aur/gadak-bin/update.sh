#!/usr/bin/env bash
# Refresh PKGBUILD pkgver and per-arch sha256sums from a GitHub Release.
#
# Usage: contrib/aur/gadak-bin/update.sh <tag>
#
#   tag — GitHub release tag, with or without a leading v (example: v0.15.2)
#
# Rewrites pkgver, resets pkgrel to 1, and replaces sha256sums_x86_64 /
# sha256sums_aarch64 with the linux amd64/arm64 lines from that tag's
# checksums.txt. Regenerates .SRCINFO when makepkg is on PATH; otherwise
# points at verify.sh, which runs makepkg in a container. Does not compute
# hashes locally.
#
# Exit 64 = usage / bad arguments
#      69 = a required tool is missing
#       1 = download failed, checksums.txt missing an asset, or PKGBUILD
#           did not contain the fields this script rewrites
#       0 = PKGBUILD updated (.SRCINFO updated only when makepkg ran)
#
# Requires: curl, awk, mktemp, rm, grep, cat.
set -euo pipefail

usage() {
  echo "usage: contrib/aur/gadak-bin/update.sh <tag>" >&2
  exit 64
}

need() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "update.sh: missing ${name}" >&2
    exit 69
  fi
}

if [[ $# -ne 1 ]]; then
  usage
fi

case "$1" in
  -h|--help)
    usage
    ;;
esac

tag="$1"
ver="${tag#v}"
# pkgver cannot contain a hyphen (https://wiki.archlinux.org/title/PKGBUILD#pkgver).
if [[ -z "$ver" || "$ver" == *"/"* || "$ver" == *"-"* || "$ver" == *" "* ]]; then
  echo "update.sh: tag must look like v0.15.2 (no hyphen, slash, or space)" >&2
  exit 64
fi
rel_tag="v${ver}"

# Builtins only (pwd, parameter expansion) so a missing dirname cannot
# run before the 69 path. need() is the first external-tool gate.
need curl
need awk
need mktemp
need rm
need grep
need cat

self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
here="${self%/*}"
pkgbuild="${here}/PKGBUILD"
if [[ ! -f "$pkgbuild" ]]; then
  echo "update.sh: PKGBUILD not found at ${pkgbuild}" >&2
  exit 1
fi

checksums_url="https://github.com/midagedev/gadak/releases/download/${rel_tag}/checksums.txt"
amd64_asset="gadak_${ver}_linux_amd64.tar.gz"
arm64_asset="gadak_${ver}_linux_arm64.tar.gz"

tmp="$(mktemp "${TMPDIR:-/tmp}/gadak-aur-checksums.XXXXXX")"
out="$(mktemp "${TMPDIR:-/tmp}/gadak-aur-pkgbuild.XXXXXX")"
cleanup() { rm -f "$tmp" "$out"; }
trap cleanup EXIT INT HUP TERM

if ! curl -fsSL -o "$tmp" "$checksums_url"; then
  echo "update.sh: download failed: ${checksums_url}" >&2
  exit 1
fi

hash_for() {
  local file="$1"
  local hash
  hash="$(awk -v f="$file" '$2 == f { print $1; exit }' "$tmp")"
  if [[ -z "$hash" ]]; then
    echo "update.sh: no checksum entry for ${file} in checksums.txt" >&2
    exit 1
  fi
  printf '%s\n' "$hash"
}

sha_amd64="$(hash_for "$amd64_asset")"
sha_arm64="$(hash_for "$arm64_asset")"

awk -v ver="$ver" -v sha64="$sha_amd64" -v shaarm="$sha_arm64" '
  /^pkgver=/ { print "pkgver=" ver; next }
  /^pkgrel=/ { print "pkgrel=1"; next }
  /^sha256sums_x86_64=/ {
    printf "sha256sums_x86_64=(\047%s\047)\n", sha64
    next
  }
  /^sha256sums_aarch64=/ {
    printf "sha256sums_aarch64=(\047%s\047)\n", shaarm
    next
  }
  { print }
' "$pkgbuild" > "$out"

if ! grep -q "^pkgver=${ver}$" "$out"; then
  echo "update.sh: PKGBUILD rewrite did not set pkgver=${ver}" >&2
  exit 1
fi
if ! grep -q "^sha256sums_x86_64=('${sha_amd64}')$" "$out"; then
  echo "update.sh: PKGBUILD rewrite did not set sha256sums_x86_64" >&2
  exit 1
fi
if ! grep -q "^sha256sums_aarch64=('${sha_arm64}')$" "$out"; then
  echo "update.sh: PKGBUILD rewrite did not set sha256sums_aarch64" >&2
  exit 1
fi

# Overwrite in place so PKGBUILD keeps its mode (mktemp files are 0600).
cat "$out" > "$pkgbuild"

echo "update.sh: pkgver=${ver} pkgrel=1"
echo "update.sh: sha256sums_x86_64=${sha_amd64}"
echo "update.sh: sha256sums_aarch64=${sha_arm64}"

if command -v makepkg >/dev/null 2>&1; then
  (cd "$here" && makepkg --printsrcinfo > .SRCINFO)
  echo "update.sh: wrote ${here}/.SRCINFO"
else
  echo "update.sh: makepkg not found; regenerate .SRCINFO and check the package with:" >&2
  echo "  contrib/aur/gadak-bin/verify.sh" >&2
fi
