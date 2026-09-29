#!/usr/bin/env bash
# Does this push change a CI job's build inputs? (GDK-1912)
#
# The staticcheck job's inline gofilter (ci.yml) proved the shape: most pushes
# touch no Go at all, yet every unfiltered tier re-ran — the census measured
# 31 of 127 commits in one cycle touching only CHANGELOG*/CLAUDE.md/docs/specs/
# artifacts/contrib, each billing ~3,201 s of race + browser + mobile +
# desktop compute. This script is that filter, generalized: name a subject,
# get run/skip.
#
# It fails OPEN, and that is the contract (same words as the gofilter): a
# forced push whose before is unreachable, a first push (all-zero before),
# workflow_dispatch, an empty event base, or any diff error runs the job. A
# gate skipped by mistake is the failure mode that matters; a gate run for
# nothing is minutes.
#
# Subject inputs are what the job actually checks out and builds, read off
# .github/workflows/ci.yml — not a guess. Each row cites its evidence, and
# the first skip that proved a row too wide is baked into it (GDK-1931, run
# 35025858519: a desktop/-only push — 8 files of a separate module plus the
# rebase-regenerated examples/backlog-snapshot.tar.gz — ran race ×3, e2e ×3
# and mobile):
#
#   go      the race tier runs `go test ./internal/server/` (sharded) +
#           `./internal/workspace/` from the root module (imports reach the
#           whole tree — GDK-270's leaked goroutine came from an import,
#           not an edit under internal/server): **/*.go, go.mod, go.sum;
#           internal/ (testdata fixtures and the //go:embed json catalogs
#           in internal/config/tokencheck/); tools/ (race-partition.sh owns
#           the deal, this filter owns the skip — a change here can rewrite
#           the gate itself); scripts/; .github/workflows/. NOT desktop/
#           (own go.mod — neither race package compiles it). NOT examples/
#           (grep -rn 'examples/' over the two packages' .go: zero reads;
#           the tests that open examples/demo.db live in internal/store,
#           internal/snapshot, cmd/gadak … and run in the always-on build
#           job, not this filtered tier).
#
#   e2e     e2e/serve.sh — the webServer playwright waits on — builds the
#           gadak binary (`go build ./cmd/gadak`) AND `npm run build` before
#           seeding: **/*.go, go.mod, go.sum, web/, e2e/, package.json,
#           package-lock.json, .nvmrc (setup-node), tools/ (e2e-partition.sh
#           + this filter), .github/workflows/. The examples/ input is the
#           exact set serve.sh and the CI specs open, not the directory:
#           examples/demo.db (SEED_DB) + examples/demo-linear.db (the
#           second serve, e2e/linear.spec.ts; both also in
#           e2e/served-digest.sh's stamp), examples/demo-source.db
#           (pr-links.spec.ts), examples/demo-i18n/ (helpers.ts
#           mediaFixture), examples/attachments/ (serve.sh
#           --import-attachments — the console-hygiene spec reads these
#           bytes). NOT desktop/ (own go.mod — serve.sh builds the root
#           module only). NOT examples/backlog-snapshot.tar.gz or the rest
#           of examples/ (dashboards/, compose/, plugins/ are read by demo
#           recordings and Go tests outside this filtered tier).
#
#   mobile  mobile/ (own src, e2e, package-lock.json, tauri) plus the web
#           slices it imports — web/src/lib/ WHOLE and web/src/app.css
#           (GDK-2044). It was two slices (lib/i18n/, lib/terminal/) until a
#           count showed the phone importing 24 modules out of that
#           directory — adf, api, issue-group, view-config, keyboard,
#           person-match and the rest — so a push touching only
#           web/src/lib/view-config.ts skipped the Mobile job while the
#           phone read that file. That is the one direction this filter
#           must never fail in: CI green over a broken phone, the inverse
#           of the local-green/CI-red class. The directory row is
#           deliberate over a 24-line enumeration, which is what rotted the
#           first time; ci-filter-test.sh derives the import set from
#           mobile/src and fails if any of it falls outside this table, so
#           narrowing the row later is red rather than silent. The root
#           package.json/package-lock.json (Playwright lives at the root);
#           docs/media/logo.png (check-brand-icons.sh diffs the phone icons
#           against the mark); internal/pairing/testdata/ (offer-vectors.json
#           is half of a contract whose other half is Go); examples/demo.db
#           (gate-serve.sh's api digest names it — the `gadak demo` it
#           builds seeds from it); **/*.go, go.mod, go.sum (gate-serve.sh
#           builds and runs `gadak demo`); tools/ (check-brand-icons.sh +
#           this filter); .nvmrc; .github/workflows/. NOT desktop/ (own
#           go.mod — gate-serve.sh builds the root module only).
#
#   desktop desktop/ (own module, Info.plist, windows-app.manifest, msix/,
#           pack scripts) plus the root module every pack script compiles —
#           build-app.sh / build-linux.sh / build-windows.ps1 each build
#           ./cmd/gadak so dist/app stays embedded: **/*.go (the root
#           module's — desktop/'s own .go is already covered by
#           dir:desktop/ above), go.mod, go.sum, web/, package.json,
#           package-lock.json; tools/ (check-desktop-tidy.sh,
#           windows-manifest.sh, this filter); .nvmrc;
#           .github/workflows/. No desktop/ carve-out here: this subject
#           IS the one that builds it.
#
#   staticcheck  tools/staticcheck.sh analyses BOTH modules over the GOOS
#           matrix (`run_module root . ./cmd/... ./internal/... ./tools/...`
#           + `run_module desktop desktop ./...`, GOOS darwin/linux/windows):
#           **/*.go with NO desktop/ carve-out — unlike subject `go`, the
#           desktop module IS an analysis input here, so desktop/main.go
#           must run this subject (the run 35025858519 push that subject
#           `go` rightly skips must run this one); go.mod, go.sum,
#           desktop/go.mod, desktop/go.sum (the two module graphs staticcheck
#           resolves). internal/ stays a whole-directory row for one
#           MEASURED reason, not the race tier's: the //go:embed files
#           there are load-bearing — `go list -e` on a package whose embed
#           target is deleted answers `pattern data.json: no matching
#           files found`, Incomplete=true (verified with a scratch module),
#           and staticcheck.sh's classifier fails the run on a load error.
#           Content is never analysed (staticcheck type-checks, never
#           executes); existence is. The measured set: catalog.json,
#           dim-catalog.json (internal/config/tokencheck/), and
#           testdata/token-vectors.json — tokencheck_test.go embeds it and
#           staticcheck loads _test.go files; other internal/ testdata is
#           inert but shares the prefix. NOT web/, package.json, .nvmrc
#           (no Node anywhere in this job), NOT examples/, contrib/ or the
#           root package embed.go (outside the ./cmd/... ./internal/...
#           ./tools/... patterns — a stray suffix:.go match on those four
#           files is cost, never a wrong skip). tools/staticcheck.sh (the
#           analysis owner) and tools/ci-filter.sh (the skip owner — the
#           same pairing as subject go's race-partition.sh row: a change
#           here can rewrite the gate itself) as exact rows, NOT dir:tools/
#           — doc-checks.sh and the e2e/race partitioners are not this
#           job's inputs and must not wake it. .github/workflows/ (the job
#           and the pinned staticcheck version live there).
#
# Visibility: a filter that never says anything is its own defect, so every
# decision — run or skip — prints one line naming the subject, the verdict,
# and why (how many paths changed, which pattern matched). Those lines are
# what the run ledger reads.
#
# Usage:
#   tools/ci-filter.sh <subject>              CI mode: env EVENT_NAME/BEFORE/
#                                             BASE_SHA/SHA, git fetch+diff
#   tools/ci-filter.sh <subject> --files <f>  decide from a path list (one
#                                             per line; '-' = stdin) — the
#                                             test harness's way in
#
# Output: `run=true|false` on stdout (and into $GITHUB_OUTPUT when set),
#         plus the reason line. Exit 0 for both verdicts, 2 = usage error.
#
# Bash 3.2 on purpose (still the /bin/bash on macOS), same as the scripts
# beside it: no mapfile, no associative arrays.
set -euo pipefail

