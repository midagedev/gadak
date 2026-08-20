import { describe, expect, test } from 'vitest'
import {
  cloneFilters,
  emptyFilters,
  filtersMatchIgnoringQuery,
  subtractFilters,
} from './view-config'

describe('GDK-479 view origin vs user overlay', () => {
  test('cloneFilters copies arrays (mutation of the clone leaves the source)', () => {
    const src = emptyFilters()
    src.status_category = ['new', 'inprogress']
    src.jira_project_not = ['XYZ']
    src.fields = { sprint: ['A'] }
    const copy = cloneFilters(src)
    copy.status_category.push('done')
    copy.jira_project_not.push('ABC')
    copy.fields.sprint.push('B')
    expect(src.status_category).toEqual(['new', 'inprogress'])
    expect(src.jira_project_not).toEqual(['XYZ'])
    expect(src.fields.sprint).toEqual(['A'])
  })

  test('filtersMatchIgnoringQuery ignores q and treats multi values as a set', () => {
    const a = emptyFilters()
    a.status_category = ['new', 'inprogress']
    a.q = 'foo'
    const b = emptyFilters()
    b.status_category = ['inprogress', 'new']
    b.q = 'bar'
    expect(filtersMatchIgnoringQuery(a, b)).toBe(true)
    b.status_category = ['new']
    expect(filtersMatchIgnoringQuery(a, b)).toBe(false)
  })

  test('subtractFilters drops view-default categories and keeps a user project', () => {
    const origin = emptyFilters()
    origin.status_category = ['new', 'inprogress']
    const current = emptyFilters()
    current.status_category = ['new', 'inprogress']
    current.jira_project = ['NMB']
    const extra = subtractFilters(current, origin)
    expect(extra.status_category).toEqual([])
    expect(extra.jira_project).toEqual(['NMB'])
  })

  test('subtractFilters keeps a JQL-applied category that is not in origin', () => {
    const origin = emptyFilters()
    origin.status_category = ['new', 'inprogress']
    const current = emptyFilters()
    current.status_category = ['done']
    const extra = subtractFilters(current, origin)
    expect(extra.status_category).toEqual(['done'])
  })

  test('keys overlay is all-or-nothing (one chip, not per key)', () => {
    const origin = emptyFilters()
    origin.keys = ['NMA-1', 'NMA-2']
    const same = emptyFilters()
    same.keys = ['NMA-1', 'NMA-2']
    expect(subtractFilters(same, origin).keys).toEqual([])
    const changed = emptyFilters()
    changed.keys = ['NMA-1', 'NMA-2', 'NMA-3']
    expect(subtractFilters(changed, origin).keys).toEqual(['NMA-1', 'NMA-2', 'NMA-3'])
  })
})
