import { describe, it, expect } from 'vitest'
import {
  buildQueue,
  groupByPriority,
  isMine,
  matchLocal,
  mergeSearch,
  openIssues,
  overlayComments,
  pendingComment,
  relTime,
  sortQueue,
  spineToken,
} from './domain'
import type { DetailComment, IssueLite, Me } from './types'

// Fixture keys use STD-* (never GDK-*: repo doc-checks scans test files).
function issue(over: Partial<IssueLite> & { issue_key: string }): IssueLite {
  return {
    summary: 'a summary',
    project_key: 'STD',
    issue_type: 'Task',
    status: 'Open',
    status_id: '1',
    status_category: 'new',
    priority: 'Medium',
    priority_rank: 3,
    assignee: null,
    assignee_id: null,
    assignee_email: null,
    reporter: null,
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-10T00:00:00Z',
    comment_count: 0,
    reopen_count: 0,
    duedate: null,
    ...over,
  }
}

const me: Me = { email: 'dev@example.com', account_id: 'acct-1', name: 'Dev' }

describe('openIssues', () => {
  it('keys on status_category, never on display names', () => {
    const rows = [
      issue({ issue_key: 'STD-1', status: '완료', status_category: 'done' }),
      issue({ issue_key: 'STD-2', status: 'Done-ish but open', status_category: 'inprogress' }),
    ]
    expect(openIssues(rows).map((i) => i.issue_key)).toEqual(['STD-2'])
  })
})

describe('isMine', () => {
  it('prefers account id over email', () => {
    const byId = issue({ issue_key: 'STD-3', assignee_id: 'acct-1', assignee_email: 'other@x.com' })
    expect(isMine(byId, me)).toBe(true)
  })
  it('falls back to email when ids are missing', () => {
    const byEmail = issue({ issue_key: 'STD-4', assignee_email: 'dev@example.com' })
    expect(isMine(byEmail, me)).toBe(true)
  })
  it('is never mine without an identity', () => {
    expect(isMine(issue({ issue_key: 'STD-5', assignee_id: 'acct-1' }), null)).toBe(false)
  })
})

describe('sortQueue', () => {
  it('sorts rank asc, then updated desc, rank 0 last', () => {
    const rows = [
      issue({ issue_key: 'STD-10', priority_rank: 0, updated_at: '2026-08-20T00:00:00Z' }),
      issue({ issue_key: 'STD-11', priority_rank: 2, updated_at: '2026-08-01T00:00:00Z' }),
      issue({ issue_key: 'STD-12', priority_rank: 1, updated_at: '2026-08-05T00:00:00Z' }),
      issue({ issue_key: 'STD-13', priority_rank: 2, updated_at: '2026-08-15T00:00:00Z' }),
    ]
    expect(sortQueue(rows).map((i) => i.issue_key)).toEqual(['STD-12', 'STD-13', 'STD-11', 'STD-10'])
  })
})

describe('groupByPriority', () => {
  it('sections a sorted queue in rank order with display labels', () => {
    const sorted = sortQueue([
      issue({ issue_key: 'STD-20', priority_rank: 1, priority: 'Highest' }),
      issue({ issue_key: 'STD-21', priority_rank: 2, priority: 'High' }),
      issue({ issue_key: 'STD-22', priority_rank: 2, priority: 'High' }),
      issue({ issue_key: 'STD-23', priority_rank: 0, priority: null }),
    ])
    const sections = groupByPriority(sorted)
    expect(sections.map((s) => [s.label, s.issues.length])).toEqual([
      ['Highest', 1],
      ['High', 2],
      ['No priority', 1],
    ])
  })
})

