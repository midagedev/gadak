#!/usr/bin/env bash
# Static server for the hosted-demo Playwright checks.
#
# Serves the built site (dist/hosted) on 127.0.0.1:4173. The site is served at
# the apex of gadak.dev, so dist/hosted IS the document root: / is the landing
# page, /demo/ the app, /backlog/ the backlog viewer (GDK-676). It used to be
# staged under dist/pages/gadak/ for the /gadak/ base path.
#
# This lives in a file rather than inline in playwright.config.ts for one
# measured reason: Playwright hands `webServer.command` to `/bin/sh`, which is
# bash on macOS and dash on the CI runner. The inline version opened with
# `set -euo pipefail` and died on the runner with "Illegal option -o pipefail"
# — green locally, red in CI. A `#!/usr/bin/env bash` file invoked as
# `bash e2e/hosted/serve.sh` names its own interpreter, the same way
# e2e/serve.sh already does for the CI suite.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

test -f "$ROOT/dist/hosted/index.html" || {
  echo "run make hosted-demo first" >&2
  exit 1
}

exec npx --yes serve "$ROOT/dist/hosted" -l 4173 --no-port-switching
