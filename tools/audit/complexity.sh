#!/usr/bin/env bash
# Complexity — size, function complexity, churn × complexity, coupling,
# and the Svelte/TS hotspots (GDK-1707, census rows Size / Function
# complexity / Churn × complexity / Coupling).
#
#   bash tools/audit/complexity.sh > page.md
#
# Counts come from `git ls-files` (tracked files only), so agent
# worktrees under .claude/worktrees/ and node_modules are structurally
# absent — the exclusion the release-audit runbook demands. Coupling joins
# TestImports with Imports: a gate package whose only importers are tests
# reads as dead otherwise (internal/archlint, v0.22 lesson).
#
# gocyclo/gocognit run through `go run …@version` and need the module
# cache (or network) — when unavailable the page says so in that section
# and the rest still measures. desktop/ is a separate module: gocyclo and
# gocognit parse the filesystem (not go list), so its functions count in
# the complexity sections; only Coupling is root-module-only (go list
# ./... at the root does not see desktop/).
#
# Exit 0 always. Every path printed is repo-relative.
set -euo pipefail
cd "$(cd "$(dirname "$0")/../.." && pwd)"
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,/^set -euo pipefail$/p' "$0"
  exit 0
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/audit-complexity.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

SOURCES=()
src() { SOURCES+=("$1"); }

BASE="$(git rev-parse --short HEAD)"
src "git rev-parse --short HEAD   # base: $BASE"

echo "# Complexity — size, hotspots, churn, coupling"
echo
echo "Base $BASE. Tracked files only (agent worktrees and node_modules are"
echo "structurally absent). desktop/ is a separate go module — its files"
echo "count in Size and its functions in complexity (the tools walk the"
echo "filesystem); only Coupling is root-module-only."
echo

# ── 1. Size ───────────────────────────────────────────────────────────────
# `git ls-files` reads the index, not the working tree: a file still tracked
# but deleted from the tree (mid-rebase, a neighbor round's rm repro) reached
# wc's stderr as `wc: …: open: No such file or directory` while the script
# still exited 0 — audit-test's stdout-only contract turns that into a red
# gate only when someone is looking. The filter below hands wc only files
# that exist; an empty section (a tree with no .ts, say) skips wc entirely
# rather than running it with zero operands, which would read stdin.
# Measured 2026-09-10 by moving one tracked .go aside: unfixed run wrote the
# wc complaint to stderr, filtered run is silent and loses only that file's
# line — a degraded census, never a broken one.
wc_of_tracked() {  # wc_of_tracked <glob> <outfile>
  local list="$WORK/ls.$$.$3"
  git ls-files -z "$1" \
    | while IFS= read -r -d '' f; do
        if [[ -f "$f" ]]; then printf '%s\0' "$f"; fi
      done > "$list"
  : > "$2"
  if [[ -s "$list" ]]; then
    xargs -0 wc -l < "$list" | grep -v ' total$' > "$2" || true
  fi
  rm -f "$list"
}
wc_of_tracked '*.go' "$WORK/go.wc" go
wc_of_tracked '*.svelte' "$WORK/svelte.wc" svelte
wc_of_tracked '*.ts' "$WORK/ts.wc" ts
src "git ls-files '*.go' '*.svelte' '*.ts' | existence filter | xargs wc -l   # test split on _test.go"

{
  echo "## Size"
  echo
  echo "| tree | non-test lines | test lines | files |"
  echo "|---|---:|---:|---:|"
  awk '{ if ($2 ~ /_test\.go$/) t += $1; else p += $1; n++ }
       END { printf "| Go | %d | %d | %d |\n", p, t, n }' "$WORK/go.wc"
  awk '{ n++ } END { printf "| Svelte | | | %d |\n", NR }' "$WORK/svelte.wc"
  awk '{ n++ } END { printf "| TS | | | %d |\n", NR }' "$WORK/ts.wc"
  echo
  echo "Svelte/TS line totals:"
  awk '{ s += $1 } END { printf "- Svelte: %d lines\n", s }' "$WORK/svelte.wc"
  awk '{ s += $1 } END { printf "- TS: %d lines\n", s }' "$WORK/ts.wc"
  echo
  echo "Go lines per package (non-test / test):"
  echo
  echo "| package | non-test | test |"
  echo "|---|---:|---:|"
}
awk '
  {
    n = split($2, seg, "/")
    if (n == 1) pkg = "(root)"
    else if (seg[1] == "internal") pkg = (n >= 3 ? "internal/" seg[2] : "internal/(root)")
    else if (seg[1] == "cmd" || seg[1] == "desktop" || seg[1] == "tools")
      pkg = (n >= 3 ? seg[1] "/" seg[2] : seg[1] "/(root)")
    else pkg = seg[1]
    if ($2 ~ /_test\.go$/) t[pkg] += $1
    else p[pkg] += $1
    seen[pkg] = 1
  }
  END {
    for (g in seen) printf "%s|%d|%d\n", g, p[g], t[g]
  }' "$WORK/go.wc" | sort | awk -F'|' '{ printf "| %s | %s | %s |\n", $1, $2, $3 }'
