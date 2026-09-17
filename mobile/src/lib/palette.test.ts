import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  CAP,
  matchScopes,
  offersTerminal,
  paletteMode,
  QUERY_MIN,
  recentIssueRows,
  scopeGroups,
  SECTION_HEADING,
  SECTION_ORDER,
} from './palette'
import type { Scope, ScopeSection } from './domain'
import type { IssueLite, VisitedRow } from './types'

function scope(over: Partial<Scope> & Pick<Scope, 'id' | 'name'>): Scope {
  return {
    section: 'views',
    kind: 'issues',
    filters: null,
    unsupported: [],
    order: null,
    ...over,
  }
}

function issue(key: string): IssueLite {
  return { issue_key: key, summary: key } as IssueLite
}

function visit(key: string): VisitedRow {
  return { key, viewed_at: '2026-09-15T00:00:00Z' } as VisitedRow
}

describe('GDK-902 the palette decides which ranking the body draws', () => {
  it('separates "no query" from "not yet a query"', () => {
    // Three states, not two: an empty field draws the owner list, one
    // character draws neither (a single letter matches most of a mirror and
    // the round trip it would start is wasted), and two draws results.
    expect(paletteMode('')).toBe('empty')
    expect(paletteMode('   ')).toBe('empty')
    expect(paletteMode('t')).toBe('short')
    expect(paletteMode(' t ')).toBe('short')
    expect(paletteMode('te')).toBe('query')
    expect(QUERY_MIN).toBe(2)
  })
})

describe('GDK-902 the owner list is the desk\'s sections, in the desk\'s order', () => {
  const scopes = [
    scope({ id: 'b1', name: 'My issues', section: 'builtin' }),
    scope({ id: 'f1', name: 'Escalations', section: 'filters' }),
    scope({ id: 'd1', name: 'Updated', section: 'docs' }),
  ]

  it('orders builtin, views, filters, docs — and heads each from the catalog', () => {
    expect(SECTION_ORDER).toEqual(['builtin', 'views', 'filters', 'docs'])
    expect(scopeGroups(scopes).map((g) => g.section)).toEqual([
      'builtin',
      'views',
      'filters',
      'docs',
    ])
    for (const section of SECTION_ORDER) {
      expect(SECTION_HEADING[section].startsWith('sidebar.')).toBe(true)
    }
  })

  it('drops an empty section — except saved views, which survives empty', () => {
    // GDK-1874: authoring a view is desk-only, and zero saved views is
    // exactly the state in which someone goes looking for where one is
    // made. Every other section has nothing to say the desk row does not.
    const only = scopeGroups([scope({ id: 'b1', name: 'My issues', section: 'builtin' })])
    expect(only.map((g) => g.section)).toEqual(['builtin', 'views'])
    expect(only.find((g) => g.section === 'views')!.rows).toEqual([])
  })

  it('caps a section at eight and reports the total so "show all N" can be offered', () => {
    const many = Array.from({ length: 11 }, (_, i) =>
      scope({ id: `v${i}`, name: `View ${i}`, section: 'views' }),
    )
    const [views] = scopeGroups(many).filter((g) => g.section === 'views')
    expect(CAP).toBe(8)
    expect(views.rows).toHaveLength(8)
    expect(views.total).toBe(11)

    const expanded = scopeGroups(many, new Set<ScopeSection>(['views']))
    expect(expanded.find((g) => g.section === 'views')!.rows).toHaveLength(11)
  })
})

describe('GDK-902 the Terminal row exists only with a stored terminal pairing', () => {
  it('is absence, not a disabled row (DESIGN.md §10)', () => {
    // The contract the tab bar's count used to assert: a default install
    // offers no shell at all, the same stance PairGate takes toward an
    // unpaired app. A greyed-out row would promise a capability the phone
    // cannot reach.
    expect(offersTerminal(null)).toBe(false)
    expect(offersTerminal(undefined)).toBe(false)
    expect(offersTerminal({ endpoint: 'http://127.0.0.1:7899', label: 'This Mac' })).toBe(true)
  })
})

