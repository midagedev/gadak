#!/usr/bin/env bash
# The single owner of how the CI browser suite is split across shards
# (GDK-1035, GDK-1702).
#
# .github/workflows/ci.yml never spells out a file list itself — it asks
# this script which spec files belong to shard N of T. --shard=N/3 dealt
# round-robin by test count, which is balance only if every file costs the
# same: the cost census measured the three shards at 220/297/261 s of test
# time, the longest shard being the job's wall clock. Here files are dealt
# by the measured table (e2e/shard-weights.tsv) instead: longest-processing-
# time-first onto the lightest shard.
#
# Why this must not quietly drop, double, or invent a file: each shard is a
# separate runner (GDK-1035 — sharding at the runner level only, the config
# keeps workers: 1 and fullyParallel: false), so a spec the partition loses
# stops running anywhere. --check is what holds that: every spec in exactly
# one shard, no empty shard, no stale weight row, and discovery cross-checked
# against `playwright test --list` — the toolchain's own notion of the CI
# set, which also catches a glob that disagrees with the config's testIgnore.
#
# Files print as e2e/-prefixed paths on purpose: playwright matches a
# positional argument against the whole spec path, and a bare basename is a
# substring match — `theme.spec.ts` would also select
# terminal-theme.spec.ts (measured). The prefix makes it an exact pick.
#
# Usage:
#   tools/e2e-partition.sh <shard> <total>   print the spec files for one shard
#   tools/e2e-partition.sh --check <total>   verify the partition: every spec
#                                            in exactly one shard, no empty
#                                            shard, no stale weight row,
#                                            discovery == playwright --list
#   tools/e2e-partition.sh --list <total>    shard / files / seconds map
#
# Exit: 0 ok, 1 partition broken (--check), 2 usage error.
set -euo pipefail
export LC_ALL=C # byte-order sort, so every machine deals the same files

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
E2E_DIR="$ROOT/e2e"
WEIGHTS="$E2E_DIR/shard-weights.tsv"
SELF="e2e-partition"

usage() {
  cat >&2 <<'EOF'
usage:
  tools/e2e-partition.sh <shard> <total>   print the spec files for one shard
  tools/e2e-partition.sh --check <total>   verify the partition (exit 1 if broken)
  tools/e2e-partition.sh --list <total>    shard / files / seconds map
EOF
  exit 2
}

