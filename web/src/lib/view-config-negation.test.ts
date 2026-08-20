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
  NEGATABLE_MULTI,
  NEGATION_FIELDS,
  NEGATION_KEY,
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

  test('negation registry: every negatable field has a twin, key, and base', () => {
    expect(NEGATABLE_MULTI).toEqual(['jira_project', 'source_project'])
    expect(NEGATION_FIELDS).toEqual(['jira_project_not', 'source_project_not'])
    expect(NEGATION_KEY).toEqual({ jira_project_not: 'pjn', source_project_not: 'spjn' })
    expect(negationOf('jira_project')).toBe('jira_project_not')
    expect(negationOf('source_project')).toBe('source_project_not')
    // Axes without negation answer null — the UI hides the exclude toggle there.
    expect(negationOf('status')).toBeNull()
    expect(negationOf('labels')).toBeNull()
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
