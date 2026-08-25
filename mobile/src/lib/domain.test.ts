import { describe, it, expect } from 'vitest'
import {
  applyFilters,
  bodyParagraphs,
  buildList,
  buildScopes,
  docsSpaceScopeId,
  effectiveCategory,
  groupByPriority,
  isMine,
  matchLocal,
  mergeSearch,
  openIssues,
  overlayComments,
  pendingComment,
  relTime,
  resolveScope,
  scopeCount,
  scopePages,
  SCOPE_ALL_OPEN,
  SCOPE_DOCS_UPDATED,
  SCOPE_ME,
  sortIssues,
  sortPages,
  spaceLabel,
  spineToken,
  unsupportedAxes,
  type Scope,
} from './domain'
import { t } from './i18n'
import type { DetailComment, IssueLite, Me, PageLite, SavedViewDoc, SourceViewDoc } from './types'

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

describe('sortIssues', () => {
  it('sorts rank asc, then updated desc, rank 0 last', () => {
    const rows = [
      issue({ issue_key: 'STD-10', priority_rank: 0, updated_at: '2026-08-20T00:00:00Z' }),
      issue({ issue_key: 'STD-11', priority_rank: 2, updated_at: '2026-08-01T00:00:00Z' }),
      issue({ issue_key: 'STD-12', priority_rank: 1, updated_at: '2026-08-05T00:00:00Z' }),
      issue({ issue_key: 'STD-13', priority_rank: 2, updated_at: '2026-08-15T00:00:00Z' }),
    ]
    expect(sortIssues(rows).map((i) => i.issue_key)).toEqual(['STD-12', 'STD-13', 'STD-11', 'STD-10'])
  })
})

