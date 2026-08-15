#!/usr/bin/env bash
# Pick an unresolved issue with fzf; the running gadak window snaps to it.
# Trap: `gadak sql` emits TAB-separated rows (cmd/gadak/sql.go:127) but
# `views open --keys -` also splits on whitespace (internal/jql/keys.go:12) —
# without `cut -f1` every word of the summary becomes a fake key.
# Aborting the picker selects nothing; extra arguments go to `gadak views open`.
set -euo pipefail
GADAK="${GADAK:-gadak}"
if ! command -v fzf >/dev/null 2>&1; then
  echo "fzf.sh: fzf not found in PATH — brew install fzf (macOS) / apt install fzf (Linux)" >&2
  exit 1
fi
rows="$("$GADAK" sql --no-header "select key, summary from issues_full
where status_category != 'done' order by priority_rank, updated_at desc")"
key="$(printf '%s\n' "$rows" | fzf --preview "$GADAK issue {1}" | cut -f1)" || true
[ -n "$key" ] || exit 0
printf '%s\n' "$key" | "$GADAK" views open --keys - "$@"
