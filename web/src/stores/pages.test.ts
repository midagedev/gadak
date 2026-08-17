import { describe, expect, test } from 'vitest'
import type { IssueLite } from '../lib/types'
import { sortIssues, type RelevanceContext } from './filters.svelte'
import { pageAuthorGroupKey } from './pages.svelte'

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
  chosungQuery: false,
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
