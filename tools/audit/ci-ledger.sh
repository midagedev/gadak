#!/usr/bin/env bash
# CI cost ledger — job seconds, wall/billable totals and failure/cancel
# counts per release tag (GDK-1707, census row "CI ledger").
#
#   bash tools/audit/ci-ledger.sh > page.md
#
# For each of the last three release tags plus HEAD it takes the `CI`
# workflow run at that commit (`gh run list --commit <full-sha>`) and
# derives per-job seconds from startedAt/completedAt. wall = the slowest
# shard (the run's deciding duration), billable = the sum across shards —
# the same pair the release-audit runbook's axis 9 tracks across releases.
#
# Needs `gh`; when gh is missing or not authenticated the page says so in
# plain words and the script exits 0 — a census reports what it could not
# measure, it does not fail the round on a missing tool. A cancelled run
# keeps its table but is flagged: a cancelled job's seconds are the time
# at which it died, not a duration (release-audit.md, v0.21.0 lesson).
#
# Exit 0 always. Read-only git; no paths outside the repo are printed.
set -euo pipefail
cd "$(cd "$(dirname "$0")/../.." && pwd)"
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,/^set -euo pipefail$/p' "$0"
  exit 0
fi

SOURCES=()
src() { SOURCES+=("$1"); }

BASE="$(git rev-parse --short HEAD)"
src "git rev-parse --short HEAD   # base: $BASE"

TAGS="$(git for-each-ref refs/tags --format='%(refname:short)' | sort -V | tail -3)"
src "git for-each-ref refs/tags --format='%(refname:short)' | sort -V | tail -3"

echo "# CI cost ledger — job seconds per release tag"
echo
echo "Base $BASE. wall = slowest shard, billable = sum of shards. A"
echo "cancelled job's seconds are the time it died, not a duration."
echo

if ! command -v gh >/dev/null 2>&1; then
  echo "**Not measured: gh is not installed.** The commands this page would"
  echo "have run are listed under Sources; run them from a host with gh to"
  echo "get the tables back."
  MISSING_REASON="absent"
elif ! gh auth status >/dev/null 2>&1; then
  echo "**Not measured: gh is installed but not authenticated"
  echo "(gh auth login).** The commands this page would have run are listed"
  echo "under Sources; run them from an authenticated host to get the"
  echo "tables back."
  MISSING_REASON="unauthenticated"
else
  MISSING_REASON=""
fi
if [[ -n "$MISSING_REASON" ]]; then
  echo
  echo "## Sources"
  echo
  echo "- \`command -v gh   # $MISSING_REASON\`"
  echo "- \`git for-each-ref refs/tags --format='%(refname:short)' | sort -V | tail -3\`"
  echo "- \`gh run list --commit <full-sha> --limit 20 --json databaseId,name,conclusion,event\`"
  echo "- \`gh run view <id> --json jobs --jq '.jobs[] | [.name, (.conclusion // \"running\"), ((.completedAt|fromdate)-(.startedAt|fromdate))] | @tsv'\`"
  exit 0
fi
src "gh auth status   # ok"

DEFAULT_BRANCH="$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null \
  | sed -e 's|^refs/remotes/||' -e 's|^[^/]*/||' || true)"
DEFAULT_BRANCH="${DEFAULT_BRANCH:-main}"

# Resolve the CI push run per ref once; reuse for both the summary row and
# the per-job table. Pairs are kept as "ref run_id" lines in RUNPAIRS.
RUNPAIRS=""
jobs_of() { # <run-id> → TSV name/conclusion/seconds
  gh run view "$1" --json jobs \
    --jq '.jobs[] | select(.startedAt != null and .completedAt != null) | [.name, (.conclusion // "running"), ((.completedAt|fromdate)-(.startedAt|fromdate))] | @tsv' \
    2>/dev/null || true
}
src "gh run list --commit <full-sha> --limit 20 --json databaseId,name,conclusion,event"
src "gh run view <id> --json jobs --jq '.jobs[] | select(.startedAt != null and .completedAt != null) | [.name, (.conclusion // \"running\"), ((.completedAt|fromdate)-(.startedAt|fromdate))] | @tsv'"

echo "## Per-tag ledger"
echo
echo "| tag | run | conclusion | wall s | billable s | jobs |"
echo "|---|---|---|---:|---:|---|"

