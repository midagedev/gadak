#!/usr/bin/env bash
# Freeze the maintainer's GDK mirror into the committed public-backlog
# snapshot (GDK-389). Run locally by the lead — CI never holds Jira
# credentials; the published data is exactly what this commit carries.
#
# The git-tracked form is one ustar.gz. Pages and the hosted viewer still
# see an exploded tree (bootstrap.json + detail/<KEY>.json);
# tools/hosted-demo/build.mjs and pages.yml unpack the archive at build.
#
#   tools/backlog-snapshot.sh [mirror.db]
#   tools/backlog-snapshot.sh --unpack <dest> [archive]
#   tools/backlog-snapshot.sh --pack-from <exported-dir>
#
# Default mirror: ~/.gadak/profiles/gdk/gadak.db — the paired workspace whose
# origin is the self-hosted tracker (GDK-1262). Refresh cadence is manual,
# release-time by default.
#
# --pack-from packs an already-exported snapshot directory without talking
# to a mirror. Conversion of the exploded tree, and pack/unpack checks,
# use it so this script is testable without ~/.gadak.
set -euo pipefail
cd "$(dirname "$0")/.."

ARCHIVE="examples/backlog-snapshot.tar.gz"
OUT="examples/backlog-snapshot"

# One-line debug header inside the archive. `tar -xOf "$ARCHIVE" MANIFEST`
# prints it — no `tar -tf` (613 names) and no jq.
write_manifest() {
  local src="$1"
  local count keys_line
  count="$(find "$src/detail" -name 'GDK-*.json' | wc -l | tr -d ' ')"
  keys_line="$(
    find "$src/detail" -name 'GDK-*.json' -print \
      | sed -n 's|.*/\(GDK-[0-9][0-9]*\)\.json$|\1|p' \
      | sort -t- -k2,2n \
      | paste -sd, -
  )"
  printf 'generated_at=%s issue_count=%s keys=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$count" "$keys_line" >"$src/MANIFEST"
}

# ustar+gzip of exact file bytes. COPYFILE_DISABLE keeps macOS AppleDouble
# members out; --format ustar is what GNU tar on Pages extracts.
pack_backlog_snapshot() {
  local src="$1"
  local dest="$2"
  local tmp
  [ -f "$src/bootstrap.json" ] || { echo "missing $src/bootstrap.json" >&2; exit 1; }
  [ -f "$src/config.json" ] || { echo "missing $src/config.json" >&2; exit 1; }
  [ -d "$src/detail" ] || { echo "missing $src/detail" >&2; exit 1; }
  write_manifest "$src"
  tmp="${dest}.tmp"
  rm -f "$tmp"
  (
    cd "$src"
    COPYFILE_DISABLE=1 tar --format ustar -czf - \
      MANIFEST bootstrap.json config.json \
      $(find detail -name 'GDK-*.json' | sort)
  ) >"$tmp"
  mv "$tmp" "$dest"
  rm -f "$src/MANIFEST"
}

unpack_backlog_snapshot() {
  local dest="${1:?}"
  local archive="${2:-$ARCHIVE}"
  [ -f "$archive" ] || { echo "missing $archive" >&2; exit 1; }
  mkdir -p "$dest"
  tar -xzf "$archive" -C "$dest"
}

if [[ "${1:-}" == "--unpack" ]]; then
  dest="${2:?usage: tools/backlog-snapshot.sh --unpack <dest> [archive]}"
  archive="${3:-$ARCHIVE}"
  unpack_backlog_snapshot "$dest" "$archive"
  n="$(find "$dest/detail" -name 'GDK-*.json' | wc -l | tr -d ' ')"
  echo "backlog-snapshot: unpacked $n issues → $dest"
  exit 0
fi

