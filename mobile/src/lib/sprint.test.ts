import { describe, expect, it } from 'vitest'
import { loadSprints, pickActiveSprint, sortSprints, sprintCounts, sprintDaysLeft } from './sprint'
import { utcZone } from '../../../web/src/lib/calendar'
import { ApiError } from './api'
import {
  buildList,
  buildScopes,
  resolveScope,
  SCOPE_ACTIVE_SPRINT,
  SCOPE_ALL_OPEN,
  scopeIssues,
  type Scope,
} from './domain'
import type { IssueLite, SprintRow } from './types'

// Fixture keys use STD-* (never GDK-*: repo doc-checks scans test files).
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
    // The Fields section's five (GDK-1870): required on the wire, so a
    // fixture that omits them is not a row the phone could ever receive.
    labels: [],
    components: [],
    fix_versions: [],
    parent_key: null,
    epic_key: null,
    sprint_id: null,
    sprint_name: null,
    sprint_state: null,
    ...over,
  }
}

function sprint(over: Partial<SprintRow> & { id: number }): SprintRow {
  return {
    board_id: 1,
    name: `Sprint ${over.id}`,
    goal: '',
    state: 'active',
    issue_count: 0,
    done: 0,
    in_progress: 0,
    todo: 0,
    ...over,
  }
}

describe('pickActiveSprint — which sprint "the sprint" means (GDK-1867)', () => {
  it('no rows at all → null: a kanban workspace has no line', () => {
    expect(pickActiveSprint([], [])).toBeNull()
  })

  it('rows but none active → null', () => {
    const rows = [sprint({ id: 41, state: 'closed' }), sprint({ id: 43, state: 'future' })]
    expect(pickActiveSprint(rows, [])).toBeNull()
  })

  it('keys on state, never on the sprint name', () => {
    // A closed sprint literally called "Active" must not be picked, and an
    // active one called "Backlog" must.
    const rows = [
      sprint({ id: 41, name: 'Active', state: 'closed' }),
      sprint({ id: 42, name: 'Backlog', state: 'active' }),
    ]
    expect(pickActiveSprint(rows, [])?.id).toBe(42)
  })

  it('exactly one active → that one', () => {
    const rows = [sprint({ id: 41, state: 'closed' }), sprint({ id: 42, state: 'active' })]
    expect(pickActiveSprint(rows, [])?.id).toBe(42)
  })

  it('several active → the one holding most of the snapshot', () => {
    const rows = [sprint({ id: 42 }), sprint({ id: 44 })]
    const issues = [
      issue({ issue_key: 'STD-1', sprint_id: 42 }),
      issue({ issue_key: 'STD-2', sprint_id: 44 }),
      issue({ issue_key: 'STD-3', sprint_id: 44 }),
    ]
    expect(pickActiveSprint(rows, issues)?.id).toBe(44)
  })

  it('a tie keeps mirror order, so the pick does not swap between syncs', () => {
    const rows = [sprint({ id: 42 }), sprint({ id: 44 })]
    const issues = [
      issue({ issue_key: 'STD-1', sprint_id: 42 }),
      issue({ issue_key: 'STD-2', sprint_id: 44 }),
    ]
    expect(pickActiveSprint(rows, issues)?.id).toBe(42)
    expect(pickActiveSprint([...rows].reverse(), issues)?.id).toBe(44)
  })
})

describe('sprintCounts — done / total over the held snapshot', () => {
  const issues = [
    issue({ issue_key: 'STD-1', sprint_id: 42, status_category: 'done' }),
    issue({ issue_key: 'STD-2', sprint_id: 42, status_category: 'new' }),
    issue({ issue_key: 'STD-3', sprint_id: 42, status_category: 'inprogress' }),
    issue({ issue_key: 'STD-4', sprint_id: 42, status_category: 'done' }),
    // Not in the sprint, and one in no sprint at all.
    issue({ issue_key: 'STD-5', sprint_id: 43, status_category: 'done' }),
    issue({ issue_key: 'STD-6', sprint_id: null, status_category: 'done' }),
  ]

  it('counts only the rows carrying that sprint id', () => {
    expect(sprintCounts(issues, 42)).toEqual({ total: 4, done: 2, pct: 50 })
  })

  it('an empty sprint reads 0 / 0 · 0%, never NaN', () => {
    expect(sprintCounts(issues, 99)).toEqual({ total: 0, done: 0, pct: 0 })
  })

  it('goes through the category alias table, not the status display name', () => {
    // `complete` is an alias of done; "완료" as a display name is not an axis.
    const rows = [
      issue({ issue_key: 'STD-7', sprint_id: 42, status: '완료', status_category: 'complete' }),
      issue({ issue_key: 'STD-8', sprint_id: 42, status: '완료', status_category: 'new' }),
    ]
    expect(sprintCounts(rows, 42)).toEqual({ total: 2, done: 1, pct: 50 })
  })

  it('rounds the percent to whole numbers', () => {
    const rows = [
      issue({ issue_key: 'STD-9', sprint_id: 42, status_category: 'done' }),
      issue({ issue_key: 'STD-10', sprint_id: 42, status_category: 'new' }),
      issue({ issue_key: 'STD-11', sprint_id: 42, status_category: 'new' }),
    ]
    expect(sprintCounts(rows, 42).pct).toBe(33)
  })
})