echo
src "awk per-package aggregation over the same wc output"

# ── 2. Function complexity (gocyclo / gocognit) ───────────────────────────
cyclo_all="$(go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -over 1 . 2>/dev/null || true)"
cognit_all="$(go run github.com/uudashr/gocognit/cmd/gocognit@v1.1.3 . 2>/dev/null || true)"
src "go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -over 1 ."
src "go run github.com/uudashr/gocognit/cmd/gocognit@v1.1.3 ."

echo "## Function complexity"
echo
if [[ -z "$cyclo_all" && -z "$cognit_all" ]]; then
  echo "**Not measured:** neither gocyclo nor gocognit could run here"
  echo "(module cache cold and no network for \`go run …@version\`). Run on"
  echo "a host with the cache:"
  echo
  echo '```'
  echo 'go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -over 1 .'
  echo 'go run github.com/uudashr/gocognit/cmd/gocognit@v1.1.3 .'
  echo '```'
else
  echo "### cyclomatic (gocyclo) — non-test"
  echo
  printf '%s\n' "$cyclo_all" | grep -v '_test\.go' | awk '
    { v = $1; n++; sum += v;
      if (v >= 30) g30++; if (v >= 60) g60++; if (v >= 69) g69++ }
    END {
      printf "functions %d, mean %.2f, >=30: %d, >=60: %d, >=69: %d\n", \
        n, (n ? sum / n : 0), g30 + 0, g60 + 0, g69 + 0
    }'
  echo
  echo "top 30 (non-test):"
  echo
  echo "| cyc | where |"
  echo "|---:|---|"
  printf '%s\n' "$cyclo_all" | grep -v '_test\.go' | sort -rn | awk 'NR<=30' \
    | awk '{ c = $1; $1 = ""; sub(/^ +/, ""); print "| " c " | `" $0 "` |" }' || true
  echo
  echo "top 10 including tests:"
  echo
  printf '%s\n' "$cyclo_all" | sort -rn | awk 'NR<=10' \
    | awk '{ c = $1; $1 = ""; sub(/^ +/, ""); print "| " c " | `" $0 "` |" }' || true
  echo
  echo "### cognitive (gocognit) — non-test, top 30"
  echo
  echo "| cogn | where |"
  echo "|---:|---|"
  printf '%s\n' "$cognit_all" | grep -v '_test\.go' | sort -rn | awk 'NR<=30' \
    | awk '{ c = $1; $1 = ""; sub(/^ +/, ""); print "| " c " | `" $0 "` |" }' || true
fi
echo

# ── 3. Svelte/TS hotspots ─────────────────────────────────────────────────
echo "## Svelte/TS hotspots"
echo
echo "\`.svelte\` files with more than three \`\$effect(\` blocks:"
echo
echo "| .svelte file | \$effect( |"
echo "|---|---:|"
awk '{ print $2 }' "$WORK/svelte.wc" | while read -r f; do
  [[ -n "$f" ]] || continue
  n="$(grep -c '\$effect(' "$f" || true)"
  if [[ "${n:-0}" -gt 3 ]]; then printf '| %s | %s |\n' "$f" "$n"; fi
done
echo
echo "largest 15 Svelte/TS files:"
echo
echo "| lines | file |"
echo "|---:|---|"
cat "$WORK/svelte.wc" "$WORK/ts.wc" | sort -rn | awk 'NR<=15' \
  | awk '{ printf "| %s | %s |\n", $1, $2 }' || true
echo
echo "functions over 60 lines (heuristic brace scan, .svelte + .ts, top 20):"
echo
echo "| lines | where |"
echo "|---:|---|"
python3 - "$WORK/svelte.wc" "$WORK/ts.wc" <<'PY' 2>/dev/null || true
import sys

paths = []
for arg in sys.argv[1:]:
    for line in open(arg):
        parts = line.split(None, 1)
        if len(parts) == 2:
            paths.append(parts[1].strip())
spans = []
for p in paths:
    try:
        lines = open(p, encoding='utf-8').read().splitlines()
    except (UnicodeDecodeError, OSError):
        continue
    i = 0
    while i < len(lines):
        ln = lines[i]
        if ('function ' in ln or '=>' in ln) and '{' in ln:
            start = i
            depth = 0
            opened = False
            j = i
            while j < len(lines):
                depth += lines[j].count('{') - lines[j].count('}')
                if '{' in lines[j]:
                    opened = True
                if opened and depth <= 0:
                    break
                j += 1
            if opened and j - start > 60:
                name = ln.strip()[:60]
                spans.append((j - start, f'{p}:{start + 1} {name}'))
                i = j
        i += 1
