#!/usr/bin/env bash
# Unattended flagship take: VHS (Claude Code) + Playwright (serve tab) →
# docs/media/claude-drive.{mp4,gif}.
#
# Live model. Requires vhs, ffmpeg, Playwright chromium, and a Claude Code
# login. Not part of `make media` (same reason as media-mcp).
#
# Usage: bash e2e/demo/record-claude-drive.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if [[ -z "${SKIP_NVM:-}" ]]; then
  export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
  if [[ -s "$NVM_DIR/nvm.sh" ]]; then
    # shellcheck disable=SC1090
    . "$NVM_DIR/nvm.sh"
    nvm use
  fi
fi

command -v ffmpeg >/dev/null || { echo "record-claude-drive: ffmpeg required" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "record-claude-drive: ffprobe required" >&2; exit 1; }
command -v vhs >/dev/null || { echo "record-claude-drive: vhs required (brew install vhs)" >&2; exit 1; }
command -v claude >/dev/null || { echo "record-claude-drive: claude CLI required" >&2; exit 1; }
if [[ ! -x node_modules/.bin/playwright ]]; then
  echo "record-claude-drive: playwright missing (npm ci)" >&2
  exit 1
fi

PORT="${GADAK_E2E_PORT:-7889}"
if ! [[ "$PORT" =~ ^[1-9][0-9]*$ ]] || [ "$PORT" -gt 65535 ]; then
  echo "record-claude-drive: GADAK_E2E_PORT must be 1-65535, got ${PORT}" >&2
  exit 1
fi
export GADAK_E2E_PORT="$PORT"

DRIVE_ROOT="/private/tmp/gadak-claude-drive"
GADAK_HOME_DIR="$DRIVE_ROOT/gadak-home"
BIN="$DRIVE_ROOT/bin/gadak"
OUT="$ROOT/e2e/.tmp/claude-drive"
TAKE_LOG="$OUT/takes.jsonl"
MAX_TAKES="${CLAUDE_DRIVE_MAX_TAKES:-4}"
TAKE_START="${CLAUDE_DRIVE_TAKE_START:-1}"

mkdir -p "$OUT"
if [[ -z "${CLAUDE_DRIVE_KEEP_LOG:-}" ]]; then
  : >"$TAKE_LOG"
fi

if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "record-claude-drive: port ${PORT} is already listening — pick another GADAK_E2E_PORT" >&2
  lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >&2 || true
  exit 1
fi

echo "record-claude-drive: GADAK_E2E_PORT=${PORT} node=$(node -v) claude=$(command -v claude)"

free_port() {
  local pids
  pids="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    echo "record-claude-drive: freeing :${PORT} ($pids)"
    # shellcheck disable=SC2086
    kill $pids 2>/dev/null || true
    sleep 1
  fi
}

start_serve() {
  echo "record-claude-drive: serving $GADAK_HOME_DIR on 127.0.0.1:${PORT}"
  GADAK_HOME="$GADAK_HOME_DIR" "$BIN" serve \
    --addr "127.0.0.1:${PORT}" --static "$ROOT/dist/app" --no-open --no-sync \
    >"$OUT/serve.log" 2>&1 &
  echo $! >"$OUT/serve.pid"
  local i
  for i in $(seq 1 50); do
    if curl -sf "http://127.0.0.1:${PORT}/healthz" >/dev/null; then
      echo "record-claude-drive: healthz ok"
      return 0
    fi
    sleep 0.2
  done
  echo "record-claude-drive: serve did not become healthy" >&2
  cat "$OUT/serve.log" >&2 || true
  return 1
}

stop_serve() {
  if [[ -f "$OUT/serve.pid" ]]; then
    kill "$(cat "$OUT/serve.pid")" 2>/dev/null || true
    rm -f "$OUT/serve.pid"
  fi
  free_port
}

cleanup() {
  if [[ -n "${WEB_PID:-}" ]]; then
    touch "$OUT/web-stop"
    sleep 1
    kill "$WEB_PID" 2>/dev/null || true
  fi
  stop_serve
}
trap cleanup EXIT

