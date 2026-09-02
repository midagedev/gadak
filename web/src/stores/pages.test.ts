import { describe, expect, test } from 'vitest'
import type { IssueLite } from '../lib/types'
import { emptyConfig, emptyFilters } from '../lib/view-config'
import { zoneNamed } from '../lib/calendar'
import { filterIssues, sortIssues, type RelevanceContext } from './filters.svelte'
import { dashboards } from './dashboards.svelte'
import { pages, pageAuthorGroupKey } from './pages.svelte'
import { showIssueList } from '../lib/show-issue-list'

describe('pageAuthorGroupKey', () => {
  test('I8: group key is author_id, then display name', () => {
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: 'acc-1' })).toBe('acc-1')
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: 'acc-2' })).toBe('acc-2')
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: '' })).toBe('Kim')
    expect(pageAuthorGroupKey({ author: 'Kim' })).toBe('Kim')
    expect(pageAuthorGroupKey({ author: '', author_id: '' })).toBe('')
  })
})

// sortIssues lives in filters.svelte.ts. The default vitest `unit` project has
// no svelte plugin (runes are `$state is not defined` there); this file is the
// pages-store project that already compiles .svelte.ts, so the deference
// gate lives here rather than in a new filters.test.ts.
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

const relevanceCtx: RelevanceContext = {
  needle: 'rel-1',
  now: Date.parse('2026-08-01T00:00:00.000Z'),
  myEmail: null,
  myAccountId: null,
  recentKeys: new Set(),
}

describe('sortIssues server deference', () => {
  const exact = issue({ issue_key: 'REL-1', summary: 'Unrelated title' })
  const mention = issue({
    issue_key: 'REL-2',
    summary: 'Notes that mention REL-1 in the title',
    updated_at: '2026-07-31T00:00:00.000Z',
  })
  const other = issue({ issue_key: 'REL-3', summary: 'Something else about rel-1 nearby' })

  test('without a server order, local key-exact still ranks first', () => {
    const got = sortIssues([mention, exact, other], 'relevance', 'desc', relevanceCtx)
    expect(got.map((i) => i.issue_key)).toEqual(['REL-1', 'REL-2', 'REL-3'])
  })

  test('when the server ranked the hits, its order wins over local score', () => {
    // Local score puts REL-1 (key exact = 1000) above REL-2 (title include = 100).
    // Server said REL-2 then REL-1 — that order is the primary key.
    const got = sortIssues([mention, exact, other], 'relevance', 'desc', relevanceCtx, undefined, [
      'REL-2',
      'REL-1',
    ])
    expect(got.map((i) => i.issue_key)).toEqual(['REL-2', 'REL-1', 'REL-3'])
  })

  test('issues the server did not rank follow, ordered by local score', () => {
    // Server ranked only REL-2. REL-1 (local 1000) and REL-3 (local 100) stay
    // behind it, still ordered by local score among themselves.
    const got = sortIssues([other, mention, exact], 'relevance', 'desc', relevanceCtx, undefined, [
      'REL-2',
    ])
    expect(got.map((i) => i.issue_key)).toEqual(['REL-2', 'REL-1', 'REL-3'])
  })
})

describe('retired jamo-only issue match (GDK-168)', () => {
  test('a jamo-only query against the issue text-match path', () => {
    const f = emptyFilters()
    f.q = 'ㅋㅌㅂ'
    const hit = issue({ issue_key: 'NMB-1', summary: '커트백 가이드' })
    const miss = issue({ issue_key: 'NMB-2', summary: 'unrelated latin title' })
    expect(() => filterIssues([hit, miss], f)).not.toThrow()
    const got = filterIssues([hit, miss], f)
    expect(got).toEqual([])
  })
})

describe('GDK-250 date calendar (FAIL-first)', () => {
  test('created_from=2026-08-18 includes a 01:00 KST instant stored as 17T16:00Z', () => {
    // Asia/Seoul 2026-08-18 01:00 == 2026-08-17T16:00:00.000Z.
    // UTC-prefix compare (iso.slice(0,10)) dropped this row (red 2026-08-18).
    // Zone is pinned so a UTC CI runner cannot hide the miss.
    const seoul = zoneNamed('Asia/Seoul')
    const f = emptyFilters()
    f.created_from = '2026-08-18'
    const it = issue({
      issue_key: 'NMB-KST',
      summary: 'created 01:00 KST',
      created_at: '2026-08-17T16:00:00.000Z',
    })
    const got = filterIssues([it], f, seoul)
    expect(got.map((row) => row.issue_key)).toEqual(['NMB-KST'])
    expect(filterIssues([it], f, zoneNamed('UTC')).map((row) => row.issue_key)).toEqual([])
  })

  test('due_from compares the stored calendar date, not a UTC instant', () => {
    const f = emptyFilters()
    f.due_from = '2026-08-20'
    const hit = issue({ issue_key: 'NMB-DUE', summary: 'due', duedate: '2026-08-20' })
    const miss = issue({ issue_key: 'NMB-EARLY', summary: 'early', duedate: '2026-08-19' })
    expect(filterIssues([hit, miss], f, zoneNamed('America/Los_Angeles')).map((r) => r.issue_key)).toEqual([
      'NMB-DUE',
    ])
  })
})

