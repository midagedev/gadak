#!/usr/bin/env bash
# Latch frozen=true on a capture GADAK_HOME (GDK-181).
#
# The promo recordings share the e2e serve home (e2e/.tmp/home-<port>), which
# serve.sh seeds from examples/demo.db with a fake credential. Frozen is still
# required: a leftover real site/token in that file would sync on any path
# that is not --no-sync. MEDIA.md: `gadak config set frozen true`.
#
# Usage: bash tools/tapes/prepare-promo.sh <GADAK_HOME>
set -euo pipefail

HOME_DIR="${1:?usage: prepare-promo.sh <GADAK_HOME>}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN="${ROOT}/e2e/.tmp/gadak"
if [[ ! -x "$BIN" ]]; then
  BIN="${ROOT}/tools/tapes/.tmp/gadak"
fi
if [[ ! -x "$BIN" ]]; then
  echo "prepare-promo: gadak binary not found (run e2e/serve.sh or tools/tapes/prepare.sh first)" >&2
  exit 1
fi
test -d "$HOME_DIR" || { echo "prepare-promo: missing $HOME_DIR" >&2; exit 1; }

echo "[promo] gadak config set frozen true (GADAK_HOME=$HOME_DIR)"
GADAK_HOME="$HOME_DIR" "$BIN" config set frozen true
GADAK_HOME="$HOME_DIR" "$BIN" status | grep -E '^frozen' || true
