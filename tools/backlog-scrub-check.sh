#!/usr/bin/env bash
# Structural leak gate for the public backlog snapshot (GDK-389).
# Asserts the whitelist-rebuild invariants on an exported directory; run by
# tools/backlog-snapshot.sh locally and by pages.yml on the artifact.
#
#   tools/backlog-scrub-check.sh <snapshot-dir>
set -euo pipefail

DIR="${1:?usage: backlog-scrub-check.sh <snapshot-dir>}"
BOOT="$DIR/bootstrap.json"
[ -f "$BOOT" ] || { echo "missing $BOOT" >&2; exit 1; }

command -v jq >/dev/null || { echo "jq required" >&2; exit 1; }

fail() { echo "backlog-scrub-check: $1" >&2; exit 1; }

[ "$(jq '.members | length' "$BOOT")" = "0" ] || fail "bootstrap carries members"
[ "$(jq '[.issues[] | select(.assignee or .reporter or .assignee_email or .reporter_email or .custom)] | length' "$BOOT")" = "0" ] \
  || fail "bootstrap carries people or custom fields"
[ "$(jq '.issues | length' "$BOOT")" -gt 0 ] || fail "bootstrap has 0 issues"

for f in "$DIR"/detail/*.json; do
  jq -e '(.description_adf == null)
     and (.attachments == []) and (.comments == [])
     and (.history == []) and (.bodies == {})' "$f" >/dev/null \
    || fail "content survived the scrub in $f"
done

# No concrete Jira site URL and no email addresses in the snapshot DATA.
# Placeholder hosts (your-site, example, bare x) are fine — same allowlist
# idea as the pages.yml tenant-neutrality gate.
#
# Scoped to the data files rather than "$DIR" (2026-08-20). The published
# directory also holds the Vite bundle, and this gate ran against it: the
# 2026-08-20 Pages build failed on `you@example.com` (the fixture placeholder)
# and on the maintainer's own contact address, which the product deliberately
# puts in the UI — a shipped feedback channel, not a leak. Widening the
# allowlist would have been the wrong repair; the assertion was never about
# the app, and the app is what the tenant-neutrality step and
# scripts/scan-internal.sh already cover. The assertion itself is unchanged
# and stays absolute on the surface it exists for: whatever the export wrote.
#
# FAIL-first for the narrowed form: put an address into bootstrap.json and this
# still fails. What no longer fails is a string that was never in the data.
DATA_FILES=("$BOOT")
[ -f "$DIR/config.json" ] && DATA_FILES+=("$DIR/config.json")
[ -d "$DIR/detail" ] && while IFS= read -r f; do DATA_FILES+=("$f"); done \
  < <(find "$DIR/detail" -name '*.json')
# Allowlist per match, not per line. The old form piped `grep -n` into
# `grep -v`, which drops a whole LINE when the placeholder appears anywhere on
# it — and bootstrap.json is a single line. One issue summary mentioning
# `x.atlassian.net` (GDK-304 does) therefore excused every other host in the
# file, so the assertion was vacuous. Measured 2026-08-20 by injecting a
# concrete (non-placeholder) site host into a summary: the gate said OK.
# Extracting the hosts with -o and filtering those is the same intent,
# actually enforced. The example host is deliberately not written out here —
# scripts/scan-internal.sh rejects one in a tracked file, and it is right to.
hosts="$(grep -ohE '[A-Za-z0-9._-]+\.atlassian\.net' "${DATA_FILES[@]}" | sort -u \
  | grep -vxE '(your-site|your-team|example|x)\.atlassian\.net' || true)"
[ -z "$hosts" ] || fail "concrete Jira site URL in snapshot data: $(echo "$hosts" | tr '\n' ' ')"

mails="$(grep -ohE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' "${DATA_FILES[@]}" | sort -u || true)"
[ -z "$mails" ] || fail "email address in snapshot data: $(echo "$mails" | tr '\n' ' ')"

# Two set assertions, both whitelist-shaped (GDK-389 review, 2026-08-20).
#
# 1. Every published issue carries `public`. The export filters on the label,
#    so a mismatch here means the filter did not run — the difference between
#    "this snapshot was reviewed" and "this snapshot is whatever the mirror
#    held". Cheap to assert, and it is the whole invariant.
UNMARKED="$(jq -r '[.issues[] | select((.labels // []) | index("public") | not) | .issue_key] | join(" ")' "$BOOT")"
[ -z "$UNMARKED" ] || fail "issues without the public label reached the snapshot: $UNMARKED"

# 2. Every label in the snapshot is in the reviewed vocabulary. A label is
#    free text in Jira; without this, the next one ships unread.
ALLOWFILE="$(dirname "$0")/backlog-label-allowlist.txt"
[ -f "$ALLOWFILE" ] || fail "missing $ALLOWFILE"
ALLOWED="$(grep -vE '^\s*(#|$)' "$ALLOWFILE" | sort -u)"
USED="$(jq -r '[.issues[].labels // [] | .[]] | unique | .[]' "$BOOT" | sort -u)"
UNKNOWN="$(comm -13 <(printf '%s\n' "$ALLOWED") <(printf '%s\n' "$USED") | tr '\n' ' ')"
[ -z "${UNKNOWN// /}" ] || fail "labels not in tools/backlog-label-allowlist.txt: $UNKNOWN"

echo "backlog-scrub-check: OK ($(jq '.issues | length' "$BOOT") issues, $(printf '%s\n' "$USED" | wc -l | tr -d ' ') labels, all reviewed)"