for ref in $TAGS HEAD; do
  sha="$(git rev-parse "$ref^{commit}" 2>/dev/null || git rev-parse HEAD)"
  runs="$(gh run list --commit "$sha" --limit 20 \
    --json databaseId,name,conclusion,event,status 2>/dev/null || true)"
  run_id="$(printf '%s' "$runs" \
    | jq -r '[.[] | select(.name == "CI" and .event == "push")][0].databaseId // empty' 2>/dev/null || true)"
  if [[ -z "$run_id" || "$run_id" == "null" ]]; then
    echo "| $ref | — | no CI run at that commit | | | |"
    continue
  fi
  RUNPAIRS="$RUNPAIRS$ref $run_id
"
  jobs="$(jobs_of "$run_id")"
  stats="$(printf '%s\n' "$jobs" | awk -F'\t' '
    { n++; sum += $3; if ($3 > max) max = $3; if ($2 == "cancelled") c++ }
    END { printf "%d %d %d %d", (n + 0), (sum + 0), (max + 0), (c + 0) }')"
  read -r njobs bill wall ncancel <<<"$stats"
  concl="$(printf '%s' "$runs" \
    | jq -r --argjson id "$run_id" '[.[] | select(.databaseId == $id)][0].conclusion // "unknown"' 2>/dev/null || echo unknown)"
  flag=""
  [[ "${ncancel:-0}" -gt 0 ]] && flag=" — $ncancel cancelled job(s), seconds not duration data"
  echo "| $ref | $run_id | $concl$flag | ${wall:-0} | ${bill:-0} | ${njobs:-0} |"
done

echo
echo "## Per-job tables"
echo
printf '%s\n' "$RUNPAIRS" | while read -r ref run_id; do
  [[ -n "$run_id" ]] || continue
  echo "### $ref (run $run_id)"
  echo
  echo "| job | conclusion | seconds |"
  echo "|---|---|---:|"
  jobs_of "$run_id" | awk -F'\t' '{ printf "| %s | %s | %s |\n", $1, $2, $3 }'
  echo
done

echo "## Failure / cancel census — last 60 runs on $DEFAULT_BRANCH"
echo
RECENT="$(gh run list --branch "$DEFAULT_BRANCH" --limit 60 \
  --json databaseId,conclusion,status,name 2>/dev/null || true)"
src "gh run list --branch $DEFAULT_BRANCH --limit 60 --json databaseId,conclusion,status,name"
if [[ -z "$RECENT" ]]; then
  echo "(no runs returned — the branch name or permissions may differ)"
else
  echo "| conclusion | runs |"
  echo "|---|---:|"
  printf '%s' "$RECENT" \
    | jq -r 'group_by(.conclusion // "in_progress")[] | "\(.[0].conclusion // "in_progress") \(length)"' \
    | awk '{ printf "| %s | %s |\n", $1, $2 }'
  echo
  echo "Non-success completed runs, failures/cancellations by job (details"
  echo "fetched for up to 20):"
  echo
  echo "| job | failed | cancelled/other |"
  echo "|---|---:|---:|"
  ids="$(printf '%s' "$RECENT" \
    | jq -r '[.[] | select(.status == "completed" and .conclusion != "success")][0:20][].databaseId | tostring' 2>/dev/null || true)"
  for id in $ids; do
    gh run view "$id" --json jobs \
      --jq '.jobs[] | select(.conclusion != null and .conclusion != "success") | [.name, .conclusion] | @tsv' \
      2>/dev/null || true
  done | awk -F'\t' '
    $2 == "failure" { f[$1]++; next }
    { c[$1]++ }
    END {
      for (j in f) printf "| %s | %d | %d |\n", j, f[j], c[j]
      for (j in c) if (!(j in f)) printf "| %s | 0 | %d |\n", j, c[j]
    }'
  src "gh run view <id> --json jobs --jq '.jobs[] | select(.conclusion != \"success\") | [.name, .conclusion] | @tsv'   # per non-success run"
  echo
fi

echo "## Sources"
echo
echo "Every number above came from a command in this section, run from the"
echo "repo root at base $BASE:"
echo
for s in "${SOURCES[@]}"; do echo "- \`$s\`"; done
