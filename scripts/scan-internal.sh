#!/usr/bin/env bash
# Fail if the repo (or the committed demo snapshot) contains secrets or
# installation-specific strings that must not ship in a public release.
#
# Patterns (T7.4):
#   - Atlassian user/org API token shapes (prefix + long payload)
#   - Concrete *.atlassian.net hosts outside the documentation / test allowlist
#   - Tailnet names and CGNAT addresses (operator machines, not product)
#   - Real home directories: /Users/<name>/ or /home/<name>/ outside the
#     placeholder allowlist — an account name, and a path nobody else has
#   - An optional deployment-specific word list (see below)
#
# Real-name patterns are intentionally skipped (too many false positives).
#
# The word list is deliberately NOT in this file. Naming the strings you are
# scrubbing publishes them: anyone reading a public scanner learns the very
# vocabulary it exists to keep out. So the words live outside the tree and this
# script only knows how to find them:
#
#   1. $GADAK_SCAN_WORDS     an extended-regex alternation, e.g. 'acme|acme-hub'
#   2. $GADAK_SCAN_WORDLIST  path to a file, one word or regex per line
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
# Linear personal API key. Kept as its own variable, not folded into
# PAT_TOKEN, because internal/secretscan asserts PAT_TOKEN equals its
# atlassian_api_token regex exactly.
PAT_LINEAR='lin_api_[A-Za-z0-9]{20,}'
PAT_HOST='atlassian\.net'
# Operator machines, not product content. A tailnet MagicDNS name or a
# 100.64/10 CGNAT address identifies a device on someone's private
# network; it is useless to an outsider and it is not ours to publish.
# Added when a runbook for the Omarchy verification VM (docs/runbooks/
# omarchy-vm.md) arrived carrying a tailnet hostname, its IP, and two
# account names. That runbook now reads them from the environment.
PAT_TAILNET='[A-Za-z0-9-]+\.[A-Za-z0-9-]+\.ts\.net|\b100\.(6[4-9]|[7-9][0-9]|1[0-1][0-9]|12[0-7])\.[0-9]{1,3}\.[0-9]{1,3}\b'
# One author's home directory. It carries the account name, and it is also a
# file path nobody else has: a `/Users/<name>/...` output path inside an e2e
# spec passed every local gate and failed CI with ENOENT (GDK-254). Documented
# placeholders are the exception — see is_placeholder_home.
PAT_HOMEPATH='(/Users/|/home/)[A-Za-z0-9._-]+/'

# Deployment-specific words, resolved from outside this file (see header).
PAT_COMPANY=""
words_source=""
if [[ -n "${GADAK_SCAN_WORDS:-}" ]]; then
  PAT_COMPANY="$GADAK_SCAN_WORDS"
  words_source="\$GADAK_SCAN_WORDS"
else
  wordlist="${GADAK_SCAN_WORDLIST:-.scan-wordlist}"
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

# Names a doc or test may stand a home directory up under. Anything else is
# somebody's actual account.
is_placeholder_home() {
  case "$1" in
    you|youruser|user|username|me|x|alice|bob|runner|home) return 0 ;;
    *) return 1 ;;
  esac
}

