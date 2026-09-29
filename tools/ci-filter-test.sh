#!/usr/bin/env bash
# Tests for tools/ci-filter.sh (GDK-1912).
#
#   bash tools/ci-filter-test.sh
#
# The filter is what stands between a docs-only push and ~3,201 s of billed
# CI, so the test asserts both directions of the contract:
#   1. the census case: a CHANGELOG/docs-only diff skips every subject
#   2. one .go change runs the tiers that compile Go (race builds the two
#      packages; e2e/serve.sh, mobile gate-serve.sh and the desktop pack
#      scripts all build the root module — ci.yml, not a guess)
#   3. the mobile-only slices (web/src/lib/i18n, lib/terminal, logo.png,
#      pairing vectors) run mobile and nothing else
#   4. fail-open: no base, all-zero base, workflow_dispatch, an unreachable
#      base, a failed diff — all run (fake git, never the real network)
#
# The guards-survival section is the point of the round (the spec calls it
# the 본체): every incident class the filtered jobs guard (GDK-270/1035/868/
# 1540/208/292/167/1407/1380/1036) fired on a code or packaging input, never
# on CHANGELOG or docs — so each one is pinned to the input that fired it,
# and the test fails the day a table edit stops catching it.
#
# The bad-input cases at the end are the FAIL-first for this gate: a filter
# that answered `run=true` for everything, or crashed on odd paths, cannot
# pass them.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/ci-filter-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# fake git on PATH for the CI-mode cases: present, scriptable. MODE says how
# it misbehaves (ok | fetch-fail | diff-fail); LIST names the file whose
# lines `git diff --name-only` prints.
mkdir -p "$WORK/fakebin"
cat > "$WORK/fakebin/git" <<'SHIM'
#!/usr/bin/env bash
# fake git for tools/ci-filter-test.sh — never touches the network.
case "${1:-}" in
  fetch)
    if [[ "${FAKE_GIT_MODE:-ok}" = fetch-fail ]]; then
      echo "fake-git: fetch: refusing (mode=fetch-fail)" >&2
      exit 1
    fi
    exit 0
    ;;
  diff)
    if [[ "${FAKE_GIT_MODE:-ok}" = diff-fail ]]; then
      echo "fake-git: diff: refusing (mode=diff-fail)" >&2
      exit 1
    fi
    if [[ -z "${FAKE_GIT_DIFF:-}" || ! -f "$FAKE_GIT_DIFF" ]]; then
      echo "fake-git: FAKE_GIT_DIFF not set to a file" >&2
      exit 1
    fi
    cat "$FAKE_GIT_DIFF"
    exit 0
    ;;
  *)
    echo "fake-git: unexpected invocation: $*" >&2
    exit 1
    ;;
esac
SHIM
chmod +x "$WORK/fakebin/git"

FAILURES=0
fail() { echo "  FAIL: $*"; FAILURES=$((FAILURES + 1)); }

# decide <subject> <newline-joined paths> — prints just the run= line.
decide() {
  printf '%s\n' "$2" | bash tools/ci-filter.sh "$1" --files - | sed -n '1p'
}

# expect_run / expect_skip <label> <subject> <paths>
expect_run() {
  local out
  out="$(decide "$2" "$3")"
  if [[ "$out" = run=true ]]; then
    echo "- $1: run"
  else
    fail "$1: subject '$2' said '$out', want run=true (paths: $(printf '%s' "$3" | tr '\n' ' '))"
  fi
}
expect_skip() {
  local out
  out="$(decide "$2" "$3")"
  if [[ "$out" = run=false ]]; then
    echo "- $1: skip"
  else
    fail "$1: subject '$2' said '$out', want run=false (paths: $(printf '%s' "$3" | tr '\n' ' '))"
  fi
}

# ci_mode <label> <want: run|skip> <subject> — runs the CI path (env + git)
# with the fake git on PATH; the env comes from the CI_ENV array so nothing
# leaks between cases (`VAR=x func` persists past the call in bash's default
# mode — exactly the silent state bleed a fail-open test cannot afford).
CI_ENV=()
ci_mode() { # $1 label, $2 want, $3 subject
  local out rc=0
  out="$(PATH="$WORK/fakebin:$PATH" env "${CI_ENV[@]}" bash tools/ci-filter.sh "$3" \
    2>"$WORK/ci.err" | sed -n '1p')" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    fail "$1: exited $rc"
    sed 's/^/    /' "$WORK/ci.err"
    return
  fi
  if [[ "$out" = "run=true" && "$2" = run ]] || [[ "$out" = "run=false" && "$2" = skip ]]; then
    echo "- $1: $2"
  else
    fail "$1: said '$out', want $2"
    sed 's/^/    /' "$WORK/ci.err" || true
  fi
}

