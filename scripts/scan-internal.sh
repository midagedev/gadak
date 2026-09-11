#!/usr/bin/env bash
# Fail if the repo (or the committed demo snapshot) contains secrets or
# installation-specific strings that must not ship in a public release.
#
# Patterns:
#   - Atlassian user/org API token shapes (prefix + long payload)
#   - Linear personal API keys
#   - Slack tokens, GitHub tokens, and HTTP Basic / Bearer Authorization headers
#   - Concrete *.atlassian.net hosts outside the documentation / test allowlist
#   - Tailnet names and CGNAT addresses (operator machines, not product)
#   - Real home directories: /Users/<name>/ or /home/<name>/ outside the
#     placeholder allowlist — an account name, and a path nobody else has
#   - An optional deployment-specific word list (see below)
#
# Real-name patterns are intentionally skipped (too many false positives).
#
# The credential shapes above are the table in internal/secretscan (Patterns()),
# which is the single owner: that package is what every outbound artifact is
# checked against, and this script is the same table pointed at the repository.
# The original scope (T7.4) was narrower — only the Atlassian and Linear shapes
# were grepped over the tree, so a real ghp_ token committed into a fixture left
# this gate green. GDK-1110 closed that: slack_token, github_token,
# http_basic_auth and http_bearer_token now run over the tree too.
#
# private_key_pem is scanned too, since GDK-1797 — every shape in that table
# now runs over the tree. It is the one shape whose coverage needed per-file
# exemption machinery: the repo carries legitimate test-vector files whose
# whole purpose is to contain a PEM header. Those live in
# internal/secretscan/pem-exemptions.txt, where every entry is an exact
# repo-relative path plus the reason that file may carry the shape, and where
# a stale entry (the file no longer matches) is a failure that tells the
# author to delete it. That file is the single home of the list — this script
# and the coverage test in internal/secretscan both read it, neither carries
# a copy — and internal/secretscan/secretscan_test.go pins the coverage
# decision so it cannot drift.
#
# Three of the four added shapes cannot be byte-identical to their Go regexes:
# POSIX ERE has no (?i), no (?:...) and no portable \b or \s. Those are spelled
# here in ERE and the agreement is asserted behaviourally instead — the Go test
# runs this script over fixtures carrying each shape. Only patterns whose Go
# spelling is already valid ERE (Atlassian, Linear, Slack, PEM) are byte-pinned.
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
pem_out="$(mktemp)"
trap 'rm -f "$hits_file" "$text_list" "$pem_out"' EXIT

