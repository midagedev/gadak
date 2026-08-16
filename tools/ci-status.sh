#!/usr/bin/env bash
# Did the commit I just pushed pass CI? (GDK-57)
#
# A push is not the end of a round — a green CI is. The defect this closes was
# not a bug in a workflow: local gates were green, the report said "done", and
# nobody looked at CI, so commits stacked on top of a red main for hours. The
# fix that sticks is a command short enough to actually run.
#
#   tools/ci-status.sh            # wait for HEAD's runs, exit non-zero if red
#   tools/ci-status.sh --no-wait  # report whatever is known right now
#   tools/ci-status.sh <sha>      # a specific commit
#
# Exit 0 = every run for that commit concluded successfully.
#        1 = at least one failed, or the wait timed out with runs still going.
#        2 = the tool cannot answer (no gh, not authenticated, no runs yet).
#
# It also looks one commit back: pushing onto an already-red main is how a
# small breakage becomes several commits deep, so that gets said out loud
# rather than left for someone to notice.
set -euo pipefail

WAIT=1
SHA=""
for arg in "$@"; do
  case "$arg" in
    --no-wait) WAIT=0 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) SHA="$arg" ;;
  esac
done

command -v gh >/dev/null || { echo "ci-status: gh is not installed — see https://cli.github.com" >&2; exit 2; }
gh auth status >/dev/null 2>&1 || { echo "ci-status: gh is not authenticated (gh auth login)" >&2; exit 2; }

cd "$(cd "$(dirname "$0")/.." && pwd)"
# Always the full 40-char SHA: `gh run list --commit` matches on the full form
# only, and a short one silently returns nothing — which would read exactly
# like "CI has not started yet".
SHA="$(git rev-parse "${SHA:-HEAD}")"
SHORT="$(git rev-parse --short "$SHA")"
SUBJECT="$(git log -1 --format=%s "$SHA")"

# runs_for prints "conclusion<TAB>status<TAB>name" for every run on this commit.
runs_for() {
  gh run list --commit "$1" --limit 20 \
    --json conclusion,status,name,databaseId \
    --jq '.[] | [.conclusion // "", .status, .name] | @tsv' 2>/dev/null || true
}

DEADLINE=$(( $(date +%s) + 900 ))   # 15 min: the longest CI here runs ~5
while :; do
  ROWS="$(runs_for "$SHA")"
  if [[ -z "$ROWS" ]]; then
    if [[ $WAIT -eq 0 ]]; then
      echo "ci-status: no runs yet for $SHORT — pushed?" >&2
      exit 2
    fi
    [[ $(date +%s) -lt $DEADLINE ]] || { echo "ci-status: no runs appeared for $SHORT within the wait" >&2; exit 2; }
    sleep 10
    continue
  fi
  PENDING="$(awk -F'\t' '$2 != "completed"' <<<"$ROWS" | wc -l | tr -d ' ')"
  if [[ "$PENDING" -eq 0 || $WAIT -eq 0 ]]; then
    break
  fi
  if [[ $(date +%s) -ge $DEADLINE ]]; then
    echo "ci-status: still running after the wait — $SHORT" >&2
    awk -F'\t' '{printf "  %-28s %s\n", $3, ($2=="completed" ? $1 : $2)}' <<<"$ROWS" >&2
    exit 1
  fi
  sleep 15
done

echo "$SHORT  $SUBJECT"
awk -F'\t' '{printf "  %-28s %s\n", $3, ($2=="completed" ? $1 : $2)}' <<<"$ROWS"

# The commit before this one: a red parent means the breakage is already
# stacked, and knowing that changes what you do next (fix forward vs revert).
# Not the immediate parent: CI runs on the head of a push, so every commit in
# a multi-commit push except the last has no runs at all, and keying on the
# parent alone means this warning silently never fires. Walk back to the most
# recent commit that CI actually judged.
for prev in $(git rev-list -n 10 "$SHA^" 2>/dev/null || true); do
  PROWS="$(runs_for "$prev")"
  [[ -n "$PROWS" ]] || continue
  if grep -q 'failure' <<<"$PROWS"; then
    echo "note: the last commit CI judged before this one ($(git rev-parse --short "$prev")) was red —" >&2
    echo "      the breakage is already stacked; fixing forward means two commits to verify, not one." >&2
  fi
  break
done

if grep -qE '^(failure|timed_out)' <<<"$ROWS"; then
  echo "ci-status: RED. tools/ci-status.sh is not the fix — read the log:" >&2
  echo "  gh run list --commit $SHORT" >&2
  echo "  gh run view <id> --log-failed" >&2
  exit 1
fi
# A cancelled run is the absence of a verdict, not a bad one — pushing twice in
# a row cancels the first commit's run by design (the workflow's concurrency
# group). Calling that red would make this tool cry wolf on every quick series,
# and a tool that cries wolf is one people stop running, which is the failure
# GDK-57 is about. Say what is actually true: this commit was never judged.
if grep -qE '^cancelled' <<<"$ROWS"; then
  echo "ci-status: no verdict for $SHORT — a run was cancelled, which normally means a later push superseded it." >&2
  echo "           Check the commit that superseded it: tools/ci-status.sh \$(git rev-parse HEAD)" >&2
  exit 2
fi
echo "ci-status: green."