# Print lines that carry a real home directory, skipping placeholder names.
# Line-oriented matching is why this is a filter and not a bare grep: one line
# can hold both a placeholder and nothing else, and reporting the line for the
# placeholder is the false positive that teaches people to ignore the gate.
filter_real_home_paths() {
  while IFS= read -r line; do
    # The delimiter is # because the pattern contains an alternation: with
    # s|…|…| the | inside (Users|home) closes the expression, sed errors, and
    # the filter silently passes a real hit (measured while writing this).
    users="$(printf '%s\n' "$line" | grep -oE "$PAT_HOMEPATH" | sed -E 's#^/(Users|home)/##; s#/$##' || true)"
    [[ -z "$users" ]] && continue
    while IFS= read -r u; do
      [[ -z "$u" ]] && continue
      if ! is_placeholder_home "$u"; then
        printf '%s\n' "$line"
        break
      fi
    done <<<"$users"
  done
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

# With --dir, scan a built artifact instead of the working tree. Build output is
# gitignored, so the default file list cannot see it — and pages.yml used to
# cover that gap with its own inline `grep -rEn atlassian.net | grep -v
# placeholder`, which is a weaker copy of the check two lines above it in the
# same step. That copy failed on 2026-08-20: it is line-oriented, and one
# published issue body discusses the string "atlassian.net" as a word, so a file
# with no tenant hostname in it at all was reported as a leak. Same class as the
# vacuous filter fixed in tools/backlog-scrub-check.sh the same day. The logic
# belongs in one place, and this is the place that already had it right —
# filter_disallowed_hosts treats a bare domain as product discussion and matches
# hostnames rather than lines.
SCAN_DIR=""
if [[ "${1:-}" == "--dir" ]]; then
  SCAN_DIR="${2:?usage: scan-internal.sh --dir <artifact-dir>}"
  [[ -d "$SCAN_DIR" ]] || { echo "scan-internal: not a directory: $SCAN_DIR" >&2; exit 1; }
fi

# Tracked files plus untracked-but-not-ignored ones. Scanning only `ls-files`
# used to hide brand new files from the gate: a fixture would pass the scan
# before `git add` and fail CI right after it. Media and the binary snapshot are
# handled separately below.
{
  if [[ -n "$SCAN_DIR" ]]; then
    find "$SCAN_DIR" -type f
  else
    git ls-files
    git ls-files --others --exclude-standard
  fi
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
if [[ -n "$SCAN_DIR" ]]; then
  echo "==> scanning ${file_count} files under ${SCAN_DIR}"
else
  echo "==> scanning ${file_count} tracked and untracked files"
fi
if [[ -n "$PAT_COMPANY" ]]; then
  echo "==> word list from ${words_source}"
else
  echo "==> no word list (set GADAK_SCAN_WORDS or .scan-wordlist) — word check skipped"
fi

if [[ -s "$text_list" ]]; then
  # shellcheck disable=SC2046
  grep -nHE "$PAT_TOKEN|$PAT_LINEAR|$PAT_TAILNET" -- $(cat "$text_list") 2>/dev/null >>"$hits_file" || true
  if [[ -n "$PAT_COMPANY" ]]; then
    # shellcheck disable=SC2046
    grep -niHE "$PAT_COMPANY" -- $(cat "$text_list") 2>/dev/null >>"$hits_file" || true
  fi
  # shellcheck disable=SC2046
  grep -niHE "$PAT_HOST" -- $(cat "$text_list") 2>/dev/null \
      | filter_disallowed_hosts >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nHE "$PAT_HOMEPATH" -- $(cat "$text_list") 2>/dev/null \
      | filter_real_home_paths >>"$hits_file" || true
fi

if [[ -z "$SCAN_DIR" && -f examples/demo.db ]]; then
  echo "==> scanning strings in examples/demo.db"
  tmp_strings="$(mktemp)"
  strings examples/demo.db >"$tmp_strings"
  grep -nE "$PAT_TOKEN|$PAT_LINEAR" "$tmp_strings" 2>/dev/null \
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
  echo "Remove the strings. If a hit is intentional documentation, extend the"
  echo "matching allowlist in scripts/scan-internal.sh: is_allowed_host for a"
  echo "tenant hostname, is_placeholder_home for a stand-in home directory"
  echo "(a real /Users/<name>/ path is also a file path no one else has)."
  exit 1
fi

if [[ -n "$PAT_COMPANY" ]]; then
  echo "OK: no token-shaped secrets, listed words, non-allowlisted tenant hosts, or real home paths."
else
  echo "OK: no token-shaped secrets, non-allowlisted tenant hosts, or real home paths (word check skipped)."
fi
exit 0
