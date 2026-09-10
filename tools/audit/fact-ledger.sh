#!/usr/bin/env bash
# Fact-ledger cross-check — docs/project/FACT_LEDGER.md against every copy
# surface (GDK-1707, census row "Fact map"; the axis-12 opener).
#
#   bash tools/audit/fact-ledger.sh > page.md
#
# The ledger is the source; the READMEs, docs/, site/, the skill and
# llms.txt are copies. This page maps copies by the fact's VALUES —
# fenced commands, URLs, the §6 measurement tokens, the §17 contract
# strings, the status minor — and reports for each value:
#
#   copies   exact copies with path:line (cap 5)
#   variant  lines carrying the same command family but a different value
#            (a DISAGREES candidate for the axis-12 reader, not a verdict)
#   guard    "by-check" when tools/doc-checks.sh asserts the value itself,
#            "ledger-marked" when the ledger names a check near it, else
#            "unguarded"
#
# The §17 `ledger-contract` rows are re-verified here even though
# doc-checks check 47 already gates them: the census page is the readable
# side of that gate. docs/decisions/ is excluded — decisions are frozen
# history (Addendum-only), not front doors.
#
# Exit 0 always. Every path printed is repo-relative.
set -euo pipefail
cd "$(cd "$(dirname "$0")/../.." && pwd)"
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,/^set -euo pipefail$/p' "$0"
  exit 0
fi

SOURCES=()
src() { SOURCES+=("$1"); }

BASE="$(git rev-parse --short HEAD)"
src "git rev-parse --short HEAD   # base: $BASE"

echo "# Fact-ledger cross-check — ledger values × copy surfaces"
echo
echo "Base $BASE. variants are DISAGREES candidates, not verdicts."
echo

python3 - <<'PY'
import re, subprocess, os

LEDGER = 'docs/project/FACT_LEDGER.md'
GUARD = 'tools/doc-checks.sh'

ledger_lines = open(LEDGER, encoding='utf-8').read().splitlines()

# ── copy surfaces: READMEs, docs/ (minus the ledger and decisions/),
#    site/, the skill, llms.txt — from git ls-files so only tracked files
files = subprocess.run(
    ['git', 'ls-files', 'README.md', 'README.ko.md', 'README.ja.md',
     'docs/**', 'site/**', 'skills/**'],
    capture_output=True, text=True, check=True).stdout.split()
surfaces = []
for f in files:
    if f == LEDGER or f.startswith('docs/decisions/'):
        continue
    if not (f.endswith(('.md', '.txt', '.astro', '.mjs', '.ts', '.html', '.js'))
            or f == 'site/public/llms.txt'):
        continue
    if os.path.exists(f):
        surfaces.append(f)
print(f'Copy surfaces: {len(surfaces)} files (READMEs, docs/, site/, '
      f'skills/; the ledger itself, docs/decisions/ and the CHANGELOGs are'
      f' excluded — decisions are frozen history, changelogs are not front'
      f' doors).')
print()

# lines[path] = list of line strings (1-based via index+1)
lines = {}
for f in surfaces:
    try:
        lines[f] = open(f, encoding='utf-8').read().splitlines()
    except UnicodeDecodeError:
        lines[f] = []

guard_text = open(GUARD, encoding='utf-8').read()

def copies_of(value, kind=None):
    # URLs and measurement tokens need boundary matching: the bare domain
    # is a prefix of every deep link, and `22 ms` is a suffix of `122 ms`.
    if kind == 'url':
        pat = re.compile(re.escape(value) + r'(?![\w/\-])')
    elif kind == 'measure':
        pat = re.compile(r'(?<![0-9])' + re.escape(value) + r'(?![0-9,×])')
    else:
        pat = None
    out = []
    for f in surfaces:
        for i, ln in enumerate(lines[f], 1):
            if (pat.search(ln) if pat else value in ln):
                out.append(f'{f}:{i}')
    return out

# ── §17 ledger-contract rows: re-verify each ------------------------------
print('## §17 contract rows (doc-checks check 47 — re-verified here)')
print()
print('| file :: string | in file | copies elsewhere |')
print('|---|---|---|')
in_contract = False
contract_rows = []
for ln in ledger_lines:
    if ln.startswith('```ledger-contract'):
        in_contract = True
        continue
    if in_contract and ln.startswith('```'):
        in_contract = False
        continue
    if in_contract and ' :: ' in ln:
        contract_rows.append(ln.strip())
for row in contract_rows:
    target, _, string = row.partition(' :: ')
    string = string.strip()
    ok = '—'
    if os.path.exists(target):
        try:
            ok = 'OK' if string in open(target, encoding='utf-8').read() \
                else '**MISSING**'
        except UnicodeDecodeError:
            ok = 'unreadable'
    # copies outside the contract target itself
    n = 0
    for f in surfaces:
        if f == target:
            continue
        for ln in lines[f]:
            if string in ln:
                n += 1
                break
    disp = row if len(row) <= 70 else row[:67] + '…'
    print(f'| `{disp}` | {ok} | {n} |')
print()

# ── value extraction: fenced commands, URLs, §6 tokens, status minor ------
values = []          # (kind, value)
in_fence = False
fence_kind = ''
for ln in ledger_lines:
    if ln.startswith('```') and not in_fence:
        in_fence = True
        fence_kind = ln[3:].strip()
        continue
    if in_fence and ln.startswith('```'):
        in_fence = False
        fence_kind = ''
        continue
    if in_fence:
        if fence_kind == 'ledger-contract':
            continue
        # single-line commands only; continuation lines start indented
        if ln and not ln[0].isspace() and ' :: ' not in ln:
            values.append(('command', ln.strip()))
        continue
    for u in re.findall(r'https?://[^ )`>,]+', ln):
        values.append(('url', u))

