import { describe, expect, test } from 'vitest'
import {
  isUnattendedInProgress,
  liveSessions,
  shellForIssue,
  shouldMarkUnattended,
  type ShellSession,
} from './issue-shells'

/*
 * GDK-1164-A. Two things are pinned here and they are different sizes.
 *
 * The small one: the join. In progress, no live shell bound to the key.
 *
 * The large one: this judgment is keyed on status_category and never on a
 * display name. `status === 'In Progress'` is silently zero rows on a Korean
 * or Japanese account — the failure mode is not an error, it is a feature
 * that never fires and that nobody files a bug about. The Korean row below
 * exists so a future rewrite that reaches for the display name turns this
 * file red instead of turning the feature off for half the users.
 */

const bound = (key: string, extra: Partial<ShellSession> = {}): ShellSession => ({
  id: `sess-${key}`,
  issue_key: key,
  ...extra,
})

const issue = (key: string, status_category: string) => ({
  issue_key: key,
  status_category,
})

describe('the join', () => {
  test('a live session bound to the key is the shell for that issue', () => {
    const sessions = [bound('GDK-1'), { id: 'idle' }, bound('GDK-2')]
    expect(shellForIssue(sessions, 'GDK-2')?.id).toBe('sess-GDK-2')
    expect(shellForIssue(sessions, 'GDK-9')).toBeNull()
    expect(shellForIssue(sessions, null)).toBeNull()
  })

  test('an exited session is not a shell anything can be placed in', () => {
    const sessions = [bound('GDK-1', { exited: true })]
    expect(liveSessions(sessions)).toEqual([])
    expect(shellForIssue(sessions, 'GDK-1')).toBeNull()
  })

  test('the oldest live shell wins when two are on the same issue', () => {
    const sessions = [bound('GDK-1', { id: 'first' }), bound('GDK-1', { id: 'second' })]
    expect(shellForIssue(sessions, 'GDK-1')?.id).toBe('first')
  })
})

describe('unattended in progress', () => {
  test('in progress with no shell on it', () => {
    expect(isUnattendedInProgress(issue('GDK-1', 'inprogress'), [bound('GDK-2')])).toBe(true)
  })

  test('a shell bound to it is not unattended', () => {
    expect(isUnattendedInProgress(issue('GDK-1', 'inprogress'), [bound('GDK-1')])).toBe(false)
  })

  test('done and new are not the population at all', () => {
    expect(isUnattendedInProgress(issue('GDK-1', 'done'), [])).toBe(false)
    expect(isUnattendedInProgress(issue('GDK-1', 'new'), [])).toBe(false)
  })

  test("Jira's own category spelling counts", () => {
    // status_category on a Jira mirror row is `indeterminate`, not the UI's
    // bucket name; effectiveCategory owns that mapping.
    expect(isUnattendedInProgress(issue('GDK-1', 'indeterminate'), [])).toBe(true)
    expect(isUnattendedInProgress(issue('GDK-1', 'complete'), [])).toBe(false)
    expect(isUnattendedInProgress(issue('GDK-1', 'todo'), [])).toBe(false)
  })

  test('a localized display name is not a category, and must not become one', () => {
    // These are the *names* a Korean and a Japanese account show for the same
    // three buckets. None of them is a status_category, so none of them may
    // change the verdict — an implementation that matched names would call
    // '진행 중' in progress and '완료' anything but done.
    for (const name of ['진행 중', '進行中', 'In Progress', '완료', '完了']) {
      expect(
        isUnattendedInProgress(issue('GDK-1', name), []),
        `display name ${name} keyed a category`,
      ).toBe(false)
    }
  })

  test('a row with no category is not accused', () => {
    // effectiveCategory falls back to 'inprogress' for an unknown value —
    // right for filtering, wrong for marking. An unmirrored status is a gap
    // in gadak, not an abandoned claim.
    expect(isUnattendedInProgress(issue('GDK-1', ''), [])).toBe(false)
    expect(isUnattendedInProgress(issue('GDK-1', '   '), [])).toBe(false)
    expect(isUnattendedInProgress(null, [])).toBe(false)
  })
})

describe('when the mark is shown', () => {
  test('no live session anywhere: the statement is true of everything, so it is drawn on nothing', () => {
    expect(isUnattendedInProgress(issue('GDK-1', 'inprogress'), [])).toBe(true)
    expect(shouldMarkUnattended(issue('GDK-1', 'inprogress'), [])).toBe(false)
    expect(shouldMarkUnattended(issue('GDK-1', 'inprogress'), [bound('GDK-9', { exited: true })])).toBe(
      false,
    )
  })

  test('one shell running turns it into information', () => {
    expect(shouldMarkUnattended(issue('GDK-1', 'inprogress'), [bound('GDK-9')])).toBe(true)
    expect(shouldMarkUnattended(issue('GDK-1', 'inprogress'), [bound('GDK-1')])).toBe(false)
    expect(shouldMarkUnattended(issue('GDK-1', 'done'), [bound('GDK-9')])).toBe(false)
  })
})
