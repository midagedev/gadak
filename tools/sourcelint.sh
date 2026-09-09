#!/usr/bin/env bash
# The repo-wide AST gate tests are source lints, not unit tests (the audit
# issue that split them out is cited in the tagged gate files themselves).
#
# Seven gates (six measured at the 0.19 audit, ~13s of whole-tree parsing per
# `go test ./...`, plus the linear-endpoint gate added after it) walk every
# non-test .go file through internal/archlint. They live behind the
# `sourcelint` build tag so the default `go test ./...` — what a dev runs for
# one package, and what every CI job runs — never pays that parse. This
# script is their single runner, and it enforces the discipline that keeps
# them there:
#
#   1. every *_test.go that calls archlint.Walk must carry the tag, so a new
#      repo-wide gate cannot quietly re-enter the default set;
#   2. `go test -tags sourcelint ./...` — a strict superset of the untagged
#      run (the tag only adds files), which is why CI's Go-tests step calls
#      this script instead of `go test ./...`.
#
# Arguments are passed through to go test (e.g. `tools/sourcelint.sh -run
# TestX -v`); the discipline check always runs first.
set -euo pipefail
cd "$(dirname "$0")/.."

status=0
while IFS= read -r -d '' f; do
	grep -q 'archlint\.Walk(' "$f" || continue
	if ! grep -q '^//go:build sourcelint' "$f"; then
		echo "untagged repo-wide AST gate: $f" >&2
		echo "  add '//go:build sourcelint' above package — tools/sourcelint.sh runs the tagged set" >&2
		status=1
	fi
# Same skip set as internal/archlint's Walk (dot dirs, vendor, node_modules,
# dist, testdata, scratch, examples) plus desktop/, which is a separate module.
done < <(find . -mindepth 1 -type d \( -name '.*' -o -name vendor -o -name node_modules -o -name dist -o -name testdata -o -name scratch -o -name examples -o -name desktop \) -prune -o -name '*_test.go' -print0)
if [ "$status" -ne 0 ]; then
	exit 1
fi

exec go test -tags sourcelint "$@" ./...
