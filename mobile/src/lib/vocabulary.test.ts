import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import { stripComments } from './source-scan'

/*
 * Recurrence layer for GDK-884: "the phone invents a word" must fail here,
 * not in a reviewer's eye.
 *
 * Three closes, in order of how a regression actually happens:
 *  1. A source scan for the invented nouns this round removed (Queue / Mine /
 *     All), so they cannot come back under a new component.
 *  2. One seam: only lib/i18n.ts may reach into web/, so the catalog cannot be
 *     imported from ten places — or, worse, copied into one.
 *  3. A ko-locale run of the scope builder. A hardcoded English string passes
 *     an English eye and fails this: the desk's word for the default plate is
 *     내 이슈, and only t() knows that.
 */

const srcDir = join(dirname(fileURLToPath(import.meta.url)), '..')

function sourceFiles(): string[] {
  const out: string[] = []
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) walk(path)
      else if (/\.(ts|svelte)$/.test(entry.name)) out.push(path)
    }
  }
  walk(srcDir)
  return out.filter((p) => !p.endsWith('.test.ts'))
}

/** Strips // and /* *​/ comments and <!-- --> so prose about the ban is not
 *  the ban. One owner since GDK-1872 part 2 — the local copy silently ate
 *  everything after `accept="image/*"`, and this gate stayed green on it. */
function code(path: string): string {
  return stripComments(readFileSync(path, 'utf8'))
}

describe('GDK-884 the phone does not invent nouns', () => {
  it('has no Queue / Mine / All left in shipped source', () => {
    const hits: string[] = []
    for (const path of sourceFiles()) {
      const text = code(path)
      for (const re of [/\bQueue\b/, /['">]Mine['"<]/, /['">]All['"<]/]) {
        if (re.test(text)) hits.push(`${relative(srcDir, path)} :: ${re}`)
      }
    }
    expect(hits).toEqual([])
  })

  it('crosses into the desktop catalog through exactly one module', () => {
    const importers = sourceFiles()
      .filter((p) => /web\/src\/lib\/i18n/.test(code(p)))
      .map((p) => relative(srcDir, p))
    expect(importers).toEqual(['lib/i18n.ts'])
  })

  it('takes the scope names from the catalog, in whatever locale is set', async () => {
    // A fresh module graph with gadak_locale=ko: web/src/lib/i18n runs
    // initLocale() at import time, so the catalog table is Korean before
    // domain.ts asks for a single word.
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (k === 'gadak_locale' ? 'ko' : null),
      setItem: () => {},
      removeItem: () => {},
    })
    vi.resetModules()
    try {
      const { locale, t } = await import('./i18n')
      const { buildScopes, SCOPE_ALL_OPEN, SCOPE_DOCS_UPDATED, SCOPE_MY_WORK } = await import('./domain')
      expect(locale()).toBe('ko')
      const pages = [
        {
          key: '1',
          title: 'Guide',
          space_key: 'ENG',
          space_name: 'Engineering',
          parent_id: '',
          author: 'Dana',
          updated_at: '2026-08-01T00:00:00Z',
          version: 1,
          url: '',
        },
      ]
      const scopes = buildScopes([], [], { email: 'dev@example.com', account_id: 'acct-1', name: 'Dev' }, pages)
      expect(scopes.find((s) => s.id === SCOPE_MY_WORK)?.name).toBe('내 이슈')
      expect(scopes.find((s) => s.id === SCOPE_ALL_OPEN)?.name).toBe('전체 미해결')
      expect(t('sidebar.docs')).toBe('문서')
      expect(scopes.find((s) => s.id === SCOPE_DOCS_UPDATED)?.name).toBe('최근 갱신')
    } finally {
      vi.unstubAllGlobals()
      vi.resetModules()
    }
  })
})

describe('GDK-885 the picker wears the desktop section headings', () => {
  // GDK-902 2026-09-15: the picker is the palette's owner list now, drawn
  // in place of the list's rows instead of in a sheet. The headings are the
  // same desktop keys in the same order, and they moved with the ordering
  // rules into lib/palette.ts — the component reads `t(group.headingKey)`,
  // so scanning only the markup would pass this file while the phone
  // authored every word. Both halves are scanned as one text.
  const sheet =
    code(join(srcDir, 'ui/Palette.svelte')) + code(join(srcDir, 'lib/palette.ts'))

  it('uses the sidebar keys, not phone-authored section labels', () => {
    // `personal.myIssues` left this list with the section it headed (vision
    // FIX 2026-09-07: Assigned to me folded into the built-ins under their
    // stance sub-label; the row itself left with GDK-1542). The claim is
    // unchanged — every heading the picker draws is a desktop key — and the
    // two stance keys are now among them.
    for (const key of [
      'sidebar.builtinViews',
      'sidebar.myViews',
      'sidebar.jiraFilters',
      'sidebar.docs',
      'sidebar.stanceMine',
      'sidebar.stanceTeam',
    ]) {
      expect(sheet, `the palette is missing ${key}`).toContain(key)
    }
  })

  it('names the object with the desktop\'s word for it, from the catalog', () => {
    // GDK-902 2026-09-15: the tab bar carried `doc.issues` as its first
    // label and is gone. The word did not go with it — it heads the issue
    // results in the palette's typed ranking, which is the one place the
    // phone still has to say what those rows are.
    expect(sheet).toContain("t('doc.issues')")
  })

  it('does not consume the view list for writing', () => {
    for (const path of sourceFiles()) {
      const text = code(path)
      expect(text, `${relative(srcDir, path)} writes to the view list`).not.toMatch(
        /issues\/views\/[^'"]*['"],\s*\{[^}]*method:\s*['"](POST|DELETE|PUT)/,
      )
    }
  })
})

