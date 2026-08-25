import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import type { IssueLite } from './types'

// The store calls request(); the rest of api.ts (ApiError, isPairingDead)
// must stay real so pairing-dead vs network is the production classifier.
vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return {
    ...actual,
    request: vi.fn(),
    configureApi: vi.fn(),
  }
})

import { request } from './api'
import { app, issuesBootKind, searchPaint, showOfflineBanner, sync, unpair } from './store.svelte'

const mem = new Map<string, string>()

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

function resetApp(): void {
  app.phase = 'paired'
  app.meta = { endpoint: '', label: 'test', expires_at: '' }
  app.rejected = false
  app.issues = []
  app.me = null
  app.views = []
  app.sources = []
  app.pages = []
  app.loaded = false
  app.syncing = false
  app.offline = false
  app.lastSyncAt = null
  app.detail = null
  app.tab = 'issues'
}

beforeEach(() => {
  mem.clear()
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
  resetApp()
  vi.mocked(request).mockReset()
})

afterEach(async () => {
  // enterPaired starts a 60s timer; unpair is the production owner that
  // clears it. Tests that never pair still call it so a leaked interval
  // cannot hold the worker.
  app.phase = 'paired'
  await unpair()
})

describe('issuesBootKind — failed bootstrap is not a skeleton (GDK-905)', () => {
  it('stays skeleton until the first attempt has settled', () => {
    expect(
      issuesBootKind({
        loaded: false,
        offline: false,
        issueCount: 0,
        pageCount: 0,
        lastSyncAt: null,
      }),
    ).toBe('skeleton')
    expect(
      issuesBootKind({
        loaded: false,
        offline: true,
        issueCount: 0,
        pageCount: 0,
        lastSyncAt: null,
      }),
    ).toBe('skeleton')
  })

  it('is failed when the attempt settled offline with nothing cached', () => {
    expect(
      issuesBootKind({
        loaded: true,
        offline: true,
        issueCount: 0,
        pageCount: 0,
        lastSyncAt: null,
      }),
    ).toBe('failed')
  })

  it('is ready when a snapshot exists, even if the latest sync failed', () => {
    expect(
      issuesBootKind({
        loaded: true,
        offline: true,
        issueCount: 1,
        pageCount: 0,
        lastSyncAt: null,
      }),
    ).toBe('ready')
    expect(
      issuesBootKind({
        loaded: true,
        offline: true,
        issueCount: 0,
        pageCount: 1,
        lastSyncAt: null,
      }),
    ).toBe('ready')
    expect(
      issuesBootKind({
        loaded: true,
        offline: true,
        issueCount: 0,
        pageCount: 0,
        lastSyncAt: new Date('2026-08-01T00:00:00Z'),
      }),
    ).toBe('ready')
    expect(
      issuesBootKind({
        loaded: true,
        offline: false,
        issueCount: 0,
        pageCount: 0,
        lastSyncAt: null,
      }),
    ).toBe('ready')
  })
})

describe('showOfflineBanner — must not claim a snapshot that does not exist', () => {
  it('is silent when offline with nothing cached', () => {
    expect(showOfflineBanner({ offline: true, issueCount: 0, pageCount: 0, lastSyncAt: null })).toBe(
      false,
    )
  })

  it('shows when offline and a snapshot (rows or a prior successful sync) exists', () => {
    expect(showOfflineBanner({ offline: true, issueCount: 2, pageCount: 0, lastSyncAt: null })).toBe(
      true,
    )
    expect(
      showOfflineBanner({
        offline: true,
        issueCount: 0,
        pageCount: 0,
        lastSyncAt: new Date('2026-08-01T00:00:00Z'),
      }),
    ).toBe(true)
  })

  it('is silent when online', () => {
    expect(showOfflineBanner({ offline: false, issueCount: 2, pageCount: 0, lastSyncAt: null })).toBe(
      false,
    )
  })
})