describe('buildQueue', () => {
  const rows = [
    issue({ issue_key: 'STD-30', assignee_id: 'acct-1', priority_rank: 2 }),
    issue({ issue_key: 'STD-31', priority_rank: 1 }),
    issue({ issue_key: 'STD-32', status_category: 'done', assignee_id: 'acct-1' }),
  ]

  it('mine scope filters to my open issues', () => {
    const v = buildQueue(rows, me, 'mine')
    expect(v.scope).toBe('mine')
    expect(v.fellBack).toBe(false)
    expect(v.total).toBe(1)
    expect(v.sections[0].issues[0].issue_key).toBe('STD-30')
  })

  it('falls back to all, honestly flagged, when nothing is mine', () => {
    const stranger: Me = { email: 'other@example.com', account_id: 'acct-9', name: null }
    const v = buildQueue(rows, stranger, 'mine')
    expect(v.scope).toBe('all')
    expect(v.fellBack).toBe(true)
    expect(v.total).toBe(2)
  })

  it('falls back to all when the serve has no identity (standalone)', () => {
    const anon: Me = { email: '', account_id: null, name: '' }
    const v = buildQueue(rows, anon, 'mine')
    expect(v.scope).toBe('all')
    expect(v.fellBack).toBe(true)
  })
})

describe('search', () => {
  const rows = [
    issue({ issue_key: 'STD-40', summary: 'Fix the pairing gate' }),
    issue({ issue_key: 'STD-41', summary: 'Ship the ledger' }),
  ]
  it('matches key and summary case-insensitively', () => {
    expect(matchLocal(rows, 'PAIRING').map((i) => i.issue_key)).toEqual(['STD-40'])
    expect(matchLocal(rows, 'std-41').map((i) => i.issue_key)).toEqual(['STD-41'])
    expect(matchLocal(rows, '  ')).toEqual([])
  })
  it('merges server keys without duplicates, dropping unknown keys', () => {
    const local = matchLocal(rows, 'pairing')
    const merged = mergeSearch(local, ['STD-40', 'STD-41', 'STD-99'], rows)
    expect(merged.map((i) => i.issue_key)).toEqual(['STD-40', 'STD-41'])
  })
})

describe('relTime', () => {
  const now = new Date('2026-08-25T12:00:00Z')
  it('steps now → m → h → d → date', () => {
    expect(relTime('2026-08-25T11:59:30Z', now)).toBe('now')
    expect(relTime('2026-08-25T11:10:00Z', now)).toBe('50m')
    expect(relTime('2026-08-25T03:00:00Z', now)).toBe('9h')
    expect(relTime('2026-08-22T12:00:00Z', now)).toBe('3d')
    expect(relTime('2026-07-01T12:00:00Z', now)).toBe('Jul 1')
  })
  it('answers empty for missing or junk input', () => {
    expect(relTime(null, now)).toBe('')
    expect(relTime('not-a-date', now)).toBe('')
  })
})

describe('spineToken', () => {
  it('reopened open issues override their category', () => {
    expect(spineToken(issue({ issue_key: 'STD-50', reopen_count: 2, status_category: 'inprogress' }))).toBe('reopen')
    expect(spineToken(issue({ issue_key: 'STD-51', reopen_count: 2, status_category: 'done' }))).toBe('done')
    expect(spineToken(issue({ issue_key: 'STD-52', status_category: '' }))).toBe('new')
  })
})

describe('overlayComments', () => {
  const origin: DetailComment = {
    comment_id: 'c-1',
    author: 'Dev',
    created_at: '2026-08-25T11:00:00Z',
    body: 'already there',
  }

  it('appends a pending row without mutating origin', () => {
    const comments = [origin]
    const pending = pendingComment('hello', me, new Date('2026-08-25T12:00:00Z'), 'temp-1')
    const next = overlayComments(comments, pending)
    expect(next).toEqual([origin, pending])
    expect(next).not.toBe(comments)
    expect(comments).toEqual([origin])
  })

  it('is a no-op when pending is null or already in the thread', () => {
    const comments = [origin]
    expect(overlayComments(comments, null)).toBe(comments)
    expect(overlayComments(comments, { ...origin })).toEqual([origin])
  })

  it('does not invent an author when the serve has no identity', () => {
    const row = pendingComment('x', null, new Date('2026-08-25T12:00:00Z'), 'temp-2')
    expect(row.author).toBeNull()
    expect(row.comment_id.startsWith('temp-')).toBe(true)
  })
})
