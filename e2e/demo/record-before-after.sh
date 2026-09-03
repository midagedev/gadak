#!/usr/bin/env bash
# The before/after take → scratch/before-after/read.mp4 (GDK-1381).
#
#   bash e2e/demo/record-before-after.sh [<previous tag>]      # default: the last tag before HEAD's
#
# Two serves of the same demo fixture — the previous release built from a
# detached worktree, and this checkout — and before-after.spec.ts recorded
# once against each. camera.mjs then cuts before-after.shots.mjs: each beat
# opens on the previous release and the new one wipes in.
#
# The pane's shell is a bare /bin/sh with no startup file: the operator's own
# prompt (account, cloud region, hostname) is exactly what MEDIA.md's privacy
# rule keeps out of a frame, and e2e/serve.sh alone would inherit it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
PREV="${1:-$(git describe --tags --abbrev=0 "$(git describe --tags --abbrev=0)^" 2>/dev/null)}"
[[ -n "$PREV" ]] || { echo "record-before-after: no previous tag found; pass one" >&2; exit 1; }

WT="$ROOT/e2e/.tmp/before-after/wt-$PREV"
PORT_BEFORE=7897 PORT_AFTER=7899
OUT="$ROOT/e2e/.tmp/ba"
SCRATCH="$ROOT/scratch/before-after"
mkdir -p "$OUT" "$SCRATCH"

# The previous release, sharing this checkout's node_modules: the lockfile has
# to be identical between the two tags for that, so check rather than assume.
if [[ ! -d "$WT" ]]; then
  git worktree add -q --detach "$WT" "$PREV"
  if git diff --quiet "$PREV" HEAD -- package-lock.json; then
    ln -s "$ROOT/node_modules" "$WT/node_modules"
    [[ -d "$ROOT/web/node_modules" ]] && ln -s "$ROOT/web/node_modules" "$WT/web/node_modules"
  else
    echo "record-before-after: package-lock.json differs from $PREV — run npm ci in $WT first" >&2
    exit 1
  fi
fi

PANE_ENV=(SHELL=/bin/sh ENV=/dev/null PS1='$ ' HISTFILE=/dev/null)
serve_pids=()
stop_serves() {
  for p in "${serve_pids[@]:-}"; do [[ -n "$p" ]] && kill "$p" 2>/dev/null || true; done
  for port in $PORT_BEFORE $PORT_AFTER; do
    lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | xargs -r kill 2>/dev/null || true
  done
}
trap stop_serves EXIT

(cd "$WT" && env "${PANE_ENV[@]}" GADAK_E2E_PORT=$PORT_BEFORE bash e2e/serve.sh >"$OUT/serve-before.log" 2>&1) &
serve_pids+=($!)
env "${PANE_ENV[@]}" GADAK_E2E_PORT=$PORT_AFTER bash e2e/serve.sh >"$OUT/serve-after.log" 2>&1 &
serve_pids+=($!)
for i in $(seq 1 240); do
  curl -sf "http://127.0.0.1:$PORT_BEFORE/healthz" >/dev/null && curl -sf "http://127.0.0.1:$PORT_AFTER/healthz" >/dev/null && break
  sleep 2
done
curl -sf "http://127.0.0.1:$PORT_AFTER/healthz" >/dev/null || { echo "record-before-after: serves never became healthy" >&2; exit 1; }
echo "record-before-after: $PREV on :$PORT_BEFORE, HEAD on :$PORT_AFTER"

for tag in before after; do
  port=$PORT_BEFORE; after=0
  [[ $tag == after ]] && port=$PORT_AFTER && after=1
  rm -rf "$ROOT/e2e/.tmp/test-results-ba-$tag"; rm -f "$OUT/proof-$tag.jsonl"
  GADAK_MEDIA=1 GADAK_BA_BASE="http://127.0.0.1:$port" GADAK_BA_AFTER=$after \
    GADAK_BA_PROOF="$OUT/proof-$tag.jsonl" GADAK_BA_RESULTS="$ROOT/e2e/.tmp/test-results-ba-$tag" \
    ./node_modules/.bin/playwright test --config e2e/demo/before-after.config.ts
  video="$(find "$ROOT/e2e/.tmp/test-results-ba-$tag" -name '*.webm' | head -1)"
  [[ -n "$video" ]] || { echo "record-before-after: no video for $tag" >&2; exit 1; }
  cp "$video" "$SCRATCH/take-$tag.webm"
done
stop_serves

ENDCARD_LINE="the same tracker, one release later." node e2e/demo/endcard.mjs "$SCRATCH/endcard.png" >/dev/null
node e2e/demo/camera.mjs e2e/demo/before-after.shots.mjs --sheet
node e2e/demo/camera.mjs e2e/demo/before-after.shots.mjs
