#!/usr/bin/env bash
# The single owner of how the CI race suite is split across shards (GDK-1035).
#
# .github/workflows/ci.yml never spells out a bucket itself — it asks this
# script which tests belong to shard N of T. Test names are discovered from
# the package source at run time (never a checked-in list: a checked-in
# list is the thing that goes stale and silently drops a test), sorted, and
# dealt round-robin, so the split is deterministic and balanced by count
# with no timing table.
#
# Why a race tier exists at all (GDK-270): a startSyncJob goroutine that
# outlives its test only shows up when the package is repeated under the
# race detector. That is why the tier runs -count=2 — and why splitting it
# must not quietly drop, skip, or double a test. The split is a
# redistribution across runners, never a relaxation.
#
# Usage:
#   tools/race-partition.sh <shard> <total>   print the -run regex for one shard
#   tools/race-partition.sh --check <total>   verify the partition: every test
#                                             in exactly one shard, no empty
#                                             shard; exit 1 with names if not
#   tools/race-partition.sh --list <total>    shard / count / first-last map
#
# Exit: 0 ok, 1 partition broken (--check), 2 usage error.
set -euo pipefail
export LC_ALL=C # byte-order sort, so every machine deals the same round

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$ROOT/internal/server"
SERVER_PKG="./internal/server/"

usage() {
  cat >&2 <<'EOF'
usage:
  tools/race-partition.sh <shard> <total>   print the -run regex for one shard
  tools/race-partition.sh --check <total>   verify the partition (exit 1 if broken)
  tools/race-partition.sh --list <total>    shard / count / first-last map
EOF
  exit 2
}

