/*
 * Narrowing the list you are looking at (GDK-1994).
 *
 * Until this round the phone had no way to narrow the list on screen. Search
 * is not narrowing — `matchLocal` runs over the whole snapshot, not the
 * scope — so "just P1" or "just this project" meant going back to the desk
 * and saving a view there. These pin the two halves of the answer: which
 * toggles are worth offering for the rows in hand, and where in `buildList`
 * the toggle lands.
 */
import { describe, it, expect } from 'vitest'
import {
  buildList,
  buildScopes,
  matchesFilters,
  narrowCount,
  narrowFacets,
  narrowHas,
  toggleNarrow,
  SCOPE_MY_WORK,
  type NarrowRow,
  type Scope,
} from './domain'
import type { IssueLite, Me, ViewFilters } from './types'

const me: Me = { email: 'dev@example.com', account_id: 'acct-1', name: 'Dev' }

// STD-*, never GDK-*: repo doc-checks scans test files for issue keys.
function issue(over: Partial<IssueLite> & { issue_key: string }): IssueLite {
  return {
    summary: 'a summary',
    project_key: 'STD',
    issue_type: 'Task',
    issue_type_id: '10001',
    status: 'Open',
    status_id: '1',
    status_category: 'new',
    priority: 'Medium',
    priority_id: '3',
    priority_rank: 3,
    assignee: null,
    assignee_id: null,
    assignee_email: null,
    reporter: null,
    reporter_email: null,
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-10T00:00:00Z',
    status_changed_at: null,
    comment_count: 0,
    reopen_count: 0,
    duedate: null,
    labels: [],
    components: [],
    fix_versions: [],
    parent_key: null,
    epic_key: null,
    ...over,
  } as IssueLite
}

/*
 * Every row is mine and open, so the two axes that cannot discriminate are
 * genuinely uniform — that is what the offering rule is measured against.
 * Priority, type, project and the reopen flag each cut the set differently.
 */
const rows: IssueLite[] = [
  issue({ issue_key: 'STD-1', assignee_id: 'acct-1', priority: 'High', priority_id: '2', priority_rank: 2 }),
  issue({ issue_key: 'STD-2', assignee_id: 'acct-1', reopen_count: 3 }),
  issue({
    issue_key: 'OPS-3',
    assignee_id: 'acct-1',
    issue_type: 'Bug',
    issue_type_id: '10004',
  }),
]

function rowOf(key: string): NarrowRow {
  for (const section of narrowFacets(rows, me)) {
    const hit = section.rows.find((r) => r.key === key)
    if (hit) return hit
  }
  throw new Error(`no toggle ${key}`)
}

describe('which toggles are offered (GDK-1994)', () => {
  it('a toggle that cannot change the list is not offered', () => {
    const facets = narrowFacets(rows, me)
    const keys = facets.flatMap((s) => s.rows.map((r) => r.key))
    // `mine` matches all three and `status_category:new` matches all three —
    // tapping either would paint the same list, so neither is a control.
    expect(keys).not.toContain('mine')
    expect(keys).not.toContain('status_category:new')
    // Nothing is unassigned here, so that toggle would empty the screen.
    expect(keys).not.toContain('unassigned')
    // These three each cut the set.
    expect(keys).toContain('reopened')
    expect(keys).toContain('priority:2')
    expect(keys).toContain('issue_type:10004')
    expect(keys).toContain('jira_project:OPS')
  })

  it('an empty list offers nothing at all', () => {
    expect(narrowFacets([], me)).toEqual([])
  })

  it('every discovered value matches the rows it was counted from', () => {
    // The trap this closes: a facet keyed on the display name while the
    // filter compares ids (or the reverse) offers a toggle that empties the
    // list. Each row's own count is re-derived through the filter itself.
    for (const section of narrowFacets(rows, me)) {
      for (const row of section.rows) {
        const f = toggleNarrow({}, row)
        const hits = rows.filter((i) => matchesFilters(i, f, me)).length
        expect(hits, `${row.key} count`).toBe(row.count)
      }
    }
  })

  it('a value section reads in the axis’ own order, not discovery order', () => {
    const priorities = narrowFacets(rows, me).find((s) => s.id === 'priority')
    // High (rank 2) before Medium (rank 3), though Medium was seen second.
    expect(priorities?.rows.map((r) => r.value)).toEqual(['2', '3'])
  })
})

describe('what a toggle does to the narrow (GDK-1994)', () => {
  it('a second value on one axis widens it, a second axis narrows', () => {
    const one = toggleNarrow({}, rowOf('priority:2'))
    expect(one).toEqual({ priority: ['2'] })
    const two = toggleNarrow(one, rowOf('priority:3'))
    expect(two).toEqual({ priority: ['2', '3'] })
    const andType = toggleNarrow(two, rowOf('issue_type:10004'))
    expect(andType).toEqual({ priority: ['2', '3'], issue_type: ['10004'] })
    expect(narrowCount(andType)).toBe(2)
  })

  it('the last value off removes the axis, never leaves an empty array', () => {
    // An empty array is the desk's "unset": left behind it would read as a
    // filter that matches everything while looking like one that matches
    // nothing.
    const on = toggleNarrow({}, rowOf('priority:2'))
    const off = toggleNarrow(on, rowOf('priority:2'))
    expect(off).toEqual({})
    expect('priority' in off).toBe(false)
    expect(narrowCount(off)).toBe(0)
  })

  it('a flag toggles on and off, and narrowHas reads either kind', () => {
    const on = toggleNarrow({}, rowOf('reopened'))
    expect(on).toEqual({ reopened: true })
    expect(narrowHas(on, rowOf('reopened'))).toBe(true)
    expect(narrowHas(on, rowOf('priority:2'))).toBe(false)
    expect(toggleNarrow(on, rowOf('reopened'))).toEqual({})
  })
})

describe('where the narrow lands in buildList (GDK-1994)', () => {
  function scopeOf(id: string): Scope {
    const hit = buildScopes([], [], me).find((s) => s.id.endsWith(id))
    if (!hit) throw new Error(`no scope ${id}`)
    return hit
  }

  it('the heading count is the narrowed count', () => {
    const scope = scopeOf('all-open')
    expect(buildList(rows, me, scope).total).toBe(3)
    const narrow: Partial<ViewFilters> = { priority: ['2'] }
    const view = buildList(rows, me, scope, narrow)
    expect(view.total).toBe(1)
    expect(view.sections.flatMap((s) => s.issues.map((i) => i.issue_key))).toEqual(['STD-1'])
  })

  it('a narrow that empties My issues does not fall back to All open', () => {
    // The placement contract: the fallback exists for the first-run empty
    // plate, so firing it on a filter the reader just set would answer a
    // question nobody asked, under a name they did not choose.
    const scope = scopeOf(SCOPE_MY_WORK.split(':')[1])
    expect(buildList(rows, me, scope).fellBack).toBe(false)
    const view = buildList(rows, me, scope, { issue_type: ['nothing-matches'] })
    expect(view.fellBack).toBe(false)
    expect(view.scopeId).toBe(SCOPE_MY_WORK)
    expect(view.total).toBe(0)
  })

  it('an empty narrow paints exactly the view’s own list', () => {
    const scope = scopeOf('all-open')
    expect(buildList(rows, me, scope, {})).toEqual(buildList(rows, me, scope))
  })
})
