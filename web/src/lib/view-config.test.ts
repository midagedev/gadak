import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en } from './i18n/en'
import { ko } from './i18n/ko'
import type { IssueLite } from './types'
import {
  KEYS_CAP,
  configToParams,
  defaultGroupBy,
  categoryFallbackSeen,
  effectiveCategory,
  emptyConfig,
  isReopen,
  matchesIdFirst,
  missingStatusCategorySeen,
  priorityNameFallbackSeen,
  normalizeKeys,
  orderColumns,
  parseConfig,
  parseView,
  prioritySortRank,
  type ViewConfig,
} from './view-config'

/*
 * GDK-35 contract coverage (clause → assertion names):
 *
 *   cap value
 *     KEYS_CAP is 500
 *     KEYS_CAP matches jql.MaxKeys
 *
 *   first-500 ordering
 *     KEYS_CAP is enforced on parse
 *     normalizeKeys keeps the first KEYS_CAP in given order
 *
 *   de-dupe still first-wins
 *     parse normalizes keys: trim, uppercase, first-wins de-dupe
 *     normalizeKeys first-wins before the cap (dupe after 500 unique is not a new slot)
 *
 *   truncation observable in the return
 *     normalizeKeys reports given on overflow
 *     parseView surfaces the same given as normalizeKeys
 *
 *   exactly-500 input does NOT report truncation (off-by-one)
 *     normalizeKeys at KEYS_CAP is not truncated
 *     parseView of exactly KEYS_CAP keys is not truncated
 *
 *   501 input DOES report truncation
 *     normalizeKeys at KEYS_CAP+1 is truncated
 *     parseView of KEYS_CAP+1 keys is truncated
 *
 *   both catalogs have the new keys
 *     en catalog has filter.keysCapped with the CLI wording
 *     ko catalog has filter.keysCapped with 키 (not 이슈)
 */

/**
 * Drop nulls the way setParams does (web/src/lib/router.svelte.ts:60–61).
 * `new URLSearchParams(configToParams(c))` would stringify null as "null"
 * and is not what the app does.
 */
function paramsOf(c: ViewConfig): URLSearchParams {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(configToParams(c))) {
    if (v !== null) sp.set(k, v)
  }
  return sp
}

function roundTrip(c: ViewConfig): ViewConfig {
  return parseConfig(paramsOf(c))
}

/**
 * "Normalized" = a config that is already in parseConfig's output shape:
 * - every multi-value field is an array (empty if unset)
 * - keys have been through normalizeKeys (trim, uppercase, first-wins de-dupe, KEYS_CAP)
 * - fields contains only aliases with a non-empty value list
 * - booleans / dates / q filled (false / null / '')
 * - display.group_by / sort / dir / columns filled; columns in catalog order
 *
 * Round-trip is parse(serialize(c)) deep-equals c only when c is already
 * normalized — serialize does not uppercase keys or reorder columns.
 */
function normalized(mutate: (c: ViewConfig) => void): ViewConfig {
  const c = emptyConfig()
  mutate(c)
  return c
}

