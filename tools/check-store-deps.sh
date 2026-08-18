#!/usr/bin/env bash
# Enforce the store/source firewall on a package's transitive import graph.
#
# Default: fail if ./internal/store depends on internal/jira, internal/atlhttp,
# or net/http. The rule lives in docs/ARCHITECTURE.md:79 (Constitution Article 6).
#
# Usage:
#   tools/check-store-deps.sh                 # gate ./internal/store
#   tools/check-store-deps.sh --graph [pkg]   # print module + checked deps
#
# Exit 0 = clean (or graph printed), 1 = forbidden dep, 2 = usage error.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MODULE="github.com/midagedev/gadak"
DEFAULT_PKG="./internal/store"

usage() {
  echo "usage: tools/check-store-deps.sh [--graph [pkg]]" >&2
  exit 2
}

list_deps() {
  go list -f '{{join .Deps "\n"}}' "$1"
}

print_graph() {
  local pkg="$1"
  local deps
  deps="$(list_deps "$pkg")"
  echo "==> $pkg"
  echo "-- module packages --"
  printf '%s\n' "$deps" | grep "^${MODULE}/" | sort || true
  echo "-- checked paths --"
  local hit
  for hit in "${MODULE}/internal/jira" "${MODULE}/internal/atlhttp" "net/http"; do
    if printf '%s\n' "$deps" | grep -Fxq "$hit"; then
      echo "PRESENT  $hit"
    else
      echo "absent   $hit"
    fi
  done
}

# The case arms below match a package and its subpackages but not a sibling
# sharing a prefix: internal/jira must not match internal/jirafields, which is
# exactly the package this firewall's own fix created.
check_pkg() {
  local pkg="$1"
  local deps
  deps="$(list_deps "$pkg")"
  local bad=()
  local seen_jira=0 seen_atlhttp=0 seen_http=0
  local line
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    case "$line" in
      "${MODULE}/internal/jira"|"${MODULE}/internal/jira"/*)
        if [[ "$seen_jira" -eq 0 ]]; then
          bad+=("${MODULE}/internal/jira")
          seen_jira=1
        fi
        ;;
      "${MODULE}/internal/atlhttp"|"${MODULE}/internal/atlhttp"/*)
        if [[ "$seen_atlhttp" -eq 0 ]]; then
          bad+=("${MODULE}/internal/atlhttp")
          seen_atlhttp=1
        fi
        ;;
      net/http|net/http/*)
        if [[ "$seen_http" -eq 0 ]]; then
          bad+=("net/http")
          seen_http=1
        fi
        ;;
    esac
  done <<<"$deps"
  if ((${#bad[@]} > 0)); then
    echo "FAIL: $pkg depends on forbidden packages:" >&2
    printf '  %s\n' "${bad[@]}" >&2
    echo >&2
    echo "docs/ARCHITECTURE.md:79 — internal/store must not import internal/jira (the store is source-neutral; Jira-shaped types belong behind a connector). The same gate also refuses internal/atlhttp and net/http on this graph." >&2
    return 1
  fi
  echo "OK: $pkg does not depend on internal/jira, internal/atlhttp, or net/http"
}

MODE="check"
PKG="$DEFAULT_PKG"

if [[ $# -eq 0 ]]; then
  :
elif [[ "$1" == "--graph" ]]; then
  MODE="graph"
  if [[ $# -eq 1 ]]; then
    PKG="$DEFAULT_PKG"
  elif [[ $# -eq 2 ]]; then
    PKG="$2"
  else
    usage
  fi
elif [[ "$1" == "-h" || "$1" == "--help" ]]; then
  usage
else
  usage
fi

if [[ "$MODE" == "graph" ]]; then
  print_graph "$PKG"
else
  check_pkg "$PKG"
fi