# User API tokens / org API keys from Atlassian.
# The Go owner of these credential shapes is internal/secretscan (Patterns()).
PAT_TOKEN='ATATT[A-Za-z0-9+/=_-]{20,}|ATCTT[A-Za-z0-9+/=_-]{20,}'
# Linear personal API key. Kept as its own variable, not folded into
# PAT_TOKEN, because internal/secretscan asserts PAT_TOKEN equals its
# atlassian_api_token regex exactly.
PAT_LINEAR='lin_api_[A-Za-z0-9]{20,}'
# Slack tokens. Byte-identical to internal/secretscan's slack_token regex
# (valid ERE as written), and pinned to it by the agreement test.
PAT_SLACK='xox[baprs]-[A-Za-z0-9-]{10,}'
# GitHub tokens. The Go regex uses \b and (?:...), neither of which is portable
# POSIX ERE, so the word boundary is spelled as an explicit leading class.
# Behaviour, not bytes, is what the Go test asserts for this one.
PAT_GITHUB='(^|[^A-Za-z0-9_])(ghp_|gho_|github_pat_)[A-Za-z0-9_]{20,}'
# HTTP Authorization headers. The Go regexes are (?i) and use \s; these are
# matched with grep -i and [[:space:]] instead.
PAT_BASIC='Authorization:[[:space:]]*Basic[[:space:]]+[A-Za-z0-9+/=_-]{8,}'
PAT_BEARER='Authorization:[[:space:]]*Bearer[[:space:]]+[A-Za-z0-9._-]{20,}'
PAT_HOST='atlassian\.net'
# Operator machines, not product content. A tailnet MagicDNS name or a
# 100.64/10 CGNAT address identifies a device on someone's private
# network; it is useless to an outsider and it is not ours to publish.
# Added when a runbook for the Omarchy verification VM (docs/runbooks/
# omarchy-vm.md) arrived carrying a tailnet hostname, its IP, and two
# account names. That runbook now reads them from the environment.
#
# Split in two (2026-08-24) so the MagicDNS half can carry the same
# placeholder exemption the other host checks already have: the pairing
# golden vectors document an offer endpoint, and a documented stand-in is
# not somebody's device. The CGNAT half keeps no exemption — an address in
# 100.64/10 is always a real allocation.
PAT_TAILNET_HOST='[A-Za-z0-9-]+\.[A-Za-z0-9-]+\.ts\.net'
PAT_TAILNET_CGNAT='\b100\.(6[4-9]|[7-9][0-9]|1[0-1][0-9]|12[0-7])\.[0-9]{1,3}\.[0-9]{1,3}\b'
PAT_TAILNET="$PAT_TAILNET_HOST|$PAT_TAILNET_CGNAT"
# One author's home directory. It carries the account name, and it is also a
# file path nobody else has: a `/Users/<name>/...` output path inside an e2e
# spec passed every local gate and failed CI with ENOENT (GDK-254). Documented
# placeholders are the exception — see is_placeholder_home.
PAT_HOMEPATH='(/Users/|/home/)[A-Za-z0-9._-]+/'
# PEM private-key headers. Byte-identical to internal/secretscan's
# private_key_pem regex (valid ERE as written) and pinned to it by the
# agreement test. Unlike every other pattern it has legitimate hits in the
# tree — test vectors — so it runs through the per-file exemption list
# parsed below instead of grepping bare.
PAT_PEM='-----BEGIN [A-Z ]*PRIVATE KEY-----'

# The exemption list for the PEM shape: which files may carry a private-key
# header, and why. One entry per line, '<path>|<reason>', '#' comments
# ignored; the path is an exact repo-relative file, never a prefix (a prefix
# would exempt files that do not exist yet). The single home of the list is
# internal/secretscan/pem-exemptions.txt: this script reads it here, the
# coverage test in internal/secretscan reads the same file, and neither side
# keeps a copy. Every entry must carry a non-blank reason — an undocumented
# exemption is how the list grows to everything — and an entry whose file no
# longer matches the shape is stale: the scan fails and says to delete the
# entry (the NO_PALETTE_ROW precedent, web/src/lib/palette-coverage.test.ts).
# The file must exist: scanning this shape without its exemption list would
# silently mean "exempt nothing" or "exempt everything", and neither reading
# is a gate anybody asked for.
PEM_EXEMPTIONS='internal/secretscan/pem-exemptions.txt'
pem_exempt_paths=''
if [[ -f "$PEM_EXEMPTIONS" ]]; then
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in ''|'#'*) continue ;; esac
    path="${line%%|*}"
    reason="${line#*|}"
    # The '*' test comes first: with no '|' in the line both expansions above
    # return the whole line, so a separator-less entry would otherwise pass
    # as its own reason and only fail later, mislabeled "stale".
    if [[ "$line" != *"|"* || -z "$path" || -z "${reason//[[:space:]]/}" ]]; then
      echo "scan-internal: malformed PEM exemption, want 'path|reason': $line" >&2
      exit 1
    fi
    pem_exempt_paths+="$path"$'\n'
  done <"$PEM_EXEMPTIONS"
else
  echo "scan-internal: $PEM_EXEMPTIONS missing — the PEM shape cannot be scanned without its exemption list" >&2
  exit 1
fi

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

# A tailnet a doc or test may stand up as an example. The tailnet label is
# the one that identifies whose network it is, so that is what this reads:
# a name under `example` names nobody, while <device>.<real-tailnet>.ts.net
# names a device on someone's network even when the host label is generic.
# (Spelling a counter-example out in full here would itself be a hit — this
# comment was one until it was rewritten.)
is_placeholder_tailnet() {
  case "$1" in
    *.example.ts.net) return 0 ;;
    *) return 1 ;;
  esac
}

