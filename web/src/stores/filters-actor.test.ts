import { afterEach, describe, expect, test } from 'vitest'
import type { IssueLite } from '../lib/types'
import { emptyFilters } from '../lib/view-config'
import { buildGroups, filterIssues, sortIssues } from './filters.svelte'
import { me } from './me.svelte'

/*
 * GDK-590 actor axis: the filter narrows on the row's actor_ids (account ids
 * from the server's issue_actors view — never display names), and grouping is
 * the one multi-membership axis: an issue two accounts touched counts in both
 * buckets. Lives in the pages-store project because filters.svelte.ts needs
 * the svelte plugin to compile its runes.
 */

function issue(partial: Pick<IssueLite, 'issue_key' | 'summary'> & Partial<IssueLite>): IssueLite {
  return {
    status: 'To Do',
    status_category: 'new',
    issue_type: 'Bug',
    priority: null,
    priority_rank: 0,
    severity: null,
    assignee: null,
    assignee_email: null,
    reporter: null,
    reporter_email: null,
    labels: [],
    fix_versions: [],
    components: [],
    team_group: null,
    epic_key: null,
    parent_key: null,
    source_project: null,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    resolved_at: null,
    status_changed_at: null,
    reopen_count: 0,
    reopened_at: null,
    reopen_reason: null,
    comment_count: 0,
    dev_project_number: null,
    related_project_number: null,
    environment: null,
    browser: null,
    found_version: null,
    occurrence: null,
    solution: null,
    critical_phenomenon: null,
    development_area: null,
    cs: null,
    development_test_assignee: null,
    development_test_assignee_email: null,
    development_test_result: null,
    qa_impact_state: '',
    qa_impact_label: '',
    qa_runs: [],
    qa_suites: [],
    ...partial,
  }
}

describe('actor filter (GDK-590)', () => {
  const botOnly = issue({ issue_key: 'NMB-1', summary: 'bot shipped', actor_ids: ['acc-bot'] })
  const both = issue({
    issue_key: 'NMB-2',
    summary: 'bot linked, human reviewed',
    actor_ids: ['acc-bot', 'acc-hc'],
  })
  const humanOnly = issue({ issue_key: 'NMB-3', summary: 'human solo', actor_ids: ['acc-hc'] })
  const untouched = issue({ issue_key: 'NMB-4', summary: 'no actors yet' })
  const all = [botOnly, both, humanOnly, untouched]

  test('narrows to issues the actor touched, union across ids', () => {
    const f = emptyFilters()
    f.actor = ['acc-bot']
    expect(filterIssues(all, f).map((i) => i.issue_key)).toEqual(['NMB-1', 'NMB-2'])
  })

  test('multiple selected actors widen (OR), like every multi axis', () => {
    const f = emptyFilters()
    f.actor = ['acc-bot', 'acc-hc']
    expect(filterIssues(all, f).map((i) => i.issue_key)).toEqual(['NMB-1', 'NMB-2', 'NMB-3'])
  })

  test('empty actor filter keeps rows without actor_ids (older servers omit it)', () => {
    expect(filterIssues(all, emptyFilters())).toHaveLength(4)
  })

  test('actor is keyed on account ids, never display names', () => {
    const f = emptyFilters()
    f.actor = ['Claude (build 1)'] // a display name must not match anything
    expect(filterIssues(all, f)).toHaveLength(0)
  })
})