describe('view-config URL contract', () => {
  test('round-trip: parse(serialize(c)) equals a normalized c', () => {
    const cases: { name: string; config: ViewConfig }[] = [
      { name: 'empty / defaults', config: normalized(() => {}) },
      {
        name: 'keys view (implicit g=none)',
        config: normalized((c) => {
          c.filters.keys = ['NMA-11', 'NMA-1', 'NMA-118']
          c.display.group_by = 'none'
        }),
      },
      {
        name: 'keys view + explicit g=status_category',
        config: normalized((c) => {
          c.filters.keys = ['NMB-1']
          c.display.group_by = 'status_category'
        }),
      },
      {
        name: 'saved sort + dir',
        config: normalized((c) => {
          c.display.sort = 'priority'
          c.display.dir = 'asc'
        }),
      },
      {
        name: 'date ranges',
        config: normalized((c) => {
          c.filters.created_from = '2026-08-01'
          c.filters.created_to = '2026-08-15'
          c.filters.updated_from = '2026-07-01'
          c.filters.updated_to = '2026-07-31'
          c.filters.due_from = '2026-08-20'
          c.filters.due_to = '2026-08-31'
          c.filters.resolved_from = '2026-08-17'
          c.filters.resolved_to = '2026-08-18'
        }),
      },
      {
        name: 'q text',
        config: normalized((c) => {
          c.filters.q = 'flaky upload'
        }),
      },
      {
        name: 'multi-select facets',
        config: normalized((c) => {
          c.filters.status_category = ['new', 'inprogress']
          c.filters.status = ['3']
          c.filters.assignee_email = ['demo-alex']
          c.filters.priority = ['High']
          c.filters.labels = ['batch', 'tech-debt']
          c.filters.issue_type = ['10004']
          c.filters.jira_project = ['NMB']
        }),
      },
      {
        name: 'flags + columns + discovered field',
        config: normalized((c) => {
          c.filters.reopened = true
          c.filters.unassigned = true
          c.filters.stale = true
          c.filters.fields = { story_points: ['8'] }
          c.display.columns = orderColumns(['stale', 'assignee', 'labels'])
          c.display.group_by = 'assignee'
          c.display.sort = 'created'
        }),
      },
    ]
    expect(cases.length).toBeGreaterThanOrEqual(8)
    for (const { name, config } of cases) {
      expect(roundTrip(config), name).toEqual(config)
    }
  })

  test('defaults are omitted from params (URLs stay short)', () => {
    const p = configToParams(emptyConfig())
    // emptyConfig is the contextual default: no g/s/d/cl/q/flags/ranges/facets.
    expect(p.g).toBeNull()
    expect(p.s).toBeNull()
    expect(p.d).toBeNull()
    expect(p.cl).toBeNull()
    expect(p.q).toBeNull()
    expect(p.fl).toBeNull()
    expect(p.cf).toBeNull()
    expect(p.ct).toBeNull()
    expect(p.uf).toBeNull()
    expect(p.ut).toBeNull()
    expect(p.df).toBeNull()
    expect(p.dt).toBeNull()
    expect(p.rf).toBeNull()
    expect(p.rt).toBeNull()
    expect(p.ks).toBeNull()
    expect(p.sc).toBeNull()
    expect(p.st).toBeNull()
    expect(p.as).toBeNull()
    expect(p.rp).toBeNull()
    expect(p.pr).toBeNull()
    expect(p.lb).toBeNull()
    expect(Object.values(p).every((v) => v === null)).toBe(true)

    const keysNone = emptyConfig()
    keysNone.filters.keys = ['NMA-11', 'NMA-1']
    keysNone.display.group_by = 'none'
    expect(configToParams(keysNone).g).toBeNull()
    expect(configToParams(keysNone).s).toBeNull()
    expect(configToParams(keysNone).d).toBeNull()
  })

  test('defaultGroupBy: keys present → none; absent/empty/undefined → status_category', () => {
    expect(defaultGroupBy({ keys: ['NMB-1'] })).toBe('none')
    expect(defaultGroupBy({ keys: [] })).toBe('status_category')
    expect(defaultGroupBy({})).toBe('status_category')
    expect(defaultGroupBy(undefined)).toBe('status_category')
    expect(defaultGroupBy(null)).toBe('status_category')

    expect(parseConfig(new URLSearchParams('ks=NMB-1')).display.group_by).toBe('none')
    expect(parseConfig(new URLSearchParams('')).display.group_by).toBe('status_category')
    expect(parseConfig(new URLSearchParams('g=none')).display.group_by).toBe('none')

    // Jira-imported / pre-keys saved views omit filters.keys. SidebarNav
    // calls configToParams on every view at boot — must not throw.
    const legacy = emptyConfig()
    delete (legacy.filters as { keys?: string[] }).keys
    expect(() => configToParams(legacy)).not.toThrow()
    expect(configToParams(legacy).g).toBeNull()
  })

  test('explicit g=status_category on a keys view survives a round trip', () => {
    const c = normalized((cfg) => {
      cfg.filters.keys = ['NMA-11', 'NMA-1', 'NMA-118']
      cfg.display.group_by = 'status_category'
    })
    expect(configToParams(c).g).toBe('status_category')
    expect(roundTrip(c).display.group_by).toBe('status_category')
    expect(parseConfig(new URLSearchParams('ks=NMA-11,NMA-1,NMA-118&g=status_category')).display.group_by).toBe(
      'status_category',
    )
  })

  // GDK-35 (2026-08-15): the old assertion treated silent truncation as
  // correct. Silence was the bug — the cap and first-500 order stay; the
  // return must also say how many keys were given.
  test('KEYS_CAP is enforced on parse', () => {
    const keys = Array.from({ length: KEYS_CAP + 3 }, (_, i) => `NMB-${i + 1}`)
    const parsed = parseConfig(new URLSearchParams({ ks: keys.join(',') }))
    expect(KEYS_CAP).toBe(500)
    expect(parsed.filters.keys).toHaveLength(KEYS_CAP)
    expect(parsed.filters.keys[0]).toBe('NMB-1')
    expect(parsed.filters.keys[KEYS_CAP - 1]).toBe(`NMB-${KEYS_CAP}`)
    const nk = normalizeKeys(keys)
    expect(nk.given).toBe(KEYS_CAP + 3)
    expect(nk.keys).toEqual(parsed.filters.keys)
    const viewed = parseView(new URLSearchParams({ ks: keys.join(',') }))
    expect(viewed.keys.given).toBe(KEYS_CAP + 3)
    expect(viewed.keys.keys).toEqual(parsed.filters.keys)
    expect(viewed.config.filters.keys).toEqual(parsed.filters.keys)
  })

  test('parse normalizes keys: trim, uppercase, first-wins de-dupe', () => {
    const parsed = parseConfig(new URLSearchParams('ks= nmb-1 , NMB-1, nmb-2 '))
    expect(parsed.filters.keys).toEqual(['NMB-1', 'NMB-2'])
  })

  test('orderColumns keeps catalog order and drops unknown / feature-gated keys', () => {
    expect(orderColumns(['labels', 'assignee', 'nope'])).toEqual(['assignee', 'labels'])
    expect(orderColumns([])).toEqual([])
    // DEFAULTS leave qa/deploy/teamGroups off — those columns are not enabled.
    expect(orderColumns(['qa_impact', 'deploy', 'team_group', 'updated'])).toEqual(['updated'])
  })
})