if [[ "${1:-}" == "--pack-from" ]]; then
  src="${2:?usage: tools/backlog-snapshot.sh --pack-from <exported-dir>}"
  [ -d "$src" ] || { echo "not a directory: $src" >&2; exit 1; }
  export BACKLOG_NAME_DENYLIST="${BACKLOG_NAME_DENYLIST:-$HOME/.gadak/backlog-name-denylist.txt}"
  tools/backlog-scrub-check.sh "$src"
  bash scripts/scan-internal.sh --dir "$src"
  pack_backlog_snapshot "$src" "$ARCHIVE"
  rm -rf "$OUT"
  echo "backlog-snapshot: $ARCHIVE ready — review the diff, then commit"
  tar -xOf "$ARCHIVE" MANIFEST
  exit 0
fi

# GDK-1262: the backlog's origin is the self-hosted tracker on the paired
# home serve, not Jira. The default mirror is the paired workspace's.
MIRROR="${1:-$HOME/.gadak/profiles/gdk/gadak.db}"

[ -f "$MIRROR" ] || { echo "mirror not found: $MIRROR" >&2; exit 1; }

go build -trimpath -o bin/gadak ./cmd/gadak

# GDK-600: a regen is a full rewrite of the published state, so it must run
# on a fresh mirror — a parallel session's label writes landed in Jira but a
# stale local mirror silently dropped them from the public page. Only the
# default mirror can be synced here (a custom path has no profile mapping).
if [ "$MIRROR" = "$HOME/.gadak/profiles/gdk/gadak.db" ]; then
  bin/gadak --workspace gdk sync --if-stale 1m
else
  echo "warning: custom mirror path — freshness is the caller's job (no sync run)" >&2
fi

STAGE="$(mktemp -d "${TMPDIR:-/tmp}/gadak-backlog-XXXXXX")"
cleanup_stage() { rm -rf "$STAGE"; }
trap cleanup_stage EXIT

bin/gadak export-static \
  --db "$MIRROR" \
  --projects GDK \
  --require-label public \
  --scrub \
  --keep-description \
  --api-base /gadak/backlog/api/v1/issues/ \
  --auth-base /gadak/backlog/api/v1/auth/ \
  "$STAGE"
# The scrub leaves no attachments; drop the empty dir so the archive has nothing to carry.
rmdir "$STAGE/attachments" 2>/dev/null || true

# GDK-600: a public entry vanishing from the snapshot is almost always a
# stale-mirror accident, not a deliberate un-publish (that is rare). Refuse
# when the regen dropped detail files the last snapshot carried, unless the
# caller says the drop is intended.
old_keys=""
if [ -f "$ARCHIVE" ]; then
  old_keys="$(tar -tzf "$ARCHIVE" | sed -n 's|^detail/\(GDK-[0-9][0-9]*\)\.json$|\1|p' | sort)"
elif [ -d "$OUT/detail" ]; then
  old_keys="$(find "$OUT/detail" -name 'GDK-*.json' -print | sed -n 's|.*/\(GDK-[0-9][0-9]*\)\.json$|\1|p' | sort)"
fi
new_keys="$(find "$STAGE/detail" -name 'GDK-*.json' -print | sed -n 's|.*/\(GDK-[0-9][0-9]*\)\.json$|\1|p' | sort)"
dropped="$(comm -23 <(printf '%s\n' "$old_keys") <(printf '%s\n' "$new_keys") | sed '/^$/d')"
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
tools/backlog-scrub-check.sh "$STAGE"
# Plaintext scan of the export, then the rest of the repo. The committed
# form is gzip, so a repo-only scan would not see these bytes; pages.yml
# also scans dist/hosted after unpack.
bash scripts/scan-internal.sh --dir "$STAGE"
bash scripts/scan-internal.sh

pack_backlog_snapshot "$STAGE" "$ARCHIVE"
rm -rf "$OUT"
echo "backlog-snapshot: $ARCHIVE ready — review the diff, then commit"
tar -xOf "$ARCHIVE" MANIFEST
