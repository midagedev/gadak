import { describe, expect, it } from 'vitest'
import { setAssignee, assigneeStaleRefusal } from './writes'
import { ApiError, type FetchLike } from './api'

// The stale-assignee refusal on the phone (GDK-2073), the wire half: what
// the sheet's pick sees when the server refuses a 409 assignee_changed —
// the same preflight `gadak assign` pays (GDK-2063). The rendered half (the
// sheet keeps standing and says who took the issue) is a source contract in
// ui/detail/detail-sheets.test.ts's territory; here the transport must hand
// the sheet the code it branches on and the body it paints from.

function fakeFetch(status: number, body: unknown): FetchLike {
  return async () => new Response(body === null ? null : JSON.stringify(body), { status })
}

const session = { endpoint: '', token: 'STD-token' }

const REFUSAL = {
  error: 'assignee_changed',
  current_assignee: { account_id: 'acc-hc', display_name: '김현철' },
  issue: { issue_key: 'STD-1', assignee: '김현철', assignee_id: 'acc-hc' },
}

describe('setAssignee 409 assignee_changed (write_issue.go handleAssignee)', () => {
  it('rejects with the code and carries the parsed body — code alone is not enough', async () => {
    await expect(
      setAssignee('STD-1', 'acc-7', { session, fetchFn: fakeFetch(409, REFUSAL) }),
    ).rejects.toMatchObject({ code: 'assignee_changed', status: 409 })
  })

  it('assigneeStaleRefusal answers the holder and the refreshed row', async () => {
    const err = await setAssignee('STD-1', 'acc-7', {
      session,
      fetchFn: fakeFetch(409, REFUSAL),
    }).catch((e: unknown) => e)
    const stale = assigneeStaleRefusal(err)
    expect(stale?.issue.issue_key).toBe('STD-1')
    expect(stale?.issue.assignee_id).toBe('acc-hc')
    expect(stale?.holderName).toBe('김현철')
  })

  it('names nobody as null holderName, so the sheet paints the Unassigned label', async () => {
    const err = await setAssignee('STD-1', 'acc-7', {
      session,
      fetchFn: fakeFetch(409, {
        error: 'assignee_changed',
        current_assignee: null,
        issue: { issue_key: 'STD-1', assignee: null, assignee_id: null },
      }),
    }).catch((e: unknown) => e)
    expect(assigneeStaleRefusal(err)?.holderName).toBeNull()
  })

  it('a body without a row is not a stale refusal — the sheet falls back to errorMessage', async () => {
    const err = await setAssignee('STD-1', 'acc-7', {
      session,
      fetchFn: fakeFetch(409, { error: 'assignee_changed', current_assignee: null }),
    }).catch((e: unknown) => e)
    expect(assigneeStaleRefusal(err)).toBeNull()
  })

  it('every other error is not a stale refusal, whatever its shape', () => {
    expect(assigneeStaleRefusal(new ApiError('assignee_changed', 500))).toBeNull()
    expect(assigneeStaleRefusal(new ApiError('credential_required', 409))).toBeNull()
    expect(assigneeStaleRefusal(new Error('network'))).toBeNull()
    expect(assigneeStaleRefusal(null)).toBeNull()
  })

  it('a success still answers the issue (the refusal changed nothing else)', async () => {
    const res = await setAssignee('STD-1', 'acc-7', {
      session,
      fetchFn: fakeFetch(200, { issue: { issue_key: 'STD-1' } }),
    })
    expect(res.issue.issue_key).toBe('STD-1')
  })
})