# Print lines that carry a real MagicDNS name, skipping placeholder tailnets.
# A filter and not a bare grep for the same reason as the host check below it:
# one line can hold a documented stand-in and nothing else, and reporting that
# line is the false positive that teaches people to ignore the gate.
filter_real_tailnets() {
  while IFS= read -r line; do
    hosts="$(printf '%s\n' "$line" | grep -oE "$PAT_TAILNET_HOST" || true)"
    [[ -z "$hosts" ]] && continue
    while IFS= read -r host; do
      [[ -z "$host" ]] && continue
      if ! is_placeholder_tailnet "$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')"; then
        printf '%s\n' "$line"
        break
      fi
    done <<<"$hosts"
  done
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

# Drop PEM hits whose file is exempted in internal/secretscan/pem-exemptions.txt.
# The exemption key is the exact repo-relative path — the text before grep's
# first colon — so an exemption is a decision about one file, and a --dir
# artifact path (absolute, foreign-rooted) never lands in the set: a built
# artifact carrying a key header is always a hit.
filter_pem_exempt() {
  # The list travels through the environment, not -v: an awk -v value cannot
  # carry a literal newline, and a multi-entry list is newline-separated.
  PEM_EXEMPT_PATHS="$pem_exempt_paths" awk -F: '
    BEGIN {
      n = split(ENVIRON["PEM_EXEMPT_PATHS"], A, "\n")
      for (i = 1; i <= n; i++) if (A[i] != "") X[A[i]] = 1
    }
    !($1 in X)'
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
    examples/demo.db|examples/demo-linear.db|*.png|*.jpg|*.jpeg|*.gif|*.webp|*.ico|*.woff|*.woff2|*.ttf|*.eot|*.mp4|*.webm|*.zip|*.gz|*.tgz)
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

# -I on every grep below: a binary that the case above did not name is
# skipped, not matched byte-wise. Without it BSD grep printed "Binary file X
# matches" on stdout (a hit) while GNU grep >= 3.5 prints it on stderr
# (discarded) — the same bytes passed CI and failed the lead's machine
# (GDK-1595: four bytes of an h264 stream matched a wordlist entry).
# Prefix each hit with the name of the shape that matched. The failure block
# used to be a bare list of file:line, so knowing *why* a line was reported
# meant re-running the greps one at a time by hand.
label_hits() { sed "s|^|[$1] |"; }

if [[ -s "$text_list" ]]; then
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_TOKEN" -- $(cat "$text_list") 2>/dev/null \
      | label_hits atlassian_api_token >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_LINEAR" -- $(cat "$text_list") 2>/dev/null \
      | label_hits linear_api_key >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_SLACK" -- $(cat "$text_list") 2>/dev/null \
      | label_hits slack_token >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_GITHUB" -- $(cat "$text_list") 2>/dev/null \
      | label_hits github_token >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -niIHE "$PAT_BASIC" -- $(cat "$text_list") 2>/dev/null \
      | label_hits http_basic_auth >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -niIHE "$PAT_BEARER" -- $(cat "$text_list") 2>/dev/null \
      | label_hits http_bearer_token >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_TAILNET_CGNAT" -- $(cat "$text_list") 2>/dev/null \
      | label_hits tailnet_cgnat >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_TAILNET_HOST" -- $(cat "$text_list") 2>/dev/null \
      | filter_real_tailnets | label_hits tailnet_host >>"$hits_file" || true
  if [[ -n "$PAT_COMPANY" ]]; then
    # shellcheck disable=SC2046
    grep -niIHE "$PAT_COMPANY" -- $(cat "$text_list") 2>/dev/null \
        | label_hits word_list >>"$hits_file" || true
  fi
  # shellcheck disable=SC2046
  grep -niIHE "$PAT_HOST" -- $(cat "$text_list") 2>/dev/null \
      | filter_disallowed_hosts | label_hits tenant_host >>"$hits_file" || true
  # shellcheck disable=SC2046
  grep -nIHE "$PAT_HOMEPATH" -- $(cat "$text_list") 2>/dev/null \
      | filter_real_home_paths | label_hits home_path >>"$hits_file" || true
  # PEM keeps its raw grep output in $pem_out because the staleness check
  # below needs the hits before exemption filtering. -e because this
  # pattern is the one in the table that starts with a dash: as a bare
  # operand before `--` grep reads it as an option cluster, dies on rc=2,
  # and the 2>/dev/null here would bury that (the probe run that caught it
  # surfaced as all-stale instead — the safe direction, but a silent grep).
  # shellcheck disable=SC2046
  grep -nIHE -e "$PAT_PEM" -- $(cat "$text_list") 2>/dev/null >"$pem_out" || true
  filter_pem_exempt <"$pem_out" | label_hits private_key_pem >>"$hits_file" || true
fi

# Committed fixtures are binaries, so the text scan above skips them (the case
# list feeds it) — this sweep runs the same patterns over their printable
# strings. One loop per fixture file: the second committed mirror
# (examples/demo-linear.db, GDK-1298) landed while the sweep was hardcoded to
# demo.db, and a secret-shaped string in it would have sailed through.
if [[ -z "$SCAN_DIR" ]]; then
  for db in examples/demo.db examples/demo-linear.db; do
    [[ -f "$db" ]] || continue
    echo "==> scanning strings in $db"
    tmp_strings="$(mktemp)"
    strings "$db" >"$tmp_strings"
    # The PEM shape runs here too (GDK-1797): a mirrored issue body can carry
    # a pasted key header, and a fixture is real data, not a test vector —
    # the exemption list never applies to this sweep.
    grep -nE "$PAT_TOKEN|$PAT_LINEAR|$PAT_SLACK|$PAT_GITHUB|$PAT_PEM" "$tmp_strings" 2>/dev/null \
        | sed "s|^|$db:strings:|" >>"$hits_file" || true
    grep -niE "$PAT_BASIC|$PAT_BEARER" "$tmp_strings" 2>/dev/null \
        | sed "s|^|$db:strings:|" >>"$hits_file" || true
    if [[ -n "$PAT_COMPANY" ]]; then
      grep -niE "$PAT_COMPANY" "$tmp_strings" 2>/dev/null \
          | sed "s|^|$db:strings:|" >>"$hits_file" || true
    fi
    grep -niE "$PAT_HOST" "$tmp_strings" 2>/dev/null \
        | filter_disallowed_hosts \
        | sed "s|^|$db:strings:|" >>"$hits_file" || true
    rm -f "$tmp_strings"
  done
fi

# A stale exemption is a failure, not a no-op: an entry whose file no longer
# matches the shape is a decision about a tree that is gone, and a list that
# keeps it quietly only grows. Judged against the same grep output the filter
# saw, in tree mode only — --dir exempts nothing, so it has nothing to be
# stale about.
if [[ -z "$SCAN_DIR" ]]; then
  stale_pem="$(mktemp)"
  while IFS= read -r p; do
    [[ -z "$p" ]] && continue
    if ! awk -F: -v p="$p" 'index($0, p ":") == 1 { ok = 1 } END { exit !ok }' "$pem_out"; then
      printf '%s\n' "$p" >>"$stale_pem"
    fi
  done <<<"$pem_exempt_paths"
  if [[ -s "$stale_pem" ]]; then
    echo ""
    echo "FAILED: stale PEM exemption(s) in $PEM_EXEMPTIONS — the file no longer"
    echo "carries a private-key header, so the entry must be deleted:"
    sort -u "$stale_pem"
    rm -f "$stale_pem"
    exit 1
  fi
  rm -f "$stale_pem"
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
  echo "A PEM private-key header is legal only in a test-vector file — add or"
  echo "review its entry in $PEM_EXEMPTIONS, with a reason."
  exit 1
fi

if [[ -n "$PAT_COMPANY" ]]; then
  echo "OK: no credential-shaped strings, listed words, non-allowlisted tenant hosts, or real home paths."
else
  echo "OK: no credential-shaped strings, non-allowlisted tenant hosts, or real home paths (word check skipped)."
fi
exit 0
