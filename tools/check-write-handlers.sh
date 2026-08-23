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

go test ./internal/server -count=1 -run '^TestWriteHandlersDoNotCallClient$'