describe('matchesIdFirst / prioritySortRank (moved from e2e/identity-web.spec.ts)', () => {
  test('I9: id wins; name is fallback; missing id still matches name', () => {
    expect(matchesIdFirst(['3'], '3', '진행 중')).toBe(true)
    expect(matchesIdFirst(['In Progress'], '3', '진행 중')).toBe(false)
    expect(matchesIdFirst(['진행 중'], '3', '진행 중')).toBe(true)
    expect(matchesIdFirst(['In Progress'], '', 'In Progress')).toBe(true)
    expect(matchesIdFirst(['In Progress'], undefined, 'In Progress')).toBe(true)
    expect(matchesIdFirst([], '3', '진행 중')).toBe(true)
    // Inputs the deleted e2e/identity-web I9 cases used: localized display
    // with an id token, and a name-only row with no status_id.
    expect(matchesIdFirst(['sid-1'], 'sid-1', '로캘-상태명')).toBe(true)
    expect(matchesIdFirst(['In Progress'], 'sid-1', '로캘-상태명')).toBe(false)
    expect(matchesIdFirst(['로캘-상태명'], 'sid-1', '로캘-상태명')).toBe(true)
    expect(matchesIdFirst(['LegacyNameOnly'], undefined, 'LegacyNameOnly')).toBe(true)
    expect(matchesIdFirst(['LegacyNameOnly'], '', 'LegacyNameOnly')).toBe(true)
    // Priority twin (moved from e2e/tail-audit): English selected name must
    // not match a localized display when an id is present on the row.
    expect(matchesIdFirst(['1'], '1', 'Highest')).toBe(true)
    expect(matchesIdFirst(['Highest'], '1', 'Highest')).toBe(true)
    expect(matchesIdFirst(['Highest'], '1', '최고')).toBe(false)
    expect(matchesIdFirst(['Highest'], '', 'Highest')).toBe(true)
    expect(matchesIdFirst(['Highest'], undefined, 'Highest')).toBe(true)
    expect(matchesIdFirst(['pri-1'], 'pri-1', '로캘-우선순위')).toBe(true)
    expect(matchesIdFirst(['Highest'], 'pri-1', '로캘-우선순위')).toBe(false)
  })

  test('L1: rank 0 (unset) sorts below Highest (1)', () => {
    expect(prioritySortRank(0)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(null)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(undefined)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(1)).toBe(1)
    expect(prioritySortRank(1)).toBeLessThan(prioritySortRank(0))
  })
})

