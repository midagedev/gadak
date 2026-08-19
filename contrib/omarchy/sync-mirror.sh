#!/usr/bin/env bash
# Push the widget payload to the distribution mirror
# (github.com/midagedev/omarchy-gadak — the git URL `omarchy plugin add`
# clones; its root must be the manifest, which is why the mirror exists).
#
# Source of truth is THIS directory. Never edit the mirror directly:
# run this after a payload change lands on main here.
#
# Lead-only: this pushes to a public repo.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$here/../.." && pwd)"
mirror_url="git@github.com:midagedev/omarchy-gadak.git"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

git clone --quiet --depth 1 "$mirror_url" "$tmp/mirror"

# Payload: everything under gadak/ lands at the mirror root.
cp "$here"/gadak/manifest.json "$here"/gadak/BarWidget.qml "$here"/gadak/query.sql "$tmp/mirror/"
cp "$repo_root/LICENSE" "$tmp/mirror/LICENSE"
# The mirror README is owned by the mirror (install-facing), not copied.

cd "$tmp/mirror"
if git diff --quiet; then
  echo "mirror is already in sync"
  exit 0
fi
git --no-pager diff --stat
version="$(python3 -c 'import json;print(json.load(open("manifest.json"))["version"])')"
git add -A
git commit -m "sync from gadak contrib/omarchy (widget ${version})"
git push origin main
echo "pushed. verify on a real guest before announcing: omarchy plugin add ${mirror_url}"