describe('sprintDaysLeft — whole calendar days, in the reader’s zone', () => {
  const zone = utcZone()
  const now = new Date('2026-09-14T09:30:00Z')

  it('ends later this week → the days between', () => {
    expect(sprintDaysLeft('2026-09-17T00:00:00.000Z', now, zone)).toBe(3)
  })

  it('ends today → 0, even when the stamp is hours away', () => {
    expect(sprintDaysLeft('2026-09-14T23:59:00.000Z', now, zone)).toBe(0)
    expect(sprintDaysLeft('2026-09-14T00:00:00.000Z', now, zone)).toBe(0)
  })

  it('ends tomorrow → 1', () => {
    expect(sprintDaysLeft('2026-09-15T04:29:00.000Z', now, zone)).toBe(1)
  })

  it('past its end → negative', () => {
    expect(sprintDaysLeft('2026-09-13T00:00:00.000Z', now, zone)).toBe(-1)
    expect(sprintDaysLeft('2026-09-04T00:00:00.000Z', now, zone)).toBe(-10)
  })

  it('no end date → null, not 0', () => {
    expect(sprintDaysLeft(null, now, zone)).toBeNull()
    expect(sprintDaysLeft(undefined, now, zone)).toBeNull()
    expect(sprintDaysLeft('', now, zone)).toBeNull()
  })
})

describe('loadSprints — an answer is adopted, a refusal is not', () => {
  it('a body becomes rows', async () => {
    const rows = await loadSprints(async () => ({ sprints: [sprint({ id: 42 })] }))
    expect(rows?.map((r) => r.id)).toEqual([42])
  })

  it('an empty list is an answer: the origin closed its last sprint', async () => {
    expect(await loadSprints(async () => ({ sprints: [] }))).toEqual([])
  })

  it('404 (a serve older than the route) → null, without throwing', async () => {
    await expect(
      loadSprints(async () => {
        throw new ApiError('not_found', 404)
      }),
    ).resolves.toBeNull()
  })

  it('501 and a dead network → null too', async () => {
    await expect(
      loadSprints(async () => {
        throw new ApiError('internal_error', 501)
      }),
    ).resolves.toBeNull()
    await expect(
      loadSprints(async () => {
        throw new ApiError('network', 0)
      }),
    ).resolves.toBeNull()
  })

  it('a 304 (no body) is not an answer either', async () => {
    expect(await loadSprints(async () => null)).toBeNull()
  })

  it('the picked sprint of a refused load is null', async () => {
    const rows = await loadSprints(async () => {
      throw new ApiError('not_found', 404)
    })
    expect(pickActiveSprint(rows ?? [], [])).toBeNull()
  })
})

describe('sortSprints — the sprint list screen’s order (GDK-1827)', () => {
  // The demo fixture's shape: 41 closed, 42 active, 43 future — arrived in
  // id order but sorted by reading order.
  it('active first, then future, then closed — whatever order the mirror sent', () => {
    const rows = [
      sprint({ id: 43, state: 'future' }),
      sprint({ id: 41, state: 'closed' }),
      sprint({ id: 42, state: 'active' }),
    ]
    expect(sortSprints(rows).map((r) => r.id)).toEqual([42, 43, 41])
  })

  it('future sprints sort by start ascending — the soonest first', () => {
    const rows = [
      sprint({ id: 44, state: 'future', start_at: '2026-10-01T00:00:00Z' }),
      sprint({ id: 43, state: 'future', start_at: '2026-09-17T00:00:00Z' }),
    ]
    expect(sortSprints(rows).map((r) => r.id)).toEqual([43, 44])
  })

  it('closed sprints sort by end descending — the most recently finished first', () => {
    const rows = [
      sprint({ id: 39, state: 'closed', end_at: '2026-08-06T00:00:00Z' }),
      sprint({ id: 41, state: 'closed', end_at: '2026-09-03T00:00:00Z' }),
    ]
    expect(sortSprints(rows).map((r) => r.id)).toEqual([41, 39])
  })

  it('a row with no date in its band goes last, not first', () => {
    const rows = [
      sprint({ id: 45, state: 'future' }),
      sprint({ id: 43, state: 'future', start_at: '2026-09-17T00:00:00Z' }),
      sprint({ id: 38, state: 'closed' }),
      sprint({ id: 41, state: 'closed', end_at: '2026-09-03T00:00:00Z' }),
    ]
    expect(sortSprints(rows).map((r) => r.id)).toEqual([43, 45, 41, 38])
  })

  it('ties keep mirror order, so two same-day sprints do not swap between syncs', () => {
    const rows = [
      sprint({ id: 43, state: 'future', start_at: '2026-09-17T00:00:00Z' }),
      sprint({ id: 44, state: 'future', start_at: '2026-09-17T00:00:00Z' }),
    ]
    expect(sortSprints(rows).map((r) => r.id)).toEqual([43, 44])
    expect(sortSprints([...rows].reverse()).map((r) => r.id)).toEqual([44, 43])
  })

  it('a state the three words do not know goes after closed, mirror order', () => {
    const rows = [
      sprint({ id: 46, state: 'paused' }),
      sprint({ id: 41, state: 'closed' }),
      sprint({ id: 45, state: 'paused' }),
    ]
    expect(sortSprints(rows).map((r) => r.id)).toEqual([41, 46, 45])
  })

  it('is pure: the caller’s array is not reordered', () => {
    const rows = [
      sprint({ id: 42, state: 'active' }),
      sprint({ id: 41, state: 'closed' }),
    ]
    sortSprints(rows)
    expect(rows.map((r) => r.id)).toEqual([42, 41])
  })

  it('an empty workspace is an empty screen — no rows, no bands', () => {
    expect(sortSprints([])).toEqual([])
  })
})