echo "== usage: bad invocations exit 2"
for args in "" "nonsense" "go --files"; do
  # shellcheck disable=SC2086 — the point is to test odd argv shapes
  rc=0
  bash tools/ci-filter.sh $args >/dev/null 2>&1 || rc=$?
  if [[ "$rc" = 2 ]]; then
    echo "- argv '$args' → exit 2"
  else
    fail "argv '$args' exited $rc, want 2"
  fi
done
rc=0
bash tools/ci-filter.sh go --files "$WORK/does-not-exist" >/dev/null 2>&1 || rc=$?
if [[ "$rc" = 2 ]]; then
  echo "- missing --files file → exit 2"
else
  fail "missing --files file exited $rc, want 2"
fi

echo
echo "== case 1: the census case — a docs-only push skips every subject"
# The 31-of-127 shape: CHANGELOG retouches, CLAUDE.md, specs/, docs prose,
# artifacts/, contrib/. Nothing here is a build input of any tier.
DOCS_ONLY="CHANGELOG.md
CHANGELOG.ko.md
CLAUDE.md
docs/project/STATE_OF_PLAY.md
specs/000-product/data-model.md
artifacts/notes.txt
contrib/README.md"
for s in go e2e mobile desktop staticcheck; do
  expect_skip "docs-only / $s" "$s" "$DOCS_ONLY"
done
# docs/media is mobile's ONE docs input (the brand-icon check reads it) —
# a markdown file next to the logo must not fire it.
expect_skip "docs prose under docs/media (not the logo)" mobile "docs/media/README.md"

echo
echo "== case 2: one .go change runs every tier that compiles Go"
# ci.yml, not taste: the race tier compiles the two packages from the root
# module; e2e/serve.sh builds the binary it serves; gate-serve.sh runs
# `gadak demo`; every desktop pack script builds ./cmd/gadak.
for s in go e2e mobile desktop staticcheck; do
  expect_run "one .go / $s" "$s" "internal/server/sync.go"
done
expect_run "cmd .go / go" go "cmd/gadak/main.go"
expect_run "module graph / go" go "go.mod"
expect_run "module graph / desktop" desktop "go.sum"

echo
echo "== case 3: the mobile-only slices"
expect_run "i18n catalog → mobile" mobile "web/src/lib/i18n/ko.json"
expect_run "terminal protocol → mobile" mobile "web/src/lib/terminal/protocol.ts"
expect_run "brand mark → mobile" mobile "docs/media/logo.png"
expect_run "pairing vectors → mobile" mobile "internal/pairing/testdata/offer-vectors.json"
expect_run "mobile src → mobile" mobile "mobile/src/screens/Shell.svelte"
# The same slices must not fire the tiers that never read them.
expect_skip "i18n catalog does not touch go" go "web/src/lib/i18n/ko.json"
expect_skip "mobile src does not touch e2e" e2e "mobile/src/screens/Shell.svelte"
# A web/ change fires e2e and desktop (the embedded UI) but not go.
expect_run "web src → e2e" e2e "web/src/routes/+page.svelte"
expect_run "web src → desktop" desktop "web/src/routes/+page.svelte"
expect_skip "web src does not touch go" go "web/src/routes/+page.svelte"

echo
echo "== case 3b: run 35025858519 — the wails pin bump (GDK-1931)"
# The filter's first real-world skip test, and the round this gate was
# narrowed in: a main push changing only desktop/ (8 files — its own
# go.mod, compiled by no root-module tier) plus examples/backlog-snapshot
# .tar.gz (regenerated by tools/rebase-pr.sh on every rebase, read by no
# tier) ran race ×3, e2e ×3 and mobile. The paths below are that push's
# verbatim diff (`git diff --name-only 5932fda7 0572842e`).
RUN_35025858519="desktop/README.md
desktop/build-windows.ps1
desktop/coldstart_test.go
desktop/deeplink.go
desktop/go.mod
desktop/go.sum
desktop/main.go
desktop/platforms_test.go
examples/backlog-snapshot.tar.gz"
expect_skip "run 35025858519 / go" go "$RUN_35025858519"
expect_skip "run 35025858519 / e2e" e2e "$RUN_35025858519"
expect_skip "run 35025858519 / mobile" mobile "$RUN_35025858519"
expect_run "run 35025858519 / desktop" desktop "$RUN_35025858519"

