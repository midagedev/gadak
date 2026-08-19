#!/usr/bin/env bash
# One-line identity of the sources that feed the e2e served artifact:
#   <HEAD> <sha256 of (git status --porcelain -uall + git diff HEAD) over build inputs>
#
# Single owner: e2e/serve.sh writes this line into the port-keyed stamp;
# e2e/helpers.ts reads it back. Do not reimplement the hash in TypeScript.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PATHS=(
  cmd/
  internal/
  web/
  go.mod
  go.sum
  e2e/serve.sh
  examples/demo.db
)
if [ -f package.json ]; then
  PATHS+=(package.json)
fi
# Spec-required exclusions. Both are gitignored already; keep the pathspecs
# so a future un-ignore cannot silently fold build output into the digest.
EXCL=(
  ':(exclude)web/node_modules'
  ':(exclude)web/dist'
)

HEAD="$(git rev-parse HEAD)"
STATUS="$(git status --porcelain -uall -- "${PATHS[@]}" "${EXCL[@]}")"

# git diff exits 1 when the tree differs from HEAD; that is the payload,
# not a failure. Exit 2+ is a real error.
set +e
DIFF="$(git diff HEAD -- "${PATHS[@]}" "${EXCL[@]}")"
diff_rc=$?
set -e
if [ "$diff_rc" -gt 1 ]; then
  echo "served-digest.sh: git diff failed (exit ${diff_rc})" >&2
  exit "$diff_rc"
fi

if command -v sha256sum >/dev/null 2>&1; then
  HASH="$(printf '%s\n%s\n' "$STATUS" "$DIFF" | sha256sum | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  HASH="$(printf '%s\n%s\n' "$STATUS" "$DIFF" | shasum -a 256 | awk '{print $1}')"
else
  echo "served-digest.sh: need sha256sum or shasum" >&2
  exit 1
fi

printf '%s %s\n' "$HEAD" "$HASH"
