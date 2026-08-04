#!/usr/bin/env bash
# Fail if the repo (or the committed demo snapshot) contains secrets or
# installation-specific strings that must not ship in a public release.
#
# Patterns (T7.4):
#   - Atlassian user/org API token shapes (prefix + long payload)
#   - A former company name that must not appear in the public tree
#   - Concrete *.atlassian.net hosts outside the documentation / test allowlist
#
# Real-name patterns are intentionally skipped (too many false positives).
#
# Usage: scripts/scan-internal.sh
# Exit 0 = clean, 1 = hits found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

hits_file="$(mktemp)"
text_list="$(mktemp)"
trap 'rm -f "$hits_file" "$text_list"' EXIT

# Patterns assembled so this script does not match itself.
# User API tokens / org API keys from Atlassian.
PAT_TOKEN='ATATT[A-Za-z0-9+/=_-]{20,}|ATCTT[A-Za-z0-9+/=_-]{20,}'
# Former company name and internal product names (split to avoid a self-hit on
# this file). `d`+`hub` also catches the hyphenated form via the optional dash.
PAT_COMPANY='imago''works|d''-?hub'
PAT_HOST='atlassian\.net'

# Allowlisted documentation / fixture hostnames.
is_allowed_host() {
  local host="$1"
  case "$host" in
    your-site.atlassian.net|your-team.atlassian.net|example.atlassian.net|x.atlassian.net)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# Print lines that mention a concrete disallowed <sub>.atlassian.net host.
filter_disallowed_hosts() {
  while IFS= read -r line; do
    hosts="$(printf '%s\n' "$line" | grep -oE '[A-Za-z0-9][A-Za-z0-9.-]*\.atlassian\.net' || true)"
    if [[ -z "$hosts" ]]; then
      # Bare product discussion of the domain, not a tenant hostname.
      continue
    fi
    while IFS= read -r host; do
      [[ -z "$host" ]] && continue
      case "$host" in
        '<'*|site.atlassian.net) continue ;;
      esac
      if ! is_allowed_host "$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')"; then
        printf '%s\n' "$line"
        break
      fi
    done <<<"$hosts"
  done
}

# Tracked text files (skip binary snapshot and media). Skip this scanner so
# pattern documentation cannot trip the gate.
git ls-files | while IFS= read -r f; do
  case "$f" in
    scripts/scan-internal.sh|examples/demo.db|*.png|*.jpg|*.jpeg|*.gif|*.webp|*.ico|*.woff|*.woff2|*.ttf|*.eot)
      continue
      ;;
  esac
  printf '%s\n' "$f"
done >"$text_list"

file_count="$(wc -l <"$text_list" | tr -d ' ')"
echo "==> scanning ${file_count} tracked files"

if [[ -s "$text_list" ]]; then
  # shellcheck disable=SC2046
  grep -nHE "$PAT_TOKEN" -- $(cat "$text_list") 2>/dev/null >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -niHE "$PAT_COMPANY" -- $(cat "$text_list") 2>/dev/null >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -niHE "$PAT_HOST" -- $(cat "$text_list") 2>/dev/null \
      | filter_disallowed_hosts >>"$hits_file" || true
fi

if [[ -f examples/demo.db ]]; then
  echo "==> scanning strings in examples/demo.db"
  tmp_strings="$(mktemp)"
  strings examples/demo.db >"$tmp_strings"
  grep -nE "$PAT_TOKEN" "$tmp_strings" 2>/dev/null \
      | sed 's|^|examples/demo.db:strings:|' >>"$hits_file" || true
  grep -niE "$PAT_COMPANY" "$tmp_strings" 2>/dev/null \
      | sed 's|^|examples/demo.db:strings:|' >>"$hits_file" || true
  grep -niE "$PAT_HOST" "$tmp_strings" 2>/dev/null \
      | filter_disallowed_hosts \
      | sed 's|^|examples/demo.db:strings:|' >>"$hits_file" || true
  rm -f "$tmp_strings"
fi

if [[ -s "$hits_file" ]]; then
  echo ""
  echo "FAILED: secret / internal-string scan found hits:"
  sort -u "$hits_file"
  echo ""
  echo "Remove the strings or extend the allowlist in scripts/scan-internal.sh if they are intentional documentation."
  exit 1
fi

echo "OK: no token-shaped secrets, forbidden company strings, or non-allowlisted tenant hosts."
exit 0