describe('priority id-first filter (GDK-275)', () => {
  test('id token matches; name token still matches via legacy fallback', () => {
    expect(matchesIdFirst(['2'], '2', '높음')).toBe(true)
    expect(matchesIdFirst(['높음'], '2', '높음')).toBe(true)
    expect(matchesIdFirst(['High'], '2', '높음')).toBe(false)
    expect(matchesIdFirst(['High'], '', 'High')).toBe(true)
  })

  test('name fallback increments priorityNameFallbackSeen', () => {
    const before = priorityNameFallbackSeen()
    expect(matchesIdFirst(['High'], '', 'High', true)).toBe(true)
    expect(priorityNameFallbackSeen()).toBe(before + 1)
    expect(matchesIdFirst(['2'], '2', 'High', true)).toBe(true)
    expect(priorityNameFallbackSeen()).toBe(before + 1)
  })
})

function nKeys(n: number, start = 1): string[] {
  return Array.from({ length: n }, (_, i) => `NMB-${start + i}`)
}

describe('GDK-35 keys cap', () => {
  test('KEYS_CAP is 500', () => {
    expect(KEYS_CAP).toBe(500)
  })

  test('KEYS_CAP matches jql.MaxKeys', () => {
    const src = readFileSync(
      resolve(dirname(fileURLToPath(import.meta.url)), '../../../internal/jql/types.go'),
      'utf8',
    )
    const m = src.match(/^\s*const MaxKeys = (\d+)\s*$/m)
    expect(m, 'const MaxKeys = N in internal/jql/types.go').toBeTruthy()
    expect(Number(m![1])).toBe(KEYS_CAP)
  })

  test('normalizeKeys keeps the first KEYS_CAP in given order', () => {
    const raw = [...nKeys(KEYS_CAP + 2), 'NMB-1']
    const nk = normalizeKeys(raw)
    expect(nk.keys).toHaveLength(KEYS_CAP)
    expect(nk.keys[0]).toBe('NMB-1')
    expect(nk.keys[1]).toBe('NMB-2')
    expect(nk.keys[KEYS_CAP - 1]).toBe(`NMB-${KEYS_CAP}`)
    expect(nk.keys).not.toContain(`NMB-${KEYS_CAP + 1}`)
  })

  test('normalizeKeys first-wins before the cap (dupe after 500 unique is not a new slot)', () => {
    const raw = [...nKeys(KEYS_CAP), 'nmb-1', ' NMB-2 ', `NMB-${KEYS_CAP + 1}`]
    const nk = normalizeKeys(raw)
    expect(nk.given).toBe(KEYS_CAP + 1)
    expect(nk.keys).toHaveLength(KEYS_CAP)
    expect(nk.keys[0]).toBe('NMB-1')
    expect(nk.keys).not.toContain(`NMB-${KEYS_CAP + 1}`)
  })

  test('normalizeKeys reports given on overflow', () => {
    const raw = nKeys(KEYS_CAP + 7)
    const nk = normalizeKeys(raw)
    expect(nk.given).toBe(KEYS_CAP + 7)
    expect(nk.keys).toHaveLength(KEYS_CAP)
    expect(nk.given).toBeGreaterThan(nk.keys.length)
  })

  test('parseView surfaces the same given as normalizeKeys', () => {
    const raw = nKeys(KEYS_CAP + 4)
    const nk = normalizeKeys(raw)
    const viewed = parseView(new URLSearchParams({ ks: raw.join(',') }))
    expect(viewed.keys.given).toBe(nk.given)
    expect(viewed.keys.keys).toEqual(nk.keys)
    expect(viewed.config.filters.keys).toEqual(nk.keys)
  })

  test('normalizeKeys at KEYS_CAP is not truncated', () => {
    const raw = nKeys(KEYS_CAP)
    const nk = normalizeKeys(raw)
    expect(nk.given).toBe(KEYS_CAP)
    expect(nk.keys).toHaveLength(KEYS_CAP)
    expect(nk.given).toBe(nk.keys.length)
  })

  test('parseView of exactly KEYS_CAP keys is not truncated', () => {
    const raw = nKeys(KEYS_CAP)
    const viewed = parseView(new URLSearchParams({ ks: raw.join(',') }))
    expect(viewed.keys.given).toBe(KEYS_CAP)
    expect(viewed.keys.keys).toHaveLength(KEYS_CAP)
    expect(viewed.keys.given).toBe(viewed.keys.keys.length)
  })

  test('normalizeKeys at KEYS_CAP+1 is truncated', () => {
    const raw = nKeys(KEYS_CAP + 1)
    const nk = normalizeKeys(raw)
    expect(nk.given).toBe(KEYS_CAP + 1)
    expect(nk.keys).toHaveLength(KEYS_CAP)
    expect(nk.keys[KEYS_CAP - 1]).toBe(`NMB-${KEYS_CAP}`)
    expect(nk.keys).not.toContain(`NMB-${KEYS_CAP + 1}`)
  })

  test('parseView of KEYS_CAP+1 keys is truncated', () => {
    const raw = nKeys(KEYS_CAP + 1)
    const viewed = parseView(new URLSearchParams({ ks: raw.join(',') }))
    expect(viewed.keys.given).toBe(KEYS_CAP + 1)
    expect(viewed.keys.keys).toHaveLength(KEYS_CAP)
    expect(viewed.config.filters.keys).toHaveLength(KEYS_CAP)
  })

  test('empty and whitespace tokens do not inflate given', () => {
    const nk = normalizeKeys(['NMB-1', '', '  ', 'nmb-1', 'NMB-2'])
    expect(nk.given).toBe(2)
    expect(nk.keys).toEqual(['NMB-1', 'NMB-2'])
    expect(nk.given).toBe(nk.keys.length)
  })

  test('en catalog has filter.keysCapped with the CLI wording', () => {
    // CLI KeyLimitMessage (internal/jql/keys.go:21):
    //   "key list has %d values; the limit is %d"
    expect(en['filter.keysCapped']).toMatch(/^key list has \{given\} values; the limit is \{limit\}/)
    expect(en['filter.keysCapped']).toContain('{shown}')
    expect(en['filter.keysCapped']).toContain('keys')
    expect(en['filter.keysCapped']).not.toMatch(/issues/i)
  })

  test('ko catalog has filter.keysCapped with 키 (not 이슈)', () => {
    expect(ko['filter.keysCapped']).toContain('{given}')
    expect(ko['filter.keysCapped']).toContain('{limit}')
    expect(ko['filter.keysCapped']).toContain('{shown}')
    expect(ko['filter.keysCapped']).toContain('키')
    expect(ko['filter.keysCapped']).not.toContain('이슈')
  })
})