# Every top-level test function in the package, one per line, sorted, with
# the Test prefix kept — go test matches -run against full test names, and
# the go-test -list comparison below holds discovery to exactly that.
# `sort -u` is belt-and-braces: Go does not compile a package that declares
# the same test function name twice.
test_names() {
  sed -n -E 's/^func (Test[A-Za-z0-9_]+)\(.*/\1/p' "$SERVER_DIR"/*_test.go |
    sort -u
}

# The sorted names dealt to one shard: round-robin, i % total.
shard_names() { # $1 = shard (1-based), $2 = total
  test_names |
    awk -v s="$1" -v t="$2" '(NR - 1) % t == (s - 1) % t'
}

# The go test -run regex for one shard. The anchors are load-bearing:
# -run matches unanchored, so without ^( )$ the shard holding
# TestCreateIssue would also run TestCreateIssueDefaultProject (prefix pairs
# exist in this package), and one test would land in two shards.
# An empty shard prints ^()$, which matches no test name — --check is what
# turns that into a failure.
shard_regex() { # $1 = shard, $2 = total
  local body
  body="$(shard_names "$1" "$2" | paste -sd '|' -)"
  if [[ -n "$body" ]]; then
    printf '^(%s)$\n' "$body"
  else
    printf '^()$\n'
  fi
}

[[ $# -eq 2 ]] || usage
mode=shard
case "$1" in
  --check) mode=check ;;
  --list) mode=list ;;
  -*) usage ;;
  *) shard="$1" ;;
esac
total="$2"
[[ "$total" =~ ^[1-9][0-9]*$ ]] || {
  echo "race-partition: total must be a positive integer, got '$total'" >&2
  exit 2
}
if [[ "$mode" = shard ]]; then
  [[ "$shard" =~ ^[1-9][0-9]*$ ]] || {
    echo "race-partition: shard must be a positive integer, got '$shard'" >&2
    exit 2
  }
  [[ "$shard" -le "$total" ]] || {
    echo "race-partition: shard $shard is out of range 1..$total" >&2
    exit 2
  }
fi

if [[ "$mode" = shard ]]; then
  re="$(shard_regex "$shard" "$total")"
  if [[ "$re" = '^()$' ]]; then
    echo "race-partition: warning: shard $shard of $total selects no test" >&2
  fi
  printf '%s\n' "$re" # stdout carries the regex and nothing else
  exit 0
fi

names="$(test_names)"
if [[ -z "$names" ]]; then
  echo "race-partition: no test functions found in $SERVER_DIR/*_test.go" >&2
  exit 1
fi

all=()
while IFS= read -r n; do all+=("$n"); done <<<"$names"

if [[ "$mode" = list ]]; then
  echo "race-partition: ${#all[@]} tests across $total shards"
  s=1
  while [[ "$s" -le "$total" ]]; do
    sn="$(shard_names "$s" "$total")"
    if [[ -n "$sn" ]]; then
      count="$(printf '%s\n' "$sn" | grep -c .)"
      first="$(printf '%s\n' "$sn" | sed -n '1p')"
      last="$(printf '%s\n' "$sn" | sed -n '$p')"
      printf 'shard %d of %d: %3d tests  %s ... %s\n' \
        "$s" "$total" "$count" "$first" "$last"
    else
      printf 'shard %d of %d:   0 tests  (empty)\n' "$s" "$total"
    fi
    s=$((s + 1))
  done
  exit 0
fi

# --check: verification, not re-derivation. The regexes under test are built
# by the same generator the workflow calls; the assertions below match those
# regexes against the discovered name list — the round-robin arithmetic that
# produced them is not consulted, so a generator bug cannot pass its own test.
#
# (d) first: discovery must agree with what the Go toolchain actually runs.
# `go test -list` is generated by the compiler from the same source, so it
# catches drift between the grep pattern and Go's own notion of a test name.
# The first draft of this script captured names without the Test prefix —
# internally consistent, (a)–(c) green, while every regex would have
# selected zero tests. Only the toolchain comparison sees that class.
if ! command -v go >/dev/null 2>&1; then
  echo "race-partition: go is not on PATH; --check needs 'go test -list' to verify discovery" >&2
  exit 1
fi
if ! go_raw="$(cd "$ROOT" && go test "$SERVER_PKG" -list '.*' 2>&1)"; then
  echo "race-partition: go test -list failed, cannot verify discovery against the toolchain:" >&2
  printf '%s\n' "$go_raw" | sed 's/^/  /' >&2
  exit 1
fi
go_names="$(printf '%s\n' "$go_raw" | { grep -E '^Test[A-Za-z0-9_]+$' || true; } | sort -u)"

only_grep=() # discovered from source, but the toolchain would not run it
only_go=()   # runnable per the toolchain, but the discovery missed it
while IFS= read -r line; do only_grep+=("$line"); done \
  < <(comm -23 <(printf '%s\n' "$names") <(printf '%s\n' "$go_names"))
while IFS= read -r line; do only_go+=("$line"); done \
  < <(comm -13 <(printf '%s\n' "$names") <(printf '%s\n' "$go_names"))

regexes=()
shard_hit_count=()
s=1
while [[ "$s" -le "$total" ]]; do
  regexes[s]="$(shard_regex "$s" "$total")"
  shard_hit_count[s]=0
  s=$((s + 1))
done

uncovered=()
doubled=()
covered=0
for name in "${all[@]}"; do
  hits=()
  s=1
  while [[ "$s" -le "$total" ]]; do
    if [[ "$name" =~ ${regexes[s]} ]]; then
      hits+=("$s")
    fi
    s=$((s + 1))
  done
  case "${#hits[@]}" in
    0) uncovered+=("$name") ;;
    1)
      covered=$((covered + 1))
      shard_hit_count[${hits[0]}]=$((shard_hit_count[${hits[0]}] + 1))
      ;;
    *)
      hit_list="$(IFS=','; printf '%s' "${hits[*]}")"
      doubled+=("$name (shards $hit_list)")
      ;;
  esac
done

empty=()
s=1
while [[ "$s" -le "$total" ]]; do
  if [[ "${shard_hit_count[s]}" -eq 0 ]]; then
    empty+=("$s")
  fi
  s=$((s + 1))
done

rc=0
if [[ "${#only_go[@]}" -gt 0 ]]; then
  i=0
  for n in "${only_go[@]}"; do
    i=$((i + 1))
    if [[ "$i" -gt 20 ]]; then
      echo "race-partition: ... and $((${#only_go[@]} - 20)) more runnable-but-undiscovered, not listed"
      break
    fi
    echo "race-partition: $n is runnable per go test -list but not discovered from source (${#only_go[@]} hidden of ${#all[@]} discovered) — the partition would skip it"
  done
  rc=1
fi
if [[ "${#only_grep[@]}" -gt 0 ]]; then
  i=0
  for n in "${only_grep[@]}"; do
    i=$((i + 1))
    if [[ "$i" -gt 20 ]]; then
      echo "race-partition: ... and $((${#only_grep[@]} - 20)) more undiscoverable-by-go, not listed"
      break
    fi
    echo "race-partition: $n is discovered from source but not runnable per go test -list (${#only_grep[@]} extra) — no regex would ever select it"
  done
  rc=1
fi
if [[ "${#uncovered[@]}" -gt 0 ]]; then
  i=0
  for n in "${uncovered[@]}"; do
    i=$((i + 1))
    if [[ "$i" -gt 20 ]]; then
      echo "race-partition: ... and $((${#uncovered[@]} - 20)) more uncovered, not listed"
      break
    fi
    echo "race-partition: $n is in no shard (${#uncovered[@]} of ${#all[@]} uncovered)"
  done
  rc=1
fi
if [[ "${#doubled[@]}" -gt 0 ]]; then
  i=0
  for d in "${doubled[@]}"; do
    i=$((i + 1))
    if [[ "$i" -gt 20 ]]; then
      echo "race-partition: ... and $((${#doubled[@]} - 20)) more double-selected, not listed"
      break
    fi
    echo "race-partition: $d — must be in exactly one shard (${#doubled[@]} of ${#all[@]} double-selected)"
  done
  rc=1
fi
if [[ "${#empty[@]}" -gt 0 ]]; then
  i=0
  for s in "${empty[@]}"; do
    i=$((i + 1))
    if [[ "$i" -gt 20 ]]; then
      echo "race-partition: ... and $((${#empty[@]} - 20)) more empty shards, not listed"
      break
    fi
    echo "race-partition: shard $s of $total selects no test (${#empty[@]} empty shards)"
  done
  rc=1
fi

if [[ "$rc" -eq 0 ]]; then
  echo "race-partition: ${#all[@]} tests covered exactly once across $total shards (covered=$covered; discovery matches go test -list)"
  s=1
  while [[ "$s" -le "$total" ]]; do
    echo "race-partition: shard $s/$total: ${shard_hit_count[s]} tests"
    s=$((s + 1))
  done
fi
exit "$rc"
