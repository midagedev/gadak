#!/usr/bin/env bash
# Surface coverage — which of the five product surfaces each Unreleased
# changelog key actually reached (GDK-1707, census row "Surface matrix").
#
#   bash tools/audit/surface-coverage.sh > page.md
#
# Method: for every [GDK-nnn] cited under `## Unreleased` in CHANGELOG.md,
# take the commits whose message cites the key (one `git log --name-only`
# pass over history) and classify the files each commit touched:
#
#   CLI    cmd/gadak/**
#   web    web/src/**
#   MCP    internal/mcp/**   (the tool descriptions shell-less agents read)
#   SKILL  skills/**
#   phone  mobile/**
#
# A cell reads YES(n) — n distinct files under that surface were touched by
# one of the key's citing commits — or MISSING. This measures where the
# change *landed*, not where it should have been documented: a surface
# delivered under a different key still shows MISSING, and the axis-7
# reader (docs/runbooks/release-audit.md) triages the gap rows by hand.
# The three CHANGELOG editions carry the same Unreleased key set
# (tools/doc-checks.sh check 27), so only CHANGELOG.md is read.
#
# Keys with no citing commit are listed as such — that is a census answer,
# not an error. Exit 0 always: a census reports what it could not measure.
# Read-only git only; every path printed is repo-relative.
set -euo pipefail
cd "$(cd "$(dirname "$0")/../.." && pwd)"
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,/^set -euo pipefail$/p' "$0"
  exit 0
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/surface-coverage.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

SOURCES=()
src() { SOURCES+=("$1"); }

BASE="$(git rev-parse --short HEAD)"
src "git rev-parse --short HEAD   # base: $BASE"

# Unreleased = `## Unreleased` up to the next `## ` heading. The runbook's
# warning holds: version headings are `## vN.N.N`, so `^## 0\.` matches
# nothing — the generic next-heading stop is what works.
awk '/^## Unreleased$/{f=1;next} /^## /{f=0} f' CHANGELOG.md \
  | grep -oE 'GDK-[0-9]+' | sort -u > "$WORK/keys"
src "awk '/^## Unreleased\$/{f=1;next} /^## /{f=0} f' CHANGELOG.md | grep -oE 'GDK-[0-9]+' | sort -u"
KEY_TOTAL="$(wc -l < "$WORK/keys" | tr -d ' ')"

# One pass: commits with their cited keys and touched files. `C|` prefixes
# commit headers; every other non-empty line is a file that commit touched.
git log --name-only --format='C|%H|%s' > "$WORK/log"
src "git log --name-only --format='C|%H|%s'   # one pass over history"