function issueRow(status: string, status_category: string): IssueLite {
  return { status, status_category } as IssueLite
}

describe('effectiveCategory (GDK-272)', () => {
  test('trusted category is returned as-is', () => {
    expect(effectiveCategory(issueRow('Anything', 'new'))).toBe('new')
    expect(effectiveCategory(issueRow('Anything', 'inprogress'))).toBe('inprogress')
    expect(effectiveCategory(issueRow('Anything', 'done'))).toBe('done')
  })

  test("empty category + status 완료 is not 'done'", () => {
    expect(effectiveCategory(issueRow('완료', ''))).not.toBe('done')
  })

  test("empty category + status Done is not 'done'", () => {
    expect(effectiveCategory(issueRow('Done', ''))).not.toBe('done')
  })

  test("empty category + status Shipped is not 'done'", () => {
    expect(effectiveCategory(issueRow('Shipped', ''))).not.toBe('done')
  })

  test('empty category increments missingStatusCategorySeen', () => {
    const before = missingStatusCategorySeen()
    effectiveCategory(issueRow('완료', ''))
    expect(missingStatusCategorySeen()).toBe(before + 1)
  })

  test('trusted category does not increment missingStatusCategorySeen', () => {
    const before = missingStatusCategorySeen()
    effectiveCategory(issueRow('완료', 'done'))
    expect(missingStatusCategorySeen()).toBe(before)
  })

  test('Jira REST key indeterminate folds to inprogress', () => {
    expect(effectiveCategory(issueRow('Anything', 'indeterminate'))).toBe('inprogress')
    expect(effectiveCategory('indeterminate')).toBe('inprogress')
    expect(effectiveCategory('INDETERMINATE')).toBe('inprogress')
  })

  test('migrated aliases: todo → new, complete/completed → done', () => {
    expect(effectiveCategory(issueRow('Anything', 'todo'))).toBe('new')
    expect(effectiveCategory('todo')).toBe('new')
    expect(effectiveCategory('TODO')).toBe('new')
    expect(effectiveCategory(issueRow('Anything', 'complete'))).toBe('done')
    expect(effectiveCategory('complete')).toBe('done')
    expect(effectiveCategory('completed')).toBe('done')
    expect(effectiveCategory('COMPLETED')).toBe('done')
  })

  test("display name 'to do' is not a category key (GDK-272)", () => {
    expect(effectiveCategory('to do')).toBe('inprogress')
    expect(effectiveCategory(issueRow('To Do', 'to do'))).toBe('inprogress')
  })

  test('unknown non-empty key increments categoryFallbackSeen', () => {
    const before = categoryFallbackSeen()
    expect(effectiveCategory(issueRow('Anything', 'undefined'))).toBe('inprogress')
    expect(categoryFallbackSeen()).toBe(before + 1)
  })

  test('trusted keys and folded aliases do not increment categoryFallbackSeen', () => {
    const before = categoryFallbackSeen()
    effectiveCategory(issueRow('Anything', 'new'))
    effectiveCategory(issueRow('Anything', 'inprogress'))
    effectiveCategory(issueRow('Anything', 'done'))
    effectiveCategory('indeterminate')
    effectiveCategory('todo')
    effectiveCategory('complete')
    effectiveCategory('completed')
    expect(categoryFallbackSeen()).toBe(before)
  })

  test('empty category increments missingStatusCategorySeen, not categoryFallbackSeen', () => {
    const missingBefore = missingStatusCategorySeen()
    const fallbackBefore = categoryFallbackSeen()
    effectiveCategory(issueRow('완료', ''))
    expect(missingStatusCategorySeen()).toBe(missingBefore + 1)
    expect(categoryFallbackSeen()).toBe(fallbackBefore)
  })
})

