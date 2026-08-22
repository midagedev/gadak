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

# GDK-600: a regen is a full rewrite of the published state, so it must run
# on a fresh mirror — a parallel session's label writes landed in Jira but a
# stale local mirror silently dropped them from the public page. Only the
# default oss mirror can be synced here (a custom path has no profile mapping).
if [ "$MIRROR" = "$HOME/.gadak/profiles/oss/gadak.db" ]; then
  bin/gadak --profile oss sync --if-stale 1m
else
  echo "warning: custom mirror path — freshness is the caller's job (no sync run)" >&2
fi

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

# GDK-600: a public entry vanishing from the snapshot is almost always a
# stale-mirror accident, not a deliberate un-publish (that is rare). Refuse
# when the regen dropped detail files git still tracks, unless the caller
# says the drop is intended.
dropped=$(git ls-files "$OUT/detail" | while read -r f; do [ -f "$f" ] || echo "$f"; done)
if [ -n "$dropped" ]; then
  if [ "${BACKLOG_SNAPSHOT_ALLOW_DROPS:-}" = "1" ]; then
    echo "warning: dropping public entries (BACKLOG_SNAPSHOT_ALLOW_DROPS=1):" >&2
    echo "$dropped" >&2
  else
    echo "FAIL: regen would drop public entries the last snapshot carried:" >&2
    echo "$dropped" >&2
    echo "  a stale mirror is the usual cause (the sync above should prevent it);" >&2
    echo "  if the un-publish is intended, re-run with BACKLOG_SNAPSHOT_ALLOW_DROPS=1" >&2
    exit 1
  fi
fi

# Descriptions ship (GDK-430), so the real-name check matters here. The list
# lives outside the repository on purpose — see backlog-scrub-check.sh. Default
# path is the maintainer's; override with BACKLOG_NAME_DENYLIST.
export BACKLOG_NAME_DENYLIST="${BACKLOG_NAME_DENYLIST:-$HOME/.gadak/backlog-name-denylist.txt}"
tools/backlog-scrub-check.sh "$OUT"
# The same scanner CI runs (tokens, tenant hostnames, tailnet names, home
# paths). It walks untracked files too, so the fresh snapshot is in scope —
# a hit fails here instead of on main (a tailnet hostname in a description
# once got past scrub-check and reached CI, 2026-08-21).
bash scripts/scan-internal.sh
echo "backlog-snapshot: $OUT ready — review the diff, then commit"