describe('searchPaint — empty, searching, and failed are distinct (GDK-905)', () => {
  it('is idle when the query is empty, regardless of other flags', () => {
    expect(
      searchPaint({ query: '', resultCount: 0, pageCount: 0, searching: false, failed: false }),
    ).toBe('idle')
    expect(
      searchPaint({ query: '  ', resultCount: 0, pageCount: 0, searching: true, failed: true }),
    ).toBe('idle')
  })

  it('paints results whenever a local or server hit exists', () => {
    expect(
      searchPaint({ query: 'ab', resultCount: 1, pageCount: 0, searching: true, failed: false }),
    ).toBe('results')
    expect(
      searchPaint({ query: 'ab', resultCount: 0, pageCount: 1, searching: false, failed: true }),
    ).toBe('results')
  })

  it('does not paint empty while a server request is still out', () => {
    expect(
      searchPaint({ query: 'ab', resultCount: 0, pageCount: 0, searching: true, failed: false }),
    ).toBe('searching')
  })

  it('paints failed, not empty, when the server refused and local is empty', () => {
    expect(
      searchPaint({ query: 'ab', resultCount: 0, pageCount: 0, searching: false, failed: true }),
    ).toBe('failed')
  })

  it('paints empty only after a settled success with no hits', () => {
    expect(
      searchPaint({ query: 'ab', resultCount: 0, pageCount: 0, searching: false, failed: false }),
    ).toBe('empty')
  })
})

describe('sync() — failed bootstrap settles, and does not write the snapshot', () => {
  it('sets loaded on a network failure with nothing cached', async () => {
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0))
    await sync()
    expect(app.loaded).toBe(true)
    expect(app.offline).toBe(true)
    expect(app.phase).toBe('paired')
    expect(app.issues).toEqual([])
  })

  it('does not write gadak.snapshot when bootstrap fails', async () => {
    vi.mocked(request).mockRejectedValue(new ApiError('internal_error', 502))
    await sync()
    expect(mem.get('gadak.snapshot')).toBeUndefined()
    expect(app.issues).toEqual([])
  })

  it('keeps a cached snapshot readable when a later sync fails', async () => {
    const cached = [issue({ issue_key: 'STD-1', summary: 'cached row' })]
    app.issues = cached
    app.loaded = true
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0))
    await sync()
    expect(app.issues).toBe(cached)
    expect(app.issues[0].issue_key).toBe('STD-1')
    expect(app.loaded).toBe(true)
    expect(app.offline).toBe(true)
    expect(mem.get('gadak.snapshot')).toBeUndefined()
  })

  it('does not settle as a painted snapshot when the pairing itself is dead', async () => {
    vi.mocked(request).mockRejectedValue(new ApiError('pairing_rejected', 401))
    await sync()
    expect(app.phase).toBe('unpaired')
    expect(app.rejected).toBe(true)
    expect(app.loaded).toBe(false)
    expect(mem.get('gadak.snapshot')).toBeUndefined()
  })

  it('writes the snapshot only after a successful bootstrap', async () => {
    const row = issue({ issue_key: 'STD-2' })
    vi.mocked(request).mockImplementation(async (path: string) => {
      if (path === 'issues/bootstrap/') {
        return {
          status: 200,
          etag: '"sv-1"',
          body: { issues: [row], server_time: '2026-08-01T00:00:00Z', sync_version: 1 },
        }
      }
      if (path === 'auth/me/') return { status: 200, etag: null, body: null }
      if (path === 'issues/views/') return { status: 200, etag: null, body: { views: [], source: [] } }
      if (path === 'issues/pages/') return { status: 200, etag: null, body: { pages: [] } }
      throw new Error(`unexpected path ${path}`)
    })
    await sync()
    expect(app.loaded).toBe(true)
    expect(app.offline).toBe(false)
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-2'])
    const raw = mem.get('gadak.snapshot')
    expect(raw).toBeTruthy()
    expect(JSON.parse(raw as string).issues[0].issue_key).toBe('STD-2')
  })
})