describe('actor grouping (GDK-590)', () => {
  test('an issue two accounts touched lands in both buckets', () => {
    const list = [
      issue({ issue_key: 'NMB-1', summary: 'bot only', actor_ids: ['acc-bot'] }),
      issue({ issue_key: 'NMB-2', summary: 'both', actor_ids: ['acc-bot', 'acc-hc'] }),
    ]
    const groups = buildGroups(list, 'actor')
    const counts = Object.fromEntries(groups.map((g) => [g.key, g.counts.total]))
    expect(counts).toEqual({ 'acc-bot': 2, 'acc-hc': 1 })
    // The shared issue is a member of both buckets — counts, not copies, are
    // what the breakdown bar asserts.
    const bot = groups.find((g) => g.key === 'acc-bot')!
    const hc = groups.find((g) => g.key === 'acc-hc')!
    expect(bot.items.map((i) => i.issue_key)).toEqual(['NMB-1', 'NMB-2'])
    expect(hc.items.map((i) => i.issue_key)).toEqual(['NMB-2'])
  })

  test('rows with no actors gather under the empty bucket, sorted last', () => {
    const groups = buildGroups(
      [
        issue({ issue_key: 'NMB-9', summary: 'untouched' }),
        issue({ issue_key: 'NMB-1', summary: 'bot', actor_ids: ['acc-bot'] }),
      ],
      'actor',
    )
    expect(groups[groups.length - 1].key).toBe('')
    expect(groups[groups.length - 1].counts.total).toBe(1)
  })

  test('group keys are account ids even when the member name is known', () => {
    const groups = buildGroups([issue({ issue_key: 'NMB-1', summary: 'x', actor_ids: ['acc-bot'] })], 'actor')
    expect(groups).toHaveLength(1)
    expect(groups[0].key).toBe('acc-bot')
  })
})

/*
 * my-work pack: identity flags (mine / delegated), the status_changed sort,
 * and the status_category reading order.
 *
 * Contract ↔ assertion table (clause → assertion names):
 *  C1 person-match is the only "is this mine" rule (id first, then email —
 *      never a display name)
 *     mine keeps issues assigned to this identity — id first, then email
 *     delegated keeps reported-by-me held by someone else or nobody
 *  C2 anonymous + identity flag = empty list, never the pool
 *     anonymous mine matches nothing; delegated too
 *  flags compose with the ordinary axes
 *     mine/delegated compose with status_category
 *  C4 status_changed: asc = longest in status first, missing last both ways,
 *      ties by newest updated_at
 *     asc puts the oldest stamp first and the missing stamp last
 *     desc puts the newest stamp first and the missing stamp still last
 *     ties break by newest updated_at
 *  status_category group order is in progress → new → done (IN_RANK; code
 *      unchanged — this is the proof, per the my-work spec)
 *     groups read in progress, then new, then done
 *
 * FAIL-first 2026-09-06: against the pre-change store, every mine/delegated
 * and status_changed test failed — emptyFilters() carried no such fields,
 * filterIssues ignored them (mine kept all 7 rows), and sortIssues threw
 * nothing but silently fell to the default 'updated' branch for the unknown
 * key, so asc returned A,B (updated-desc actually) instead of B,A,C.
 */