SELF="ci-filter"

usage() {
  cat >&2 <<'EOF'
usage:
  tools/ci-filter.sh <subject>              subject: go | e2e | mobile | desktop | staticcheck
  tools/ci-filter.sh <subject> --files <f>  decide from a path list ('-' = stdin)
EOF
  exit 2
}

# ── the input tables ────────────────────────────────────────────────────────
# One pattern per line; a changed path matches a subject when it matches any
# line. Four shapes, and only these:
#   dir:<d>/    directory prefix — everything under it
#   suffix:<s>  filename suffix (extension-class matches)
#   exact:<f>   one file (matched against the whole path)
#   not:<d>/    carve-out — no path under this prefix is an input of this
#               subject, however else it would match. Written FIRST in a
#               table so the carve-out reads before what it carves: the
#               reason is always "this prefix belongs to a build the subject
#               never invokes" (desktop/ is its own go.mod — the one carve-
#               out in the tables below).
subject_patterns() { # $1 = subject; unknown subject → stderr + exit 2
  case "$1" in
    go)
      cat <<'EOF'
not:desktop/
dir:internal/
dir:tools/
dir:scripts/
dir:.github/workflows/
suffix:.go
exact:go.mod
exact:go.sum
EOF
      ;;
    e2e)
      cat <<'EOF'
