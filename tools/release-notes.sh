#!/usr/bin/env bash
# Put the release's own CHANGELOG section on top of its GitHub Release page.
#
#   tools/release-notes.sh <tag>            # dry run: writes the body, prints sizes
#   tools/release-notes.sh <tag> --apply    # gh release edit <tag> --notes-file …
#
# goreleaser publishes a grouped commit log plus the install footer. The
# config has always said the curated summary "still gets written by hand on
# the release page" (.goreleaser.yaml, changelog:), and by hand is exactly why
# it went missing: v0.20.0 and v0.21.0 got one, v0.22.0 did not, and the page
# a reader lands on from the Store or a tweet was 290 commit subjects while
# gadak.dev/changelog/ said something a person would read. Nothing tied the
# two. This is the tie: the summary is the CHANGELOG.md section for the tag,
# its GDK references are defined under it (a bare [GDK-n] renders as literal
# brackets on GitHub), and the generated log stays underneath as the tail.
#
# Rerunnable. Whatever sits above the first "## Changelog" is treated as the
# previous summary and replaced, so editing CHANGELOG.md after a release and
# running this again is the whole update path — which is the case that made
# this script: the 0.22.0 section was cut from 4,983 words to 1,500 the day
# after the tag, and the release page kept the old shape.
#
# It also drops the update-check sentence the footer carried through v0.22.0.
# There is no update check (GDK-1626); the claim sat in every release body
# from v0.19 on, and editing a page without removing it would re-publish it.
set -euo pipefail
cd "$(dirname "$0")/.."

tag="${1:?usage: tools/release-notes.sh <tag> [--apply]}"
apply="${2:-}"
out="${TMPDIR:-/tmp}/gadak-release-notes-${tag}.md"

command -v gh >/dev/null || { echo "release-notes: gh is required" >&2; exit 2; }

current="$(gh release view "$tag" --json body --jq .body)" || {
  echo "release-notes: no GitHub release named $tag" >&2
  exit 2
}

TAG="$tag" OUT="$out" CURRENT="$current" python3 - <<'PY'
import os, re, sys

tag, out, current = os.environ["TAG"], os.environ["OUT"], os.environ["CURRENT"]
md = open("CHANGELOG.md", encoding="utf-8").read()

m = re.search(r"^## " + re.escape(tag) + r"\b[^\n]*\n(.*?)(?=^## |\Z)", md, re.M | re.S)
if not m:
    print(f"release-notes: CHANGELOG.md has no '## {tag}' section", file=sys.stderr)
    sys.exit(3)
# The file's reference tail follows the last section; never carry it inside.
section = re.sub(r"^\[GDK-\d+\]:.*\n?", "", m.group(1), flags=re.M).strip()

# CHANGELOG.md is hard-wrapped at 80 columns, and a GitHub release page keeps
# every newline as a line break, so v0.24.1's page read as ragged 80-column
# lines. A prose paragraph goes on the page as one line; anything that is not
# plain prose (a fence, a list, a table, an indented block) keeps its lines.
def unwrap(par):
    lines = par.split("\n")
    if any(l.startswith(("```", "|", "    ", "- ", "* ", "#", ">")) for l in lines):
        return par
    return " ".join(l.strip() for l in lines)

section = "\n\n".join(unwrap(p) for p in re.split(r"\n\s*\n", section))

defs = dict(re.findall(r"^\[(GDK-\d+)\]:\s*(\S+)", md, re.M))
cited = sorted(set(re.findall(r"\[(GDK-\d+)\]", section)), key=lambda k: int(k.split("-")[1]))
missing = [k for k in cited if k not in defs]
if missing:
    print(f"release-notes: {tag} cites keys with no reference definition: {missing}", file=sys.stderr)
    sys.exit(4)

i = current.find("## Changelog")
if i < 0:
    # Not a goreleaser body. Refuse rather than guess which part is ours:
    # prepending onto an unknown body is how a summary ends up twice.
    print(f"release-notes: the {tag} body has no generated '## Changelog' tail to keep", file=sys.stderr)
    sys.exit(5)
tail = current[i:]
tail = re.sub(r"\s*Running installs notice new releases on their own \([^)]*\)\.", "", tail)

body = section + "\n\n" + "\n".join(f"[{k}]: {defs[k]}" for k in cited) + "\n\n" + tail.rstrip() + "\n"
open(out, "w", encoding="utf-8").write(body)

prev = len(current[:i].encode())
print(f"release-notes: {tag}")
print(f"  summary      {len(section.encode()):7} B  (was {prev} B above the generated log)")
print(f"  references   {len(cited):7}")
print(f"  generated    {len(tail.encode()):7} B  (update-check claim stripped: {'daily anonymous check' in current})")
print(f"  body         {len(current.encode()):7} B -> {len(body.encode())} B")
print(f"  written      {out}")
PY

if [ "$apply" = "--apply" ]; then
  gh release edit "$tag" --notes-file "$out"
  echo "release-notes: $tag release page updated"
else
  echo "release-notes: dry run — review $out, then rerun with --apply"
fi
