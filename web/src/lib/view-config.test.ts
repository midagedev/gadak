import { describe, expect, test } from 'vitest'
import {
  KEYS_CAP,
  configToParams,
  defaultGroupBy,
  emptyConfig,
  matchesIdFirst,
  orderColumns,
  parseConfig,
  prioritySortRank,
  type ViewConfig,
} from './view-config'

/**
 * Drop nulls the way setParams does (web/src/lib/router.svelte.ts:79–80).
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

  test('KEYS_CAP is enforced on parse', () => {
    const keys = Array.from({ length: KEYS_CAP + 3 }, (_, i) => `NMB-${i + 1}`)
    const parsed = parseConfig(new URLSearchParams({ ks: keys.join(',') }))
    expect(KEYS_CAP).toBe(500)
    expect(parsed.filters.keys).toHaveLength(KEYS_CAP)
    expect(parsed.filters.keys[0]).toBe('NMB-1')
    expect(parsed.filters.keys[KEYS_CAP - 1]).toBe(`NMB-${KEYS_CAP}`)
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
  })

  test('L1: rank 0 (unset) sorts below Highest (1)', () => {
    expect(prioritySortRank(0)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(null)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(undefined)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(1)).toBe(1)
    expect(prioritySortRank(1)).toBeLessThan(prioritySortRank(0))
  })
})