not:desktop/
dir:web/
dir:e2e/
exact:examples/demo.db
exact:examples/demo-linear.db
exact:examples/demo-source.db
dir:examples/demo-i18n/
dir:examples/attachments/
dir:tools/
dir:.github/workflows/
suffix:.go
exact:go.mod
exact:go.sum
exact:package.json
exact:package-lock.json
exact:.nvmrc
EOF
      ;;
    mobile)
      cat <<'EOF'
not:desktop/
dir:mobile/
dir:web/src/lib/
exact:web/src/app.css
exact:docs/media/logo.png
dir:internal/pairing/testdata/
exact:examples/demo.db
dir:tools/
dir:.github/workflows/
suffix:.go
exact:go.mod
exact:go.sum
exact:package.json
exact:package-lock.json
exact:.nvmrc
EOF
      ;;
    desktop)
      cat <<'EOF'
dir:desktop/
dir:web/
dir:tools/
dir:.github/workflows/
suffix:.go
exact:go.mod
exact:go.sum
exact:package.json
exact:package-lock.json
exact:.nvmrc
EOF
      ;;
    staticcheck)
      cat <<'EOF'
suffix:.go
exact:go.mod
exact:go.sum
exact:desktop/go.mod
exact:desktop/go.sum
dir:internal/
exact:tools/staticcheck.sh
exact:tools/ci-filter.sh
dir:.github/workflows/
EOF
      ;;
    *)
      echo "$SELF: unknown subject '$1' (go | e2e | mobile | desktop | staticcheck)" >&2
      exit 2
      ;;
  esac
}

# Does one changed path match one pattern? [[ == ]] is a glob match, and the
# trailing '/' in a dir: pattern keeps dir:web/ from matching a top-level
# file named web-thing. Runs once per (path, pattern) and diff lists are
# short, so no subshell per path. not: lines never match here — decide()
# collects them as carve-outs before any path is tested.
pat_match() { # $1 = pattern, $2 = path
  case "$1" in
    'dir:'*) [[ "$2" == "${1#dir:}"* ]] ;;
    'suffix:'*) [[ "$2" == *"${1#suffix:}" ]] ;;
    'exact:'*) [[ "$2" == "${1#exact:}" ]] ;;
    *) return 1 ;;
  esac
}

