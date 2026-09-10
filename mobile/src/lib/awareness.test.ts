import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildScopes,
  relatchBoundary,
  resumeChanges,
  resumeLine,
  resumeSince,
  rowAge,
  rowAgeBand,
  rowAgeDays,
  rowAgeTitle,
  rowIsStale,
  scopeIssues,
  sessionDelta,
  sessionLine,
  setFlow,
  SESSION_GAP_MS,
  SCOPE_ALL_OPEN,
  SCOPE_MY_WORK,
  type Scope,
} from './domain'
import type { DetailResponse, IssueLite, Me } from './types'

/*
 * The 0.21 awareness concepts on the phone (GDK-1495 / GDK-1497 A4): the
 * session strip's boundary, the row's work-item age, the resume card's diff,
 * and the five built-in views.
 *
 * Every rule under test is the desktop's — imported, never re-spelled. What
 * these tests pin is the *seam*: that the phone hands the desktop's function
 * the right fields in the right order, and that a later round cannot quietly
 * swap an imported rule for a phone-authored copy. The rules' own edge cases
 * are covered by their owners (web/src/lib/session-strip.test.ts,
 * resume-card.test.ts, view-config.test.ts).
 */

// Fixture keys use STD-* (never GDK-*: repo doc-checks scans test files).
function issue(over: Partial<IssueLite> & { issue_key: string }): IssueLite {
  return {
    summary: 'a summary',
    project_key: 'STD',
    issue_type: 'Task',
    issue_type_id: '10001',
    status: 'Open',
    status_id: '1',
    status_category: 'inprogress',
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
    ...over,
  }
}

const me: Me = { email: 'dev@example.com', account_id: 'acct-1', name: 'Dev' }

/** `hours` before the frozen clock, as an ISO string. */
function ago(hours: number): string {
  return new Date(Date.now() - hours * 3_600_000).toISOString()
}

function scopeOf(list: Scope[], id: string): Scope {
  const hit = list.find((s) => s.id === id)
  if (!hit) throw new Error(`no scope ${id} in [${list.map((s) => s.id).join(', ')}]`)
  return hit
}

afterEach(() => {
  vi.useRealTimers()
  setFlow(null)
})

describe('row age — the started clock, in the desktop order (GDK-1495 ②)', () => {
  it('counts from started_at when the mirror knows when work began', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    // Eleven days underway, moved to review an hour ago: the status clock
    // would read one hour, the work clock reads eleven days.
    const age = rowAge(
      issue({
        issue_key: 'STD-1',
        started_at: ago(264),
        status_changed_at: ago(1),
        updated_at: ago(1),
      }),
    )
    expect(age.basis).toBe('started')
    expect(Math.round(age.hours)).toBe(264)
  })

  it('falls back to status_changed_at, then updated_at, then knows nothing', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    expect(
      rowAge(issue({ issue_key: 'STD-2', status_changed_at: ago(48), updated_at: ago(1) })).basis,
    ).toBe('status')
    expect(
      rowAge(issue({ issue_key: 'STD-3', status_changed_at: null, updated_at: ago(30) })).hours,
    ).toBeGreaterThan(29)
    expect(
      rowAge(issue({ issue_key: 'STD-4', status_changed_at: null, updated_at: null })),
    ).toEqual({ hours: 0, basis: 'none' })
  })

  it('floors the badge at day 1 so a fresh row never reads "0d"', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    expect(rowAgeDays(issue({ issue_key: 'STD-5', started_at: ago(2) }))).toBe(1)
    expect(rowAgeDays(issue({ issue_key: 'STD-6', started_at: ago(72) }))).toBe(3)
  })
})

