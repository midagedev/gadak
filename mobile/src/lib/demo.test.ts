import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, request, type FetchLike } from './api'
import { demoRequest, isDemoSession, setDemoSession } from './demo'

// Secure is a spy surface here, not a mock: the demo must never call it,
// and the assertion is the call count (0), not canned behavior.
vi.mock('./secure', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./secure')>()
  return {
    ...actual,
    tokenGet: vi.fn(actual.tokenGet),
    tokenSet: vi.fn(actual.tokenSet),
    tokenDel: vi.fn(actual.tokenDel),
  }
})

import { tokenGet, tokenSet, tokenDel } from './secure'
import { app, boot, enterDemo, exitDemo, rememberSearch, setScope, sync, unpair } from './store.svelte'
import type { BootstrapResponse } from './types'

const HERE = dirname(fileURLToPath(import.meta.url))
const BUNDLE = join(HERE, '../../public/demo')

/* ── localStorage: a real-storage-shaped stub, byte-comparable ── */

const mem = new Map<string, string>()

function primeRealWorkspace(): Record<string, string> {
  const entries: Record<string, string> = {
    'gadak.pairing.meta': JSON.stringify({ endpoint: 'https://desk.example.ts.net', label: 'desk', expires_at: '' }),
    'gadak.snapshot': JSON.stringify({
      etag: '"sv-real"',
      issues: [{ issue_key: 'REAL-7', summary: 'a real row' }],
      serverTime: '2026-08-01T00:00:00Z',
    }),
    'gadak.views': JSON.stringify({ views: [], sources: [] }),
    'gadak.pages': JSON.stringify({ pages: [] }),
    'gadak.issues.scope': '"SCOPE-v_real"',
    'gadak.search.recents': JSON.stringify(['real query']),
  }
  for (const [k, v] of Object.entries(entries)) mem.set(k, v)
  return { ...entries }
}

/* ── window.fetch stub: routes by URL so each transport is identifiable ── */

const DEMO_BOOTSTRAP: BootstrapResponse = {
  server_time: '2026-08-28T00:00:00Z',
  sync_version: 1,
  issues: [
    {
      issue_key: 'DEMO-1',
      summary: 'a demo row',
      project_key: 'DEMO',
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
    },
  ],
}

const REAL_BOOTSTRAP: BootstrapResponse = {
  server_time: '2026-08-28T00:00:00Z',
  sync_version: 1,
  issues: [{ ...DEMO_BOOTSTRAP.issues[0], issue_key: 'REAL-7', summary: 'a real row' }],
}

function jsonResponse(doc: unknown, status = 200): Response {
  return new Response(JSON.stringify(doc), { status, headers: { 'Content-Type': 'application/json' } })
}

const fetchCalls: string[] = []
const stubFetch: FetchLike = async (url) => {
  fetchCalls.push(url)
  if (url === '/demo/bootstrap.json') return jsonResponse(DEMO_BOOTSTRAP)
  if (url.startsWith('/demo/detail/')) return jsonResponse({ issue_key: 'DEMO-1', comments: [], linked_issues: [] })
  if (url.includes('/api/v1/issues/bootstrap/')) return jsonResponse(REAL_BOOTSTRAP)
  return jsonResponse({ error: 'not_found' }, 404)
}

beforeEach(() => {
  mem.clear()
  fetchCalls.length = 0
  globalThis.localStorage = {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => {
      mem.set(k, v)
    },
    removeItem: (k: string) => {
      mem.delete(k)
    },
    clear: () => mem.clear(),
    key: () => null,
    length: 0,
  } as Storage
  ;(globalThis as { window?: unknown }).window = { fetch: stubFetch }
  vi.mocked(tokenGet).mockClear()
  vi.mocked(tokenSet).mockClear()
  vi.mocked(tokenDel).mockClear()
})

afterEach(async () => {
  if (app.demo) exitDemo()
  app.phase = 'paired'
  await unpair()
  delete (globalThis as { window?: unknown }).window
})

/* ── transport contract (demo.ts) ── */

