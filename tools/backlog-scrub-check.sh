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

echo "backlog-scrub-check: OK ($(jq '.issues | length' "$BOOT") issues)"
