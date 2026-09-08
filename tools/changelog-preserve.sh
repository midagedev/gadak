#!/usr/bin/env bash
# changelog-preserve.sh — what did a changelog edit actually change?
#
# The changelog may be rewritten for narrative (user decision 2026-09-09):
# entries can be merged, reordered, compressed, and cut when they carry no
# issue key. Four things still may not move, and this tool is how you know
# they did not:
#
#   1. release headings and their order
#   2. every GDK key, still in its own release
#   3. numbers, commands, paths, code spans, fenced blocks and links
#   4. the reference-link tail
#
# It compares the working file against a git ref and prints every axis that
# moved. Read the output rather than only its exit code: a restructuring round
# will legitimately show fewer code spans and links (a cut entry took its own
# with it), while a heading, key or number that moved is a real failure.
#
# Run it before committing a changelog pass, and again after resolving a rebase:
#
#   bash tools/changelog-preserve.sh CHANGELOG.md            # against HEAD
#   bash tools/changelog-preserve.sh CHANGELOG.ja.md v0.21.0
#
# It is not a doc-checks check, because it needs a baseline: doc-checks runs on
# one tree and can only assert what a file says, not what it used to say. The
# cross-file contract (en/ko/ja cite the same keys per section) is check 27.
set -euo pipefail

file="${1:-CHANGELOG.md}"
ref="${2:-HEAD}"

if [[ ! -f "$file" ]]; then
  echo "changelog-preserve: no such file: $file" >&2
  exit 2
fi
if ! git show "$ref:$file" >/dev/null 2>&1; then
  echo "changelog-preserve: $ref has no $file — nothing to compare against" >&2
  exit 2
fi

old="$(mktemp)"; new="$(mktemp)"
trap 'rm -f "$old" "$new"' EXIT
git show "$ref:$file" > "$old"
cp "$file" "$new"

python3 - "$old" "$new" "$file" "$ref" <<'PY'
import re, sys
from collections import Counter

old_path, new_path, name, ref = sys.argv[1:5]
old = open(old_path, encoding='utf-8').read()
new = open(new_path, encoding='utf-8').read()

def headings(s):
    # The release list, in order. A polished changelog has the same releases
    # in the same order; ko/ja headings are the same strings by contract.
    return re.findall(r'^## .*$', s, re.M)

def keys_per_section(s):
    out, cur = {}, '(preamble)'
    for line in s.splitlines():
        if line.startswith('## '):
            cur = line
            out.setdefault(cur, Counter())
            continue
        if re.match(r'^\[GDK-\d+\]:\s', line):   # a reference-tail definition
            continue
        # A citation that happens to start a line is still a citation: an
        # earlier version skipped every line starting with "[GDK-", so
        # re-wrapping a paragraph moved keys in and out of a section with no
        # word changed (measured 2026-09-09, three false positives in one
        # round). Only the `[KEY]: url` form is the tail.
        out.setdefault(cur, Counter()).update(re.findall(r'GDK-\d+', line))
    return out

def tails(s):
    return dict(re.findall(r'^\[(GDK-\d+)\]:\s*(\S+)\s*$', s, re.M))

def spans(s):
    return Counter(re.findall(r'`[^`\n]+`', s))

def fences(s):
    return Counter(re.findall(r'```[\s\S]*?```', s))

def links(s):
    return Counter(re.findall(r'\]\(([^)]+)\)', s))

def numbers(s):
    # Every number a reader could act on. Issue keys are counted separately,
    # so strip them first or GDK-1647 would read as "1647". Trailing
    # separators are stripped too: moving a sentence break so that "401" ends
    # a sentence should not read as 401 -> "401." (measured 2026-09-09).
    found = re.findall(r'\d[\d,.]*', re.sub(r'GDK-\d+', '', s))
    return Counter(n.rstrip('.,') for n in found)

fails = []

o, n = headings(old), headings(new)
if o != n:
    only_o = [h for h in o if h not in n]
    only_n = [h for h in n if h not in o]
    fails.append('release headings changed:\n    gone: %s\n    new:  %s'
                 % (only_o[:5] or '(none)', only_n[:5] or '(none)'))

ok_, nk = keys_per_section(old), keys_per_section(new)
for sec in ok_:
    if sec not in nk:
        continue  # a missing section is already reported above
    lost = ok_[sec] - nk[sec]
    gained = nk[sec] - ok_[sec]
    if lost or gained:
        fails.append('section %r: keys changed (lost %s, gained %s)'
                     % (sec.strip(), dict(lost) or {}, dict(gained) or {}))

ot, nt = tails(old), tails(new)
if ot != nt:
    for k in sorted(set(ot) | set(nt)):
        if ot.get(k) != nt.get(k):
            fails.append('reference tail for %s: %r -> %r' % (k, ot.get(k), nt.get(k)))

for label, fn in (('code spans', spans), ('fenced blocks', fences),
                  ('links', links), ('numbers', numbers)):
    a, b = fn(old), fn(new)
    lost, gained = a - b, b - a
    if lost or gained:
        show_l = list(lost.elements())[:6]
        show_g = list(gained.elements())[:6]
        fails.append('%s changed:\n    gone: %s\n    new:  %s' % (label, show_l, show_g))

if fails:
    print('changelog-preserve: %s differs from %s in more than wording:' % (name, ref))
    for f in fails:
        print('  FAIL: %s' % f)
    print('\n  Headings, keys and numbers moving is a real failure — fix the file.')
    print('  Code spans and links moving is expected when a restructuring round')
    print('  cut an entry: read the list, confirm each one left with its entry,')
    print('  and say so in the commit message.')
    sys.exit(1)

print('changelog-preserve: %s vs %s — nothing moved '
      '(headings, keys, tails, code, links and numbers all preserved)' % (name, ref))
PY
