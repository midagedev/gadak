import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'

// The store calls request(); the rest of api.ts (ApiError, isPairingDead)
// must stay real so a dead probe is classified by the production code —
// the same split store.test.ts makes.
vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return {
    ...actual,
    request: vi.fn(),
    configureApi: vi.fn(),
  }
})

import { request } from './api'
import { hasTauri, runtimeMode } from './runtime'
import { app, boot, unpair } from './store.svelte'

/*
 * GDK-1966 — the hosted runtime mode, the one ladder that owns it, and the
 * boot branch that consumes it.
 *
 * The ladder is decided by globals this file can stub exactly the way the
 * three real environments shape them:
 *
 *   tauri    the packaged webview carries __TAURI_INTERNALS__
 *   hosted   a plain browser page — forced here by the ?hosted URL param,
 *            the same seam the e2e gate uses (its bundle is a DEV build,
 *            so DEV alone cannot say hosted)
 *   dev      the vite webview: no internals, no param, DEV true
 *
 * The fourth arm — a production bundle with no internals and no param —
 * falls through to 'hosted' by the same code the param arm takes; it is
 * not reachable from vitest, where import.meta.env.DEV is pinned true.
 * The e2e pair of that arm is hosted.spec.ts's no-Bearer boot.
 */

function stubTauriWindow(): void {
  ;(globalThis as { window?: unknown }).window = { __TAURI_INTERNALS__: {} }
}

function stubLocation(search: string, host: string): void {
  ;(globalThis as { location?: unknown }).location = {
    search,
    host,
    protocol: 'http:',
  }
}

function clearStubbedGlobals(): void {
  delete (globalThis as { window?: unknown }).window
  delete (globalThis as { location?: unknown }).location
}

describe('runtimeMode() — one ladder: tauri > ?hosted > dev > hosted (GDK-1966)', () => {
  afterEach(clearStubbedGlobals)

  it('dev when nothing marks the page — the vitest baseline', () => {
    expect(runtimeMode()).toBe('dev')
  })

  it('tauri when the window carries __TAURI_INTERNALS__, even in a DEV build', () => {
    // The dev shell's webview on a device has both; the native side must
    // keep winning or the dev shell would silently become a hosted page.
    stubTauriWindow()
    expect(runtimeMode()).toBe('tauri')
  })

  it('hosted when the URL carries ?hosted — the param outranks DEV', () => {
    // The e2e gate builds with NODE_ENV=development (gate-serve.sh), so
    // DEV is true in the gate bundle; the param is how the gate reaches
    // the hosted branch anyway.
    stubLocation('?hosted', 'tail.example:9')
    expect(runtimeMode()).toBe('hosted')
  })

  it('tauri outranks the param — the packaged webview ignores ?hosted', () => {
    stubTauriWindow()
    stubLocation('?hosted', 'tail.example:9')
    expect(runtimeMode()).toBe('tauri')
  })

  it('hasTauri reads the window on every call, capturing nothing at module load', () => {
    expect(hasTauri()).toBe(false)
    stubTauriWindow()
    expect(hasTauri()).toBe(true)
    clearStubbedGlobals()
    expect(hasTauri()).toBe(false)
  })
})

/*
 * boot()'s hosted branch, against the fake transport shape writes.test.ts
 * uses: record every call, answer from a canned router. request() is the
 * seam boot() owns (the store.test.ts idiom); the assertions below read
 * the recorded calls the way fakeFetch's { fn, calls } does.
 */
const mem = new Map<string, string>()

function installMemStorage(): void {
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
}

/** Every key the pairing machinery owns — none may exist after a hosted boot. */
function pairingKeysInStorage(): string[] {
  return [...mem.keys()].filter(
    (k) =>
      k.startsWith('gadak.pairing') ||
      k.startsWith('gadak.hosts') ||
      k.startsWith('gadak.dev.token'),
  )
}

const envelope = (body: unknown) => ({ status: 200, etag: null, body }) as never

function resetForBoot(): void {
  app.phase = 'boot'
  app.hosted = false
  app.meta = null
  app.rejected = false
  app.terminal = null
  app.issues = []
  app.loaded = false
  app.offline = false
}

beforeEach(() => {
  mem.clear()
  installMemStorage()
  resetForBoot()
  vi.mocked(request).mockReset()
})

afterEach(async () => {
  clearStubbedGlobals()
  // enterPaired starts a 60s timer; unpair is the production owner that
  // clears it (store.test.ts's own afterEach rule).
  app.phase = 'paired'
  await unpair()
})

describe("boot() hosted — probe, adopt, never touch pairing storage (GDK-1966)", () => {
  it('pairs hosted when the probe answers: RAM meta labelled by the host, no token, no storage', async () => {
    stubLocation('?hosted', 'tail.example:9')
    const calls: { path: string; session: unknown }[] = []
    vi.mocked(request).mockImplementation(
      async (path: string, opts?: { session?: unknown }) => {
        calls.push({ path, session: opts?.session })
        if (path === 'auth/me/') {
          return envelope({ email: null, account_id: 'STD-me', name: 'Me' })
        }
        // bootstrap/views/sprints/feed — content-free but well-formed, so
        // the void sync() enterPaired fires settles instead of throwing.
        return envelope({ issues: [], views: [], sources: [] })
      },
    )

    await boot()
    // Let the fire-and-forget sync settle before reading storage.
    await new Promise((resolve) => setTimeout(resolve, 20))

    expect(app.phase).toBe('paired')
    expect(app.hosted).toBe(true)
    expect(app.meta?.endpoint).toBe('')
    expect(app.meta?.label).toBe('tail.example:9')
    // The probe dialed same-origin with no token: no Bearer, no Keychain.
    expect(calls[0]).toEqual({
      path: 'auth/me/',
      session: { endpoint: '', token: null },
    })
    // The shell is a RAM offer on the page origin, labelled by the host —
    // the same-origin WebSocket branch owns its dial.
    expect(app.terminal?.endpoint).toBe('')
    expect(app.terminal?.label).toBe('tail.example:9')
    // Nothing the pairing machinery owns was read into existence.
    expect(pairingKeysInStorage()).toEqual([])
  })

  it('dead probe → unpaired under the hosted flag, no bootstrap chase, nothing stored', async () => {
    stubLocation('?hosted', 'tail.example:9')
    const calls: string[] = []
    vi.mocked(request).mockImplementation(async (path: string) => {
      calls.push(path)
      throw new ApiError('network', 0)
    })

    await boot()

    // PairGate renders t('settings.hostedUnreachable') keyed off app.hosted;
    // the pairing form itself never shows on a hosted page.
    expect(app.phase).toBe('unpaired')
    expect(app.hosted).toBe(true)
    expect(calls).toEqual(['auth/me/'])
    expect(mem.size).toBe(0)
  })
})