echo
echo "== case 3c: the carve-outs — desktop/ is its own module, examples/ narrows to what tiers open"
# desktop/ has its own go.mod (desktop CI jobs build it separately): the
# race tier compiles ./internal/server + ./internal/workspace from the root
# module, e2e/serve.sh builds ./cmd/gadak, gate-serve.sh builds `gadak demo`
# — none of them reads a desktop/*.go file. Only the desktop subject (its
# pack scripts also build the root ./cmd/gadak) takes it.
for s in go e2e mobile; do
  expect_skip "desktop module .go does not touch $s" "$s" "desktop/main.go"
done
expect_run "desktop module .go → desktop" desktop "desktop/main.go"
# examples/ narrowed to the files a tier actually opens (GDK-1931): the
# seeds (serve.sh SEED_DB; gate-serve.sh's api digest lists examples/
# demo.db), the committed PR fixture (pr-links.spec.ts), the i18n strings
# (helpers.ts mediaFixture), the attachment cache (serve.sh
# --import-attachments). The rebase artifact and any other stray file
# under examples/ fire nothing.
expect_skip "backlog snapshot (rebase-pr artifact) / go" go "examples/backlog-snapshot.tar.gz"
expect_skip "backlog snapshot (rebase-pr artifact) / e2e" e2e "examples/backlog-snapshot.tar.gz"
expect_run "demo-source.db (pr-links fixture) → e2e" e2e "examples/demo-source.db"
expect_run "demo-i18n strings → e2e" e2e "examples/demo-i18n/ko.json"
expect_run "attachment cache → e2e" e2e "examples/attachments/manifest.json"
expect_run "demo.db seed → mobile" mobile "examples/demo.db"
expect_skip "a stray file under examples/ / go" go "examples/notes.txt"
expect_skip "a stray file under examples/ / e2e" e2e "examples/notes.txt"
expect_skip "a stray file under examples/ / mobile" mobile "examples/notes.txt"

echo
echo "== case 3d: subject staticcheck — both modules, no desktop carve-out (GDK-2003)"
# The defect this subject closes: the staticcheck job's inline gofilter
# regex carried `^desktop/`, so desktop/README.md alone woke the 335 s GOOS
# matrix. The inverse trap is worse and is the reason this is a NEW subject
# rather than subject `go`: the go table carves desktop/ out (no
# root-module tier compiles it), but tools/staticcheck.sh DOES analyse the
# desktop module (`run_module desktop desktop ./...`) — reusing `go` would
# skip a desktop/main.go-only push and hide a Go change from a gate. The
# run/skip pair below pins both directions; desktop/main.go is the row that
# catches the reuse-go trap.
expect_skip "desktop/README.md does not touch staticcheck" staticcheck "desktop/README.md"
expect_run "desktop module .go → staticcheck (the reuse-go trap)" staticcheck "desktop/main.go"
expect_run "run 35025858519 / staticcheck" staticcheck "$RUN_35025858519"
expect_run "desktop module graph → staticcheck" staticcheck "desktop/go.mod"
expect_run "desktop module sums → staticcheck" staticcheck "desktop/go.sum"
expect_skip "pack scripts are not analysis inputs" staticcheck "desktop/build-linux.sh"
expect_skip "the syso pair is a link input, not an analysis one" staticcheck "desktop/windows-app.manifest"
expect_run "root module graph → staticcheck" staticcheck "go.mod"
expect_run "root .go → staticcheck" staticcheck "internal/statuscat/category.go"
# internal/ stays a whole-directory row for one measured reason: the
# //go:embed files there are load-bearing. `go list -e` on a package whose
# embed target is deleted reports "pattern data.json: no matching files
# found" with Incomplete=true — the load itself fails, which
# staticcheck.sh's classifier fails the run on. Content is never analysed;
# existence is. (testdata/ is inert for staticcheck but shares the prefix.)
expect_run "embed catalogs are load-bearing" staticcheck "internal/config/tokencheck/catalog.json"
expect_run "test-file embeds load too" staticcheck "internal/config/tokencheck/testdata/token-vectors.json"
# The analysis owner and the skip owner, each its own row (not dir:tools/:
# doc-checks.sh edits must not wake this job).
expect_run "the analysis script → staticcheck" staticcheck "tools/staticcheck.sh"
expect_run "the filter owns this skip → staticcheck" staticcheck "tools/ci-filter.sh"
expect_run "the workflow → staticcheck" staticcheck ".github/workflows/ci.yml"
expect_skip "web does not touch staticcheck" staticcheck "web/src/routes/+page.svelte"
expect_skip "docs-only / staticcheck (single file)" staticcheck "CHANGELOG.md"