// The id-first *call sites*. web/src/lib/view-config.test.ts pins
// matchesIdFirst itself; what lives only here is that filters.svelte.ts still
// hands it the id at all. Stop passing status_id/priority_id/issue_type_id and
// the filter silently falls back to display names — the Korean-account 0-rows
// trap, arriving through the call site instead of through the matcher.
//
// What this does NOT catch: swapping the id and name arguments. matchesIdFirst
// accepts a match on either one by design (view-config.ts), so the swap is
// behaviour-neutral and no assertion here can see it. Measured, not assumed.
//
// e2e used to cover this by driving `?st=` / `?pr=` in a browser (identity-web,
// tail-audit); GDK-289 deleted those, so this is where that half went — the
// wire keys themselves are pinned in view-config.test.ts. issue_type has no
// deleted e2e behind it and is included anyway: it is the third field the
// display-name rule names, and its call site can rot the same way.
describe('id-first filter call sites (GDK-289, moved down from e2e)', () => {
  test('status: an id filter matches a row whose display name is localized', () => {
    const f = emptyFilters()
    f.status = ['sid-1']
    const hit = issue({
      issue_key: 'NMB-ST1',
      summary: 'localized status, wanted id',
      status_id: 'sid-1',
      status: '로캘-상태명',
    })
    const miss = issue({
      issue_key: 'NMB-ST2',
      summary: 'localized status, other id',
      status_id: 'sid-2',
      status: '로캘-상태명',
    })
    expect(filterIssues([hit, miss], f).map((r) => r.issue_key)).toEqual(['NMB-ST1'])
  })

  test('status: an English name does not match a localized row that carries an id', () => {
    const f = emptyFilters()
    f.status = ['In Progress']
    const it = issue({
      issue_key: 'NMB-ST3',
      summary: 'the Korean-account 0-rows trap',
      status_id: 'sid-1',
      status: '로캘-상태명',
    })
    expect(filterIssues([it], f)).toEqual([])
  })

  test('status: a row with no status_id still matches by name', () => {
    const f = emptyFilters()
    f.status = ['LegacyNameOnly']
    const it = issue({ issue_key: 'NMB-ST4', summary: 'name only', status: 'LegacyNameOnly' })
    expect(filterIssues([it], f).map((r) => r.issue_key)).toEqual(['NMB-ST4'])
  })

  test('priority: an id filter matches a localized priority display', () => {
    const f = emptyFilters()
    f.priority = ['pri-1']
    const hit = issue({
      issue_key: 'NMB-PR1',
      summary: 'localized priority, wanted id',
      priority_id: 'pri-1',
      priority: '로캘-우선순위',
    })
    const miss = issue({
      issue_key: 'NMB-PR2',
      summary: 'localized priority, other id',
      priority_id: 'pri-2',
      priority: '로캘-우선순위',
    })
    expect(filterIssues([hit, miss], f).map((r) => r.issue_key)).toEqual(['NMB-PR1'])
  })

  test('issue type: an id filter matches a localized type display', () => {
    const f = emptyFilters()
    f.issue_type = ['it-1']
    const hit = issue({
      issue_key: 'NMB-IT1',
      summary: 'localized type, wanted id',
      issue_type_id: 'it-1',
      issue_type: '버그',
    })
    const miss = issue({
      issue_key: 'NMB-IT2',
      summary: 'localized type, other id',
      issue_type_id: 'it-2',
      issue_type: '버그',
    })
    expect(filterIssues([hit, miss], f).map((r) => r.issue_key)).toEqual(['NMB-IT1'])
  })
})

describe('parent filter (GDK-521)', () => {
  test('parent matches parent_key case-insensitively', () => {
    const f = emptyFilters()
    f.parent = ['GDK-126']
    const hit = issue({ issue_key: 'GDK-1', summary: 'child', parent_key: 'GDK-126' })
    const fold = issue({ issue_key: 'GDK-2', summary: 'fold', parent_key: 'gdk-126' })
    const miss = issue({ issue_key: 'GDK-3', summary: 'other', parent_key: 'GDK-1' })
    const none = issue({ issue_key: 'GDK-4', summary: 'none', parent_key: null })
    expect(filterIssues([hit, fold, miss, none], f).map((r) => r.issue_key)).toEqual(['GDK-1', 'GDK-2'])
  })
})

/*
 * GDK-815: a dashboard that holds the main column must give it up like any
 * other full-column surface. showIssueList closed feed/docs/history but not
 * `dashboards`, so applying a view painted the list behind the dashboard and
 * the URL said one thing while the screen said another. Store-level red:
 * the sidebar tint hole is pinned in e2e (dashboards.spec.ts), where the
 * row rendering lives.
 */
describe('GDK-815 the dashboard releases the main column', () => {
  test('showIssueList closes a dashboard that holds the column', () => {
    // applyConfig writes the applied view into the hash; node has no history
    // API. Only that URL write is stubbed — the assertion is about the
    // store, not the hash.
    ;(globalThis as { history?: Partial<History> }).history ??= {
      replaceState: () => {},
      pushState: () => {},
    }
    ;(globalThis as { location?: Partial<Location> }).location ??= { hash: '' }
    dashboards.open('e2e-gdk815')
    expect(dashboards.openId).toBe('e2e-gdk815')
    showIssueList(emptyConfig())
    // Red before the fix: openId keeps the id, and the applied view renders
    // behind the dashboard.
    expect(dashboards.openId).toBeNull()
  })

  test('pages.open counts the history view', () => {
    // `open` feeds the keymap's docsOpen; history owning the column without
    // `open` knowing is the same hole one layer down.
    pages.openHistory()
    expect(pages.historyView).toBe(true)
    // Red before the fix: open = docsView || spaceView — history invisible.
    expect(pages.open).toBe(true)
    pages.closeHistory()
  })
})
