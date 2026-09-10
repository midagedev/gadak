/*
 * GDK-1538: the phone records the reads it makes.
 *
 * The defect class is "a surface consumes an awareness signal it never
 * feeds": the detail response carries last_visited_at / previous_visit_at,
 * the serve computes both from recorded visits, and the phone read them
 * while posting none — so on a workspace opened only from a phone the
 * resume card could never appear, and no test noticed because every
 * assertion was on the *rendering* of a boundary the fixture supplied.
 *
 * These tests measure the feeding side: opening posts, on the desk's own
 * route, with the desk's debounce, and never as a gate on the screen.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { VISIT_DEBOUNCE_MS } from '../../../web/src/lib/history'

vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return { ...actual, request: vi.fn(), configureApi: vi.fn() }
})

import { request } from './api'
import { app, closeIssue, openIssue, openPage, recordVisit, resetVisitDebounce } from './store.svelte'

const mockRequest = vi.mocked(request)

/** Every POST this test's subject made, in order. */
function visits(): Array<{ kind: string; key: string }> {
  return mockRequest.mock.calls
    .filter(([path]) => path === 'issues/history/visits/')
    .map(([, opts]) => (opts as { body: { kind: string; key: string } }).body)
}

beforeEach(() => {
  mockRequest.mockReset()
  mockRequest.mockResolvedValue({ status: 201, etag: null, body: {} as never })
  resetVisitDebounce()
  app.demo = false
  closeIssue()
})

describe('GDK-1538 the phone feeds the visit history it reads', () => {
  it('opening an issue posts one visit on the desk route, and opening a page posts a page visit', () => {
    openIssue('STD-30')
    openPage('9912')
    expect(visits()).toEqual([
      { kind: 'issue', key: 'STD-30' },
      { kind: 'page', key: '9912' },
    ])
    // The route and method are the desk's (internal/server/server.go:234 —
    // note the `issues/` base every serve route sits under; a path without
    // it 404s, which is what the e2e beside this measured); the
    // body carries no `source` — the server decides that, so the phone
    // cannot claim to be something else.
    const [path, opts] = mockRequest.mock.calls[0] as [string, { method: string; body: object }]
    expect(path).toBe('issues/history/visits/')
    expect(opts.method).toBe('POST')
    expect(Object.keys(opts.body).sort()).toEqual(['key', 'kind'])
  })

  it('collapses a remount inside the desk window and counts A → B → A as two reads of A', () => {
    const t0 = 1_000_000
    recordVisit('issue', 'STD-30', t0)
    recordVisit('issue', 'STD-30', t0 + VISIT_DEBOUNCE_MS - 1)
    expect(visits()).toHaveLength(1)
    recordVisit('issue', 'STD-31', t0 + 10)
    recordVisit('issue', 'STD-30', t0 + 20)
    expect(visits()).toEqual([
      { kind: 'issue', key: 'STD-30' },
      { kind: 'issue', key: 'STD-31' },
      { kind: 'issue', key: 'STD-30' },
    ])
  })

  it('opens the screen anyway when the visit is refused, and never posts in demo', async () => {
    mockRequest.mockRejectedValue(new Error('offline'))
    expect(() => openIssue('STD-30')).not.toThrow()
    expect(app.detail).toEqual({ kind: 'issue', key: 'STD-30' })
    await Promise.resolve()

    mockRequest.mockReset()
    app.demo = true
    openIssue('STD-31')
    expect(app.detail).toEqual({ kind: 'issue', key: 'STD-31' })
    expect(visits()).toEqual([])
    app.demo = false
  })
})
