#!/usr/bin/env bash
# Keep desktop/go.mod and desktop/go.sum in lockstep with the root module.
#
# desktop/ is a nested module (replace github.com/midagedev/gadak => ../).
# A new third-party import under the root module updates the root go.sum on
# `go test ./...` / `go mod tidy`, but not desktop/go.sum. Local root gates
# stay green; the three desktop CI jobs then fail on a missing sum (GDK-635).
# tools/doc-checks.sh only compares the issuetap pin string, not the rest of
# the graph.
#
# Usage: tools/check-desktop-tidy.sh
# Exit 0 = tidy is a no-op on desktop/go.mod and desktop/go.sum.
#      1 = those files drifted; run: cd desktop && go mod tidy
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f desktop/go.mod ]]; then
  echo "check-desktop-tidy: desktop/go.mod is missing" >&2
  exit 1
fi

echo "checking desktop/go.mod and desktop/go.sum against go mod tidy — a new third-party import in the root module requires: cd desktop && go mod tidy"

cd desktop
go mod tidy
if git diff --exit-code go.mod go.sum; then
  echo "desktop/go.mod and desktop/go.sum match go mod tidy"
else
  echo "FAIL: desktop/go.mod or desktop/go.sum drifted after go mod tidy. A new third-party import in the root module is not in the desktop nested module. Run: cd desktop && go mod tidy" >&2
  exit 1
fi
