#!/usr/bin/env bash
# Snapshot GitHub release asset download_count and the repo traffic
# (clones / views) window into one JSON file.
#
# Traffic endpoints only keep a 14-day rolling window — without a periodic
# snapshot the history disappears. That is why this file exists.
#
# Deploy-side only (GitHub's own API, invoked by a workflow or a human).
# It is not product telemetry and does not run inside the gadak binary;
# the product outbound rule (CLAUDE.md / SECURITY.md) is untouched.
#
#   tools/release-stats.sh --out <dir> [--repo <owner/name>] [--stamp <YYYY-MM-DD>]
#
# Writes <dir>/<stamp>.json (UTC). Same-day reruns overwrite the file so
# the history does not accumulate duplicate rows for one date.
#
# Exit 64 = usage / bad arguments
#      69 = gh or jq is not installed
#       1 = every endpoint failed (no file written — an empty snapshot is worse)
#       0 = at least one endpoint produced data
#
# Requires: gh, jq.
set -euo pipefail

usage() {
  echo "usage: tools/release-stats.sh --out <dir> [--repo <owner/name>] [--stamp <YYYY-MM-DD>]" >&2
  exit 64
}

die_usage() {
  echo "release-stats: $*" >&2
  usage
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,24p' "$0"
  exit 0
fi

OUT=""
REPO="midagedev/gadak"
STAMP=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      [[ $# -ge 2 ]] || die_usage "--out needs a directory"
      OUT="$2"
      shift 2
      ;;
    --repo)
      [[ $# -ge 2 ]] || die_usage "--repo needs owner/name"
      REPO="$2"
      shift 2
      ;;
    --stamp)
      [[ $# -ge 2 ]] || die_usage "--stamp needs YYYY-MM-DD"
      STAMP="$2"
      shift 2
      ;;
    -h|--help)
      sed -n '2,24p' "$0"
      exit 0
      ;;
    *)
      die_usage "unknown argument: $1"
      ;;
  esac
done

[[ -n "$OUT" ]] || die_usage "missing --out <dir>"

# Conservative --repo: owner/name only. Rejects shell metacharacters so the
# value can be interpolated into a gh path without quoting surprises.
[[ "$REPO" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]] || \
  die_usage "--repo must be owner/name"

if [[ -n "$STAMP" ]]; then
  [[ "$STAMP" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || \
    die_usage "--stamp must be YYYY-MM-DD"
else
  STAMP="$(date -u +%Y-%m-%d)"
fi

# --out: allowlist of path characters, no `..` components, no system prefixes.
# `../../etc` is the motivating case; quoting alone would still mkdir it.
if [[ "$OUT" == *$'\n'* || "$OUT" == *$'\r'* ]]; then
  die_usage "--out must not contain a newline"
fi
if [[ ! "$OUT" =~ ^[A-Za-z0-9._/+\ -]+$ ]]; then
  die_usage "--out contains a rejected character"
fi
_rest="$OUT"
while [[ "$_rest" == */* ]]; do
  _part="${_rest%%/*}"
  _rest="${_rest#*/}"
  if [[ "$_part" == ".." ]]; then
    die_usage "--out must not contain .."
  fi
done
if [[ "$_rest" == ".." ]]; then
  die_usage "--out must not contain .."
fi
unset _rest _part

if [[ "$OUT" != /* ]]; then
  OUT="$(pwd)/$OUT"
fi
case "$OUT" in
  /etc|/etc/*|/usr|/usr/*|/bin|/bin/*|/sbin|/sbin/*|/System|/System/*|/private/etc|/private/etc/*|/root|/root/*)
    die_usage "--out refuses a system directory"
    ;;
esac

if [[ -e "$OUT" && ! -d "$OUT" ]]; then
  die_usage "--out is not a directory: $OUT"
fi
mkdir -p "$OUT" || { echo "release-stats: cannot create $OUT" >&2; exit 1; }

command -v gh >/dev/null || {
  echo "release-stats: gh is not installed — see https://cli.github.com" >&2
  exit 69
}
command -v jq >/dev/null || {
  echo "release-stats: jq is not installed — see https://jqlang.github.io/jq/" >&2
  exit 69
}

# gh pretty-prints JSON with ANSI when it thinks it has a TTY (this harness
# is a PTY even when stdout is a file). Strip SGR before handing bytes to jq.
decolor() {
  sed $'s/\x1b\\[[0-9;]*[A-Za-z]//g'
}

http_status() {
  # gh stderr looks like: "gh: Not Found (HTTP 404)" or "gh: HTTP 403: …"
  local text
  text="$(tr '\n' ' ' <"$1")"
  if [[ "$text" =~ HTTP\ ([0-9]{3}) ]]; then
    echo "${BASH_REMATCH[1]}"
  else
    echo "error"
  fi
}

err_message() {
  local line=""
  IFS= read -r line <"$1" || true
  line="${line#gh: }"
  echo "${line:0:200}"
}

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/release-stats.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

ERRORS='[]'
add_error() {
  local endpoint="$1" status="$2" message="$3"
  ERRORS="$(jq -c -n \
    --argjson cur "$ERRORS" \
    --arg endpoint "$endpoint" \
    --arg status "$status" \
    --arg message "$message" \
    '$cur + [{endpoint: $endpoint, status: $status, message: $message}]')"
}

CAPTURED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OK=0

# --- releases (paginated) -------------------------------------------------
# --slurp cannot be combined with --jq (gh 2.92). Slurp pages, then jq.
REL_ENDPOINT="repos/${REPO}/releases"
if gh api --paginate --slurp "$REL_ENDPOINT" \
     >"$WORKDIR/releases.raw" 2>"$WORKDIR/releases.err"; then
  decolor <"$WORKDIR/releases.raw" >"$WORKDIR/releases.decolor"
  if jq -e 'type == "array"' "$WORKDIR/releases.decolor" >/dev/null 2>&1; then
    jq '
      if length == 0 then []
      elif (.[0] | type) == "array" then (add // [])
      else .
      end
    ' "$WORKDIR/releases.decolor" >"$WORKDIR/releases.flat"
    if jq -e 'type == "array"' "$WORKDIR/releases.flat" >/dev/null 2>&1; then
      jq '
        map(select(type == "object"))
        | map(
            ((.assets // []) | map(select(type == "object"))) as $assets
            | {
                tag: (.tag_name // ""),
                published_at: (.published_at // null),
                assets: ($assets | map({
                  name: (.name // ""),
                  download_count: (
                    if has("download_count") and (.download_count | type) == "number"
                    then .download_count
                    else 0
                    end
                  )
                })),
                missing: ($assets | map(select(has("download_count") | not)) | length)
              }
          )
        | {
            releases: map({
              tag,
              published_at,
              assets,
              downloads_total: ([.assets[].download_count] | add // 0)
            }),
            missing: ([.[].missing] | add // 0)
          }
      ' "$WORKDIR/releases.flat" >"$WORKDIR/releases.mapped"
      OK=$((OK + 1))
      missing="$(jq -r '.missing' "$WORKDIR/releases.mapped")"
      if [[ "$missing" -gt 0 ]]; then
        add_error "releases" "missing_download_count" \
          "${missing} asset(s) had no download_count; recorded as 0"
      fi
      jq '.releases' "$WORKDIR/releases.mapped" >"$WORKDIR/releases.json"
    else
      add_error "releases" "invalid_json" "paginated body was not a JSON array after flatten"
      echo '[]' >"$WORKDIR/releases.json"
    fi
  else
    add_error "releases" "invalid_json" "response was not a JSON array"
    echo '[]' >"$WORKDIR/releases.json"
  fi
else
  add_error "releases" "$(http_status "$WORKDIR/releases.err")" \
    "$(err_message "$WORKDIR/releases.err")"
  echo '[]' >"$WORKDIR/releases.json"
fi

# --- traffic/clones -------------------------------------------------------
CLONES_ENDPOINT="repos/${REPO}/traffic/clones"
if gh api "$CLONES_ENDPOINT" \
     >"$WORKDIR/clones.raw" 2>"$WORKDIR/clones.err"; then
  decolor <"$WORKDIR/clones.raw" >"$WORKDIR/clones.decolor"
  if jq -e 'type == "object"' "$WORKDIR/clones.decolor" >/dev/null 2>&1; then
    jq '{
      count: (.count // 0),
      uniques: (.uniques // 0),
      days: (.clones // [])
    }' "$WORKDIR/clones.decolor" >"$WORKDIR/clones.json"
    OK=$((OK + 1))
  else
    add_error "traffic/clones" "invalid_json" "response was not a JSON object"
    echo '{"count":0,"uniques":0,"days":[]}' >"$WORKDIR/clones.json"
  fi
else
  add_error "traffic/clones" "$(http_status "$WORKDIR/clones.err")" \
    "$(err_message "$WORKDIR/clones.err")"
  echo '{"count":0,"uniques":0,"days":[]}' >"$WORKDIR/clones.json"
fi

# --- traffic/views --------------------------------------------------------
VIEWS_ENDPOINT="repos/${REPO}/traffic/views"
if gh api "$VIEWS_ENDPOINT" \
     >"$WORKDIR/views.raw" 2>"$WORKDIR/views.err"; then
  decolor <"$WORKDIR/views.raw" >"$WORKDIR/views.decolor"
  if jq -e 'type == "object"' "$WORKDIR/views.decolor" >/dev/null 2>&1; then
    jq '{
      count: (.count // 0),
      uniques: (.uniques // 0),
      days: (.views // [])
    }' "$WORKDIR/views.decolor" >"$WORKDIR/views.json"
    OK=$((OK + 1))
  else
    add_error "traffic/views" "invalid_json" "response was not a JSON object"
    echo '{"count":0,"uniques":0,"days":[]}' >"$WORKDIR/views.json"
  fi
else
  add_error "traffic/views" "$(http_status "$WORKDIR/views.err")" \
    "$(err_message "$WORKDIR/views.err")"
  echo '{"count":0,"uniques":0,"days":[]}' >"$WORKDIR/views.json"
fi

if [[ "$OK" -eq 0 ]]; then
  echo "release-stats: every endpoint failed; not writing $OUT/$STAMP.json" >&2
  echo "release-stats: errors=$ERRORS" >&2
  exit 1
fi

OUTFILE="$OUT/$STAMP.json"
jq -n \
  --arg stamp "$STAMP" \
  --arg captured_at "$CAPTURED_AT" \
  --arg repo "$REPO" \
  --slurpfile releases "$WORKDIR/releases.json" \
  --slurpfile clones "$WORKDIR/clones.json" \
  --slurpfile views "$WORKDIR/views.json" \
  --argjson errors "$ERRORS" \
  '{
    stamp: $stamp,
    captured_at: $captured_at,
    repo: $repo,
    releases: $releases[0],
    downloads_total: ([($releases[0] // [])[].downloads_total] | add // 0),
    traffic: {
      clones: $clones[0],
      views: $views[0]
    },
    errors: $errors
  }' >"$WORKDIR/snapshot.json"

mv "$WORKDIR/snapshot.json" "$OUTFILE"
echo "$OUTFILE"