# Every spec file the CI set runs: top-level e2e/*.spec.ts. The config's
# testIgnore (demo/, hosted/, perf/) has no top-level spec, so this glob and
# `playwright --list` agree — check (e) below holds them to it.
spec_files() {
  (cd "$ROOT" && ls e2e/*.spec.ts) | sort
}

# "weight<TAB>path" for every discovered spec. A file absent from the table
# is dealt the table's median weight: a new spec starts average-heavy until
# the next census refresh, never silently free and never skipped.
weighted_specs() {
  spec_files | awk -v weights="$WEIGHTS" '
    BEGIN {
      while ((getline line < weights) > 0) {
        if (line ~ /^#/ || line ~ /^$/) continue
        split(line, f, "\t")
        w[f[1]] = f[2] + 0
        n++
        v[n] = f[2] + 0
      }
      close(weights)
      if (n > 0) {
        # insertion sort — n is a file count, and this stays awk-portable
        for (i = 2; i <= n; i++) {
          x = v[i]
          for (j = i - 1; j >= 1 && v[j] > x; j--) v[j + 1] = v[j]
          v[j + 1] = x
        }
        median = (n % 2) ? v[int((n + 1) / 2)] : (v[n / 2] + v[n / 2 + 1]) / 2
      } else {
        median = 0
      }
    }
    { printf "%.1f\t%s\n", (($0 in w) ? w[$0] : median), $0 }
  '
}

# LPT deal: heaviest file first, each onto the lightest shard; weight ties
# break by path, load ties by the lower shard number, so the assignment is
# a function of the table alone. stdout: "shard<TAB>path<TAB>weight",
# nothing else.
deal() { # $1 = total
  weighted_specs |
    sort -t"$(printf '\t')" -k1,1nr -k2,2 |
    awk -F'\t' -v t="$1" '
      {
        b = 1
        for (i = 2; i <= t; i++) if (load[i] < load[b]) b = i
        load[b] += $1
        printf "%d\t%s\t%.1f\n", b, $2, $1
      }'
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
  echo "$SELF: total must be a positive integer, got '$total'" >&2
  exit 2
}
if [[ "$mode" = shard ]]; then
  [[ "$shard" =~ ^[1-9][0-9]*$ ]] || {
    echo "$SELF: shard must be a positive integer, got '$shard'" >&2
    exit 2
  }
  [[ "$shard" -le "$total" ]] || {
    echo "$SELF: shard $shard is out of range 1..$total" >&2
    exit 2
  }
fi

if [[ "$mode" = shard ]]; then
  deal "$total" | awk -F'\t' -v s="$shard" '$1 == s { print $2 }' # stdout: paths only
  exit 0
fi

names="$(spec_files)"
if [[ -z "$names" ]]; then
  echo "$SELF: no spec files found under $E2E_DIR/*.spec.ts" >&2
  exit 1
fi
all=()
while IFS= read -r n; do all+=("$n"); done <<<"$names"

# (a) stale or malformed weight rows: a row naming a file that no longer
# exists is the table rotting in place — the partition stays valid while
# lying about what it balances. Non-numeric seconds make LPT sort garbage.
bad_rows=()
while IFS= read -r line; do
  path="$(printf '%s\n' "$line" | cut -f1)"
  secs="$(printf '%s\n' "$line" | cut -f2)"
  if ! printf '%s' "$secs" | grep -Eq '^[0-9]+([.][0-9]+)?$'; then
    bad_rows+=("$path (seconds '$secs' is not a number)")
  elif ! printf '%s\n' "$names" | grep -qx "$path"; then
    bad_rows+=("$path (no such spec in the CI set)")
  fi
done < <(grep -v -E '^(#|$)' "$WEIGHTS")

# The deal under test is built by the same generator the workflow calls;
# the assertions below interrogate its output, not the LPT arithmetic that
# produced it, so a generator bug cannot pass its own check.
assignment="$(deal "$total")"
shard_files=()
counts=()
seconds=()
s=1
while [[ "$s" -le "$total" ]]; do
  shard_files[s]="$(printf '%s\n' "$assignment" | awk -F'\t' -v s="$s" '$1 == s { print $2 }')"
  counts[s]="$(printf '%s\n' "${shard_files[s]}" | grep -c .)"
  seconds[s]="$(printf '%s\n' "$assignment" | awk -F'\t' -v s="$s" '$1 == s { sum += $3 } END { printf "%.1f", sum }')"
  s=$((s + 1))
done

uncovered=()
doubled=()
covered=0
for name in "${all[@]}"; do
  hits=()
  s=1
  while [[ "$s" -le "$total" ]]; do
    if printf '%s\n' "${shard_files[s]}" | grep -qx "$name"; then
      hits+=("$s")
    fi
    s=$((s + 1))
  done
  case "${#hits[@]}" in
    0) uncovered+=("$name") ;;
    1) covered=$((covered + 1)) ;;
    *)
      hit_list="$(IFS=','; printf '%s' "${hits[*]}")"
      doubled+=("$name (shards $hit_list)")
      ;;
  esac
done

empty=()
s=1
while [[ "$s" -le "$total" ]]; do
  if [[ "${counts[s]}" -eq 0 ]]; then
    empty+=("$s")
  fi
  s=$((s + 1))
done

# (e) discovery must agree with what the playwright toolchain actually
# collects. `--list` applies the config's testMatch/testIgnore; a glob that
# disagrees would partition files the runner never loads, or — the class
# that matters — miss files it does. It lists test paths relative to the
# config's testDir, i.e. bare spec basenames here. The extraction matches
# the one "<name>.spec.ts:<line>" token per line: byte-anchored awk
# match() under LC_ALL=C, deliberately not a multibyte separator split (a
# space+›+space FS is three-plus bytes and BWK awk splits on each of them)
# and not sed (s/:[0-9]+$// misbehaves under some sed builds).
only_glob=() # in the glob, but the toolchain would not collect it
only_pw=()   # collected by the toolchain, but the glob missed it
if [[ "$mode" = check ]]; then
  if ! command -v npx >/dev/null 2>&1; then
    echo "$SELF: npx is not on PATH; --check needs 'playwright test --list' to verify discovery" >&2
    exit 1
  fi
  if ! pw_raw="$(cd "$ROOT" && npx playwright test --config e2e/playwright.config.ts --list 2>&1)"; then
    echo "$SELF: playwright --list failed, cannot verify discovery against the toolchain:" >&2
    printf '%s\n' "$pw_raw" | sed 's/^/  /' >&2
    exit 1
  fi
  pw_files="$(printf '%s\n' "$pw_raw" |
    LC_ALL=C awk '
      match($0, /[A-Za-z0-9._-]+\.spec\.ts:[0-9]+/) {
        f = substr($0, RSTART, RLENGTH)
        sub(/:[0-9]+$/, "", f)
        print f
      }' | sort -u)"
  base_names="$(printf '%s\n' "$names" | sed 's|^e2e/||')"
  while IFS= read -r line; do only_glob+=("$line"); done \
    < <(comm -23 <(printf '%s\n' "$base_names") <(printf '%s\n' "$pw_files"))
  while IFS= read -r line; do only_pw+=("$line"); done \
    < <(comm -13 <(printf '%s\n' "$base_names") <(printf '%s\n' "$pw_files"))
fi

if [[ "$mode" = check ]]; then
  fail=0
  if [[ ${#bad_rows[@]} -gt 0 ]]; then
    echo "$SELF: weight rows that are stale or not numeric:" >&2
    printf '  %s\n' "${bad_rows[@]}" >&2
    fail=1
  fi
  if [[ ${#uncovered[@]} -gt 0 ]]; then
    echo "$SELF: specs no shard would run:" >&2
    printf '  %s\n' "${uncovered[@]}" >&2
    fail=1
  fi
  if [[ ${#doubled[@]} -gt 0 ]]; then
    echo "$SELF: specs dealt to more than one shard:" >&2
    printf '  %s\n' "${doubled[@]}" >&2
    fail=1
  fi
  if [[ ${#empty[@]} -gt 0 ]]; then
    echo "$SELF: empty shards: ${empty[*]}" >&2
    fail=1
  fi
  if [[ ${#only_glob[@]} -gt 0 || ${#only_pw[@]} -gt 0 ]]; then
    echo "$SELF: discovery disagrees with 'playwright test --list':" >&2
    if [[ ${#only_glob[@]} -gt 0 ]]; then
      printf '  glob has, toolchain has not:\n  %s\n' "${only_glob[@]}" >&2
    fi
    if [[ ${#only_pw[@]} -gt 0 ]]; then
      printf '  toolchain has, glob has not:\n  %s\n' "${only_pw[@]}" >&2
    fi
    fail=1
  fi
  if [[ "$fail" -ne 0 ]]; then
    echo "$SELF: partition broken" >&2
    exit 1
  fi
fi

# grep -c exits 1 on a zero count — the every-file-measured case — so the
# count is read with the exit status masked.
median_fallback="$(comm -23 <(spec_files) <(grep -v -E '^(#|$)' "$WEIGHTS" | cut -f1) | grep -c . || true)"
total_seconds="$(printf '%s\n' "$assignment" | awk -F'\t' '{ sum += $3 } END { printf "%.1f", sum }')"
echo "$SELF: ${#all[@]} spec files across $total shards, ${total_seconds} s weighted; $median_fallback file(s) on median fallback; stale weight rows: ${#bad_rows[@]}"
s=1
while [[ "$s" -le "$total" ]]; do
  printf 'shard %d of %d: %3d files  %6.1f s  %s ... %s\n' \
    "$s" "$total" "${counts[s]}" "${seconds[s]}" \
    "$(printf '%s\n' "${shard_files[s]}" | sed -n '1p' | sed 's|^e2e/||')" \
    "$(printf '%s\n' "${shard_files[s]}" | sed -n '$p' | sed 's|^e2e/||')"
  s=$((s + 1))
done
