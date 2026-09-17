/*
 * The list reads in the order its view asked for (GDK-1992).
 *
 * The defect: `sortIssues` had the axis fixed — priority asc, then updated
 * desc — so every scope came out in the same order whatever the desk's view
 * said. Four of the desk's five built-ins carry an authored order, and two of
 * them were plainly wrong on a phone: *Handed off* exists to read the
 * quietest first (`updated asc`) and came out newest-first, and *Reopened* is
 * named for an axis (`reopen_count desc`) the phone never looked at.
 *
 * The assertion is agreement with the desk, not a hand-typed order: each
 * scope's flattened list is compared against `compareIssues` — the same
 * comparator `stores/filters.svelte.ts` sorts with — run over the same rows.
 * A hand-typed expectation would pin today's opinion; this pins that the two
 * surfaces cannot drift.
 */
import { describe, it, expect } from 'vitest'
import { buildList, buildScopes, openIssues, type Scope } from './domain'
import { compareIssues } from '../../../web/src/lib/issue-sort'
import { builtinViews } from '../../../web/src/lib/builtin-views'
import type { IssueLite, Me } from './types'

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
 * Rows built so the four authored axes disagree with each other and with the
 * old fixed order: same priority on three of them (so priority cannot be
 * doing the work), updated/created deliberately counter-ordered, and reopen
 * counts that rank the list a fourth way.
 */
const rows: IssueLite[] = [
  issue({
    issue_key: 'STD-1',
    priority_rank: 3,
    reopen_count: 0,
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-20T00:00:00Z',
    assignee_id: 'acct-1',
    reporter_id: 'acct-1',
  }),
  issue({
    issue_key: 'STD-2',
    priority_rank: 3,
    reopen_count: 5,
    created_at: '2026-08-05T00:00:00Z',
    updated_at: '2026-08-02T00:00:00Z',
    assignee_id: 'acct-9',
    reporter_id: 'acct-1',
  }),
  issue({
    issue_key: 'STD-3',
    priority_rank: 1,
    reopen_count: 2,
    created_at: '2026-08-09T00:00:00Z',
    updated_at: '2026-08-11T00:00:00Z',
    assignee_id: 'acct-9',
    reporter_id: 'acct-1',
  }),
  issue({
    issue_key: 'STD-4',
    priority_rank: 3,
    reopen_count: 1,
    created_at: '2026-08-03T00:00:00Z',
    updated_at: '2026-08-06T00:00:00Z',
    assignee_id: null,
    reporter_id: 'acct-2',
  }),
  // Second unassigned row, so Unassigned new is a real ordering and not a
  // list of one: same rank as STD-4 (priority cannot separate them), created
  // later, updated earlier — so `created desc` and the default disagree.
  issue({
    issue_key: 'STD-5',
    priority_rank: 3,
    reopen_count: 0,
    created_at: '2026-08-07T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z',
    assignee_id: null,
    reporter_id: 'acct-2',
  }),
]

const keys = (list: IssueLite[]): string[] => list.map((i) => i.issue_key)

/**
 * Every row the screen paints. The desk's answer below is cut by the phone's
 * own bands, so without this count a row the phone dropped would simply be
 * absent from both sides and the comparison would agree with itself.
 */
function phoneCount(scope: Scope): number {
  return buildList(rows, me, scope).sections.reduce((n, s) => n + s.issues.length, 0)
}

/**
 * What the screen actually paints, section by section.
 *
 * Not the flattened list: the sections are priority bands, and a band is the
 * outer key of any grouped list — so the view's sort orders the rows *inside*
 * a band, exactly as it does on the desk. Which axis the band should be is a
 * separate question (GDK-1993); that it must not be the sort's is not.
 */
function phoneSections(scope: Scope): string[][] {
  return buildList(rows, me, scope).sections.map((s) => keys(s.issues))
}

/** The desk's answer, cut into the same bands, so the two are comparable. */
function deskSections(scope: Scope, selected: IssueLite[]): string[][] {
  const phone = buildList(rows, me, scope).sections
  const view = builtinViews().find((v) => scope.id.endsWith(v.id))
  if (!view) throw new Error(`no built-in behind ${scope.id}`)
  const { sort, dir } = view.config.display
  const ordered = [...selected].sort((a, b) => compareIssues(a, b, sort, dir))
  return phone.map((s) => {
    const want = new Set(s.issues.map((i) => i.issue_key))
    return keys(ordered.filter((i) => want.has(i.issue_key)))
  })
}