# §6 measurement tokens, bounded to the section
sec6 = []
in6 = False
for ln in ledger_lines:
    if ln.startswith('## 6.'):
        in6 = True
        continue
    if in6 and ln.startswith('## '):
        in6 = False
    if in6:
        sec6.append(ln)
sec6_text = '\n'.join(sec6)
for tok in sorted(set(re.findall(r'[0-9][0-9,]* ms|[0-9]+×|2026-08-26|3,296', sec6_text))):
    values.append(('measure', tok))

m = re.search(r'\*\*Status: ([0-9]+\.[0-9]+)', '\n'.join(ledger_lines))
if m:
    values.append(('status', m.group(1)))

# dedupe, keep order
seen = set()
uniq = []
for kind, v in values:
    if (kind, v) not in seen:
        seen.add((kind, v))
        uniq.append((kind, v))

# ledger line numbers per value for the guard-context rule
ledger_pos = {}
for i, ln in enumerate(ledger_lines, 1):
    for kind, v in uniq:
        if v in ln:
            ledger_pos.setdefault((kind, v), i)

def guard_of(kind, v):
    if v in guard_text:
        return 'by-check'
    pos = ledger_pos.get((kind, v))
    if pos:
        ctx = ledger_lines[max(0, pos - 6):pos + 6]
        if any('doc-checks' in x or re.search(r'check \d+', x) for x in ctx):
            return 'ledger-marked'
    return 'unguarded'

print('## Value copy map')
print()
print(f'{len(uniq)} values ({sum(1 for k, _ in uniq if k == "command")} '
      f'commands, {sum(1 for k, _ in uniq if k == "url")} URLs, '
      f'{sum(1 for k, _ in uniq if k == "measure")} measurement tokens, '
      f'{sum(1 for k, _ in uniq if k == "status")} status).')
print()
print('| kind | value | copies | sample | guard |')
print('|---|---|---:|---|---|')
variant_pairs = []
for kind, v in uniq:
    cps = copies_of(v, kind)
    sample = ', '.join(cps[:3])
    if len(cps) > 3:
        sample += f' …(+{len(cps) - 3})'
    disp = v if len(v) <= 48 else v[:45] + '…'
    print(f'| {kind} | `{disp}` | {len(cps)} | {sample} | {guard_of(kind, v)} |')
    if kind == 'command':
        fam = ' '.join(v.split()[:2])
        if fam:
            variant_pairs.append((v, fam))
print()

# command-family variants: same family head, different value, same file
print('## Command-family variants (DISAGREES candidates)')
print()
print('Lines that carry a ledger command\'s first two words but not the'
      ' ledger value itself. Includes legitimate siblings (the other brew'
      ' formula) — the axis-12 reader decides.')
print()
found = 0
shown = 0
for v, fam in variant_pairs:
    hits = []
    for f in surfaces:
        for i, ln in enumerate(lines[f], 1):
            if fam in ln and v not in ln:
                hits.append(f'{f}:{i} {ln.strip()[:70]}')
                if len(hits) >= 3:
                    break
        if len(hits) >= 3:
            break
    if hits:
        found += 1
        if shown < 25:
            shown += 1
            print(f'- ledger `{v[:60]}`')
            for h in hits:
                print(f'  - {h}')
print()
print(f'{found} of {len(variant_pairs)} command families have a variant'
      f' line (first 25 shown).')
print()

# status minor across editions
print('## Status minor across editions')
print()
st = [v for k, v in uniq if k == 'status']
if st:
    minor = st[0]
    pats = {'README.md': r'\*\*Status: ([0-9]+\.[0-9]+)',
            'README.ko.md': r'\*\*상태: ([0-9]+\.[0-9]+)',
            'README.ja.md': r'\*\*状態: ([0-9]+\.[0-9]+)'}
    for f, pat in pats.items():
        got = re.search(pat, '\n'.join(lines.get(f, [])))
        verdict = '—'
        if not got:
            verdict = '**pattern missing**'
        elif got.group(1) != minor:
            verdict = f'**DISAGREES: {got.group(1)}**'
        else:
            verdict = f'agrees ({got.group(1)})'
        print(f'- {f}: {verdict}')
print()

unguarded = [(k, v) for k, v in uniq if guard_of(k, v) == 'unguarded'
             and copies_of(v, k)]
print(f'## Unguarded values with copies: {len(unguarded)}')
print()
print('Values no doc-checks check asserts and no ledger clause marks — a'
      ' copy that drifts here turns wrong silently (axis-12 reading list).')
print()
for k, v in unguarded[:30]:
    disp = v if len(v) <= 60 else v[:57] + '…'
    print(f'- {k} `{disp}`')
print()
print('Scan: python3 in-script over the surfaces; guards via '
      '`grep -F <value> tools/doc-checks.sh` plus the ledger context rule'
      ' (±6 lines).')
print()
PY
src "python3 value/copy scan in this script (surfaces from git ls-files, values from docs/project/FACT_LEDGER.md fences, URLs, §6 tokens, §17 rows)"
src "guard rule: grep -F <value> tools/doc-checks.sh, else ledger ±6-line doc-checks/check-N mention"

echo "## Sources"
echo
echo "Every number above came from a command in this section, run from the"
echo "repo root at base $BASE:"
echo
for s in "${SOURCES[@]}"; do echo "- \`$s\`"; done
