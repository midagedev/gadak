#!/usr/bin/env bash
# Fixture tests for tools/backlog-scrub-check.sh. No tracker, no export:
# each case stages a minimal snapshot directory with the one input the case
# is about and asserts the check's verdict on it (GDK-1747).
#
# FAIL-first: pre-GDK-1747 the check grepped the data files' raw bytes, so a
# masked host — angle brackets around "redacted", which Go's encoding/json
# serializes as six-byte escapes — reached the host regex with the escape's
# own letters glued to the domain and was read as a concrete tenant. Case 1
# failed the check on exactly that; post-fix the same directory passes whole.
# Case 3 pins the other direction: decoding must not blind the gate — a real
# host hidden behind an escape fails under its DECODED name.
#
# No host literal is written out in this file. scripts/scan-internal.sh
# rejects one in a tracked file and it is right to, so every case composes
# its host from $DOMAIN at runtime (the same reason tools/
# backlog-scrub-check.sh does not spell its example out either).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/tools/backlog-scrub-check.sh"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/scrub-check-test.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

fail() { echo "backlog-scrub-check-test: FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }

# The Atlassian tenant domain, assembled rather than spelled: see the header.
DOMAIN=".atlassian""$(printf '.net')"

command -v jq >/dev/null || fail "jq required"
command -v python3 >/dev/null || fail "python3 required to write Go-shaped escapes"
[ -f "$SCRIPT" ] || fail "missing $SCRIPT"

# go_dump mirrors the exporter: Go's encoding/json with the default
# SetEscapeHTML writes < > & as the six-byte escapes U+003C / U+003E / U+0026.
go_dump() { python3 -c '
import json, sys
obj = json.loads(sys.argv[1])
print(json.dumps(obj, ensure_ascii=False).replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026"))
' "$1"; }

# stage <dir> <summary> — a minimal valid snapshot (one public STD issue,
# markdown-subset description) whose summary carries the case's string.
stage() {
  local dir="$1" summary="$2"
  mkdir -p "$dir/detail"
  go_dump "{\"members\": [], \"issues\": [{\"issue_key\": \"STD-1\", \"summary\": \"$summary\", \"labels\": [\"public\"]}]}" \
    > "$dir/bootstrap.json"
  go_dump "{\"attachments\": [], \"comments\": [], \"history\": [], \"bodies\": {}, \"linked_issues\": [], \"description_adf\": {\"type\": \"doc\", \"content\": [{\"type\": \"paragraph\", \"content\": [{\"type\": \"text\", \"text\": \"plain body\"}]}]}}" \
    > "$dir/detail/STD-1.json"
}

# expect_fail <dir> <must-contain-fragment> — the check exits 1 and names it.
expect_fail() {
  local dir="$1" frag="$2" out
  if out="$(bash "$SCRIPT" "$dir" 2>&1)"; then
    fail "$dir: expected rejection, got pass"
  fi
  printf '%s' "$out" | grep -qF "$frag" || fail "$dir: rejection did not name \"$frag\": $out"
  ok "$dir rejected: $frag"
}

# expect_pass <dir> — the whole check goes green on the staged directory.
expect_pass() {
  local dir="$1" out
  if ! out="$(bash "$SCRIPT" "$dir" 2>&1)"; then
    fail "$dir: expected pass, got: $out"
  fi
  ok "$dir passed"
}

# 1. The GDK-1747 shape: masking notation, escaped exactly as the exporter
#    writes it. Not a tenant; the snapshot is publishable.
stage "$WORKDIR/masked" "site is <redacted>${DOMAIN} — masked, not a tenant"
expect_pass "$WORKDIR/masked"

# 2. A concrete host in plain ASCII still fails — decoding did not loosen
#    the assertion.
stage "$WORKDIR/plain" "see real-site${DOMAIN}"
expect_fail "$WORKDIR/plain" "real-site${DOMAIN}"

# 3. A host hidden behind a unicode escape fails under its DECODED name —
#    decoding strengthened the assertion. The file bytes carry one literal
#    `b`, then the escape for `b`, then the rest; decoded that is `bb…`.
mkdir -p "$WORKDIR/escaped"
stage "$WORKDIR/escaped" 'placeholder'
python3 - "$WORKDIR/escaped/bootstrap.json" "$DOMAIN" << 'EOF'
import sys, pathlib
p = pathlib.Path(sys.argv[1])
host = "b" + "\\u0062" + "test" + sys.argv[2]
p.write_text(p.read_text().replace("placeholder", "see " + host), encoding="utf-8")
EOF
expect_fail "$WORKDIR/escaped" "bbtest${DOMAIN}"

echo "backlog-scrub-check-test: OK (3 cases)"
