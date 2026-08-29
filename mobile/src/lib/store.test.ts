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
import { getActiveHostId, hostIdForEndpoint, listHosts, setActiveHostId, upsertHostFromPairing } from './hosts'
import { tokenGet, tokenSet } from './secure'
import {
  app,
  boot,
  issuesBootKind,
  removeRosterHost,
  searchPaint,
  showOfflineBanner,
  switchHost,
  sync,
  unpair,
} from './store.svelte'

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

describe('unpair() — forgetting the server forgets the shell too', () => {
  // Both tokens are issued by the same serve. Dropping only the serve slot
  // left a live shell Bearer in the Keychain and a TERM_META_KEY that
  // loadTerminal() re-adopts on the next launch, so the phone came back with
  // a Shell tab pointed at a server the user had unpaired from.
  // Assertions are presence-only: a token value never reaches a log or a test.
  it('deletes the terminal token and its meta', async () => {
    await tokenSet('placeholder-not-a-real-token', 'terminal')
    localStorage.setItem(
      'gadak.pairing.meta.terminal',
      JSON.stringify({ endpoint: 'http://127.0.0.1:7877', label: 'desk', expires_at: '' }),
    )
    app.terminal = { endpoint: 'http://127.0.0.1:7877', label: 'desk', expires_at: '' }
    expect(await tokenGet('terminal')).not.toBeNull()

    app.phase = 'paired'
    await unpair()

    expect(await tokenGet('terminal')).toBeNull()
    expect(mem.get('gadak.pairing.meta.terminal')).toBeUndefined()
    expect(app.terminal).toBeNull()
  })
})