validate_take() {
  local take="$1"
  local reason=""
  local tokens colors list html_has_chart web_accent web_label web_dash color_sets

  tokens="$(GADAK_HOME="$GADAK_HOME_DIR" "$BIN" config get ui.tokens --json 2>/dev/null || true)"
  colors="$(GADAK_HOME="$GADAK_HOME_DIR" "$BIN" config get ui.dataColors --json 2>/dev/null || true)"
  list="$(GADAK_HOME="$GADAK_HOME_DIR" "$BIN" dashboards list 2>/dev/null || true)"

  color_sets=0
  if printf '%s' "$tokens" | grep -Eq '"accent"|#[0-9a-fA-F]{3,8}'; then
    color_sets=$((color_sets + 1))
  fi
  if printf '%s' "$colors" | grep -Eq '"label"|"type"|"status"|#[0-9a-fA-F]{3,8}'; then
    color_sets=$((color_sets + 1))
  fi

  html_has_chart=0
  if [[ -f "$GADAK_HOME_DIR/local.db" ]]; then
    if sqlite3 "$GADAK_HOME_DIR/local.db" "select config from dashboards;" 2>/dev/null | grep -Ei 'uplot|<canvas' >/dev/null; then
      html_has_chart=1
    fi
  fi

  web_accent=0
  web_label=0
  web_dash=0
  if [[ -f "$OUT/web-timeline.jsonl" ]]; then
    grep -q '"accent_changed"' "$OUT/web-timeline.jsonl" && web_accent=1 || true
    grep -q '"label_changed"' "$OUT/web-timeline.jsonl" && web_label=1 || true
    grep -q '"dashboard_open"' "$OUT/web-timeline.jsonl" && web_dash=1 || true
  fi

  local web_color_events=$((web_accent + web_label))
  if [[ "$color_sets" -lt 2 && "$web_color_events" -lt 2 ]]; then
    reason="colour changes < 2 (config_sets=${color_sets} web_events=${web_color_events})"
  elif ! printf '%s' "$list" | grep -Eq '[[:alnum:]]'; then
    reason="no dashboard saved (dashboards list empty)"
  elif [[ "$html_has_chart" -eq 0 ]]; then
    reason="saved dashboard HTML has no uPlot/canvas chart"
  elif [[ "$web_dash" -eq 0 ]]; then
    reason="serve tab never opened a dashboard frame"
  fi

  python3 -c '
import json, sys
rec = {
    "take": int(sys.argv[1]),
    "ok": sys.argv[2] == "",
    "reason": sys.argv[2] or None,
    "tokens": sys.argv[3][:500],
    "dataColors": sys.argv[4][:500],
    "dashboards_list": sys.argv[5][:500],
    "web_accent": sys.argv[6] == "1",
    "web_label": sys.argv[7] == "1",
    "web_dash": sys.argv[8] == "1",
    "html_has_chart": sys.argv[9] == "1",
    "color_sets": int(sys.argv[10]),
}
open(sys.argv[11], "a").write(json.dumps(rec) + "\n")
print(json.dumps(rec, indent=2))
' "$take" "$reason" "$tokens" "$colors" "$list" "$web_accent" "$web_label" "$web_dash" "$html_has_chart" "$color_sets" "$TAKE_LOG"
  if [[ -n "$reason" ]]; then
    echo "record-claude-drive: take ${take} FAIL: $reason" >&2
    return 1
  fi
  echo "record-claude-drive: take ${take} PASS"
  return 0
}

echo "record-claude-drive: preparing capture home + skill"
bash "$ROOT/tools/tapes/prepare-claude-drive.sh"

if [[ ! -x "$BIN" ]]; then
  echo "record-claude-drive: missing $BIN" >&2
  exit 1
fi

# Login/plan preflight — abort immediately, no workaround (spec).
if ! HOME="$DRIVE_ROOT/agent" claude --version >/dev/null 2>&1; then
  echo "record-claude-drive: claude --version failed in isolated HOME (login/plan?)" >&2
  exit 1
fi

