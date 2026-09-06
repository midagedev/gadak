import { describe, expect, test } from 'vitest'
import type { IssueLite } from './types'
import { assignedTo, delegatedBy, isSamePerson, reportedBy } from './person-match'

const me = { accountId: 'acc-1', email: 'Dana@Example.com' }
const issue = (over: Partial<IssueLite>): IssueLite => over as IssueLite

describe('person-match', () => {
  test('id wins, email is the fallback and case-insensitive', () => {
    expect(isSamePerson('acc-1', null, me)).toBe(true)
    expect(isSamePerson(null, 'dana@example.com', me)).toBe(true)
    expect(isSamePerson('acc-2', 'dana@example.com', me)).toBe(true) // email still matches
    expect(isSamePerson('acc-2', 'other@example.com', me)).toBe(false)
  })

  test('empty on either side never matches', () => {
    expect(isSamePerson(null, null, me)).toBe(false)
    expect(isSamePerson('', '', me)).toBe(false)
    expect(isSamePerson('acc-1', 'dana@example.com', { accountId: null, email: null })).toBe(false)
    expect(isSamePerson('', 'dana@example.com', { accountId: 'acc-1', email: '' })).toBe(false)
  })

  test('assignedTo / reportedBy / delegatedBy', () => {
    const mine = issue({ assignee_id: 'acc-1', assignee_email: null, reporter_id: 'acc-9', reporter_email: null })
    expect(assignedTo(mine, me)).toBe(true)
    expect(reportedBy(mine, me)).toBe(false)
    expect(delegatedBy(mine, me)).toBe(false)

    const handedOff = issue({ reporter_id: 'acc-1', reporter_email: null, assignee_id: 'acc-9', assignee_email: null })
    expect(delegatedBy(handedOff, me)).toBe(true)

    const unassignedOwn = issue({ reporter_email: 'dana@example.com', assignee_id: null, assignee_email: null })
    expect(delegatedBy(unassignedOwn, me)).toBe(true) // nobody holds it: still handed off

    const selfReported = issue({ reporter_id: 'acc-1', assignee_id: 'acc-1' })
    expect(delegatedBy(selfReported, me)).toBe(false)
  })
})