describe('identity flags (my-work pack)', () => {
  afterEach(() => {
    me.email = null
    me.accountId = null
  })

  const rows = [
    issue({ issue_key: 'NMB-1', summary: 'assigned by id', assignee_id: 'acc-dana', status_category: 'inprogress' }),
    issue({
      issue_key: 'NMB-2',
      summary: 'assigned by email, case mismatch',
      assignee_email: 'dana@example.com',
      status_category: 'new',
    }),
    issue({ issue_key: 'NMB-3', summary: 'someone else holds it', assignee_id: 'acc-alex' }),
    issue({ issue_key: 'NMB-4', summary: 'nobody holds it', assignee: null, assignee_email: null }),
    issue({
      issue_key: 'NMB-5',
      summary: 'reported by dana, held by alex',
      reporter_id: 'acc-dana',
      assignee_id: 'acc-alex',
    }),
    issue({
      issue_key: 'NMB-6',
      summary: 'reported by dana, unassigned',
      reporter_id: 'acc-dana',
      assignee: null,
      assignee_email: null,
    }),
    issue({
      issue_key: 'NMB-7',
      summary: 'reported and held by dana',
      reporter_id: 'acc-dana',
      assignee_id: 'acc-dana',
      status_category: 'inprogress',
    }),
  ]

  test('mine keeps issues assigned to this identity — id first, then email', () => {
    me.accountId = 'acc-dana'
    me.email = 'Dana@Example.com' // case differs from NMB-2 on purpose
    const f = emptyFilters()
    f.mine = true
    // NMB-7 counts too: reported by dana AND held by dana is still mine.
    expect(filterIssues(rows, f).map((i) => i.issue_key)).toEqual(['NMB-1', 'NMB-2', 'NMB-7'])
  })

  test('delegated keeps reported-by-me held by someone else or nobody', () => {
    me.accountId = 'acc-dana'
    me.email = 'dana@example.com'
    const f = emptyFilters()
    f.delegated = true
    // NMB-7 is excluded: still mine, not a hand-off.
    expect(filterIssues(rows, f).map((i) => i.issue_key)).toEqual(['NMB-5', 'NMB-6'])
  })

  test('anonymous mine/delegated matches nothing — empty list, never the pool', () => {
    me.accountId = null
    me.email = null
    const f = emptyFilters()
    f.mine = true
    expect(filterIssues(rows, f)).toEqual([])
    f.mine = false
    f.delegated = true
    expect(filterIssues(rows, f)).toEqual([])
  })

  test('mine/delegated compose with status_category', () => {
    me.accountId = 'acc-dana'
    me.email = 'dana@example.com'
    const f = emptyFilters()
    f.mine = true
    f.status_category = ['new']
    expect(filterIssues(rows, f).map((i) => i.issue_key)).toEqual(['NMB-2'])
  })
})

describe('status_changed sort (my-work pack)', () => {
  const keyOf = (i: IssueLite): string => i.issue_key
  const rows = [
    issue({
      issue_key: 'A',
      summary: 'fresh switch',
      status_changed_at: '2026-09-01T00:00:00.000Z',
      updated_at: '2026-09-05T00:00:00.000Z',
    }),
    issue({
      issue_key: 'B',
      summary: 'old switch',
      status_changed_at: '2026-06-01T00:00:00.000Z',
      updated_at: '2026-06-02T00:00:00.000Z',
    }),
    issue({
      issue_key: 'C',
      summary: 'no stamp',
      status_changed_at: null,
      updated_at: '2026-08-01T00:00:00.000Z',
    }),
  ]

  test('asc puts the oldest stamp first and the missing stamp last', () => {
    expect(sortIssues(rows, 'status_changed', 'asc').map(keyOf)).toEqual(['B', 'A', 'C'])
  })

  test('desc puts the newest stamp first and the missing stamp still last', () => {
    expect(sortIssues(rows, 'status_changed', 'desc').map(keyOf)).toEqual(['A', 'B', 'C'])
  })

  test('ties break by newest updated_at', () => {
    const tied = [
      issue({
        issue_key: 'X',
        summary: 'tied, older update',
        status_changed_at: '2026-07-01T00:00:00.000Z',
        updated_at: '2026-07-02T00:00:00.000Z',
      }),
      issue({
        issue_key: 'Y',
        summary: 'tied, newer update',
        status_changed_at: '2026-07-01T00:00:00.000Z',
        updated_at: '2026-08-02T00:00:00.000Z',
      }),
    ]
    expect(sortIssues(tied, 'status_changed', 'asc').map(keyOf)).toEqual(['Y', 'X'])
  })
})

describe('status_category group order (my-work pack)', () => {
  test('groups read in progress, then new, then done — regardless of row order', () => {
    // Input deliberately shuffled (done first) so insertion order cannot pass.
    const groups = buildGroups(
      [
        issue({ issue_key: 'NMB-9', summary: 'done', status_category: 'done' }),
        issue({ issue_key: 'NMB-1', summary: 'new', status_category: 'new' }),
        issue({ issue_key: 'NMB-5', summary: 'in progress', status_category: 'inprogress' }),
      ],
      'status_category',
    )
    expect(groups.map((g) => g.key)).toEqual(['inprogress', 'new', 'done'])
  })
})
