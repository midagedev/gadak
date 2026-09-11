#!/usr/bin/env bash
# The phone gate's two servers, and the identity stamp both of them carry
# (GDK-1540).
#
# Why this file exists. The gate used to serve the app from `vite`, the dev
# server — which watches mobile/ and web/src and reloads the page whenever
# either changes. In a tree where several agents work at once that is not a
# corner case: an edit to a screen mid-run throws every spec in flight back
# to the first tab, and they die on click timeouts that say nothing about the
# cause (GDK-1526 closed the harness half of that; this closes the app-source
# half). A built bundle behind `vite preview` has no watcher and no HMR
# channel, so the bytes a spec is looking at are the bytes that were built
# when the gate started, and nothing on disk can change them mid-run.
#
# The build is `--mode development` with NODE_ENV=development on purpose.
# `import.meta.env.DEV` is load-bearing in this app — lib/api.ts routes
# through the /api proxy in DEV and through tauri-plugin-http otherwise, the
# pairing screens show their dev affordances, and PairingTab renders the
# `viewport-probe` element the gate measures. A production build serves a
# different app than the one the gate is written against (measured: with a
# plain `vite build`, `viewport-probe` is absent from the bundle and the
# demo-tour chunk is not emitted; with NODE_ENV=development both are there).
# Vite derives `DEV` from `process.env.NODE_ENV !== 'production'`
# (node_modules/vite/dist/node/chunks/dep-Dm0c1Wj2.js:49070,49175), and
# `--mode development` alone does not set NODE_ENV for a build — hence both.
#
# The stamp is the same contract e2e/serve.sh + e2e/helpers.ts hold for the
# desktop suite: a server that `reuseExistingServer` adopted has to prove it
# came from this worktree before a single spec runs, because a gate that
# photographs another tree's code is worse than a gate that fails.
#
# Usage (the ports and paths come from mobile/e2e/serve.ts, the single owner;
# nothing here invents a default):
#   gate-serve.sh stamp ui|api        print the identity JSON, nothing else
#   gate-serve.sh ui                  build + stamp + exec vite preview
#   gate-serve.sh api                 build + stamp + exec gadak demo
set -euo pipefail

MOBILE="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$MOBILE/.." && pwd)"

# need <NAME> <value> — the value is passed rather than looked up indirectly,
# so this runs the same under bash 3.2 (still the /bin/bash on macOS) as under
# the 5.x on the CI runner.
need() {
  if [ -z "$2" ]; then
    echo "gate-serve.sh: $1 is not set — mobile/e2e/serve.ts owns it" >&2
    exit 2
  fi
  printf '%s' "$2"
}

# Build inputs per role. Only what actually feeds the artifact: the harness
# (e2e/, shots/, the two Playwright configs, the vitest units) is deliberately
# absent from the ui set, because those files are edited *while the gate runs*
# and a digest that moved under a running gate would fail the very isolation
# this change buys.
digest_paths() {
  case "$1" in
    ui)
      printf '%s\n' \
        mobile/src mobile/public mobile/index.html mobile/vite.config.ts \
        mobile/package.json mobile/tsconfig.json web/src package.json
      ;;
    api)
      printf '%s\n' cmd internal go.mod go.sum examples/demo.db
      ;;
    *)
      echo "gate-serve.sh: unknown role '$1' (want ui|api)" >&2
      exit 2
      ;;
  esac
}

digest_excludes() {
  case "$1" in
    ui)
      printf '%s\n' \
        ':(exclude,glob)mobile/src/**/*.test.ts' \
        ':(exclude,glob)web/src/**/*.test.ts' \
        ':(exclude)mobile/node_modules' \
        ':(exclude)web/node_modules'
      ;;
    api) printf '%s\n' ':(exclude)internal/**/testdata' ;;
  esac
}

# One line of JSON: who built this, from which tree, at which commit, with
# what uncommitted on top. `dirty` is a flag rather than part of the equality
# test on purpose — see serve.ts for which fields the check compares and why.
emit_stamp() {
  local role="$1" built_at="${2:-}" out_dir="${3:-}"
  local paths=() excludes=()
  while IFS= read -r p; do [ -e "$ROOT/$p" ] && paths+=("$p"); done < <(digest_paths "$role")
  while IFS= read -r e; do [ -n "$e" ] && excludes+=("$e"); done < <(digest_excludes "$role")

  # `git -C`, never `cd`: this function is called from the ui/api branches
  # just before they exec a server, and a cd that leaks out of it would hand
  # that server the wrong working directory. It did, once — the first run of
  # this file left `vite preview` in the repo root, where it loaded the *web*
  # app's vite.config.ts and proxied /api to the desktop suite's 7777. Every
  # spec that needed the API timed out and nothing said why.
  local worktree head status diff diff_rc dirty payload hash
  worktree="$(git -C "$ROOT" rev-parse --show-toplevel)"
  head="$(git -C "$ROOT" rev-parse HEAD)"
  status="$(git -C "$ROOT" status --porcelain -uall -- "${paths[@]}" "${excludes[@]}")"
  # git diff exits 1 when the tree differs from HEAD; that is the payload,
  # not a failure. 2+ is a real error.
  set +e
  diff="$(git -C "$ROOT" diff HEAD -- "${paths[@]}" "${excludes[@]}")"
  diff_rc=$?
  set -e
  if [ "$diff_rc" -gt 1 ]; then
    echo "gate-serve.sh: git diff failed (exit ${diff_rc})" >&2
    exit "$diff_rc"
  fi
  if [ -n "$status" ] || [ -n "$diff" ]; then dirty=true; else dirty=false; fi

  payload="$(printf '%s\n%s\n' "$status" "$diff")"
  if command -v shasum >/dev/null 2>&1; then
    hash="$(printf '%s' "$payload" | shasum -a 256 | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    hash="$(printf '%s' "$payload" | sha256sum | awk '{print $1}')"
  else
    echo "gate-serve.sh: need shasum or sha256sum" >&2
    exit 1
  fi

  ROLE="$role" WORKTREE="$worktree" HEAD="$head" DIRTY="$dirty" DIGEST="$hash" \
    BUILT_AT="$built_at" OUT_DIR="$out_dir" node --input-type=module -e '
      import { stdout } from "node:process"
      const e = process.env
      const doc = {
        role: e.ROLE,
        worktree: e.WORKTREE,
        head: e.HEAD,
        dirty: e.DIRTY === "true",
        digest: e.DIGEST,
      }
      // A number, not the env string it arrives as: serve.ts compares it
      // against the Playwright process start time to tell "we built this"
      // from "reuseExistingServer adopted it", and a string silently made
      // every bundle look adopted.
      if (e.BUILT_AT) doc.builtAt = Number(e.BUILT_AT)
      if (e.OUT_DIR) doc.outDir = e.OUT_DIR
      stdout.write(JSON.stringify(doc) + "\n")
    '
}