describe('status-category folding stays parity with the desktop', () => {
  it('accepts every alias the desktop owner accepts, with the same fold', async () => {
    // GDK-2042 moved the table into web/src/lib/status-category.ts. The
    // owner is parsed from source, not imported, so a phone-side
    // re-declaration of the table cannot satisfy this gate by construction.
    const owner = readFileSync(
      join(srcDir, '../../web/src/lib/status-category.ts'),
      'utf8',
    )
    const start = owner.indexOf('export const CATEGORY_ALIASES')
    expect(start, 'the owner table parsed').toBeGreaterThan(0)
    const table = owner.slice(start, owner.indexOf('\n}', start))
    const desk: Record<string, string> = {}
    for (const m of table.matchAll(/'?([A-Za-z][A-Za-z ]*)'?: '(new|inprogress|done)'/g)) {
      desk[m[1]] = m[2]
    }
    expect(Object.keys(desk).length, 'aliases parsed from the owner table').toBeGreaterThan(5)
    const { categoryAliases } = await import('./domain')
    expect(categoryAliases()).toEqual(desk)
  })

  it('folds an unknown key and an empty key the way the desk does', async () => {
    // GDK-2042: the axis this suite never had. The alias test above
    // compares the table and stayed green while the phone folded unknown
    // keys to 'inprogress' and the desk to 'new' — two views of one
    // workspace bucketing the same unmirrored row differently. Both
    // answers are pinned, not just their agreement.
    const { effectiveCategory: deskCategory } = await import(
      '../../../web/src/lib/view-config'
    )
    const { effectiveCategory } = await import('./domain')
    const phoneRow = (status_category: string) =>
      ({ status_category }) as Parameters<typeof effectiveCategory>[0]
    for (const raw of ['nosuch-key', '']) {
      expect(deskCategory(raw), `the desk folds ${JSON.stringify(raw)}`).toBe('new')
      expect(
        effectiveCategory(phoneRow(raw)),
        `the phone folds ${JSON.stringify(raw)}`,
      ).toBe('new')
    }
  })
})

describe('relative-time ladder stays parity with the desktop', () => {
  // GDK-1927: relTime and the desk's relativeTimeParts are two ladders over
  // the same catalog keys (time.justNow/minute/hour/day). Where they cut is
  // a shared fact — "5m" must mean the same minute on both surfaces — so
  // the desk's thresholds are parsed from its source and asserted equal,
  // the same string-marker pattern as the status-alias test above. What
  // stays deliberately unpinned: the desk continues past a week (2w/1mo)
  // while the phone shows the calendar date — that divergence is the
  // comment on relTime, and merging the ladders is the lead's call, not
  // this gate's.
  it('cuts the four catalog-keyed stages at the same thresholds', () => {
    const owner = readFileSync(join(srcDir, '../../web/src/lib/i18n/index.ts'), 'utf8')
    // The desk's ladder constants, in declaration order (60_000, 60 * MIN,
    // 24 * HOUR) — resolved against each other, never restated here.
    const consts: Record<string, number> = {}
    for (const m of owner.matchAll(/const (MIN|HOUR|DAY) = (.+)$/gm)) {
      let v = 1
      for (const term of m[2].replace(/_/g, '').split('*')) {
        const t = term.trim()
        v *= /^[0-9]+$/.test(t) ? Number(t) : consts[t]
      }
      consts[m[1]] = v
    }
    expect(consts.DAY, 'the desk ladder constants parsed').toBeGreaterThan(0)
    const webCuts = [...owner.matchAll(/diff < (MIN|HOUR|DAY)\)/g)].map(
      (m) => consts[m[1]] / 1000,
    )
    const dayCut = owner.match(/days < (\d+)/)
    expect(dayCut, 'the desk day-stage cutoff parsed').not.toBeNull()
    webCuts.push((Number(dayCut![1]) * consts.DAY) / 1000)
    expect(webCuts, 'four desk stages parsed').toHaveLength(4)

    const phone = readFileSync(join(srcDir, 'lib/domain.ts'), 'utf8')
    const body = phone.slice(phone.indexOf('export function relTime'))
    const rel = body.slice(0, body.indexOf('\n}'))
    const stages = [...rel.matchAll(/sec < (.+?)\) return t\('time\.(justNow|minute|hour|day)'/g)]
    expect(stages.map((m) => m[2]), 'the phone ladder keys, in stage order').toEqual([
      'justNow',
      'minute',
      'hour',
      'day',
    ])
    const phoneCuts = stages.map((m) => {
      let v = 1
      for (const term of m[1].split('*')) v *= Number(term.trim())
      return v
    })
    expect(phoneCuts, 'four phone stages parsed, all numeric').toHaveLength(4)
    expect(phoneCuts.every(Number.isFinite)).toBe(true)
    expect(phoneCuts, 'the same minute/hour/day/week on both surfaces').toEqual(webCuts)
  })
})
