#!/usr/bin/env bash
# i18n census — catalog completeness, byte-equality fall-throughs, English
# literals outside t(), and locale-sensitive format sites (GDK-1707, census
# row "i18n gaps"; the axis-11 opener).
#
#   bash tools/audit/i18n-census.sh > page.md
#
# What it measures:
#   1. keys per catalog file under web/src/lib/i18n/messages/, and any key
#      with a missing or empty locale value (the type makes a missing
#      locale an error; the census proves the zero).
#   2. ko/ja values byte-equal to en, split into allowlisted (with the
#      reason recorded in web/src/lib/i18n/catalog.test.ts) and findings.
#      The vitest gate already fails un-allowlisted byte-equality; this
#      page is the readable list, including stale allowlist entries.
#   3. candidate user-visible English literals outside t() in web/src and
#      mobile/src — a HEURISTIC text-node/attribute scan, candidates for a
#      human, not findings.
#   4. every Intl./toLocale/xx-XX site in web/src, mobile/src and site/src,
#      classified takes-locale-arg / literal-locale / runtime-default.
#
# Not measured (say so honestly): English runs on the rendered ko/ja site
# pages — that needs the built site, and the recording round (CLAUDE.md)
# owns that half.
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

echo "# i18n census — catalog, fall-throughs, literals, locale sites"
echo
echo "Base $BASE. Heuristics are labelled; candidates are not findings."
echo

python3 - <<'PY'
import re, glob, os, sys

msgs = sorted(glob.glob('web/src/lib/i18n/messages/*.ts'))
key_re = re.compile(r"^\s*'([^']+)':\s*\{")
val_re = re.compile(
    r"^\s*(en|ko|ja):\s*(?:'((?:[^'\\]|\\.)*)'|\"((?:[^\"\\]|\\.)*)\")\s*,?\s*$")

catalog = {}   # key -> {locale: value, file}
order = []
inline_re = re.compile(
    r"\b(en|ko|ja):\s*(?:'((?:[^'\\]|\\.)*)'|\"((?:[^\"\\]|\\.)*)\")")
for path in msgs:
    cur = None
    for n, line in enumerate(open(path, encoding='utf-8'), 1):
        m = key_re.match(line)
        if m:
            cur = m.group(1)
            if cur not in catalog:
                catalog[cur] = {'file': path, 'line': n}
                order.append(cur)
            # single-line form: 'key': { en: '…', ko: '…', ja: '…' },
            for lm in inline_re.finditer(line):
                catalog[cur][lm.group(1)] = lm.group(2) or lm.group(3) or ''
            continue
        if cur:
            v = val_re.match(line)
            if v:
                catalog[cur][v.group(1)] = v.group(2) or v.group(3) or ''

total = len(order)
per_file = {}
for k in order:
    per_file[catalog[k]['file']] = per_file.get(catalog[k]['file'], 0) + 1

print('## Catalog key census')
print()
print(f'{total} keys across {len(msgs)} files:')
print()
print('| file | keys |')
print('|---|---:|')
for f in msgs:
    print(f"| {f} | {per_file.get(f, 0)} |")
print()

missing, empty = [], []
for k in order:
    e = catalog[k]
    for loc in ('en', 'ko', 'ja'):
        if loc not in e:
            missing.append(f"{loc} {k} ({e['file']}:{e['line']})")
        elif e[loc] == '':
            empty.append(f"{loc} {k} ({e['file']}:{e['line']})")
print(f'Missing locale value: {len(missing)} (the type makes this an error; '
      f'the census proves the zero).')
for x in missing[:20]:
    print(f'- {x}')
print(f'Empty-string locale value: {len(empty)}.')
for x in empty[:20]:
    print(f'- {x}')
print()

# Byte-equality against the catalog.test.ts allowlist.
allow_re = re.compile(r"^\s*\['(ko|ja) ([^']+)',\s*'([^']*)'\],")
allowed = {}
for line in open('web/src/lib/i18n/catalog.test.ts', encoding='utf-8'):
    m = allow_re.match(line)
    if m:
        allowed[f'{m.group(1)} {m.group(2)}'] = m.group(3)

be_ko = [k for k in order
         if 'en' in catalog[k] and catalog[k].get('ko') == catalog[k]['en']]
be_ja = [k for k in order
         if 'en' in catalog[k] and catalog[k].get('ja') == catalog[k]['en']]
print('## Byte-equality (ko/ja == en)')
print()
print(f'ko == en: {len(be_ko)} keys; ja == en: {len(be_ja)} keys. '
      f'Allowlist in catalog.test.ts: {len(allowed)} entries.')
print()
unallow, stale = [], []
for loc, keys in (('ko', be_ko), ('ja', be_ja)):
    for k in keys:
        ident = f'{loc} {k}'
        if ident in allowed:
            continue
        unallow.append(f'{ident} = {catalog[k][loc]!r} ({catalog[k]["file"]})')
for ident in allowed:
    loc, k = ident.split(' ', 1)
    if k not in catalog:
        stale.append(f'{ident}: key no longer exists')
    elif catalog[k].get(loc) != catalog[k].get('en'):
        stale.append(f'{ident}: value no longer byte-equal')
