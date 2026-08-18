#!/usr/bin/env bash
# Fail if the in-repo gadak-bin PKGBUILD pkgver is not the latest git tag.
#
# Usage: contrib/aur/gadak-bin/check-pkgver.sh [PKGBUILD]
#
# The recipe itself lives at contrib/aur/gadak-bin/ (existing convention;
# this directory is only the offline gate). Version ownership matches
# tools/doc-checks.sh check 6: `git describe --tags --abbrev=0`. That is
# also what GoReleaser stamps into the binary
# (`.goreleaser.yaml` ldflags `-X main.version={{.Version}}`).
# `gadak version` is *not* the source here: an unstamped tree prints
# `0.0.0-dev` (`cmd/gadak/main.go`).
#
# A tagless checkout (shallow CI clone) skips, same as doc-checks.sh,
# because there is then no tag to disagree with.
#
# Exit 64 = usage
#      1  = pkgver does not match the latest tag, or PKGBUILD is unreadable
#      0  = match, or no tag reachable
set -euo pipefail

usage() {
  echo "usage: contrib/aur/gadak-bin/check-pkgver.sh [PKGBUILD]" >&2
  exit 64
}

if [[ $# -gt 1 ]]; then
  usage
fi
case "${1:-}" in
  -h|--help) usage ;;
esac

# Builtins only for ROOT so a missing dirname cannot run first.
self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
# contrib/aur/gadak-bin/check-pkgver.sh → repo root is ../../..
root="${self%/*}/../../.."
cd "$root"
root="$(pwd)"

pkgbuild="${1:-$root/contrib/aur/gadak-bin/PKGBUILD}"
if [[ "$pkgbuild" != /* ]]; then
  pkgbuild="$root/$pkgbuild"
fi

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

ok() {
  echo "ok: $*"
}

if [[ ! -f "$pkgbuild" ]]; then
  fail "PKGBUILD not found at ${pkgbuild}"
fi

tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [[ -z "$tag" ]]; then
  ok "no tag reachable — pkgver guard skipped"
  exit 0
fi

want="${tag#v}"
# pkgver cannot contain a hyphen (https://wiki.archlinux.org/title/PKGBUILD#pkgver).
# Same skip as the release-workflow refresh job: an rc tag is not the
# AUR version, and failing the gate on it would block a registered CI
# check for the whole prerelease window.
if [[ "$want" == *"-"* ]]; then
  ok "latest tag ${tag} is not a PKGBUILD pkgver (hyphen) — skipped"
  exit 0
fi

pkgver="$(awk -F= '/^pkgver=/ { v=$2; gsub(/['\''"]/, "", v); print v; exit }' "$pkgbuild")"
if [[ -z "$pkgver" ]]; then
  fail "${pkgbuild} has no pkgver= line"
fi

if [[ "$pkgver" != "$want" ]]; then
  fail "PKGBUILD pkgver=${pkgver} does not match latest tag ${tag} (want ${want})"
fi

ok "PKGBUILD pkgver=${pkgver} matches latest tag ${tag}"