now_ms() { node -e 'process.stdout.write(String(Date.now()))'; }

case "${1:-}" in
  stamp)
    emit_stamp "${2:-}"
    ;;

  ui)
    PORT="$(need GADAK_MOBILE_E2E_PORT "${GADAK_MOBILE_E2E_PORT:-}")"
    OUT="$(need GADAK_MOBILE_GATE_OUTDIR "${GADAK_MOBILE_GATE_OUTDIR:-}")"
    STAMP_NAME="$(need GADAK_MOBILE_GATE_STAMP_FILE "${GADAK_MOBILE_GATE_STAMP_FILE:-}")"
    cd "$MOBILE"
    echo "[mobile-gate] building the phone bundle into ${OUT}…"
    # --emptyOutDir is required because OUT sits outside the vite root (it is
    # under TMPDIR so that mobile/dist stays exactly what `tauri build` ships
    # — tauri.conf.json frontendDist is "../dist").
    # --config is spelled out rather than left to cwd. The repo root holds a
    # vite.config.ts of its own (the desktop web app, /api → 7777), and vite
    # picks its config from the working directory — so "which config" must
    # not be something a stray cd can answer differently.
    NODE_ENV=development npx vite build --config "$MOBILE/vite.config.ts" \
      --mode development --outDir "$OUT" --emptyOutDir
    emit_stamp ui "$(now_ms)" "$OUT" >"$OUT/$STAMP_NAME"
    echo "[mobile-gate] $(cat "$OUT/$STAMP_NAME")"
    # No watcher, no HMR client: `vite preview` is sirv over the built dir
    # plus the /api proxy, which it inherits from server.proxy
    # (dep-Dm0c1Wj2.js:48365 `proxy: preview?.proxy ?? server.proxy`).
    exec npx vite preview --config "$MOBILE/vite.config.ts" \
      --outDir "$OUT" --port "$PORT" --strictPort --host 127.0.0.1
    ;;

  api)
    PORT="$(need GADAK_MOBILE_API_PORT "${GADAK_MOBILE_API_PORT:-}")"
    STAMP="$(need GADAK_MOBILE_API_STAMP "${GADAK_MOBILE_API_STAMP:-}")"
    BIN="$(need GADAK_MOBILE_API_BIN "${GADAK_MOBILE_API_BIN:-}")"
    cd "$ROOT"
    # GDK-1555: the binary itself carries the digest it was built from, so
    # /healthz can prove it and the gate's globalSetup can compare against
    # this tree over HTTP — same strength as the UI bundle's stamp, with no
    # side file in the trust path. The commit is stamped too: Go's buildvcs
    # declines inside a linked worktree (measured 2026-09-11), and parallel
    # gates are exactly the worktree case. The side stamp below is still
    # written: it is the build record the log line prints and a debugger
    # reads, not the thing the check believes. emit_stamp is deterministic for
    # a given tree, so the values stamped into the binary and the ones
    # re-computed by the check's own expectedStamp agree.
    API_STAMP_JSON="$(emit_stamp api)"
    API_DIGEST="$(printf '%s' "$API_STAMP_JSON" | sed -n 's/.*"digest":"\([^"]*\)".*/\1/p')"
    API_HEAD="$(printf '%s' "$API_STAMP_JSON" | sed -n 's/.*"head":"\([^"]*\)".*/\1/p')"
    if [ -z "$API_DIGEST" ] || [ -z "$API_HEAD" ]; then
      echo "gate-serve.sh: could not read the api digest/head from emit_stamp" >&2
      exit 1
    fi
    echo "[mobile-gate] building ${BIN} (head ${API_HEAD} digest ${API_DIGEST})…"
    CGO_ENABLED=0 go build -ldflags "-X main.buildCommit=${API_HEAD} -X main.buildDigest=${API_DIGEST}" -o "$BIN" ./cmd/gadak
    emit_stamp api "$(now_ms)" "$BIN" >"$STAMP"
    echo "[mobile-gate] $(cat "$STAMP")"
    exec "$BIN" demo --addr "127.0.0.1:${PORT}" --no-open
    ;;

  *)
    echo "usage: gate-serve.sh stamp <ui|api> | ui | api" >&2
    exit 2
    ;;
esac
