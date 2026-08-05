#!/usr/bin/env bash
# Fail if the repo (or the committed demo snapshot) contains secrets or
# installation-specific strings that must not ship in a public release.
#
# Patterns (T7.4):
#   - Atlassian user/org API token shapes (prefix + long payload)
#   - Concrete *.atlassian.net hosts outside the documentation / test allowlist
#   - An optional deployment-specific word list (see below)
#
# Real-name patterns are intentionally skipped (too many false positives).
#
# The word list is deliberately NOT in this file. Naming the strings you are
# scrubbing publishes them: anyone reading a public scanner learns the very
# vocabulary it exists to keep out. So the words live outside the tree and this
# script only knows how to find them:
#
#   1. $SCRY_SCAN_WORDS     an extended-regex alternation, e.g. 'acme|acme-hub'
#   2. $SCRY_SCAN_WORDLIST  path to a file, one word or regex per line
#   3. .scan-wordlist       same format, repo root, gitignored
#
# With none of those present the word check is skipped and says so — an outside
# contributor has no list and must not be blocked by a gate they cannot satisfy.
# The token and hostname checks always run, for everyone.
#
# Usage: scripts/scan-internal.sh
# Exit 0 = clean, 1 = hits found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

hits_file="$(mktemp)"
text_list="$(mktemp)"
trap 'rm -f "$hits_file" "$text_list"' EXIT

# User API tokens / org API keys from Atlassian.
PAT_TOKEN='ATATT[A-Za-z0-9+/=_-]{20,}|ATCTT[A-Za-z0-9+/=_-]{20,}'
PAT_HOST='atlassian\.net'

# Deployment-specific words, resolved from outside this file (see header).
PAT_COMPANY=""
words_source=""
if [[ -n "${SCRY_SCAN_WORDS:-}" ]]; then
  PAT_COMPANY="$SCRY_SCAN_WORDS"
  words_source="\$SCRY_SCAN_WORDS"
else
  wordlist="${SCRY_SCAN_WORDLIST:-.scan-wordlist}"
  if [[ -f "$wordlist" ]]; then
    # One pattern per line; blank lines and # comments ignored.
    PAT_COMPANY="$(grep -vE '^\s*(#|$)' "$wordlist" | paste -sd '|' -)"
    words_source="$wordlist"
  fi
fi

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

# Tracked files plus untracked-but-not-ignored ones. Scanning only `ls-files`
# used to hide brand new files from the gate: a fixture would pass the scan
# before `git add` and fail CI right after it. Media and the binary snapshot are
# handled separately below.
{
  git ls-files
  git ls-files --others --exclude-standard
} | sort -u | while IFS= read -r f; do
  case "$f" in
    examples/demo.db|*.png|*.jpg|*.jpeg|*.gif|*.webp|*.ico|*.woff|*.woff2|*.ttf|*.eot)
      continue
      ;;
  esac
  [[ -f "$f" ]] || continue
  printf '%s\n' "$f"
done >"$text_list"

file_count="$(wc -l <"$text_list" | tr -d ' ')"
echo "==> scanning ${file_count} tracked and untracked files"
if [[ -n "$PAT_COMPANY" ]]; then
  echo "==> word list from ${words_source}"
else
  echo "==> no word list (set SCRY_SCAN_WORDS or .scan-wordlist) — word check skipped"
fi

if [[ -s "$text_list" ]]; then
  # shellcheck disable=SC2046
  grep -nHE "$PAT_TOKEN" -- $(cat "$text_list") 2>/dev/null >>"$hits_file" || true
  if [[ -n "$PAT_COMPANY" ]]; then
    # shellcheck disable=SC2046
    grep -niHE "$PAT_COMPANY" -- $(cat "$text_list") 2>/dev/null >>"$hits_file" || true
  fi
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
  if [[ -n "$PAT_COMPANY" ]]; then
    grep -niE "$PAT_COMPANY" "$tmp_strings" 2>/dev/null \
        | sed 's|^|examples/demo.db:strings:|' >>"$hits_file" || true
  fi
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
  echo "Remove the strings, or extend the hostname allowlist in scripts/scan-internal.sh"
  echo "if they are intentional documentation."
  exit 1
fi

if [[ -n "$PAT_COMPANY" ]]; then
  echo "OK: no token-shaped secrets, listed words, or non-allowlisted tenant hosts."
else
  echo "OK: no token-shaped secrets or non-allowlisted tenant hosts (word check skipped)."
fi
exit 0