describe('stale gate — the server teaches the threshold (GDK-1495 ②)', () => {
  it('is never stale in the done category, however old', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    const old = { issue_key: 'STD-7', started_at: ago(4000) }
    expect(rowIsStale(issue({ ...old, status_category: 'done' }))).toBe(false)
    expect(rowIsStale(issue(old))).toBe(true)
  })

  it('defaults to 72h when the serve sent no flow', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    expect(rowIsStale(issue({ issue_key: 'STD-8', started_at: ago(71) }))).toBe(false)
    expect(rowIsStale(issue({ issue_key: 'STD-9', started_at: ago(73) }))).toBe(true)
  })

  it('prefers the learned p85, and refuses one that stands on too few issues', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    const row = issue({ issue_key: 'STD-10', started_at: ago(100) })
    setFlow({ cycle_p85_hours: 200, samples: 47 })
    expect(rowIsStale(row)).toBe(false)
    // Below the sample bar the payload cannot lower (or raise) the line.
    setFlow({ cycle_p85_hours: 200, samples: 3 })
    expect(rowIsStale(row)).toBe(true)
  })

  it('weights the band by multiples of the threshold in force', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    expect(rowAgeBand(issue({ issue_key: 'STD-11', started_at: ago(60) }))).toBeNull()
    expect(rowAgeBand(issue({ issue_key: 'STD-12', started_at: ago(100) }))).toBe('quiet')
    expect(rowAgeBand(issue({ issue_key: 'STD-13', started_at: ago(250) }))).toBe('mid')
    expect(rowAgeBand(issue({ issue_key: 'STD-14', started_at: ago(400) }))).toBe('loud')
  })

  it('names its basis in the title, and names the learned rule only when learned', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T00:00:00Z'))
    const started = issue({ issue_key: 'STD-15', started_at: ago(240) })
    expect(rowAgeTitle(started)).toBe('10 days since work started')
    const status = issue({ issue_key: 'STD-16', status_changed_at: ago(240) })
    expect(rowAgeTitle(status)).toBe('10 days in this status')
    setFlow({ cycle_p85_hours: 48, samples: 47 })
    expect(rowAgeTitle(started)).toContain('85% of the 47 issues')
  })
})

describe('session strip — the re-latch rule is the desktop’s (GDK-1495 ①)', () => {
  it('re-latches only when the app was away longer than the session gap', () => {
    const hidden = Date.parse('2026-09-07T00:00:00.000Z')
    expect(relatchBoundary(hidden, hidden + SESSION_GAP_MS + 1)).toBe('2026-09-07T00:00:00.000Z')
    expect(relatchBoundary(hidden, hidden + SESSION_GAP_MS)).toBeNull()
    expect(relatchBoundary(hidden, hidden + 60_000)).toBeNull()
  })

  it('has nothing to re-latch when the app was never hidden', () => {
    expect(relatchBoundary(null, Date.now())).toBeNull()
    expect(relatchBoundary(Number.NaN, Date.now())).toBeNull()
  })

  it('counts what moved after the boundary, and whose it is', () => {
    const since = '2026-09-06T00:00:00Z'
    const rows = [
      issue({ issue_key: 'STD-20', updated_at: '2026-09-06T01:00:00Z', assignee_id: 'acct-1' }),
      issue({ issue_key: 'STD-21', updated_at: '2026-09-06T02:00:00Z', assignee_id: 'acct-9' }),
      // Exactly at the boundary was seen on the last read — not a change.
      issue({ issue_key: 'STD-22', updated_at: since }),
      issue({ issue_key: 'STD-23', updated_at: null }),
    ]
    const delta = sessionDelta(rows, since, me)
    expect(delta?.keys).toEqual(['STD-20', 'STD-21'])
    expect(delta?.mine).toBe(1)
  })

  it('says nothing at all when nothing changed', () => {
    expect(sessionDelta([issue({ issue_key: 'STD-24', updated_at: null })], '2026-09-06T00:00:00Z', me)).toBeNull()
  })

  it('writes the line from the catalog, and drops the mine part when it is zero', () => {
    const delta = { keys: ['STD-20', 'STD-21'], mine: 1 }
    expect(sessionLine(delta, '3d', me)).toBe(
      'Since last session 3d · 2 issues changed · 1 of them assigned here',
    )
    expect(sessionLine({ keys: ['STD-20'], mine: 0 }, '3d', me)).toBe(
      'Since last session 3d · 1 issue changed',
    )
    expect(sessionLine(delta, '3d', null)).toBe('Since last session 3d · 2 issues changed')
  })
})