describe('loadTerminal() — dev adopts the proxy for the shell (GDK-1118)', () => {
  // boot() has always adopted the vite proxy for the serve session; the
  // shell did not, so the Shell tab was unreachable in dev without a QR
  // dance and the capture tour listed "a stored terminal pairing" as an
  // environment precondition the operator arranged by hand. The server
  // side already agreed — terminalGate's local rule admits a loopback peer
  // with no Bearer (internal/server/terminal.go terminalLocal) — so the
  // gap was entirely client-side.
  //
  // Driven through boot()'s own dev adoption — no meta, no token, the proxy
  // answering — because that is the only path production takes to
  // loadTerminal(), and a test seam here would assert a function nothing
  // else calls.
  //
  // FAIL-first (2026-08-29, before the loadTerminal change): the first case
  // asserted `app.terminal?.label` and got `undefined`.
  const answer = (over: Record<string, unknown> = {}) =>
    ({ status: 200, etag: null, body: { sessions: [], ...over } }) as never

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('adopts when the probe answers, with no meta and no token', async () => {
    vi.stubEnv('VITE_DEV_SHELL', '1')
    vi.mocked(request).mockResolvedValue(answer())

    await boot()

    expect(app.phase).toBe('paired')
    expect(app.terminal?.label).toBe('This Mac (dev)')
    expect(app.terminal?.endpoint).toBe('')
  })

  it('armed, it wins over a stored pairing and takes the label from the arm', async () => {
    // A rig has to put the app in a known state; a simulator that scanned
    // an offer months ago would otherwise keep pointing the pane — and the
    // heading a recording shows — at that old pairing.
    localStorage.setItem(
      'gadak.pairing.meta.terminal',
      JSON.stringify({ endpoint: 'http://127.0.0.1:9999', label: 'stale-pairing', expires_at: '' }),
    )
    await tokenSet('placeholder-terminal-token', 'terminal')
    vi.stubEnv('VITE_DEV_SHELL', '1')
    vi.stubEnv('VITE_DEV_SHELL_LABEL', 'home')
    vi.mocked(request).mockResolvedValue(answer())

    await boot()

    expect(app.terminal?.label).toBe('home')
    expect(app.terminal?.endpoint).toBe('')
  })

  it('stays out of the way unarmed — the three-tab default is reachable in dev', async () => {
    // The adoption is opt-in for this row. mobile/e2e/shell.spec.ts asserts
    // a default install shows three tabs and that the Pairing tab offers the
    // terminal field; adopting on sight made both unreachable in dev, which
    // is the only place that suite runs.
    vi.mocked(request).mockResolvedValue(answer())

    await boot()

    expect(app.phase).toBe('paired')
    expect(app.terminal).toBeNull()
  })

  it('leaves the tab absent when the terminal gate refuses', async () => {
    vi.stubEnv('VITE_DEV_SHELL', '1')
    // The serve half still adopts; only the shell probe is refused — the
    // shape of a serve whose terminal gate wants a token this device does
    // not hold.
    vi.mocked(request).mockImplementation(async (path: string) => {
      if (path.startsWith('terminal/')) throw new ApiError('scope_rejected', 403)
      return answer()
    })

    await boot()

    expect(app.phase).toBe('paired')
    expect(app.terminal).toBeNull()
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

describe('boot() legacy→roster migration (GDK-1097 B1) — no failure path unpairs', () => {
  // Keychain is simulated on the dev path (localStorage dev slots), the
  // same convention secure.test.ts uses. Endpoints are TEST-NET.
  const EP = 'http://192.0.2.10:7877'

  function seedLegacyPairing(): void {
    localStorage.setItem(
      'gadak.pairing.meta',
      JSON.stringify({ endpoint: EP, label: 'desk', expires_at: '' }),
    )
    localStorage.setItem(
      'gadak.pairing.meta.terminal',
      JSON.stringify({ endpoint: EP, label: 'desk shell', expires_at: '' }),
    )
  }

  /** Replaces the mem storage with one whose setItem throws for chosen keys. */
  function breakSetItem(forKeys: (key: string) => boolean): void {
    globalThis.localStorage = {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => {
        if (forKeys(k)) throw new Error('storage write refused')
        mem.set(k, v)
      },
      removeItem: (k: string) => {
        mem.delete(k)
      },
      clear: () => mem.clear(),
      key: () => null,
      length: 0,
    } as Storage
  }

  it('success: verified copy, roster row, legacy slots retired, paired entry', async () => {
    seedLegacyPairing()
    await tokenSet('placeholder-serve-token', 'serve')
    await tokenSet('placeholder-terminal-token', 'terminal')
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0))
    const hostId = await hostIdForEndpoint(EP)

    await boot()

    expect(app.phase).toBe('paired')
    // Token now lives in the host slot — value equality, never a log.
    expect(mem.get(`gadak.dev.token@${hostId}`)).toBe('placeholder-serve-token')
    expect(mem.get(`gadak.dev.token.terminal@${hostId}`)).toBe('placeholder-terminal-token')
    // Legacy slots retired.
    expect(mem.get('gadak.dev.token')).toBeUndefined()
    expect(mem.get('gadak.dev.token.terminal')).toBeUndefined()
    // One roster row, active, revision 1.
    const doc = JSON.parse(mem.get('gadak.hosts.v1') as string)
    expect(doc.schema).toBe(1)
    expect(doc.hosts).toHaveLength(1)
    expect(doc.hosts[0].id).toBe(hostId)
    expect(doc.hosts[0].pairingRevision).toBe(1)
    expect(doc.hosts[0].label).toBe('desk')
    expect(mem.get('gadak.hosts.active')).toBe(hostId)
    // The terminal pairing rode along.
    expect(app.terminal?.label).toBe('desk shell')
  })

  it('token copy fails: legacy preserved, no roster, boot still enters paired', async () => {
    seedLegacyPairing()
    await tokenSet('placeholder-serve-token', 'serve')
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0))
    breakSetItem(() => true) // every write fails — tokenSet cannot land

    await boot()

    // Legacy untouched — this is the no-unpair guarantee.
    expect(mem.get('gadak.dev.token')).toBe('placeholder-serve-token')
    expect(await tokenGet('serve')).toBe('placeholder-serve-token')
    expect(mem.get('gadak.hosts.v1')).toBeUndefined()
    expect(mem.get('gadak.hosts.active')).toBeUndefined()
    expect(app.phase).toBe('paired')
  })

  it('roster write fails quietly: legacy is NOT retired (read-back gate)', async () => {
    seedLegacyPairing()
    await tokenSet('placeholder-serve-token', 'serve')
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0))
    const hostId = await hostIdForEndpoint(EP)
    // Token writes succeed; only the roster doc and the active pointer
    // cannot land. A silent failure here must not delete the legacy slot
    // — that path would strand boot with no token address at all.
    breakSetItem((k) => k === 'gadak.hosts.v1' || k === 'gadak.hosts.active')

    await boot()

    expect(mem.get(`gadak.dev.token@${hostId}`)).toBe('placeholder-serve-token') // copy made
    expect(mem.get('gadak.dev.token')).toBe('placeholder-serve-token') // legacy kept
    expect(mem.get('gadak.hosts.v1')).toBeUndefined()
    expect(app.phase).toBe('paired')
  })

  it('idempotent: a second boot does not re-migrate or resurrect legacy slots', async () => {
    seedLegacyPairing()
    await tokenSet('placeholder-serve-token', 'serve')
    await tokenSet('placeholder-terminal-token', 'terminal')
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0))

    await boot()
    const afterFirst = JSON.parse(mem.get('gadak.hosts.v1') as string)

    await boot()
    const afterSecond = JSON.parse(mem.get('gadak.hosts.v1') as string)

    expect(afterSecond.hosts).toHaveLength(1)
    expect(afterSecond.hosts[0].id).toBe(afterFirst.hosts[0].id)
    expect(afterSecond.hosts[0].createdAt).toBe(afterFirst.hosts[0].createdAt)
    expect(afterSecond.hosts[0].pairingRevision).toBe(1)
    expect(mem.get('gadak.dev.token')).toBeUndefined()
    expect(mem.get('gadak.dev.token.terminal')).toBeUndefined()
    expect(app.phase).toBe('paired')
  })
})

