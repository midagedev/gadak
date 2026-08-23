/*
 * GDK-438 — project filter negation (`pjn`/`spjn`, `<field>_not` config keys).
 *
 * Covers the three contract areas the task spec names:
 *   1. serialization round-trip (config → URL params → config, negation included)
 *   2. matching semantics (include∩ minus exclude; exclude wins on overlap)
 *   3. legacy invariance (URLs/saved views with only the old keys parse identically)
 *
 * The matching seam itself is `matchesMulti` — the single semantic owner the
 * filter application (stores/filters.svelte.ts filterIssues) calls. This file
 * stays in the default `unit` vitest project on purpose: it must not import
 * the store graph (runes), which only the pages-store project compiles.
 * End-to-end filterIssues coverage would have to live there (see report).
 */
import { describe, expect, test } from 'vitest'
import {
  MULTI_FIELDS,
  NEGATABLE_MULTI,
  NEGATION_BASE,
  NEGATION_FIELDS,
  NEGATION_KEY,
  VIEW_PARAM_KEYS,
  configToParams,
  emptyConfig,
  hasAnyFilter,
  matchesMulti,
  negationOf,
  parseConfig,
} from './view-config'

/** Drop nulls the way setParams does (same helper as view-config.test.ts). */
function paramsOf(c: ReturnType<typeof emptyConfig>): URLSearchParams {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(configToParams(c))) {
    if (v !== null) sp.set(k, v)
  }
  return sp
}

function roundTrip(c: ReturnType<typeof emptyConfig>): ReturnType<typeof emptyConfig> {
  return parseConfig(paramsOf(c))
}

describe('GDK-438 serialization round-trip', () => {
  test('negation lists survive config → params → config', () => {
    const c = emptyConfig()
    c.filters.jira_project = ['ABC', 'NMB']
    c.filters.jira_project_not = ['XYZ']
    c.filters.source_project = ['demo']
    c.filters.source_project_not = ['scratch', 'archive']
    const back = roundTrip(c)
    expect(back.filters.jira_project).toEqual(['ABC', 'NMB'])
    expect(back.filters.jira_project_not).toEqual(['XYZ'])
    expect(back.filters.source_project).toEqual(['demo'])
    expect(back.filters.source_project_not).toEqual(['scratch', 'archive'])
  })

  test('short param names are pjn / spjn (URL contract pin)', () => {
    const c = emptyConfig()
    c.filters.jira_project_not = ['XYZ']
    c.filters.source_project_not = ['SRC2']
    const p = configToParams(c)
    expect(p.pjn).toBe('XYZ')
    expect(p.spjn).toBe('SRC2')
  })

  test('negation params are recognized view params (viewKey keeps them)', () => {
    // isViewParam gates the store's #viewKey reparse — a negation param that
    // is not a view param would be dropped between URL and store.
    const sp = new URLSearchParams('pjn=XYZ&spjn=SRC')
    expect([...sp.keys()].every((k) => k === 'pjn' || k === 'spjn')).toBe(true)
    const parsed = parseConfig(sp)
    expect(parsed.filters.jira_project_not).toEqual(['XYZ'])
    expect(parsed.filters.source_project_not).toEqual(['SRC'])
  })

  test('defaults stay omitted (pjn/spjn null on an empty config)', () => {
    const p = configToParams(emptyConfig())
    expect(p.pjn).toBeNull()
    expect(p.spjn).toBeNull()
  })

  test('empty negation param parses to no constraint', () => {
    const parsed = parseConfig(new URLSearchParams('pj=ABC&pjn='))
    expect(parsed.filters.jira_project).toEqual(['ABC'])
    expect(parsed.filters.jira_project_not).toEqual([])
  })

  test('negation-only view counts as a filter (save-view button)', () => {
    const c = emptyConfig()
    expect(hasAnyFilter(c.filters)).toBe(false)
    c.filters.jira_project_not = ['XYZ']
    expect(hasAnyFilter(c.filters)).toBe(true)
  })

  // Contract rewrite (GDK-771, 2026-08-24): the previous assertion pinned
  // negation to the two project axes — the half-adoption GDK-438 itself
  // warned about ("why here and not there"). It was green on the pre-change
  // source; every visible multi axis now has a twin.
  test('negation registry: every visible multi field has a twin, key, and base', () => {
    expect(NEGATABLE_MULTI).toEqual(MULTI_FIELDS.filter((f) => f !== 'keys' && f !== 'parent'))
    expect(NEGATION_FIELDS).toEqual(NEGATABLE_MULTI.map((f) => `${f}_not`))
    // Derived URL keys: include key + 'n', the two originals unchanged.
    expect(NEGATION_KEY.jira_project_not).toBe('pjn')
    expect(NEGATION_KEY.source_project_not).toBe('spjn')
    expect(NEGATION_KEY.status_not).toBe('stn')
    expect(NEGATION_KEY.labels_not).toBe('lbn')
    for (const f of NEGATABLE_MULTI) {
      expect(negationOf(f)).toBe(`${f}_not`)
      expect(NEGATION_BASE[`${f}_not` as keyof typeof NEGATION_BASE]).toBe(f)
    }
    // Hidden (non-picker) axes stay include-only.
    expect(negationOf('keys')).toBeNull()
    expect(negationOf('parent')).toBeNull()
  })

  test('all view param keys stay unique after the twin expansion', () => {
    const keys = VIEW_PARAM_KEYS as readonly string[]
    expect(new Set(keys).size).toBe(keys.length)
  })

  test('a new-axis exclusion round-trips through the URL', () => {
    const c = emptyConfig()
    c.filters.status_not = ['10001']
    c.filters.labels_not = ['noise']
    const p = configToParams(c)
    expect(p.stn).toBe('10001')
    expect(p.lbn).toBe('noise')
    const parsed = roundTrip(c)
    expect(parsed.filters.status_not).toEqual(['10001'])
    expect(parsed.filters.labels_not).toEqual(['noise'])
  })
})

