#!/usr/bin/env bash
# Refresh gadak.json version and per-arch hashes from a GitHub Release.
#
# Usage: contrib/scoop/update.sh <tag>
#
#   tag — GitHub release tag, with or without a leading v (example: v0.15.2)
#
# Rewrites version, the two windows zip URLs, and their sha256 values from
# that tag's checksums.txt. Does not compute hashes locally — checksums.txt
# is the owner (same contract as contrib/aur/gadak-bin/update.sh).
#
# Exit 64 = usage / bad arguments
#      69 = a required tool is missing
#       1 = download failed, checksums.txt missing an asset, or rewrite failed
#       0 = gadak.json updated
#
# Requires: curl, python3, awk, mktemp, rm, grep, cat.
set -euo pipefail

usage() {
  echo "usage: contrib/scoop/update.sh <tag>" >&2
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
if [[ -z "$ver" || "$ver" == *"/"* || "$ver" == *"-"* || "$ver" == *" "* ]]; then
  echo "update.sh: tag must look like v0.15.2 (no hyphen, slash, or space)" >&2
  exit 64
fi
rel_tag="v${ver}"

need curl
need python3
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
manifest="${here}/gadak.json"
if [[ ! -f "$manifest" ]]; then
  echo "update.sh: gadak.json not found at ${manifest}" >&2
  exit 1
fi

checksums_url="https://github.com/midagedev/gadak/releases/download/${rel_tag}/checksums.txt"
amd64_asset="gadak_${ver}_windows_amd64.zip"
arm64_asset="gadak_${ver}_windows_arm64.zip"

tmp="$(mktemp "${TMPDIR:-/tmp}/gadak-scoop-checksums.XXXXXX")"
cleanup() { rm -f "$tmp"; }
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

GADAK_SCOOP_MANIFEST="$manifest" GADAK_SCOOP_VER="$ver" \
GADAK_SCOOP_SHA_AMD64="$sha_amd64" GADAK_SCOOP_SHA_ARM64="$sha_arm64" \
python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["GADAK_SCOOP_MANIFEST"])
ver = os.environ["GADAK_SCOOP_VER"]
sha_amd64 = os.environ["GADAK_SCOOP_SHA_AMD64"]
sha_arm64 = os.environ["GADAK_SCOOP_SHA_ARM64"]

data = json.loads(path.read_text())
data["version"] = ver
arch = data.setdefault("architecture", {})
amd = arch.setdefault("64bit", {})
arm = arch.setdefault("arm64", {})
amd["url"] = f"https://github.com/midagedev/gadak/releases/download/v{ver}/gadak_{ver}_windows_amd64.zip"
amd["hash"] = sha_amd64
arm["url"] = f"https://github.com/midagedev/gadak/releases/download/v{ver}/gadak_{ver}_windows_arm64.zip"
arm["hash"] = sha_arm64

# Stable key order so a no-op update is a tiny (or empty) diff.
ordered = {}
for key in (
    "$schema",
    "version",
    "description",
    "homepage",
    "license",
    "##",
    "architecture",
    "bin",
    "checkver",
    "autoupdate",
):
    if key in data:
        ordered[key] = data[key]
for key, value in data.items():
    if key not in ordered:
        ordered[key] = value

text = json.dumps(ordered, indent=4, ensure_ascii=False) + "\n"
path.write_text(text)
PY

if ! grep -q '"version": "'"${ver}"'"' "$manifest"; then
  echo "update.sh: gadak.json rewrite did not set version=${ver}" >&2
  exit 1
fi
if ! grep -q '"hash": "'"${sha_amd64}"'"' "$manifest"; then
  echo "update.sh: gadak.json rewrite did not set 64bit hash" >&2
  exit 1
fi
if ! grep -q '"hash": "'"${sha_arm64}"'"' "$manifest"; then
  echo "update.sh: gadak.json rewrite did not set arm64 hash" >&2
  exit 1
fi

echo "update.sh: version=${ver}"
echo "update.sh: 64bit  ${amd64_asset}  ${sha_amd64}"
echo "update.sh: arm64  ${arm64_asset}  ${sha_arm64}"
