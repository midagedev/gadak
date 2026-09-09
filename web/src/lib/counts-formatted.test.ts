/*
 * GDK-1560 recurrence gate: a count on screen goes through formatNumber()
 * in lib/i18n — the one owner of digit grouping — so 10184 renders as
 * 10,184 (en), 10,184 (ko) and 10,184 (ja) instead of a bare run of digits.
 * The bug was that the toolbar count used the helper while the breakdown
 * legend, the group header and the filter dropdown interpolated the raw
 * number, so one frame showed "1,195건" and "10184" side by side.
 *
 * The gate reads the class, not the three sites: any markup interpolation
 * whose whole expression is a count-named property ({v.count},
 * {group.counts.total}, {issue.comment_count}) is a bare number and fails.
 * Wrapping it — {formatNumber(v.count)} — no longer matches, because the
 * expression then starts with the call.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const COMPONENTS = join(HERE, '..', 'components')

/** `{count}` / `{foo.bar_count}` / `{group.counts.total}` — an interpolation
 *  that is nothing but a count-named path, which is exactly the shape that
 *  skips the formatter. Template holes (`${v.count}`) count: the string is
 *  rendered. A prop pass (`count={relatedDocCount}`) does not — the receiving
 *  component is the render site, and it is checked on its own line. Anything
 *  with a call or an operator in it is out of scope. */
const BARE_COUNT =
  /(^|[^=])\{(?:count|[A-Za-z_$][A-Za-z0-9_$.]*(?:[Cc]ount|[Cc]ounts\.[A-Za-z0-9_$.]*))\}/

type Exception = { file: string; why: string }

const ALLOWED: Exception[] = [
  {
    file: 'retro/RetroClosed.svelte',
    why: 'owned by a parallel round (w1-web may not edit components/retro); same fix applies there',
  },
  // These two hold an already-formatted string, not a number — the caller
  // grouped it (DocsView passes "12 / 340", HistoryView passes
  // formatNumber(...), palette.docCount is a whole translated sentence).
  // Formatting again is a type error, which is how they were found.
  {
    file: 'ui/ColumnHeader.svelte',
    why: 'count prop is typed string: every caller formats before passing it',
  },
  {
    file: 'palette/CommandPalette.svelte',
    why: 'docCount is a t() sentence built from formatNumber() parts, not a number',
  },
]

function bareCountSites(dir: string): string[] {
  const out: string[] = []
  for (const e of readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  )) {
    const p = join(dir, e.name)
    if (e.isDirectory()) out.push(...bareCountSites(p))
    else if (e.name.endsWith('.svelte')) {
      const rel = relative(COMPONENTS, p)
      if (ALLOWED.some((a) => rel === a.file)) continue
      readFileSync(p, 'utf8')
        .split('\n')
        .forEach((line, i) => {
          if (BARE_COUNT.test(line)) out.push(`${rel}:${i + 1}: ${line.trim()}`)
        })
    }
  }
  return out
}

describe('GDK-1560 counts share one formatter', () => {
  test('no component interpolates a bare count', () => {
    const sites = bareCountSites(COMPONENTS)
    expect(
      sites.join('\n') || '(none)',
      'digit grouping belongs to formatNumber() in lib/i18n (GDK-1560)',
    ).toBe('(none)')
  })

  // The three tags formatNumber can resolve to. The reported frame had
  // "1,195건" beside "10184"; every locale groups, so neither shape is a
  // locale's own convention — it was one surface skipping the formatter.
  test.each([
    ['en-US', 'en'],
    ['ko-KR', 'ko'],
    ['ja-JP', 'ja'],
  ])('%s groups thousands', (tag) => {
    expect(new Intl.NumberFormat(tag).format(10184)).toBe('10,184')
    expect(new Intl.NumberFormat(tag).format(1195)).toBe('1,195')
    expect(new Intl.NumberFormat(tag).format(999)).toBe('999')
  })

  // The exception stays honest: an entry whose file no longer renders a bare
  // count is a claim about code that is gone.
  test('every allowed file still renders a bare count', () => {
    for (const a of ALLOWED) {
      const src = readFileSync(join(COMPONENTS, a.file), 'utf8')
      expect(BARE_COUNT.test(src), `${a.file}: ${a.why}`).toBe(true)
    }
  })
})