function scopeOf(id: string): Scope {
  const hit = buildScopes([], [], me).find((s) => s.id.endsWith(id))
  if (!hit) throw new Error(`no scope ${id}`)
  return hit
}

describe('the list reads in the view’s order (GDK-1992)', () => {
  it('Handed off reads the quietest first, not the newest', () => {
    const scope = scopeOf('delegated')
    const selected = rows.filter((r) => r.reporter_id === 'acct-1' && r.assignee_id !== 'acct-1')
    expect(phoneCount(scope)).toBe(selected.length)
    // The defect in one line: the old fixed order put the newest update at
    // the top of the band this list exists to read oldest-first.
    expect(phoneSections(scope)).toEqual(deskSections(scope, selected))
    expect(phoneSections(scope).flat()[0]).toBe('STD-3')
  })

  it('Reopened reads by reopen count, the axis it is named for', () => {
    const scope = scopeOf('reopened')
    const selected = rows.filter((r) => r.reopen_count > 0)
    expect(phoneCount(scope)).toBe(selected.length)
    expect(phoneSections(scope)).toEqual(deskSections(scope, selected))
  })

  it('Unassigned new reads newest-created first', () => {
    const scope = scopeOf('unassigned-new')
    const selected = rows.filter((r) => r.assignee_id === null)
    expect(phoneCount(scope)).toBe(selected.length)
    expect(phoneSections(scope)).toEqual(deskSections(scope, selected))
    // Both rows share a rank, so they share a band — and inside it the
    // newest-created leads even though it is the least recently touched.
    // The old fixed order put STD-4 first on both its axes.
    expect(phoneSections(scope)).toEqual([['STD-5', 'STD-4']])
  })

  it('My issues reads urgent first', () => {
    const scope = scopeOf('my-work')
    const selected = rows.filter((r) => r.assignee_id === 'acct-1')
    expect(phoneCount(scope)).toBe(selected.length)
    expect(phoneSections(scope)).toEqual(deskSections(scope, selected))
  })

  it('All open reads the catalog default, which is the order the phone already had', () => {
    const scope = scopeOf('all-open')
    expect(phoneCount(scope)).toBe(openIssues(rows).length)
    expect(phoneSections(scope)).toEqual(deskSections(scope, openIssues(rows)))
  })

  it('every built-in the phone offers agrees with the desk', () => {
    // The guard against a sixth view landing with an axis the phone drops.
    for (const scope of buildScopes([], [], me).filter((s) => s.section === 'builtin')) {
      const view = builtinViews().find((v) => scope.id.endsWith(v.id))
      if (!view) continue // the active-sprint row has no stored view
      expect(scope.order).toEqual({
        sort: view.config.display.sort,
        dir: view.config.display.dir,
      })
    }
  })

  it('a section is a bucket, so its key cannot repeat', () => {
    // The crash GDK-1992 uncovered: groupByPriority used to open a new
    // section whenever the rank changed from the row before, which is only
    // the same thing while the sort IS priority. Under `updated asc` the
    // ranks interleave, the same rank is emitted twice, and `rank` is the key
    // of a keyed `#each` — Svelte refused the whole screen with
    // `each_key_duplicate`, so Reopened painted a heading reading "·95" over
    // a list with no rows in it.
    const byScope = buildScopes([], [], me)
      .filter((s) => s.section === 'builtin')
      .map((s) => [s.id, buildList(rows, me, s).sections.map((x) => x.rank)] as const)
    // Uniqueness first, across every scope — that is the one that crashed.
    for (const [id, ranks] of byScope) {
      expect(new Set(ranks).size, `${id} section keys`).toBe(ranks.length)
    }
    // And in rank order, which is what makes the bands readable.
    for (const [id, ranks] of byScope) {
      expect([...ranks].sort((a, b) => a - b), `${id} band order`).toEqual(ranks)
    }
  })
})
