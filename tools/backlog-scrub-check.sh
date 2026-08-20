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

# No concrete Jira site URL and no email addresses anywhere in the snapshot.
# Placeholder hosts (your-site, example, bare x) are fine — same allowlist
# idea as the pages.yml tenant-neutrality gate.
if grep -rEn "atlassian\.net" "$DIR" | grep -vE "(your-site|your-team|example|\bx)\.atlassian\.net"; then
  fail "concrete Jira site URL"
fi
if grep -rEn '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' "$DIR"; then fail "email address"; fi

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