describe('demoRequest — the response contract of the bundled bundle', () => {
  const fileFetch = (doc: unknown, status = 200): { fn: FetchLike; urls: string[] } => {
    const urls: string[] = []
    return {
      urls,
      fn: async (url) => {
        urls.push(url)
        return jsonResponse(doc, status)
      },
    }
  }

  it('serves bootstrap from /demo/bootstrap.json', async () => {
    const f = fileFetch(DEMO_BOOTSTRAP)
    const res = await demoRequest<BootstrapResponse>('issues/bootstrap/', { fetchFn: f.fn })
    expect(f.urls).toEqual(['/demo/bootstrap.json'])
    expect(res.status).toBe(200)
    expect(res.body?.issues[0].issue_key).toBe('DEMO-1')
  })

  it('serves an issue detail from /demo/detail/<KEY>.json, key shape-gated', async () => {
    const f = fileFetch({ issue_key: 'DEMO-1', comments: [], linked_issues: [] })
    await demoRequest('issues/DEMO-1/detail/', { fetchFn: f.fn })
    expect(f.urls).toEqual(['/demo/detail/DEMO-1.json'])
    // Path traversal shapes never become file URLs.
    await expect(demoRequest('issues/..%2Fetc/detail/', { fetchFn: f.fn })).rejects.toBeInstanceOf(ApiError)
    expect(f.urls).toHaveLength(1)
  })

  it('answers auth/me with the local-origin no-identity shape', async () => {
    const f = fileFetch(null)
    const res = await demoRequest<{ email: null; account_id: null; name: null }>('auth/me/', { fetchFn: f.fn })
    expect(res.body).toEqual({ email: null, account_id: null, name: null })
    expect(f.urls).toHaveLength(0)
  })

  it('answers views with the empty ViewsResponse the picker degrades on', async () => {
    const res = await demoRequest<{ views: unknown[]; source: unknown[] }>('issues/views/', { fetchFn: fileFetch(null).fn })
    expect(res.body).toEqual({ views: [], source: [] })
  })

  it('answers feed with zero unread so the glance strip is absent, not broken', async () => {
    const res = await demoRequest<{ items: unknown[]; unread_counts: Record<string, number> }>(
      'issues/feed/?focus=all&limit=20',
      { fetchFn: fileFetch(null).fn },
    )
    expect(res.body).toEqual({ items: [], unread_counts: { all: 0, assignee: 0, reporter: 0, mention: 0 } })
  })

  it('answers pages 404 — v1 demo has no Documents section', async () => {
    await expect(demoRequest('issues/pages/', { fetchFn: fileFetch(null).fn })).rejects.toMatchObject({
      code: 'not_found',
      status: 404,
    })
    await expect(demoRequest('issues/pages/DEMO-1/', { fetchFn: fileFetch(null).fn })).rejects.toMatchObject({
      code: 'not_found',
    })
  })

  it('answers server search 404 — search stays local-first over the snapshot', async () => {
    await expect(
      demoRequest('issues/search/?q=edge&limit=50', { fetchFn: fileFetch(null).fn }),
    ).rejects.toMatchObject({ code: 'not_found', status: 404 })
  })

  it('answers the transitions sheet with the serve write refusal (409 credential_required)', async () => {
    // Not 404: the sheet exists to fire a write, and the 409 rides Detail's
    // writes-off degradation instead of an error sheet.
    await expect(demoRequest('issues/DEMO-1/transitions/', { fetchFn: fileFetch(null).fn })).rejects.toMatchObject({
      code: 'credential_required',
      status: 409,
    })
  })

  it('refuses every write with 409 credential_required before any fetch', async () => {
    const f = fileFetch({ changed: true })
    for (const method of ['POST', 'PUT', 'DELETE', 'PATCH']) {
      await expect(demoRequest('issues/DEMO-1/comment/', { method, fetchFn: f.fn })).rejects.toMatchObject({
        code: 'credential_required',
        status: 409,
      })
    }
    expect(f.urls).toHaveLength(0)
  })

  it('maps a failed bundle fetch to ApiError(network) and unknown paths to 404', async () => {
    const throwing: FetchLike = async () => {
      throw new TypeError('Load failed')
    }
    await expect(demoRequest('issues/bootstrap/', { fetchFn: throwing })).rejects.toMatchObject({ code: 'network' })
    await expect(demoRequest('terminal/sessions/', { fetchFn: fileFetch(null).fn })).rejects.toMatchObject({
      code: 'not_found',
      status: 404,
    })
  })
})

describe('request() delegation — one branch, invisible when off', () => {
  it('delegates to the bundle while the demo session is on', async () => {
    setDemoSession(true)
    try {
      const res = await request<BootstrapResponse>('issues/bootstrap/', { fetchFn: stubFetch })
      expect(res.body?.issues[0].issue_key).toBe('DEMO-1')
    } finally {
      setDemoSession(false)
    }
  })

  it('is byte-identical to the old path when the demo is off (non-interference)', async () => {
    expect(isDemoSession()).toBe(false)
    const res = await request<BootstrapResponse>('issues/bootstrap/', {
      session: { endpoint: 'https://desk.example.ts.net', token: null },
      dev: true,
      fetchFn: stubFetch,
    })
    expect(fetchCalls).toEqual(['/api/v1/issues/bootstrap/'])
    expect(res.body?.issues[0].issue_key).toBe('REAL-7')
  })
})

/* ── contamination gates (GDK-1051): the demo never touches persistence ── */