print(f'Byte-equal WITHOUT an allowlist entry (findings — the vitest gate '
      f'is red on these): {len(unallow)}')
for x in unallow[:30]:
    print(f'- {x}')
print()
print(f'Stale allowlist entries (census note; the gate flags them too): '
      f'{len(stale)}')
for x in stale[:30]:
    print(f'- {x}')
print()
print('Allowlisted byte-equals, with reasons:')
print()
print('| id | reason |')
print('|---|---|')
for loc in ('ko', 'ja'):
    for k in order:
        ident = f'{loc} {k}'
        if ident in allowed and catalog[k].get(loc) == catalog[k].get('en'):
            print(f'| {ident} | {allowed[ident]} |')
print()

# Heuristic English-literal scan outside t().
print('## Candidate English literals outside t() (heuristic)')
print()
print('Text nodes and the usual label attributes in web/src and '
      'mobile/src, excluding the catalog itself and tests. Interpolated '
      '(brace) content is skipped. Each row is a candidate for a human, '
      'not a finding.')
print()
attr_re = re.compile(
    r'\b(placeholder|title|aria-label|aria-placeholder|alt|label|data-tooltip)="([^"{}]+)"')
text_re = re.compile(r'>([^<>{}]*[A-Za-z]{2,}[^<>{}]*[A-Za-z]{2,}[^<>{}]*)<')
word_re = re.compile(r'[A-Za-z]{2,}')
rows = []
for root in ('web/src', 'mobile/src'):
    for dirpath, dirnames, filenames in os.walk(root):
        # web/src/lib is non-UI helpers plus the catalog itself; the phone's
        # lib/ is UI and stays in.
        if dirpath == 'web/src':
            dirnames[:] = [d for d in dirnames if d != 'lib']
        dirnames[:] = [d for d in dirnames if d not in ('node_modules', 'dist')]
        for fn in filenames:
            if not fn.endswith(('.svelte', '.ts')):
                continue
            if '.test.' in fn or '.spec.' in fn:
                continue
            p = os.path.join(dirpath, fn)
            try:
                lines = open(p, encoding='utf-8').read().splitlines()
            except UnicodeDecodeError:
                continue
            for n, line in enumerate(lines, 1):
                for m in attr_re.finditer(line):
                    if len(word_re.findall(m.group(2))) >= 2:
                        rows.append(f'{p}:{n} {m.group(1)}="{m.group(2)[:60]}"')
                        break
                # text nodes only in markup, and skip code-shaped lines —
                # `a > b` comparisons in .ts read as text otherwise
                if p.endswith('.svelte') and not any(
                        tok in line for tok in ('=>', 'return', '||', '&&')):
                    m = text_re.search(line)
                    if m and len(word_re.findall(m.group(1))) >= 2:
                        t = m.group(1).strip()[:60]
                        rows.append(f'{p}:{n} text: {t}')
print(f'{len(rows)} candidate lines (capped display at 80):')
print()
print('```')
for r in rows[:80]:
    print(r)
if len(rows) > 80:
    print(f'… (+{len(rows) - 80} more)')
print('```')
print()
print('Heuristic commands: python3 scan in this script (os.walk over '
      'web/src and mobile/src, .svelte/.ts, tests and web/src/lib '
      'excluded).')
print()
PY
src "python3 catalog parse over web/src/lib/i18n/messages/*.ts (in-script)"
src "grep allowlist entries out of web/src/lib/i18n/catalog.test.ts"

echo "## Locale-sensitive format sites"
echo
echo "Every \`Intl.\` / \`toLocale\` / quoted \`xx-XX\` site in web/src,"
echo "mobile/src and site/src, classified by the call shape on the line"
echo "(heuristic — a variable argument counts as taking the active locale):"
echo
echo '| class | site |'
echo '|---|---|'
grep -rnE "Intl\.|toLocale|'ko-KR'|\"ko-KR\"|'ja-JP'|'en-US'|'ko_KR'|'ja_JP'" \
  web/src mobile/src site/src 2>/dev/null \
  | grep -vE '\.test\.|\.spec\.|app\.css' \
  | awk -F: '
      {
        line = $0
        sub(/^[^:]*:[0-9]*:/, "", line)
        cls = "takes-locale-arg"
        if (line ~ /locale\(\)|, locale|locale,|locale \?|lang\b/) cls = "takes-locale-arg"
        else if (line ~ /ko-KR|ja-JP|en-US|ko_KR|ja_JP/) cls = "literal-locale"
        else cls = "runtime-default-or-unclassified"
        printf "| %s | %s:%s — %s |\n", cls, $1, $2, substr(line, 1, 90)
      }' | sort
src "grep -rnE \"Intl\\.|toLocale|'ko-KR'|'ja-JP'|'en-US'\" web/src mobile/src site/src"

echo
echo "## Not measured"
echo
echo "- English runs on the **rendered** ko/ja site pages — needs the built"
echo "  site; the ko/ja clip recording round (CLAUDE.md) owns that half."
echo "- Tone and terminology across editions — axis 6 reads what this page"
echo "  cannot."
echo
echo "## Sources"
echo
echo "Every number above came from a command in this section, run from the"
echo "repo root at base $BASE:"
echo
for s in "${SOURCES[@]}"; do echo "- \`$s\`"; done
