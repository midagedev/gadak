import { describe, expect, test } from 'vitest'
import type { IssueLite } from '../lib/types'
import { emptyFilters } from '../lib/view-config'
import { buildGroups, filterIssues } from './filters.svelte'

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