describe('the sprint scope (GDK-1867)', () => {
  const active = sprint({ id: 42, name: 'Sprint 42', goal: 'Work down Triage.' })
  const issues = [
    issue({ issue_key: 'STD-1', sprint_id: 42, status_category: 'done' }),
    issue({ issue_key: 'STD-2', sprint_id: 42, status_category: 'new' }),
    issue({ issue_key: 'STD-3', sprint_id: 42, status_category: 'inprogress' }),
    issue({ issue_key: 'STD-4', sprint_id: 43, status_category: 'new' }),
    issue({ issue_key: 'STD-5', sprint_id: null, status_category: 'new' }),
  ]

  function sprintScope(): Scope {
    const found = buildScopes([], [], null, [], active).find((s) => s.id === SCOPE_ACTIVE_SPRINT)
    expect(found, 'the sprint scope is offered').toBeTruthy()
    return found!
  }

  it('is absent with no active sprint, and present with one', () => {
    expect(buildScopes([], [], null).map((s) => s.id)).not.toContain(SCOPE_ACTIVE_SPRINT)
    expect(buildScopes([], [], null, [], active).map((s) => s.id)).toContain(SCOPE_ACTIVE_SPRINT)
  })

  it('sits in the built-in section, last, in the team stance', () => {
    const builtin = buildScopes([], [], null, [], active).filter((s) => s.section === 'builtin')
    expect(builtin[builtin.length - 1].id).toBe(SCOPE_ACTIVE_SPRINT)
    expect(builtin[builtin.length - 1].stance).toBe('team')
  })

  it('selects on sprint_id, and keeps the done rows', () => {
    const rows = scopeIssues(issues, null, sprintScope())
    expect(rows?.map((i) => i.issue_key).sort()).toEqual(['STD-1', 'STD-2', 'STD-3'])
  })

  it('groups by status category, not by priority', () => {
    // Re-pinned 2026-09-18 (GDK-1993): the header order is the desk's now,
    // through the shared grouper — in progress, then new, then done. The
    // phone used to read new → inprogress → done, and one of the two had to
    // give when the two surfaces stopped keeping separate groupers. FAIL-first
    // read `expected [ 'new', 'inprogress', 'done' ] to deeply equal
    // [ 'inprogress', 'new', 'done' ]`. What GDK-1867 wanted is unchanged: a
    // sprint reads as what is moving, what is left, what landed — and leading
    // with what is moving is the stronger reading of that sentence anyway.
    const view = buildList(issues, null, sprintScope())
    expect(view.total).toBe(3)
    expect(view.fellBack).toBe(false)
    expect(view.sections.map((s) => s.key)).toEqual(['inprogress', 'new', 'done'])
    expect(view.sections.map((s) => s.issues.map((i) => i.issue_key))).toEqual([
      ['STD-3'],
      ['STD-2'],
      ['STD-1'],
    ])
  })

  it('omits a category with no rows rather than drawing an empty header', () => {
    const only = [issue({ issue_key: 'STD-6', sprint_id: 42, status_category: 'done' })]
    const view = buildList(only, null, sprintScope())
    expect(view.sections.map((s) => s.key)).toEqual(['done'])
  })

  it('an empty sprint stays empty — it does not fall back to All open', () => {
    const view = buildList([issue({ issue_key: 'STD-7', sprint_id: 43 })], null, sprintScope())
    expect(view.total).toBe(0)
    expect(view.fellBack).toBe(false)
  })

  it('a phone that stored the sprint scope lands on the pool when the sprint closes', () => {
    // The scope id is a *want* and survives in localStorage; the row is
    // offered only while a sprint is active. Closing one must not leave the
    // screen on a name nothing answers to.
    const none = buildScopes([], [], null)
    expect(resolveScope(none, SCOPE_ACTIVE_SPRINT, null)?.id).toBe(SCOPE_ALL_OPEN)
    const some = buildScopes([], [], null, [], active)
    expect(resolveScope(some, SCOPE_ACTIVE_SPRINT, null)?.id).toBe(SCOPE_ACTIVE_SPRINT)
  })
})