describe('isReopen (GDK-272)', () => {
  test('done-category → non-done is a reopen', () => {
    expect(isReopen({ field: 'status', from_category: 'done', to_category: 'inprogress' })).toBe(true)
    expect(isReopen({ field: 'status', from_category: 'done', to_category: 'new' })).toBe(true)
  })

  test('non-done → done is not a reopen', () => {
    expect(isReopen({ field: 'status', from_category: 'inprogress', to_category: 'done' })).toBe(false)
  })

  test('non-status field is never a reopen', () => {
    expect(isReopen({ field: 'assignee', from_category: 'done', to_category: 'inprogress' })).toBe(false)
  })

  test('both categories empty is not a reopen, even when names look resolved', () => {
    const korean = {
      field: 'status',
      from: '완료',
      to: '진행 중',
      from_category: null as string | null,
      to_category: null as string | null,
    }
    const english = {
      field: 'status',
      from: 'Done',
      to: 'To Do',
      from_category: '',
      to_category: '',
    }
    const custom = {
      field: 'status',
      from: 'Shipped',
      to: 'To Do',
      from_category: null as string | null,
      to_category: null as string | null,
    }
    expect(isReopen(korean)).toBe(false)
    expect(isReopen(english)).toBe(false)
    expect(isReopen(custom)).toBe(false)
  })

  test('empty categories increment missingStatusCategorySeen', () => {
    const before = missingStatusCategorySeen()
    isReopen({ field: 'status', from_category: null, to_category: null })
    expect(missingStatusCategorySeen()).toBe(before + 1)
  })
})
