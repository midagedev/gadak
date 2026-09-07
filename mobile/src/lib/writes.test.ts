import { describe, it, expect } from 'vitest'
import {
  setAssignee,
  setPriority,
  setSummary,
  setDescription,
  createIssue,
  getCreateMeta,
  getPriorities,
  searchUsers,
} from './writes'
import type { FetchLike } from './api'

// The wire contract for every write the phone sends (GDK-1497 A2): the
// method, the path, and the exact JSON body the server's handlers in
// internal/server/write.go parse. A wrapper that starts sending a display
// name where the server wants an id fails here before it ships.

function fakeFetch(status: number, body: unknown): {
  fn: FetchLike
  calls: { url: string; init: RequestInit }[]
} {
  const calls: { url: string; init: RequestInit }[] = []
  const fn: FetchLike = async (url, init) => {
    calls.push({ url, init })
    return new Response(body === null ? null : JSON.stringify(body), { status })
  }
  return { fn, calls }
}

const session = { endpoint: '', token: 'STD-token' }

describe('setAssignee — PUT <key>/assignee/ {account_id} (write.go:993)', () => {
  it('sends account_id null for the Unassigned row', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    const res = await setAssignee('STD-1', null, { session, fetchFn: fn })
    expect(calls[0].init.method).toBe('PUT')
    expect(calls[0].url).toContain('issues/STD-1/assignee/')
    expect(calls[0].init.body).toBe('{"account_id":null}')
    expect(res.issue.issue_key).toBe('STD-1')
  })
  it('sends the account id when a person is picked', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    await setAssignee('STD-1', 'acc-7', { session, fetchFn: fn })
    expect(calls[0].init.body).toBe('{"account_id":"acc-7"}')
  })
})

describe('setPriority — PUT <key>/priority/ {priority_id} (write.go:749)', () => {
  it('sends the site id, never the display name', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    await setPriority('STD-1', '3', { session, fetchFn: fn })
    expect(calls[0].init.method).toBe('PUT')
    expect(calls[0].url).toContain('issues/STD-1/priority/')
    expect(calls[0].init.body).toBe('{"priority_id":"3"}')
  })
  it('sends null to clear (the None row)', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    await setPriority('STD-1', null, { session, fetchFn: fn })
    expect(calls[0].init.body).toBe('{"priority_id":null}')
  })
})

describe('setSummary — PUT <key>/summary/ {summary} (write.go:830)', () => {
  it('sends the trimmed draft as summary', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    await setSummary('STD-1', 'Renamed from the phone', { session, fetchFn: fn })
    expect(calls[0].init.method).toBe('PUT')
    expect(calls[0].url).toContain('issues/STD-1/summary/')
    expect(calls[0].init.body).toBe('{"summary":"Renamed from the phone"}')
  })
})

describe('setDescription — PUT <key>/description/ (write.go:861)', () => {
  it('sends the text alone on a first save', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    await setDescription('STD-1', 'New body text', { session, fetchFn: fn })
    expect(calls[0].init.method).toBe('PUT')
    expect(calls[0].url).toContain('issues/STD-1/description/')
    expect(calls[0].init.body).toBe('{"description":"New body text"}')
  })
  it('adds force:true only when the 409 format_loss was answered with Replace', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-1' } })
    await setDescription('STD-1', 'New body text', { session, fetchFn: fn, force: true })
    expect(calls[0].init.body).toBe('{"description":"New body text","force":true}')
  })
})

describe('createIssue — POST create/ (write.go:1135)', () => {
  it('sends only the fields that were filled', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-531' } })
    const res = await createIssue({ summary: 'From the phone' }, { session, fetchFn: fn })
    expect(calls[0].init.method).toBe('POST')
    expect(calls[0].url).toContain('issues/create/')
    expect(calls[0].init.body).toBe('{"summary":"From the phone"}')
    expect(res.issue.issue_key).toBe('STD-531')
  })
  it('rides the project key and description when present', async () => {
    const { fn, calls } = fakeFetch(200, { issue: { issue_key: 'STD-532' } })
    await createIssue(
      { summary: 'Filed under NMB', description_text: 'With a body', project_key: 'NMB' },
      { session, fetchFn: fn },
    )
    expect(calls[0].init.body).toBe(
      '{"summary":"Filed under NMB","description_text":"With a body","project_key":"NMB"}',
    )
  })
})

describe('catalog reads', () => {
  it('getCreateMeta GETs create-meta/ (write.go:1271)', async () => {
    const { fn, calls } = fakeFetch(200, { projects: [] })
    const res = await getCreateMeta({ session, fetchFn: fn })
    expect(calls[0].init.method).toBe('GET')
    expect(calls[0].init.body).toBeUndefined()
    expect(calls[0].url).toContain('issues/create-meta/')
    expect(res.projects).toEqual([])
  })
  it('getPriorities GETs the per-key catalog (write.go:736)', async () => {
    const { fn, calls } = fakeFetch(200, { priorities: [{ id: '1', name: 'High' }] })
    const res = await getPriorities('STD-1', { session, fetchFn: fn })
    expect(calls[0].init.method).toBe('GET')
    expect(calls[0].url).toContain('issues/STD-1/priorities/')
    expect(res.priorities[0].name).toBe('High')
  })
  it('searchUsers encodes the query and forwards the abort signal (write.go:1407)', async () => {
    const { fn, calls } = fakeFetch(200, { users: [] })
    const ctl = new AbortController()
    await searchUsers('STD-1', 'jane do', { session, fetchFn: fn, signal: ctl.signal })
    expect(calls[0].init.method).toBe('GET')
    expect(calls[0].url).toContain('issues/STD-1/users/?q=jane%20do')
    expect(calls[0].init.signal).toBe(ctl.signal)
  })
})