describe('groupByPriority', () => {
  it('sections a sorted list in rank order with display labels', () => {
    const sorted = sortIssues([
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

/* ── scopes: the heading is the current scope's name (GDK-885) ── */

function savedView(id: string, name: string, filters: Record<string, unknown>): SavedViewDoc {
  return {
    id,
    name,
    owner_email: null,
    owner_name: null,
    config: { filters } as SavedViewDoc['config'],
    created_at: null,
    updated_at: null,
  }
}

function jiraFilter(
  id: string,
  name: string,
  filters: Record<string, unknown>,
  unsupported: string[] = [],
): SourceViewDoc {
  return {
    id,
    name,
    config: { filters } as SourceViewDoc['config'],
    jql: 'project = STD',
    favourite: false,
    applied: [],
    unsupported,
  }
}

const scopeOf = (list: Scope[], id: string): Scope => {
  const hit = list.find((s) => s.id === id)
  if (!hit) throw new Error(`no scope ${id}`)
  return hit
}

describe('effectiveCategory', () => {
  it('folds the desk aliases, unknown reads as inprogress', () => {
    expect(effectiveCategory(issue({ issue_key: 'STD-60', status_category: 'todo' }))).toBe('new')
    expect(effectiveCategory(issue({ issue_key: 'STD-61', status_category: 'indeterminate' }))).toBe('inprogress')
    expect(effectiveCategory(issue({ issue_key: 'STD-62', status_category: 'completed' }))).toBe('done')
    expect(effectiveCategory(issue({ issue_key: 'STD-63', status_category: 'weird' }))).toBe('inprogress')
  })
})

describe('buildScopes', () => {
  it('names the two hardcoded scopes from the desktop catalog, never its own words', () => {
    const list = buildScopes([], [], me)
    expect(scopeOf(list, SCOPE_ME).name).toBe(t('personal.myAssignee'))
    expect(scopeOf(list, SCOPE_ME).name).toBe('Assigned to me')
    expect(scopeOf(list, SCOPE_ALL_OPEN).name).toBe(t('view.allOpen.name'))
    expect(scopeOf(list, SCOPE_ALL_OPEN).name).toBe('All open')
  })

  it('offers Assigned to me only when the serve has an identity', () => {
    expect(buildScopes([], [], null).map((s) => s.id)).toEqual([SCOPE_ALL_OPEN])
  })

  it('sections saved views and imported Jira filters apart', () => {
    const list = buildScopes(
      [savedView('v1', 'Stale bugs', { status_category: ['new'] })],
      [jiraFilter('s1', 'Sprint board', { jira_project: ['STD'] })],
      me,
    )
    expect(list.map((s) => [s.section, s.name])).toEqual([
      ['me', 'Assigned to me'],
      ['builtin', 'All open'],
      ['views', 'Stale bugs'],
      ['filters', 'Sprint board'],
    ])
  })

  it('disables a view whose axes the phone cannot evaluate', () => {
    const list = buildScopes([savedView('v2', 'By label', { labels: ['infra'] })], [], me)
    expect(scopeOf(list, 'view:v2').unsupported).toEqual(['labels'])
  })

  it('disables an imported filter the desktop could not compile', () => {
    const list = buildScopes([], [jiraFilter('s2', 'Watched', {}, ['watcher'])], me)
    expect(scopeOf(list, 'source:s2').unsupported).toEqual(['watcher'])
  })
})

describe('unsupportedAxes', () => {
  it('accepts exactly the axes an IssueLite can answer', () => {
    expect(
      unsupportedAxes({
        status_category: ['new'],
        status_category_not: ['done'],
        assignee_email: ['acct-1'],
        assignee_email_not: ['acct-2'],
        unassigned: false,
        issue_type: ['Bug'],
        priority: ['Highest'],
        jira_project: ['STD'],
        jira_project_not: ['OTH'],
        labels: [],
        q: '',
      }),
    ).toEqual([])
  })

  it('names every axis it cannot honor, dynamic fields included', () => {
    expect(
      unsupportedAxes({
        reporter_email: ['x'],
        stale: true,
        updated_from: '2026-01-01',
        q: 'ledger',
        fields: { severity: ['S1'], area: [] },
      }),
    ).toEqual(['fields.severity', 'q', 'reporter_email', 'stale', 'updated_from'])
  })

  it('treats a missing config as unhonorable rather than as no filter', () => {
    expect(unsupportedAxes(null)).toEqual(['config'])
  })
})

describe('applyFilters', () => {
  const rows = [
    issue({ issue_key: 'STD-70', assignee_id: 'acct-1', issue_type: 'Bug', issue_type_id: '10004' }),
    issue({ issue_key: 'OTH-71', status_category: 'done', priority: 'Highest', priority_id: '1' }),
    issue({ issue_key: 'STD-72', assignee_email: 'DEV@example.com' }),
  ]

  it('keys status on the effective category, not the display name', () => {
    expect(applyFilters(rows, { status_category: ['done'] }).map((i) => i.issue_key)).toEqual(['OTH-71'])
    expect(applyFilters(rows, { status_category_not: ['done'] }).map((i) => i.issue_key)).toEqual([
      'STD-70',
      'STD-72',
    ])
  })

  it('matches an assignee by account id first, then by email case-insensitively', () => {
    expect(applyFilters(rows, { assignee_email: ['acct-1'] }).map((i) => i.issue_key)).toEqual(['STD-70'])
    expect(applyFilters(rows, { assignee_email: ['dev@example.com'] }).map((i) => i.issue_key)).toEqual([
      'STD-72',
    ])
    expect(applyFilters(rows, { assignee_email_not: ['acct-1'] }).map((i) => i.issue_key)).toEqual([
      'OTH-71',
      'STD-72',
    ])
  })

  it('honors unassigned', () => {
    expect(applyFilters(rows, { unassigned: true }).map((i) => i.issue_key)).toEqual(['OTH-71'])
  })

  it('honors the stored id first and the stored name as the fallback', () => {
    expect(applyFilters(rows, { issue_type: ['10004'] }).map((i) => i.issue_key)).toEqual(['STD-70'])
    expect(applyFilters(rows, { issue_type: ['Bug'] }).map((i) => i.issue_key)).toEqual(['STD-70'])
    expect(applyFilters(rows, { priority: ['1'] }).map((i) => i.issue_key)).toEqual(['OTH-71'])
  })

  it('keys jira_project on the issue-key prefix', () => {
    expect(applyFilters(rows, { jira_project: ['STD'] }).map((i) => i.issue_key)).toEqual([
      'STD-70',
      'STD-72',
    ])
    expect(applyFilters(rows, { jira_project_not: ['STD'] }).map((i) => i.issue_key)).toEqual(['OTH-71'])
  })
})

describe('buildList', () => {
  const rows = [
    issue({ issue_key: 'STD-30', assignee_id: 'acct-1', priority_rank: 2 }),
    issue({ issue_key: 'STD-31', priority_rank: 1 }),
    issue({ issue_key: 'STD-32', status_category: 'done', assignee_id: 'acct-1' }),
  ]
  const scopes = (who: Me | null) => buildScopes([savedView('v1', 'Done work', { status_category: ['done'] })], [], who)

  it('Assigned to me filters to my open issues', () => {
    const v = buildList(rows, me, scopeOf(scopes(me), SCOPE_ME))
    expect(v.scopeId).toBe(SCOPE_ME)
    expect(v.fellBack).toBe(false)
    expect(v.total).toBe(1)
    expect(v.sections[0].issues[0].issue_key).toBe('STD-30')
  })

  it('falls back to All open, honestly flagged, when nothing is mine', () => {
    const stranger: Me = { email: 'other@example.com', account_id: 'acct-9', name: null }
    const v = buildList(rows, stranger, scopeOf(scopes(stranger), SCOPE_ME))
    expect(v.scopeId).toBe(SCOPE_ALL_OPEN)
    expect(v.fellBack).toBe(true)
    expect(v.total).toBe(2)
  })

  it('falls back to All open when the serve has no identity (standalone)', () => {
    const anon: Me = { email: '', account_id: null, name: '' }
    const v = buildList(rows, anon, scopeOf(scopes(anon), SCOPE_ALL_OPEN))
    expect(v.scopeId).toBe(SCOPE_ALL_OPEN)
    expect(v.fellBack).toBe(false)
  })

  it('a saved view paints exactly what it selects, done rows included', () => {
    const v = buildList(rows, me, scopeOf(scopes(me), 'view:v1'))
    expect(v.scopeId).toBe('view:v1')
    expect(v.total).toBe(1)
    expect(v.sections[0].issues[0].issue_key).toBe('STD-32')
  })
})

describe('resolveScope', () => {
  const list = buildScopes(
    [savedView('v1', 'Mine only', { assignee_email: ['acct-1'] }), savedView('v2', 'By label', { labels: ['x'] })],
    [],
    me,
  )

  it('restores the persisted scope', () => {
    expect(resolveScope(list, 'view:v1')?.id).toBe('view:v1')
  })

  it('falls back silently when the saved view is gone', () => {
    expect(resolveScope(list, 'view:deleted')?.id).toBe(SCOPE_ME)
  })

  it('never restores a scope the phone refuses', () => {
    expect(resolveScope(list, 'view:v2')?.id).toBe(SCOPE_ME)
  })
})

describe('scopeCount (GDK-886)', () => {
  const rows = [
    issue({ issue_key: 'STD-80', assignee_id: 'acct-1' }),
    issue({ issue_key: 'STD-81', assignee_id: 'acct-1' }),
    issue({ issue_key: 'STD-82' }),
  ]
  const list = buildScopes(
    [savedView('v1', 'Nobody', { assignee_email: ['acct-none'] }), savedView('v2', 'By label', { labels: ['x'] })],
    [],
    me,
  )

  it('counts each scope in memory', () => {
    expect(scopeCount(rows, me, scopeOf(list, SCOPE_ME))).toBe(2)
    expect(scopeCount(rows, me, scopeOf(list, SCOPE_ALL_OPEN))).toBe(3)
  })

  it('a view matching nothing shows 0 and stays selectable', () => {
    const empty = scopeOf(list, 'view:v1')
    expect(scopeCount(rows, me, empty)).toBe(0)
    expect(empty.unsupported).toEqual([])
  })

  it('a disabled view has no count', () => {
    expect(scopeCount(rows, me, scopeOf(list, 'view:v2'))).toBeNull()
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

function page(over: Partial<PageLite> & { key: string }): PageLite {
  return {
    title: 'a page',
    space_key: 'ENG',
    space_name: 'Engineering',
    parent_id: '',
    author: 'Dana',
    updated_at: '2026-08-10T00:00:00Z',
    version: 1,
    url: '',
    excerpt: '',
    ...over,
  }
}

describe('documents scopes (GDK-887)', () => {
  const pages = [
    page({ key: '2', space_key: 'PROD', space_name: 'Product', updated_at: '2026-08-12T00:00:00Z' }),
    page({ key: '1', space_key: 'ENG', space_name: 'Engineering', updated_at: '2026-08-20T00:00:00Z' }),
    page({ key: '3', space_key: 'ENG', space_name: 'Engineering', updated_at: '2026-08-01T00:00:00Z' }),
  ]

  it('omits the Documents section when the page list is empty', () => {
    expect(buildScopes([], [], me).some((s) => s.section === 'docs')).toBe(false)
  })

  it('names the whole-mirror plate from the catalog and keys spaces on space_key', () => {
    const list = buildScopes([], [], me, pages)
    const docs = list.filter((s) => s.section === 'docs')
    expect(docs[0].id).toBe(SCOPE_DOCS_UPDATED)
    expect(docs[0].name).toBe(t('docs.tabUpdated'))
    expect(docs[0].name).toBe('Updated')
    expect(docs[0].kind).toBe('pages')
    expect(docs[0].spaceKey).toBeNull()
    expect(docs.slice(1).map((s) => [s.id, s.spaceKey, s.name])).toEqual([
      [docsSpaceScopeId('ENG'), 'ENG', 'Engineering'],
      [docsSpaceScopeId('PROD'), 'PROD', 'Product'],
    ])
  })

  it('falls back to space_key when space_name is empty', () => {
    const list = buildScopes([], [], me, [page({ key: '9', space_key: 'OPS', space_name: '' })])
    expect(scopeOf(list, docsSpaceScopeId('OPS')).name).toBe('OPS')
    expect(spaceLabel(page({ key: '9', space_key: 'OPS', space_name: '  ' }))).toBe('OPS')
  })

  it('places Documents after Jira filters', () => {
    const list = buildScopes(
      [savedView('v1', 'Stale bugs', { status_category: ['new'] })],
      [jiraFilter('s1', 'Sprint board', { jira_project: ['STD'] })],
      me,
      pages,
    )
    expect(list.map((s) => s.section)).toEqual(['me', 'builtin', 'views', 'filters', 'docs', 'docs', 'docs'])
  })

  it('counts pages in memory, keyed on space_key', () => {
    const list = buildScopes([], [], me, pages)
    expect(scopeCount([], me, scopeOf(list, SCOPE_DOCS_UPDATED), pages)).toBe(3)
    expect(scopeCount([], me, scopeOf(list, docsSpaceScopeId('ENG')), pages)).toBe(2)
    expect(scopeCount([], me, scopeOf(list, docsSpaceScopeId('PROD')), pages)).toBe(1)
  })

  it('sorts a documents plate by updated_at desc', () => {
    const list = buildScopes([], [], me, pages)
    expect(scopePages(pages, scopeOf(list, SCOPE_DOCS_UPDATED)).map((p) => p.key)).toEqual(['1', '2', '3'])
    expect(sortPages(pages).map((p) => p.key)).toEqual(['1', '2', '3'])
  })
})

describe('bodyParagraphs', () => {
  it('splits on blank lines and yields nothing for empty body', () => {
    expect(bodyParagraphs('')).toEqual([])
    expect(bodyParagraphs('  \n\n  ')).toEqual([])
    expect(bodyParagraphs('one\n\ntwo\nthree')).toEqual(['one', 'two\nthree'])
  })
})