for span, where in sorted(spans, reverse=True)[:20]:
    print(f'| {span} | `{where}` |')
PY
src "grep -c '\$effect(' per .svelte; wc -l over git ls-files '*.svelte' '*.ts'"
src "python3 brace-span heuristic (in-script) for functions over 60 lines"

# ── 4. Churn × complexity ────────────────────────────────────────────────
echo "## Churn — top 20 most-changed files (all history)"
echo
echo "| commits | file |"
echo "|---:|---|"
git log --name-only --format='' | grep -v '^$' | sort | uniq -c | sort -rn \
  | awk 'NR<=20' | awk '{ printf "| %s | %s |\n", $1, $2 }' || true
echo
src "git log --name-only --format='' | sort | uniq -c | sort -rn | head -20"

# ── 5. Coupling ──────────────────────────────────────────────────────────
echo "## Coupling — internal package fan-out, fan-in, cycles"
echo
MODULE="$(go list -m)"
go list -f '{{.ImportPath}}|{{range .Imports}}{{.}} {{end}}|{{range .TestImports}}{{.}} {{end}}' ./... \
  > "$WORK/pkgs.tsv" 2>/dev/null || true
src "go list -f '{{.ImportPath}}|{{range .Imports}}{{.}} {{end}}|{{range .TestImports}}{{.}} {{end}}' ./...   # Imports joined with TestImports"
python3 - "$WORK/pkgs.tsv" "$MODULE" <<'PY'
import sys

path, module = sys.argv[1], sys.argv[2]
pkgs = []
for line in open(path):
    parts = line.rstrip('\n').split('|')
    if len(parts) != 3:
        continue
    pkgs.append((parts[0], parts[1].split(), parts[2].split()))

def internal(p):
    return p.startswith(module + '/')

fanout, fanin, testin, edges = {}, {}, {}, {}
for ip, imps, timps in pkgs:
    prod = {i for i in imps if internal(i)}
    fanout[ip] = len(prod)
    edges[ip] = prod
    for i in prod:
        fanin[i] = fanin.get(i, 0) + 1
    for t in timps:
        if internal(t) and t not in prod:
            testin[t] = testin.get(t, 0) + 1

print(f'{len(pkgs)} packages in the root module (desktop/ excluded: its own'
      f' go.mod). Top fan-out:')
print()
print('| package | fan-out | fan-in (prod) | fan-in (test-only) |')
print('|---|---:|---:|---:|')
for ip in sorted((p for p, _, _ in pkgs), key=lambda p: -fanout.get(p, 0))[:15]:
    print(f'| {ip} | {fanout.get(ip, 0)} | {fanin.get(ip, 0)} | {testin.get(ip, 0)} |')
print()

WHITE, GREY, BLACK = 0, 1, 2
color = {p: WHITE for p, _, _ in pkgs}
stack, cycles = [], []
sys.setrecursionlimit(10000)

def dfs(p):
    color[p] = GREY
    stack.append(p)
    for q in sorted(edges.get(p, ())):
        if color.get(q, BLACK) == BLACK and q not in color:
            continue
        if q not in color:
            continue
        if color[q] == GREY:
            k = stack.index(q) if q in stack else 0
            cycles.append(stack[k:] + [q])
        elif color[q] == WHITE:
            dfs(q)
    stack.pop()
    color[p] = BLACK

for p, _, _ in pkgs:
    if color[p] == WHITE:
        dfs(p)
print(f'Import cycles: {len(cycles)}')
for c in cycles[:5]:
    print(f'- {" -> ".join(c)}')
print()

prod_single = [p for p, _, _ in pkgs if fanin.get(p, 0) == 1]
test_only = [p for p, _, _ in pkgs
             if fanin.get(p, 0) == 0 and testin.get(p, 0) > 0]
orphan = [p for p, _, _ in pkgs
          if fanin.get(p, 0) == 0 and testin.get(p, 0) == 0]
print(f'Packages with exactly one production importer: {len(prod_single)}'
      f' (fold-or-justify candidates; audit axis 1 opens here):')
for p in sorted(prod_single)[:15]:
    print(f'- {p}')
print()
print(f'Packages with zero production importers but test importers (gate'
      f' packages — NOT dead; the archlint lesson): {len(test_only)}')
for p in sorted(test_only)[:10]:
    print(f'- {p} (test importers: {testin.get(p, 0)})')
print()
print(f'Packages with no importers at all (main packages and tools land'
      f' here): {len(orphan)}')
for p in sorted(orphan)[:10]:
    print(f'- {p}')
PY
src "python3 fan-in/fan-out + cycle DFS over that graph (in-script)"

echo "## Sources"
echo
echo "Every number above came from a command in this section, run from the"
echo "repo root at base $BASE:"
echo
for s in "${SOURCES[@]}"; do echo "- \`$s\`"; done