describe('resume card — the same diff the desk runs (GDK-1495 ③)', () => {
  function detail(over: Partial<DetailResponse>): DetailResponse {
    return {
      issue_key: 'STD-30',
      description_adf: null,
      attachments: [],
      comments: [],
      linked_issues: [],
      history: [],
      ...over,
    }
  }

  it('has no boundary at all when the issue was never opened', () => {
    expect(resumeSince(detail({}))).toBeNull()
  })

  it('steps back to the previous visit when the newest read is this very open', () => {
    const now = Date.parse('2026-09-07T00:00:00Z')
    vi.useFakeTimers()
    vi.setSystemTime(new Date(now))
    const d = detail({
      last_visited_at: new Date(now - 5_000).toISOString(),
      previous_visit_at: '2026-09-05T00:00:00Z',
    })
    expect(resumeSince(d)).toBe('2026-09-05T00:00:00Z')
    // An older newest read is already the boundary.
    const older = detail({
      last_visited_at: '2026-09-06T00:00:00Z',
      previous_visit_at: '2026-09-05T00:00:00Z',
    })
    expect(resumeSince(older)).toBe('2026-09-06T00:00:00Z')
  })

  it('counts history by field and comments by time, and stays silent on nothing', () => {
    const since = '2026-09-05T00:00:00Z'
    const d = detail({
      history: [
        { at: '2026-09-06T01:00:00Z', field: 'status', from: 'a', to: 'b', by: null },
        { at: '2026-09-06T02:00:00Z', field: 'assignee', from: null, to: 'x', by: null },
        { at: '2026-09-06T03:00:00Z', field: 'labels', from: null, to: 'y', by: null },
        { at: '2026-09-01T00:00:00Z', field: 'status', from: 'a', to: 'b', by: null },
      ],
      comments: [
        { comment_id: 'c1', author: null, created_at: '2026-09-06T04:00:00Z', raw_body: null, body: 'new' },
        { comment_id: 'c2', author: null, created_at: '2026-09-01T00:00:00Z', raw_body: null, body: 'old' },
      ],
    })
    const delta = resumeChanges(d, since)
    expect(delta).toEqual({ statusChanges: 1, comments: 1, assigneeChanged: true, other: 1 })
    expect(resumeChanges(detail({}), since)).toBeNull()
    expect(resumeChanges(d, null)).toBeNull()
  })

  it('writes the line from the catalog, omitting the zero parts', () => {
    expect(
      resumeLine({ statusChanges: 1, comments: 1, assigneeChanged: true, other: 1 }, '2d'),
    ).toBe(
      'Since last opened 2d · 1 status change · 1 new comment · assignee changed · 1 other change',
    )
    expect(
      resumeLine({ statusChanges: 0, comments: 2, assigneeChanged: false, other: 0 }, '2d'),
    ).toBe('Since last opened 2d · 2 new comments')
  })
})

describe('the five built-in views (GDK-1495 ④)', () => {
  const rows = [
    issue({ issue_key: 'STD-40', assignee_id: 'acct-1', reporter_id: 'acct-1' }),
    issue({ issue_key: 'STD-41', assignee_id: 'acct-2', reporter_id: 'acct-1' }),
    issue({ issue_key: 'STD-42', status_category: 'new' }),
    issue({ issue_key: 'STD-43', reopen_count: 2, assignee_id: 'acct-2' }),
    issue({ issue_key: 'STD-44', status_category: 'done', assignee_id: 'acct-1', reporter_id: 'acct-1' }),
  ]

  it('offers the desk’s five, under the desk’s names, in the desk’s order', () => {
    // Five rows, no sixth: the phone-authored "Assigned to me" that led this
    // section was `my-work` asked twice and left with GDK-1542.
    const builtin = buildScopes([], [], me).filter((s) => s.section === 'builtin')
    expect(builtin.map((s) => s.id)).toEqual([
      SCOPE_MY_WORK,
      'builtin:delegated',
      SCOPE_ALL_OPEN,
      'builtin:unassigned-new',
      'builtin:reopened',
    ])
    expect(builtin.map((s) => s.name)).toEqual([
      'My issues',
      'Handed off',
      'All open',
      'Unassigned new',
      'Reopened',
    ])
  })

  it('hides the two that need an identity when the serve has none', () => {
    const builtin = buildScopes([], [], null).filter((s) => s.section === 'builtin')
    expect(builtin.map((s) => s.id)).toEqual([
      SCOPE_ALL_OPEN,
      'builtin:unassigned-new',
      'builtin:reopened',
    ])
  })

  it('paints every one of them — none is refused for an axis the phone cannot read', () => {
    for (const s of buildScopes([], [], me).filter((x) => x.section === 'builtin')) {
      expect(s.unsupported, `${s.id} is refused`).toEqual([])
    }
  })

  it('selects rows by the desk’s flags, not by display names', () => {
    const list = buildScopes([], [], me)
    const pick = (id: string) => scopeIssues(rows, me, scopeOf(list, id))?.map((i) => i.issue_key)
    expect(pick('builtin:my-work')).toEqual(['STD-40'])
    expect(pick('builtin:delegated')).toEqual(['STD-41'])
    expect(pick('builtin:unassigned-new')).toEqual(['STD-42'])
    expect(pick('builtin:reopened')).toEqual(['STD-43'])
    expect(pick(SCOPE_ALL_OPEN)).toEqual(['STD-40', 'STD-41', 'STD-42', 'STD-43'])
  })
})