verdict() { # $1 = run|skip, $2 = reason; $subject comes from the caller
  local out
  if [[ "$1" = run ]]; then out=true; else out=false; fi
  echo "run=$out"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf 'run=%s\n' "$out" >> "$GITHUB_OUTPUT"
  fi
  echo "$SELF: $subject: $1 — $2"
}

# ── the verdict from a path list ────────────────────────────────────────────
# Changed paths on stdin, one per line. Never fails: an empty list is a
# decision (skip — nothing changed), and the CI-mode callers have already
# failed open on every error path before getting here.
decide() { # $1 = subject
  local subject="$1" path p n=0 hit="" first_hit=""
  local pats xcl ex excluded
  pats=""
  pats="$(subject_patterns "$subject")"
  # not: lines are carve-outs, collected in one pass over the table: every
  # prefix they name leaves this subject's input set however else it would
  # match (a suffix:.go under desktop/ is another module's .go). A path
  # under a carve-out still counts in n — it changed — it just cannot be
  # the hit.
  xcl=""
  while IFS= read -r p; do
    [[ -z "$p" ]] && continue
    if [[ "$p" == 'not:'* ]]; then
      xcl="${xcl}${p#not:}
"
    fi
  done <<<"$pats"
  while IFS= read -r path; do
    [[ -z "$path" ]] && continue
    n=$((n + 1))
    excluded=0
    while IFS= read -r ex; do
      [[ -z "$ex" ]] && continue
      if [[ "$path" == "$ex"* ]]; then
        excluded=1
        break
      fi
    done <<<"$xcl"
    [[ "$excluded" = 1 ]] && continue
    if [[ -z "$first_hit" ]]; then
      while IFS= read -r p; do
        [[ -z "$p" ]] && continue
        if pat_match "$p" "$path"; then
          first_hit="$p matched $path"
          break
        fi
      done <<<"$pats"
    fi
  done
  if [[ -n "$first_hit" ]]; then
    verdict run "$n changed path(s); $first_hit"
  else
    verdict skip "$n changed path(s); none touch $subject inputs"
  fi
}

# ── argument parsing ────────────────────────────────────────────────────────
[[ $# -ge 1 ]] || usage
subject="$1"
shift
[[ "$subject" != -* ]] || usage
subject_patterns "$subject" >/dev/null # validates the subject (exits 2 if unknown)

files_from=""
if [[ $# -gt 0 ]]; then
  [[ "$1" = "--files" && $# -eq 2 ]] || usage
  files_from="$2"
fi

# ── mode: explicit path list (the test harness) ─────────────────────────────
if [[ -n "$files_from" ]]; then
  if [[ "$files_from" = "-" ]]; then
    decide "$subject"
  else
    [[ -f "$files_from" ]] || {
      echo "$SELF: no such file: $files_from" >&2
      exit 2
    }
    decide "$subject" < "$files_from"
  fi
  exit 0
fi

# ── mode: CI — the same base resolution and fail-open ladder as the gofilter
EVENT_NAME="${EVENT_NAME:-}"
BEFORE="${BEFORE:-}"
BASE_SHA="${BASE_SHA:-}"
SHA="${SHA:-}"

case "$EVENT_NAME" in
  pull_request) base="$BASE_SHA" ;;
  push) base="$BEFORE" ;;
  *)
    verdict run "event '$EVENT_NAME' has no before — fail open"
    exit 0
    ;;
esac
if [[ -z "$base" ]] || printf '%s' "$base" | grep -Eq '^0+$'; then
  verdict run "no usable base sha (first push, forced push, or tag push) — fail open"
  exit 0
fi
if ! git fetch --no-tags --depth=1 origin "$base" 2>/dev/null; then
  verdict run "base $base not fetchable — fail open"
  exit 0
fi
if ! changed="$(git diff --name-only "$base" "$SHA")"; then
  verdict run "git diff failed — fail open"
  exit 0
fi
decide "$subject" <<<"$changed"
