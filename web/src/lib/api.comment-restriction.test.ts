/*
 * GDK-528: the comment request body is a wire contract. An untouched
 * composer must produce the exact bytes a pre-GDK-528 client sent —
 * `{"text":…,"mentions":…,"attachment_ids":…}` and nothing else — so the
 * restriction is additive, never a re-shape. The opts form rides only the
 * keys that were chosen.
 *
 * Node env, fetch stubbed per case (api.session-boundary.test.ts's shape);
 * config() falls back to DEFAULTS so nothing is dialed.
 */
import { afterEach, describe, expect, test, vi } from 'vitest'
import { postComment } from './api'

const OK_COMMENT = {
  issue: { issue_key: 'NMB-1' },
  comment: {
    comment_id: 'c1',
    author: 'Dana',
    body: 'hello',
    created_at: '2026-09-11T00:00:00.000Z',
  },
}

/** Stub fetch, capture the init jsonW handed it, answer a comment 200. */
function captureBody(): () => string {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: unknown, init?: RequestInit) => {
      calls.push(String(init?.body))
      return new Response(JSON.stringify(OK_COMMENT), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
  return () => calls[0]
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('postComment body (GDK-528)', () => {
  test('no opts → the exact pre-GDK-528 bytes (byte-identical public comment)', async () => {
    const body = captureBody()
    await postComment('NMB-1', 'hello')
    expect(body()).toBe(JSON.stringify({ text: 'hello', mentions: [], attachment_ids: [] }))
  })

  test('visibility rides as the server decodes it: {type, value}', async () => {
    const body = captureBody()
    await postComment('NMB-1', 'hello', [], [], {
      visibility: { type: 'role', value: 'Administrators' },
    })
    expect(JSON.parse(body())).toEqual({
      text: 'hello',
      mentions: [],
      attachment_ids: [],
      visibility: { type: 'role', value: 'Administrators' },
    })
  })

  test('internal true rides; false is omitted, not sent as false', async () => {
    const body = captureBody()
    await postComment('NMB-1', 'hello', [], [], { internal: true })
    const parsed = JSON.parse(body()) as Record<string, unknown>
    expect(parsed.internal).toBe(true)
    expect('visibility' in parsed).toBe(false)

    const again = captureBody()
    await postComment('NMB-1', 'hello', [], [], { internal: false })
    expect(JSON.parse(again())).toEqual({ text: 'hello', mentions: [], attachment_ids: [] })
  })

  test('both chosen: visibility and internal share the body', async () => {
    const body = captureBody()
    await postComment('NMB-1', 'hello', [], [], {
      visibility: { type: 'group', value: 'jira-admins' },
      internal: true,
    })
    expect(JSON.parse(body())).toEqual({
      text: 'hello',
      mentions: [],
      attachment_ids: [],
      visibility: { type: 'group', value: 'jira-admins' },
      internal: true,
    })
  })
})