describe('GDK-902 a typed query matches owners by name', () => {
  const scopes = [
    scope({ id: 'b1', name: 'My issues', section: 'builtin' }),
    scope({ id: 'v1', name: 'Release blockers', section: 'views' }),
    scope({ id: 'f1', name: 'Escalations', section: 'filters' }),
  ]

  it('is a case-insensitive substring, not a fuzzy ranker', () => {
    // A phone list of eight is read, not scored. A fuzzy match that puts an
    // unrelated view first is the failure mode worth avoiding, so "rb" must
    // NOT reach "Release blockers".
    expect(matchScopes(scopes, 'RELEASE').map((s) => s.id)).toEqual(['v1'])
    expect(matchScopes(scopes, 'ion').map((s) => s.id)).toEqual(['f1'])
    expect(matchScopes(scopes, 'rb')).toEqual([])
  })

  it('answers nothing below the threshold the field uses', () => {
    expect(matchScopes(scopes, 'm')).toEqual([])
    expect(matchScopes(scopes, '')).toEqual([])
  })
})

describe('GDK-902 the empty plate leads with the visit ledger', () => {
  const issues = [issue('STD-1'), issue('STD-2'), issue('STD-3')]

  it('keeps the ledger\'s order and skips a key the pool no longer carries', () => {
    const rows = recentIssueRows([visit('STD-3'), visit('GONE-9'), visit('STD-1')], issues)
    expect(rows.map((r) => r.issue_key)).toEqual(['STD-3', 'STD-1'])
  })

  it('caps at the five the query recents take — one grammar for the plate', () => {
    const pool = Array.from({ length: 9 }, (_, i) => issue(`STD-${i}`))
    const visits = pool.map((p) => visit(p.issue_key))
    expect(recentIssueRows(visits, pool)).toHaveLength(5)
  })
})

/*
 * The plate claims Search.test.ts made, re-pointed (GDK-902 2026-09-15).
 *
 * screens/Search.test.ts died with the screen it scanned. Its four claims
 * did not: the palette draws the same four plates from the same catalog
 * keys, so they are asserted here against the component that draws them
 * now. Nothing is loosened — only the path changed.
 */
describe('GDK-905 the palette\'s plates are distinct and catalog-backed', () => {
  const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '../ui/Palette.svelte'), 'utf8')

  it('holds a searching flag and does not paint empty over an open fetch', () => {
    expect(src).toContain('searchPaint')
    expect(src).toMatch(/searching\s*=/)
    expect(src).not.toMatch(/else if results\.length === 0 && serverPages\.length === 0/)
  })

  it('records a failed server search instead of swallowing it', () => {
    expect(src).toMatch(/searchFailed\s*=\s*true/)
    expect(src).toContain("t('list.searchFailed')")
  })

  it('uses catalog keys for clear and for no-results', () => {
    // GDK-1928: the clear × owns app.searchClear — same word as the desk's
    // clear-X, but without list.searchClear's "(Esc)" hint a phone cannot
    // honor. Still a catalog key, which is what this test pins.
    expect(src).toContain("t('app.searchClear')")
    expect(src).toContain("t('common.noResults')")
    expect(src).not.toContain('aria-label="Clear search"')
    expect(src).not.toContain('title="No matches"')
  })

  it('does not center a brochure hero on the idle empty screen', () => {
    expect(src).not.toContain('Search the mirror')
    // The empty query draws the owner list, and a query too short to run
    // draws a hint — neither is an EmptyState brochure.
    expect(src).toMatch(/idle-hint|idleHint/)
  })

  it('names its own dismiss, and does not borrow the Sheet\'s', () => {
    // GDK-902: `button.cancel` belongs to Sheet.svelte, which registers
    // itself with systemBack. The palette is not a sheet (see back.test.ts)
    // and must not answer that selector — twenty-odd specs click it.
    expect(src).toContain('palette-cancel')
    expect(src).not.toMatch(/class="cancel"/)
    expect(src).toContain("t('common.cancel')")
  })
})

describe('GDK-902 2026-09-15 the typed query labels the owners it matched', () => {
  const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '../ui/Palette.svelte'), 'utf8')

  it('puts a section label above the scope rows in the typed branch', () => {
    // The matched owners used to lead unlabeled, so the first rows of a
    // typed query belonged to no section while everything under them did.
    // The desk's own palette calls this exact set Views
    // (web/src/components/palette/CommandPalette.svelte:899), so the word
    // is the catalog's and not the phone's to invent (DESIGN.md §3.6).
    const typedBranch = src.indexOf("{:else if mode === 'short'}")
    const label = src.indexOf("t('palette.sectionViews')")
    const rows = src.indexOf('{#each scopeHits as scope')
    expect(typedBranch).toBeGreaterThan(-1)
    expect(rows).toBeGreaterThan(-1)
    expect(label).toBeGreaterThan(typedBranch)
    expect(label).toBeLessThan(rows)
  })

  it('no longer explains why the matched owners have no word', () => {
    expect(src).not.toContain('unlabeled on purpose')
  })
})
