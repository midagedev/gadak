/*
 * GDK-2004: the grouping twin of effectiveCategory's unknown-key fold. Go's
 * internal/statuscat.Category answers "new" for a key its table does not
 * know — it can only ever miss a reopen, never invent one. Both web folds
 * now give the same answer, and this file pins the grouper's half of that.
 */
import { describe, expect, test } from 'vitest'
import { groupCategory, type GroupableIssue } from './issue-group'

function row(status_category: string): GroupableIssue {
  return {
    issue_key: 'STD-70',
    status: null,
    status_category,
    assignee: null,
    priority: null,
    priority_rank: null,
    issue_type: null,
  }
}

describe('groupCategory (GDK-2004)', () => {
  test("unknown key folds to 'new', matching internal/statuscat and effectiveCategory", () => {
    expect(groupCategory(row('not-a-category'))).toBe('new')
  })
})
