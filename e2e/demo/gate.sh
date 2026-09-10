#!/usr/bin/env bash
# GDK-1349: the rot gate for e2e/demo specs.
#
# The demo specs are GADAK_MEDIA=1-gated and the CI Playwright job runs only
# e2e/*.spec.ts, so a UI or fixture change that breaks a walkthrough surfaces
# at the next recording take — months later, mid-recording, when the fix is
# expensive. This script runs the same configs the make media-* targets run,
# with the same env, but WITHOUT the export step (no ffmpeg, no docs/media
# writes): it is a green/red gate over the specs themselves, cheap enough for
# a weekly cadence or a pre-recording check.
#
#   bash e2e/demo/gate.sh              # core configs (the make media set)
#   bash e2e/demo/gate.sh --with-scale # + the 20k scale take (minutes)
#
# Port: GADAK_E2E_PORT flows through e2eServePort() to every config — set it
# when another worktree owns the default. Not parallel with anything sharing
# that port; the run is serial by design (one serve per config at a time).
#
# Excluded on purpose, with the reason that keeps them out:
#   scale      — 20k snapshot + slowest take; --with-scale opts in
#   mcp        — live Claude Code login + vhs (make media-mcp)
#   roundtrip / before-after / claude-drive / promo splits — record-*.sh owns
#     the serves and the marks; a bare config run is not the artifact they test
set -euo pipefail
cd "$(dirname "$0")/../.."

CONFIGS=(playwright search sprint retro sprint-retro-hero agent groupby history terminal)
if [[ "${1:-}" == "--with-scale" ]]; then
  CONFIGS+=(scale)
elif [[ -n "${1:-}" ]]; then
  echo "usage: bash e2e/demo/gate.sh [--with-scale]" >&2
  exit 2
fi

if [[ ! -x node_modules/.bin/playwright ]]; then
  echo "gate: node_modules/.bin/playwright missing — npm ci first" >&2
  exit 1
fi
command -v go >/dev/null || { echo "gate: go required (serve.sh builds the binary)" >&2; exit 1; }

# The four fixture-backed configs seed exactly what the make targets seed at
# locale en (MEDIA_FIXTURE_DB == examples/demo.db); the others default to the
# same path inside e2e/serve.sh. Stated here so a locale take can override by
# exporting GADAK_SEED_DB itself.
SEED_DB="$(pwd)/examples/demo.db"

# Config name → its outputDir (each config declares its own so exports cannot
# pick up another take's video.webm). Cleared before the run like the make
# targets do, and removed after — the videos are the export step's input, and
# this gate never exports.
results_dir() {
  case "$1" in
    playwright) echo e2e/demo/test-results ;;
    *) echo "e2e/demo/test-results-$1" ;;
  esac
}

failed=()
echo "gate: ${#CONFIGS[@]} config(s) on GADAK_E2E_PORT=${GADAK_E2E_PORT:-7877}"
for cfg in "${CONFIGS[@]}"; do
  out="$(results_dir "$cfg")"
  rm -rf "$out"
  echo
  echo "── $cfg ($(date +%H:%M:%S)) ──────────────────────────────────"
  # run[] is never empty, so "${run[@]}" is safe under set -u on every bash
  # this repo's contributors run: 3.2 (macOS /bin/bash) treats an EMPTY
  # array's [@] expansion as unbound — the first gate run died exactly there.
  run=(env GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config "e2e/demo/$cfg.config.ts")
  case "$cfg" in
    search | sprint | retro | sprint-retro-hero | scale)
      run=(env GADAK_MEDIA=1 GADAK_SEED_DB="$SEED_DB" \
        ./node_modules/.bin/playwright test --config "e2e/demo/$cfg.config.ts")
      ;;
  esac
  if ! "${run[@]}"; then
    failed+=("$cfg")
    echo "gate: $cfg FAILED — keeping $out for the report"
    continue
  fi
  rm -rf "$out"
done

echo
if [[ ${#failed[@]} -eq 0 ]]; then
  echo "gate: OK — ${#CONFIGS[@]} demo config(s) green"
  exit 0
fi
echo "gate: FAILED config(s): ${failed[*]}"
echo "gate: rerun a single one with:"
for cfg in "${failed[@]}"; do
  echo "  GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/$cfg.config.ts"
done
exit 1