describe('demo session contamination gates', () => {
  // Gate 1 — a demo round-trip leaves every persistent key byte-identical.
  // Failure mode this catches: if enterDemo rode the real sync() path,
  // CACHE_KEY would be rewritten with demo rows and VIEWS_KEY/PAGES_KEY
  // with demo empties; if exitDemo reused unpair(), the keys would be
  // dropped outright. Either drift changes these bytes.
  it('enter→browse→exit leaves localStorage byte-identical and the Keychain untouched', async () => {
    const primed = primeRealWorkspace()
    await enterDemo()
    expect(app.demo).toBe(true)
    expect(app.phase).toBe('paired')
    // Browse activity that has persistence-capable side paths.
    setScope('demo-scope')
    rememberSearch('demo query')
    await sync() // must be a no-op in demo, not a cache write
    const detailRes = await request<{ issue_key: string }>('issues/DEMO-1/detail/')
    expect(detailRes.body?.issue_key).toBe('DEMO-1')
    exitDemo()

    expect(app.demo).toBe(false)
    expect(app.phase).toBe('unpaired')
    const now: Record<string, string> = {}
    for (const [k, v] of mem.entries()) now[k] = v
    expect(now).toEqual(primed)
    expect(vi.mocked(tokenSet)).not.toHaveBeenCalled()
    expect(vi.mocked(tokenDel)).not.toHaveBeenCalled()
  })

  // Gate 2 — after exit, boot() from the real pairing restores the real
  // snapshot, proving the demo transport let go. Failure mode: a demo flag
  // stuck on would serve DEMO rows into a paired session.
  it('boot() after demo exit restores the real workspace snapshot', async () => {
    await enterDemo()
    exitDemo()
    mem.clear()
    primeRealWorkspace()
    mem.set('gadak.dev.token', 'placeholder-not-a-real-token')
    fetchCalls.length = 0
    await boot()
    expect(app.demo).toBe(false)
    expect(app.phase).toBe('paired')
    expect(app.issues.map((i) => i.issue_key)).toEqual(['REAL-7'])
    expect(fetchCalls.every((u) => !u.startsWith('/demo/'))).toBe(true)
  })

  // Gate 3 — demo exit is not an unpair: the permanent-unpaired marker must
  // not appear, or a later dev re-adoption / fresh pairing would be blocked
  // by debris from a session that never had credentials.
  it('never records the unpaired marker during a demo session', async () => {
    await enterDemo()
    exitDemo()
    expect(mem.get('gadak.pairing.unpaired')).toBeUndefined()
  })
})

/* ── bundle gates: the committed assets match what the transport promises ── */

function walkFiles(dir: string, acc: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walkFiles(p, acc)
    else acc.push(p)
  }
  return acc
}

describe('committed demo bundle (mobile/public/demo)', () => {
  // Failure mode: a regenerated bootstrap without matching detail files (or
  // vice versa) makes taps dead-end in the not-found plate. Regeneration
  // drift is exactly what this pins, in both directions.
  it('has a detail file for every bootstrap key, and no detail files beyond them', () => {
    const boot = JSON.parse(readFileSync(join(BUNDLE, 'bootstrap.json'), 'utf8')) as {
      issues: { issue_key: string }[]
    }
    expect(boot.issues.length).toBeGreaterThan(0)
    const keys = new Set(boot.issues.map((i) => i.issue_key))
    const detailFiles = new Set(
      readdirSync(join(BUNDLE, 'detail'))
        .filter((n) => n.endsWith('.json'))
        .map((n) => n.replace(/\.json$/, '')),
    )
    expect([...keys].filter((k) => !detailFiles.has(k)), 'bootstrap keys missing a detail file').toEqual([])
    expect([...detailFiles].filter((k) => !keys.has(k)), 'stale detail files without a bootstrap row').toEqual([])
  })

  it('contains the wire fields the phone parses, on every row', () => {
    const boot = JSON.parse(readFileSync(join(BUNDLE, 'bootstrap.json'), 'utf8')) as {
      issues: { issue_key: string; status_category?: string; priority_rank?: number }[]
    }
    for (const row of boot.issues) {
      expect(row.status_category, `${row.issue_key} status_category`).toMatch(/^(new|inprogress|done)$/)
      expect(typeof row.priority_rank, `${row.issue_key} priority_rank`).toBe('number')
    }
  })

  // Failure mode: the TestFlight .ipa contract greps the shipped bundle for
  // the reserved tour string; this catches the leak at test time instead of
  // upload time. (The regeneration script guards the same before staging.)
  it('never contains the reserved tour string in any bundled file', () => {
    for (const p of walkFiles(BUNDLE)) {
      expect(readFileSync(p, 'utf8').includes('demo-tour'), `${p} contains the reserved string`).toBe(false)
    }
  })
})