describe('switchHost() — per-host caches never cross (GDK-1097 B2)', () => {
  // The contamination contract, same class as the demo gates: what host A
  // synced must never be visible while host B is active, and switching back
  // must restore A's snapshot from its own bytes — B's session never
  // overwrote them. Endpoints are TEST-NET; token values are placeholders.
  const EP_A = 'http://192.0.2.10:7877'
  const EP_B = 'http://192.0.2.11:7877'

  async function seedHost(ep: string, label: string, issueKey: string): Promise<string> {
    const id = await hostIdForEndpoint(ep)
    await upsertHostFromPairing({ endpoint: ep, label })
    mem.set(`gadak.dev.token@${id}`, `placeholder-token-${label}`)
    mem.set(`gadak.pairing.meta@${id}`, JSON.stringify({ endpoint: ep, label, expires_at: '' }))
    mem.set(
      `gadak.snapshot@${id}`,
      JSON.stringify({
        etag: null,
        issues: [issue({ issue_key: issueKey })],
        serverTime: '2026-08-01T00:00:00Z',
      }),
    )
    return id
  }

  async function bootOn(a: string): Promise<void> {
    setActiveHostId(a)
    vi.mocked(request).mockRejectedValue(new ApiError('network', 0)) // caches are the story
    await boot()
    expect(app.phase).toBe('paired')
  }

  it('loads the target host snapshot on switch and leaves the old host bytes untouched', async () => {
    const a = await seedHost(EP_A, 'desk A', 'STD-A1')
    const b = await seedHost(EP_B, 'desk B', 'STD-B1')
    await bootOn(a)
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-A1'])

    expect(await switchHost(b)).toBe(true)
    expect(getActiveHostId()).toBe(b)
    expect(app.meta?.label).toBe('desk B')
    // B's rows — A's rows left the view without being deleted.
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-B1'])
    expect(JSON.parse(mem.get(`gadak.snapshot@${a}`) as string).issues[0].issue_key).toBe('STD-A1')

    // And back: A's snapshot still answers from its own address.
    expect(await switchHost(a)).toBe(true)
    expect(getActiveHostId()).toBe(a)
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-A1'])
    expect(JSON.parse(mem.get(`gadak.snapshot@${b}`) as string).issues[0].issue_key).toBe('STD-B1')
  })

  it('refuses the switch when the target token is gone, changing nothing', async () => {
    const a = await seedHost(EP_A, 'desk A', 'STD-A1')
    const b = await seedHost(EP_B, 'desk B', 'STD-B1')
    await bootOn(a)
    mem.delete(`gadak.dev.token@${b}`)
    const rosterBefore = mem.get('gadak.hosts.v1')

    expect(await switchHost(b)).toBe(false)
    expect(getActiveHostId()).toBe(a)
    expect(app.meta?.label).toBe('desk A')
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-A1'])
    expect(mem.get('gadak.hosts.v1')).toBe(rosterBefore) // not even lastUsedAt moved
  })

  it('refuses the switch when the target meta is gone, changing nothing', async () => {
    const a = await seedHost(EP_A, 'desk A', 'STD-A1')
    const b = await seedHost(EP_B, 'desk B', 'STD-B1')
    await bootOn(a)
    mem.delete(`gadak.pairing.meta@${b}`)

    expect(await switchHost(b)).toBe(false)
    expect(getActiveHostId()).toBe(a)
    expect(app.meta?.label).toBe('desk A')
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-A1'])
  })

  it("unpair() forgets the active host's documents at both addresses", async () => {
    const a = await seedHost(EP_A, 'desk A', 'STD-A1')
    // Bare-key residue from before the B2 rename — unpair must take both.
    for (const base of [
      'gadak.pairing.meta',
      'gadak.pairing.meta.terminal',
      'gadak.snapshot',
      'gadak.views',
      'gadak.pages',
      'gadak.issues.scope',
    ]) {
      mem.set(base, 'legacy residue')
    }
    await bootOn(a)

    await unpair()

    for (const base of [
      'gadak.pairing.meta',
      'gadak.pairing.meta.terminal',
      'gadak.snapshot',
      'gadak.views',
      'gadak.pages',
      'gadak.issues.scope',
    ]) {
      expect(mem.get(`${base}@${a}`), `${base}@host dropped`).toBeUndefined()
      expect(mem.get(base), `${base} bare residue dropped`).toBeUndefined()
    }
    expect(listHosts()).toEqual([])
    expect(app.phase).toBe('unpaired')
  })

  it("removeRosterHost() drops an inactive host's slots, row and documents — the active host untouched", async () => {
    const a = await seedHost(EP_A, 'desk A', 'STD-A1')
    const b = await seedHost(EP_B, 'desk B', 'STD-B1')
    await bootOn(a)

    await removeRosterHost(b)

    expect(listHosts().map((h) => h.id)).toEqual([a])
    expect(getActiveHostId()).toBe(a)
    expect(mem.get(`gadak.dev.token@${b}`)).toBeUndefined()
    expect(mem.get(`gadak.pairing.meta@${b}`)).toBeUndefined()
    expect(mem.get(`gadak.snapshot@${b}`)).toBeUndefined()
    expect(mem.get(`gadak.dev.token@${a}`)).toBeDefined()
    expect(app.phase).toBe('paired')
    expect(app.issues.map((i) => i.issue_key)).toEqual(['STD-A1'])
  })
})
