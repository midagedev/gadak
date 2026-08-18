#!/usr/bin/env bash
# Reverse contrib/omarchy/install.sh: disable+remove the plugin and the
# web-app desktop file. Does not remove the gadak binary or a unit written
# by `gadak install-service` — this recipe never installed those.
set -euo pipefail

fail() {
  echo "uninstall.sh: $*" >&2
  exit 1
}

os_id=""
if [[ -f /etc/os-release ]]; then
  # shellcheck disable=SC1091
  os_id="$(. /etc/os-release && printf '%s' "${ID:-}")"
fi
if [[ "$os_id" != "omarchy" ]]; then
  fail "this machine is not Omarchy (ID=${os_id:-unset}; want ID=omarchy)."
fi

self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
here="${self%/*}"
src="${here}/gadak"
id="io.github.midagedev.gadak"
if [[ -f "$src/manifest.json" ]]; then
  id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$src/manifest.json")"
fi
dest="${HOME}/.config/omarchy/plugins/${id}"

if command -v omarchy-plugin-remove >/dev/null 2>&1 && [[ -e "$dest" || -L "$dest" ]]; then
  omarchy-plugin-remove --yes "$id"
elif [[ -e "$dest" || -L "$dest" ]]; then
  if command -v omarchy-plugin-disable >/dev/null 2>&1; then
    omarchy-plugin-disable "$id" || true
  fi
  rm -rf "$dest"
  echo "removed $dest"
  if command -v omarchy-shell >/dev/null 2>&1; then
    omarchy-shell shell rescanPlugins >/dev/null || true
  fi
else
  echo "plugin not installed at $dest"
fi

desktop="${HOME}/.local/share/applications/gadak.desktop"
if command -v omarchy-webapp-remove >/dev/null 2>&1 && [[ -f "$desktop" ]]; then
  OMARCHY_REMOVE_NOTIFY=false omarchy-webapp-remove gadak || omarchy-webapp-remove gadak
elif [[ -f "$desktop" ]]; then
  rm -f "$desktop"
  echo "removed $desktop"
else
  echo "web app not installed"
fi
