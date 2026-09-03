#!/usr/bin/env bash
# Structural leak gate for the public backlog snapshot (GDK-389).
# Asserts the whitelist-rebuild invariants on an exported directory; run by
# tools/backlog-snapshot.sh locally and by pages.yml on the artifact.
#
#   tools/backlog-scrub-check.sh <snapshot-dir>
#   tools/backlog-scrub-check.sh <snapshot.tar.gz>
#
# A .tar.gz is unpacked to a temp dir (the committed form is one archive;
# Pages still checks the exploded tree).
set -euo pipefail

DIR="${1:?usage: backlog-scrub-check.sh <snapshot-dir-or-archive>}"
if [[ -f "$DIR" ]]; then
  case "$DIR" in
    *.tar.gz) ;;
    *) echo "backlog-scrub-check: snapshot file must be a .tar.gz (got $DIR)" >&2; exit 1 ;;
  esac
  _unpack="$(mktemp -d "${TMPDIR:-/tmp}/gadak-scrub-XXXXXX")"
  tar -xzf "$DIR" -C "$_unpack"
  DIR="$_unpack"
  trap 'rm -rf "$_unpack"' EXIT
fi
BOOT="$DIR/bootstrap.json"
[ -f "$BOOT" ] || { echo "missing $BOOT" >&2; exit 1; }

command -v jq >/dev/null || { echo "jq required" >&2; exit 1; }

fail() { echo "backlog-scrub-check: $1" >&2; exit 1; }

[ "$(jq '.members | length' "$BOOT")" = "0" ] || fail "bootstrap carries members"
[ "$(jq '[.issues[] | select(.assignee or .reporter or .assignee_email or .reporter_email or .custom)] | length' "$BOOT")" = "0" ] \
  || fail "bootstrap carries people or custom fields"
[ "$(jq '.issues | length' "$BOOT")" -gt 0 ] || fail "bootstrap has 0 issues"

# The description is a published surface now (GDK-430) and the rest is not.
# Comments and history carry other people's words and actions, attachments carry
# arbitrary files, `bodies` carries linked issues' text — none of those is the
# reporter's own, so all four stay pinned to empty. The description is checked
# for shape instead: nothing but paragraphs and text, because an ADF document
# can also hold `mention` nodes (an account id and a display name), `media`
# (attachment ids) and `inlineCard` (a URL, which is how a site host would get
# out). Measured on the 2026-08-20 snapshot: doc/paragraph/text only.
# Links between published issues are public structure; a link to an
# unpublished issue carries that issue's key and summary and is a leak.
# Rescoped 2026-08-23 (GDK-675): the node-type sweep used to walk the whole
# document (`..`) and caught this class only by accident — a link's
# "type":"Relates" collided with the ADF allowlist. The sweep now names its
# surface (.description_adf), and links get their own two assertions: every
# target key is in the published bootstrap, and every entry carries only the
# reviewed link fields. FAIL-first: the 2026-08-23 snapshot, where published
# GDK-661 linked an unpublished issue, fails the membership check.
PUBKEYS="$(jq -c '[.issues[].issue_key]' "$BOOT")"
for f in "$DIR"/detail/*.json; do
  jq -e '(.attachments == []) and (.comments == [])
     and (.history == []) and (.bodies == {})' "$f" >/dev/null \
    || fail "other people's content survived the scrub in $f"
  # The allowlist is decision 0012's markdown subset (internal/adf/adf.go
  # markdownTypes + markdownMarks): the export reads the server's /detail/, and
  # since GDK-1386 that derives a typed body as the markdown it is, so `bd ready` and **bold** arrive as code and
  # strong marks. FAIL-first: the 2026-09-03 snapshot failed on GDK-1008 with
  # "code strong" under the old doc/paragraph/text/hardBreak list. mention,
  # media, inlineCard and every other origin-only node stay refused — those
  # are the channels an account id, an attachment id or a site host leaks by.
  bad="$(jq -r '[.description_adf // {} | .. | objects | select(has("type")) | .type]
     - ["doc","paragraph","text","hardBreak","codeBlock","heading","rule",
        "bulletList","orderedList","listItem","blockquote",
        "table","tableRow","tableHeader","tableCell",
        "strong","em","code","strike","link"] | unique | join(" ")' "$f")"
  [ -z "$bad" ] || fail "description in $f carries node types beyond the markdown subset: $bad"
  unpublished="$(jq -r --argjson pub "$PUBKEYS" \
     '[.linked_issues // [] | .[].key | select(. as $k | $pub | index($k) | not)] | unique | join(" ")' "$f")"
  [ -z "$unpublished" ] || fail "link in $f targets an unpublished issue: $unpublished"
  extralink="$(jq -r '[.linked_issues // [] | .[] | keys[]]
     - ["key","type","direction","summary","status_category"] | unique | join(" ")' "$f")"
  [ -z "$extralink" ] || fail "link in $f carries non-whitelisted fields: $extralink"
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

# Two addresses are allowed through, for the same reason `your-site` is above.
# Anything at example.com is a documentation placeholder, and the maintainer's
# own address is a shipped feedback channel — it is in the app's About tab, the
# desktop menu and the hosted links (git grep it), so a backlog issue that
# quotes the channel is not disclosing it. Every other address still fails.
# Widened 2026-08-20 when descriptions started publishing (GDK-430); before
# that the snapshot carried no prose for an address to appear in.
mails="$(grep -ohE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' "${DATA_FILES[@]}" | sort -u \
  | grep -vxE '([A-Za-z0-9._%+-]+@example\.(com|org|net)|midagedev@gmail\.com)' || true)"
[ -z "$mails" ] || fail "email address in snapshot data: $(echo "$mails" | tr '\n' ' ')"

# A home directory names its owner and says what machine wrote the issue. Three
# GDK bodies carried one (`/Users/<name>/repo/issuetap`) and would have shipped
# it the moment descriptions were published.
homes="$(grep -ohE '(/Users|/home)/[A-Za-z0-9._-]+' "${DATA_FILES[@]}" | sort -u || true)"
[ -z "$homes" ] || fail "home directory path in snapshot data: $(echo "$homes" | tr '\n' ' ')"

# Real names must not come back. The list of them is deliberately NOT in this
# repository: which people were consulted is the private fact, so committing a
# denylist of their handles would publish exactly what the lens-* pseudonyms
# exist to keep out. Point BACKLOG_NAME_DENYLIST at a local file (one name or
# handle per line, `#` comments) and this enforces it; unset, it says so rather
# than passing silently. tools/backlog-snapshot.sh is the place to set it.
if [ -n "${BACKLOG_NAME_DENYLIST:-}" ] && [ -f "$BACKLOG_NAME_DENYLIST" ]; then
  names="$(grep -vE '^\s*(#|$)' "$BACKLOG_NAME_DENYLIST" || true)"
  found=""
  while IFS= read -r n; do
    [ -n "$n" ] || continue
    if grep -oiqE "(^|[^A-Za-z0-9])$n([^A-Za-z0-9]|$)" "${DATA_FILES[@]}" 2>/dev/null; then
      found="$found $n"
    fi
  done <<< "$names"
  [ -z "${found// /}" ] || fail "name from the local denylist in snapshot data:$found"
  echo "backlog-scrub-check: name denylist enforced ($(printf '%s\n' "$names" | wc -l | tr -d ' ') entries)"
else
  echo "backlog-scrub-check: BACKLOG_NAME_DENYLIST unset — real-name check skipped"
fi

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
