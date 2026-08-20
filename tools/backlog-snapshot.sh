#!/usr/bin/env bash
# Freeze the maintainer's GDK mirror into the committed public-backlog
# snapshot (GDK-389). Run locally by the lead — CI never holds Jira
# credentials; the published data is exactly what this commit carries.
#
#   tools/backlog-snapshot.sh [mirror.db]
#
# Default mirror: ~/.gadak/profiles/oss/gadak.db. Refresh cadence is manual,
# release-time by default.
set -euo pipefail
cd "$(dirname "$0")/.."

MIRROR="${1:-$HOME/.gadak/profiles/oss/gadak.db}"
OUT="examples/backlog-snapshot"

[ -f "$MIRROR" ] || { echo "mirror not found: $MIRROR" >&2; exit 1; }

go build -trimpath -o bin/gadak ./cmd/gadak
rm -rf "$OUT"
bin/gadak export-static \
  --db "$MIRROR" \
  --projects GDK \
  --require-label public \
  --scrub \
  --keep-description \
  --api-base /gadak/backlog/api/v1/issues/ \
  --auth-base /gadak/backlog/api/v1/auth/ \
  "$OUT"
# The scrub leaves no attachments; drop the empty dir so git has nothing to track.
rmdir "$OUT/attachments" 2>/dev/null || true

# Descriptions ship (GDK-430), so the real-name check matters here. The list
# lives outside the repository on purpose — see backlog-scrub-check.sh. Default
# path is the maintainer's; override with BACKLOG_NAME_DENYLIST.
export BACKLOG_NAME_DENYLIST="${BACKLOG_NAME_DENYLIST:-$HOME/.gadak/backlog-name-denylist.txt}"
tools/backlog-scrub-check.sh "$OUT"
echo "backlog-snapshot: $OUT ready — review the diff, then commit"
