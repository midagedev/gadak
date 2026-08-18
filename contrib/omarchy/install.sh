#!/usr/bin/env bash
# Install the gadak omarchy-shell bar widget on this machine.
#
# Copies contrib/omarchy/gadak/ into ~/.config/omarchy/plugins/<id>/,
# validates if omarchy-plugin-validate exists, rescans, and enables.
# omarchy-plugin-clone is the built-in-clone verb (bin/omarchy-plugin-clone)
# and refuses anything that is not first-party — it cannot install this plugin.
# omarchy-plugin-add clones a git URL whose root is a manifest; this directory
# lives inside the gadak repo, so the install is a copy.
set -euo pipefail

fail() {
  echo "install.sh: $*" >&2
  exit 1
}

os_id=""
if [[ -f /etc/os-release ]]; then
  # shellcheck disable=SC1091
  os_id="$(. /etc/os-release && printf '%s' "${ID:-}")"
fi
if [[ "$os_id" != "omarchy" ]]; then
  fail "this machine is not Omarchy (ID=${os_id:-unset}; want ID=omarchy). The widget is for omarchy-shell."
fi

self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
here="${self%/*}"
src="${here}/gadak"
[[ -f "$src/manifest.json" ]] || fail "plugin missing at $src/manifest.json"

id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$src/manifest.json")"
[[ -n "$id" ]] || fail "manifest id is empty"

if ! command -v gadak >/dev/null 2>&1; then
  cat >&2 <<'EOF'
install.sh: gadak is not on PATH. The widget will show "no gadak" until it is.

The AUR package is not registered (docs/INSTALL.md). Honest Linux options today:

  1. Release tarball from https://github.com/midagedev/gadak/releases/latest
       gadak_<version>_linux_amd64.tar.gz
       gadak_<version>_linux_arm64.tar.gz
       checksums.txt
     Then (README.md):
       sha256sum --ignore-missing -c checksums.txt
       tar -xzf gadak_<version>_linux_amd64.tar.gz
       # put `gadak` on PATH

  2. brew install midagedev/tap/gadak-cli

  3. From a clone: contrib/aur/gadak-bin && makepkg -si
     (fetches the same release tarball; not `pacman -S gadak`)

Continuing with the plugin copy so the bar slot exists.
EOF
fi

dest="${HOME}/.config/omarchy/plugins/${id}"
mkdir -p "$dest"
# Copy, do not symlink: omarchy-plugin-validate refuses any symlink inside
# the plugin folder (bin/omarchy-plugin-validate).
cp -a "$src/." "$dest/"
rm -rf "$dest/.git"
echo "copied plugin to $dest"

if command -v omarchy-plugin-validate >/dev/null 2>&1; then
  omarchy-plugin-validate "$dest"
  echo "omarchy-plugin-validate: ok"
else
  echo "install.sh: omarchy-plugin-validate not on PATH — skipped"
fi

if command -v omarchy-shell >/dev/null 2>&1; then
  omarchy-shell shell rescanPlugins >/dev/null || true
fi

if command -v omarchy-plugin-enable >/dev/null 2>&1; then
  already=0
  if command -v omarchy-plugin-list >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
    if omarchy-plugin-list --json 2>/dev/null | jq -e --arg id "$id" 'any(.[]; .id == $id and .enabled == true)' >/dev/null; then
      already=1
    fi
  fi
  if (( already )); then
    echo "already enabled: $id"
  else
    omarchy-plugin-enable "$id"
  fi
else
  echo "install.sh: omarchy-plugin-enable not on PATH — enable later with: omarchy-plugin-enable $id"
fi

# serve.go default --addr is 127.0.0.1:7777. Three arguments: fewer opens
# the interactive gum prompt (bin/omarchy-webapp-install).
serve_url="http://127.0.0.1:7777"
desktop="${HOME}/.local/share/applications/gadak.desktop"
if command -v omarchy-webapp-install >/dev/null 2>&1; then
  if [[ -f "$desktop" ]]; then
    echo "web app already present: $desktop"
  else
    omarchy-webapp-install gadak "$serve_url" web-browser
    echo "web app: gadak -> $serve_url"
  fi
else
  echo "install.sh: omarchy-webapp-install not on PATH — skipped"
fi

cat <<EOF
Keep gadak serve running across reboot with \`gadak install-service\`
(systemd --user unit on Linux; cmd/gadak/service.go). This script does
not install that unit.

Default serve URL (cmd/gadak/serve.go --addr): $serve_url
Click the widget or the web app to open it. gadak:// is not registered
on Linux (cmd/gadak/views.go deepLinkURL; macOS app bundle only).
EOF