echo
echo "== case 4: fail-open — no diff obtainable means run, always"
: > "$WORK/empty.txt"
printf 'CHANGELOG.md\n' > "$WORK/chg.txt"
export FAKE_GIT_DIFF="$WORK/chg.txt"

# 4a. workflow_dispatch / schedule: no before at all.
CI_ENV=(EVENT_NAME=workflow_dispatch SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "workflow_dispatch → run" run go
ci_mode "workflow_dispatch → run (staticcheck, same ladder)" run staticcheck

# 4b. push with no before (first push) and all-zero before (forced/tag push).
CI_ENV=(EVENT_NAME=push BEFORE= SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "empty before → run" run go
CI_ENV=(EVENT_NAME=push BEFORE=0000000000000000000000000000000000000000 SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "all-zero before → run" run go

# 4c. base unreachable: fetch fails, the diff never runs.
CI_ENV=(EVENT_NAME=push BEFORE=cafe1234 SHA=deadbeef FAKE_GIT_MODE=fetch-fail)
ci_mode "unreachable base → run" run go

# 4d. diff itself fails.
CI_ENV=(EVENT_NAME=push BEFORE=cafe1234 SHA=deadbeef FAKE_GIT_MODE=diff-fail)
ci_mode "diff failed → run" run desktop

# 4e. the ladder is not just decoration: a fetchable base with a docs-only
# diff DOES skip through the real code path.
CI_ENV=(EVENT_NAME=push BEFORE=cafe1234 SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "fetchable base + docs-only diff → skip" skip go

# 4f. and the same ladder runs a Go-touching diff.
printf 'internal/server/sync.go\n' > "$WORK/chg.txt"
CI_ENV=(EVENT_NAME=push BEFORE=cafe1234 SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "fetchable base + .go diff → run" run go

# 4g. pull_request uses BASE_SHA, not BEFORE (a PR whose base is empty —
# e.g. the payload shape changed — fails open).
CI_ENV=(EVENT_NAME=pull_request BASE_SHA= SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "pull_request without BASE_SHA → run" run e2e
printf 'web/src/app.css\n' > "$WORK/chg.txt"
CI_ENV=(EVENT_NAME=pull_request BASE_SHA=cafe1234 SHA=deadbeef FAKE_GIT_MODE=ok)
ci_mode "pull_request BASE_SHA diff → run" run e2e

echo
echo "== guards survival: the incident classes these jobs guard"
# Every guarded incident fired on a code or packaging input, never on
# CHANGELOG/docs. One row per incident, pinned to the input that fired it —
# a table edit that stops catching one of these fails here, not in a
# release audit.
expect_run "GDK-270/1035 race tier" go "internal/server/sync_test.go"
expect_run "GDK-1035 race deal owner" go "tools/race-partition.sh"
expect_run "GDK-1035 e2e deal owner" e2e "tools/e2e-partition.sh"
expect_run "GDK-672/e2e seeds" e2e "examples/demo.db"
expect_run "GDK-868 mobile viewport gate" mobile "mobile/e2e/shell.spec.ts"
expect_run "GDK-1540 mobile built-bundle serve" mobile "mobile/e2e/gate-serve.sh"
expect_run "GDK-208/292 linux pack script" desktop "desktop/build-linux.sh"
expect_run "GDK-167 bundle claims gadak://" desktop "desktop/build-app.sh"
expect_run "GDK-1407 syso pair vs manifest" desktop "desktop/windows-app.manifest"
expect_run "GDK-1380 msix pack" desktop "desktop/msix/AppxManifest.xml"
expect_run "GDK-1036 go.sum cache key" desktop "desktop/go.sum"
expect_run "a workflow rewrite re-runs every tier" go ".github/workflows/ci.yml"
expect_run "  (same, e2e)" e2e ".github/workflows/ci.yml"
expect_run "  (same, mobile)" mobile ".github/workflows/ci.yml"
expect_run "  (same, desktop)" desktop ".github/workflows/ci.yml"
expect_run "  (same, staticcheck)" staticcheck ".github/workflows/ci.yml"
# The filter itself lives under tools/: editing it re-runs every tier, so a
# broken filter never ships hidden behind its own verdict.
expect_run "filter self-edit → go" go "tools/ci-filter.sh"
expect_run "filter self-edit → staticcheck" staticcheck "tools/ci-filter.sh"

echo
echo "== stdout contract: verdict line first, reason second, GITHUB_OUTPUT written"
out="$(printf 'CHANGELOG.md\n' | bash tools/ci-filter.sh desktop --files -)"
if [[ "$(printf '%s\n' "$out" | sed -n '1p')" = run=false &&
      "$(printf '%s\n' "$out" | sed -n '2p')" = "ci-filter: desktop: skip — 1 changed path(s); none touch desktop inputs" ]]; then
  echo "- skip prints run=false then the reason line"
else
  fail "stdout shape changed: $(printf '%s' "$out" | tr '\n' '|')"
fi
GITHUB_OUTPUT="$WORK/ghout" bash tools/ci-filter.sh go --files - <<<"internal/server/x.go" >/dev/null
if grep -qx 'run=true' "$WORK/ghout" 2>/dev/null; then
  echo "- GITHUB_OUTPUT carries run=true"
else
  fail "GITHUB_OUTPUT did not carry run=true: $(cat "$WORK/ghout" 2>/dev/null || echo '(empty)')"
fi

echo
echo "== FAIL-first: the checks go red on bad answers"
# These are the two ways this gate could die silently: matching everything
# (the filter never skips — pure cost) and pattern-shape confusion (a
# top-level file whose NAME merely starts like a directory in the table).
# The expectations above already pin the positive/negative pairs; this
# section pins the nastiest negatives so a regex rewrite cannot loosen them.
expect_skip "webmap.ts is not web/" e2e "webmap.ts"
expect_skip "a nested go.mod that is not the root one" go "docs/go.mod"
expect_skip "a .go suffix inside a filename, not at the end" go "proto.go.txt"
expect_skip "  (same, staticcheck)" staticcheck "proto.go.txt"
expect_skip "gomodulate is not go.mod" desktop "gomodulate.sh"
expect_skip "desktop/gomod.sh is not desktop/go.mod" staticcheck "desktop/gomod.sh"
expect_skip "docs/go.mod is not the root go.mod" staticcheck "docs/go.mod"

echo
echo "== the mobile table covers everything the phone imports out of web/ (GDK-2044)"
# Derived, never enumerated. The mobile subject's web rows rotted once: the
# table named lib/i18n/ and lib/terminal/ while mobile/src imported 24
# modules out of web/src/lib, so a push touching only
# web/src/lib/view-config.ts skipped the Mobile job and CI went green over
# a phone that reads that file. An enumeration is what rotted, so this
# reads the imports off the phone's own source and asks the filter itself.
# Only real import/@import statements — a path a comment merely names is
# not an input (mobile/src cites several web components in prose).
mobile_web_imports() {
  grep -rhE "^[[:space:]]*(import|@import|} from|export .* from)[^\n]*web/src/" mobile/src mobile/e2e 2>/dev/null \
    | grep -oE "web/src/[A-Za-z0-9/_.-]+" \
    | sed "s#/*\$##" | sort -u
}
imported="$(mobile_web_imports)"
n_imports=$(printf '%s\n' "$imported" | grep -c . || true)
if [[ "$n_imports" -lt 8 ]]; then
  fail "only $n_imports web/ imports found in mobile/src (floor 8) — the scan rotted, fix the scan and not the floor"
fi
uncovered=""
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  # Module specifiers drop the extension; resolve to whatever is on disk.
  if [[ ! -f "$f" ]]; then
    if [[ -f "$f.ts" ]]; then f="$f.ts"
    elif [[ -f "$f.svelte" ]]; then f="$f.svelte"
    elif [[ -f "$f/index.ts" ]]; then f="$f/index.ts"
    else continue
    fi
  fi
  if [[ "$(decide mobile "$f")" != run=true ]]; then
    uncovered="$uncovered $f"
  fi
done <<<"$imported"
if [[ -n "$uncovered" ]]; then
  fail "mobile/src imports these out of web/, and the mobile subject skips them —
      a push touching only one would leave the Mobile job unrun:$uncovered"
else
  echo "- all $n_imports web/ paths the phone imports run the mobile subject"
fi

echo
if [[ "$FAILURES" -eq 0 ]]; then
  echo "ci-filter-test: all green"
else
  echo "ci-filter-test: $FAILURES failure(s)"
  exit 1
fi
