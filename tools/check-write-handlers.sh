#!/usr/bin/env bash
# GDK-681: issue write handlers in internal/server/write.go must not call
# s.client() — that mint is origin.Client, Jira-only. A Linear apiKey still
# passes HasCredential, so the 409 gate does not save those handlers.
#
# Catalog GETs may keep s.client(); each remaining caller is named with a
# reason in TestWriteHandlersDoNotCallClient (clientCallAllowlist). Adding a
# write handler that calls s.client() without a reason fails this gate.
#
# Usage: tools/check-write-handlers.sh
# Exit 0 = clean, 1 = a write handler still mints via s.client().
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# -count=1 was dropped in the 2026-09-10 doc-checks cost round (GDK-1488
# follow-through): it forced a full re-run on every local doc-checks pass
# for no second opinion — the go test cache is content-keyed (sources of
# the package and its dependencies, flags included), so a cache hit means
# the identical test already passed on the identical tree. CI is unaffected
# either way: this script runs in the build job before any other `go test`
# touches internal/server, so the cache is always cold there. The
# assertion set and the -run selection are unchanged.
go test ./internal/server -run '^TestWriteHandlersDoNotCallClient$'