chosen=""
take="$TAKE_START"
TAKE_END=$((TAKE_START + MAX_TAKES - 1))
while [[ "$take" -le "$TAKE_END" ]]; do
  echo "record-claude-drive: ===== take ${take} (start=${TAKE_START} end=${TAKE_END}) ====="
  rm -rf "$ROOT/e2e/.tmp/test-results-claude-drive"
  rm -f "$OUT/web-stop" "$OUT/web-ready-epoch" "$OUT/vhs-show-epoch" "$OUT/term.mp4"
  mkdir -p "$OUT"
  : >"$OUT/web-timeline.jsonl"

  if [[ "$take" -gt 1 ]]; then
    echo "record-claude-drive: reseeding for take ${take}"
    bash "$ROOT/tools/tapes/prepare-claude-drive.sh"
  fi

  start_serve

  echo "record-claude-drive: starting Playwright web recorder"
  GADAK_MEDIA=1 GADAK_E2E_PORT="$PORT" \
    ./node_modules/.bin/playwright test --config e2e/demo/claude-drive.config.ts \
    >"$OUT/web-playwright.log" 2>&1 &
  WEB_PID=$!

  ready=0
  for i in $(seq 1 90); do
    if [[ -f "$OUT/web-ready-epoch" ]]; then
      ready=1
      break
    fi
    if ! kill -0 "$WEB_PID" 2>/dev/null; then
      echo "record-claude-drive: playwright exited before ready" >&2
      cat "$OUT/web-playwright.log" >&2 || true
      break
    fi
    sleep 0.5
  done
  if [[ "$ready" -ne 1 ]]; then
    echo "record-claude-drive: web never became ready (take ${take})" >&2
    python3 -c "import json; open('$TAKE_LOG','a').write(json.dumps({'take':$take,'ok':False,'reason':'web never ready'})+'\n')"
    touch "$OUT/web-stop" || true
    kill "$WEB_PID" 2>/dev/null || true
    wait "$WEB_PID" 2>/dev/null || true
    WEB_PID=""
    stop_serve
    take=$((take + 1))
    continue
  fi
  echo "record-claude-drive: web ready epoch=$(cat "$OUT/web-ready-epoch")"

  echo "record-claude-drive: recording VHS tape (live Claude Code)…"
  date +%s.%N >"$OUT/vhs-process-epoch"
  set +e
  vhs "$ROOT/tools/tapes/claude-drive.tape" >"$OUT/vhs.log" 2>&1
  vhs_st=$?
  set -e
  echo "record-claude-drive: vhs exit=$vhs_st"
  if [[ -f "$DRIVE_ROOT/vhs-show-epoch" ]]; then
    cp -f "$DRIVE_ROOT/vhs-show-epoch" "$OUT/vhs-show-epoch"
  fi
  if [[ "$vhs_st" -ne 0 ]]; then
    echo "record-claude-drive: vhs failed (take ${take})" >&2
    tail -n 80 "$OUT/vhs.log" >&2 || true
    if grep -Ei 'not logged in|unauthorized|login|subscription|out of extra usage|credit' "$OUT/vhs.log" >/dev/null 2>&1; then
      echo "record-claude-drive: claude login/plan failure — aborting (no workaround)" >&2
      python3 -c "import json; open('$TAKE_LOG','a').write(json.dumps({'take':$take,'ok':False,'reason':'claude login/plan failure; see vhs.log'})+'\n')"
      exit 1
    fi
  fi

  touch "$OUT/web-stop"
  echo "record-claude-drive: waiting for playwright to finish"
  wait "$WEB_PID" || true
  WEB_PID=""
  stop_serve

  mkdir -p "$OUT/take-${take}"
  cp -f "$OUT/term.mp4" "$OUT/take-${take}/term.mp4" 2>/dev/null || true
  cp -f "$OUT/web-timeline.jsonl" "$OUT/take-${take}/web-timeline.jsonl" 2>/dev/null || true
  cp -f "$OUT/vhs.log" "$OUT/take-${take}/vhs.log" 2>/dev/null || true
  find "$ROOT/e2e/.tmp/test-results-claude-drive" -name 'video.webm' -exec cp {} "$OUT/take-${take}/web.webm" \; 2>/dev/null || true

  if validate_take "$take"; then
    chosen="$take"
    break
  fi
  take=$((take + 1))
done

if [[ -z "$chosen" ]]; then
  echo "record-claude-drive: all ${MAX_TAKES} takes failed — see $TAKE_LOG" >&2
  cat "$TAKE_LOG" >&2
  exit 1
fi

echo "record-claude-drive: using take ${chosen}"
echo "$chosen" >"$OUT/chosen-take"
# Point export at the chosen take's sources.
if [[ -f "$OUT/take-${chosen}/term.mp4" ]]; then
  cp -f "$OUT/take-${chosen}/term.mp4" "$OUT/term.mp4"
fi
if [[ -f "$OUT/take-${chosen}/web.webm" ]]; then
  cp -f "$OUT/take-${chosen}/web.webm" "$OUT/web.webm"
fi
if [[ -f "$OUT/take-${chosen}/web-timeline.jsonl" ]]; then
  cp -f "$OUT/take-${chosen}/web-timeline.jsonl" "$OUT/web-timeline.jsonl"
fi

bash "$ROOT/e2e/demo/export-claude-drive.sh"

echo "record-claude-drive: done (take ${chosen})"