describe('GDK-438 matching semantics (matchesMulti)', () => {
  test('include-only behaves like the old single-list match', () => {
    expect(matchesMulti(['ABC'], [], 'ABC')).toBe(true)
    expect(matchesMulti(['ABC'], [], 'XYZ')).toBe(false)
    expect(matchesMulti([], [], 'ABC')).toBe(true) // no constraint
  })

  test('exclude-only subtracts from everything', () => {
    expect(matchesMulti([], ['XYZ'], 'ABC')).toBe(true)
    expect(matchesMulti([], ['XYZ'], 'XYZ')).toBe(false)
  })

  test('include first, then exclude (intersection minus difference)', () => {
    expect(matchesMulti(['ABC', 'NMB'], ['NMB'], 'ABC')).toBe(true)
    expect(matchesMulti(['ABC', 'NMB'], ['NMB'], 'NMB')).toBe(false)
    expect(matchesMulti(['ABC', 'NMB'], ['NMB'], 'XYZ')).toBe(false) // outside the include set
  })

  test('the same value in both lists is excluded — exclude wins', () => {
    expect(matchesMulti(['ABC'], ['ABC'], 'ABC')).toBe(false)
  })

  test('empty row token (no project) parity with the old include semantics', () => {
    // Old: `f.source_project.length && !(it.source_project && …includes)` — a
    // non-empty include list dropped rows without a project. Keep that.
    expect(matchesMulti(['SRC'], [], '')).toBe(false)
    // But exclusion only rejects rows whose value IS in the list: "everything
    // except SRC" keeps rows with no project at all.
    expect(matchesMulti([], ['SRC'], '')).toBe(true)
  })
})

describe('GDK-438 legacy invariance (old keys only)', () => {
  test('a legacy URL parses exactly as before the negation keys existed', () => {
    const parsed = parseConfig(new URLSearchParams('pj=ABC&spj=demo&sc=new&st=3&ks=NMB-1'))
    expect(parsed.filters.jira_project).toEqual(['ABC'])
    expect(parsed.filters.source_project).toEqual(['demo'])
    expect(parsed.filters.status_category).toEqual(['new'])
    expect(parsed.filters.status).toEqual(['3'])
    expect(parsed.filters.keys).toEqual(['NMB-1'])
    expect(parsed.filters.jira_project_not).toEqual([])
    expect(parsed.filters.source_project_not).toEqual([])
    // And re-serializes without inventing negation params.
    const p = configToParams(parsed)
    expect(p.pjn).toBeNull()
    expect(p.spjn).toBeNull()
    expect(p.pj).toBe('ABC')
    expect(p.spj).toBe('demo')
  })

  test('pj/spj meaning is unchanged when a negation is also present', () => {
    const parsed = parseConfig(new URLSearchParams('pj=ABC&pjn=XYZ'))
    expect(parsed.filters.jira_project).toEqual(['ABC'])
    expect(parsed.filters.jira_project_not).toEqual(['XYZ'])
  })
})