# Single classifier pass. Key matching is boundary-exact (GDK-11 does not
# match inside GDK-1132): the token regex requires a non-digit or end of
# subject right after the number, and the trailing separator is stripped.
awk -F'|' -v keys_file="$WORK/keys" '
  BEGIN {
    while ((getline line < keys_file) > 0) {
      keys[++nkeys] = line
      want[line] = 1
    }
    surfs[1] = "cli"; surfs[2] = "web"; surfs[3] = "mcp"
    surfs[4] = "skill"; surfs[5] = "phone"
    surfs[6] = "engine"; surfs[7] = "docs"; surfs[8] = "e2e"; surfs[9] = "other"
  }
  function classify(path) {
    if (path ~ /^cmd\/gadak\//) return "cli"
    if (path ~ /^web\/src\//) return "web"
    if (path ~ /^internal\/mcp\//) return "mcp"
    if (path ~ /^skills\//) return "skill"
    if (path ~ /^mobile\//) return "phone"
    if (path ~ /^internal\//) return "engine"
    if (path ~ /^(docs|specs)\//) return "docs"
    if (path ~ /^e2e\//) return "e2e"
    return "other"
  }
  /^C\|/ {
    sha = substr($2, 1, 7)
    s = $3
    n = 0
    while (match(s, /GDK-[0-9]+([^0-9]|$)/)) {
      tok = substr(s, RSTART, RLENGTH)
      sub(/[^0-9]$/, "", tok)
      if (want[tok]) {
        # first sighting of this key in this commit
        new = 1
        for (i = 1; i <= n; i++) if (hit[i] == tok) new = 0
        if (new) hit[++n] = tok
      }
      s = substr(s, RSTART + RLENGTH)
    }
    next
  }
  /^[[:space:]]*$/ { next }
  {
    surf = classify($0)
    for (i = 1; i <= n; i++) {
      k = hit[i]
      if (!((k SUBSEP surf SUBSEP $0) in seen)) {
        seen[k, surf, $0] = 1
        cnt[k, surf]++
        if (files[k, surf] == "") files[k, surf] = $0
        else if (cnt[k, surf] <= 10) files[k, surf] = files[k, surf] "\n" $0
        else overflow[k, surf]++
      }
      if (sha != lastsha[k]) {
        shas[k] = shas[k] (shas[k] == "" ? "" : " ") sha
        lastsha[k] = sha
      }
    }
  }
  END {
    print "## Matrix"
    print ""
    print "| key | CLI | web | MCP | SKILL | phone | commits |"
    print "|---|---|---|---|---|---|---|"
    noc = 0
    for (j = 1; j <= nkeys; j++) {
      k = keys[j]
      if (!(k in shas)) { noc++; continue }
      row = "| " k " "
      for (i = 1; i <= 5; i++) {
        s = surfs[i]
        row = row "| " ((cnt[k, s] > 0) ? "YES(" cnt[k, s] ") " : "MISSING ")
      }
      print row "| " shas[k] " |"
    }
    print ""
    print "## Keys with no citing commit: " noc
    print ""
    for (j = 1; j <= nkeys; j++) {
      k = keys[j]
      if (!(k in shas)) printf "%s ", k
    }
    print ""
    print ""
    print "## Agent-surface gaps (axis-7 rows)"
    print ""
    gap_both = gap_skill = gap_mcp = 0
    for (j = 1; j <= nkeys; j++) {
      k = keys[j]
      if (!(k in shas)) continue
      if (!(cnt[k, "cli"] > 0 || cnt[k, "web"] > 0)) continue
      m = !(cnt[k, "mcp"] > 0); s = !(cnt[k, "skill"] > 0)
      if (m && s) gap_both++
      else if (s) gap_skill++
      else if (m) gap_mcp++
      else continue
    }
    print "CLI or web reached while the skill and/or the MCP descriptions"
    print "did not: " gap_both " keys miss both, " gap_skill " miss SKILL"
    print "only, " gap_mcp " miss MCP only. Triage by hand — a MISS here"
    print "may be correct (no agent verb in the change) or may be the"
    print "axis-7 finding."
    print ""
    print "| key | missing |"
    print "|---|---|"
    for (j = 1; j <= nkeys; j++) {
      k = keys[j]
      if (!(k in shas)) continue
      if (!(cnt[k, "cli"] > 0 || cnt[k, "web"] > 0)) continue
      m = !(cnt[k, "mcp"] > 0); s = !(cnt[k, "skill"] > 0)
      if (m && s) print "| " k " | SKILL MCP |"
      else if (s) print "| " k " | SKILL |"
      else if (m) print "| " k " | MCP |"
    }
    print ""
    print "## Per-key detail (files per surface, capped at 10)"
    print ""
    for (j = 1; j <= nkeys; j++) {
      k = keys[j]
      if (!(k in shas)) continue
      print "**" k "**"
      print "- commits: " shas[k]
      for (i = 1; i <= 9; i++) {
        s = surfs[i]
        if (!(cnt[k, s] > 0)) continue
        print "- " s ":"
        nf = split(files[k, s], fl, "\n")
        for (f = 1; f <= nf; f++) if (fl[f] != "") print "  - " fl[f]
        if (overflow[k, s] > 0) print "  - … (+" overflow[k, s] " more)"
      }
    }
  }
' "$WORK/log" > "$WORK/body"

{
  echo "# Surface coverage — Unreleased keys × {CLI, web, MCP, SKILL, phone}"
  echo
  echo "Base $BASE. $KEY_TOTAL keys cited under \`## Unreleased\` in"
  echo "CHANGELOG.md. Cell = YES(n) distinct files touched by the key's"
  echo "citing commits, else MISSING. Method and its limit: commit-file"
  echo "classification — a surface delivered under a different key still"
  echo "reads MISSING (see Sources)."
  echo
  cat "$WORK/body"
  echo "## Sources"
  echo
  echo "Every number above came from a command in this section, run from"
  echo "the repo root at base $BASE:"
  echo
  for s in "${SOURCES[@]}"; do echo "- \`$s\`"; done
}
