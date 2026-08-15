import { afterEach, beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

/**
 * Compile-time gate OFF. This file runs in the default `unit` vitest project,
 * where VITE_HOSTED_DEMO is pinned empty (same gate as `gadak serve` /
 * desktop: !== '1'). The hosted-on rewrite table lives in
 * hosted-fetch.test.ts (hosted-adapter project). config.ts has no setter;
 * this is the isHostedDemo() seam the module already calls per request —
 * default off so a leaky compile flag is the only thing that can still
 * install the adapter.
 */
const hosted = vi.hoisted(() => ({ on: false }))

vi.mock('./config', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./config')>()
  return {
    ...actual,
    isHostedDemo: () => hosted.on,
    basePath: () => '/',
  }
})

type FetchFn = typeof fetch
type TestWindow = {
  fetch: FetchFn
  location: { href: string; origin: string }
}

type NativeCall = { href: string; method: string }

const nativeCalls: NativeCall[] = []
const files = new Map<string, () => Response>()

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const stubFetch: FetchFn = async (input, init) => {
  const href = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  const method = (init?.method ?? 'GET').toUpperCase()
  nativeCalls.push({ href, method })
  const path = new URL(href, 'http://127.0.0.1/').pathname
  const hit = files.get(path)
  if (!hit) return new Response('missing', { status: 404 })
  return hit()
}

const originalFetch = globalThis.fetch

/** Fresh module so `installed` from a prior test cannot swallow a later install. */
async function loadInstall(): Promise<typeof import('./hosted-fetch').installHostedFetch> {
  vi.resetModules()
  const { installHostedFetch } = await import('./hosted-fetch')
  return installHostedFetch
}

/**
 * Browser-like: assigning window.fetch must replace globalThis.fetch.
 * installHostedFetch writes `window.fetch = hostedFetch`; in a real window
 * those are the same binding. A detached `{ fetch }` object would make the
 * identity assertion a false green.
 */
function attachWindow(fetchImpl: FetchFn): TestWindow {
  globalThis.fetch = fetchImpl
  const w: TestWindow = {
    get fetch() {
      return globalThis.fetch
    },
    set fetch(fn: FetchFn) {
      globalThis.fetch = fn
    },
    location: { href: 'http://127.0.0.1/', origin: 'http://127.0.0.1' },
  }
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    writable: true,
    value: w,
  })
  return w
}

function win(): TestWindow {
  return (globalThis as unknown as { window: TestWindow }).window
}

const LIVE_CONTENT_URL =
  'https://nimbus.example.com/api/v1/issues/NMB-1/attachments/att-1/content/'

const NESTED_BOOTSTRAP = {
  issues: [
    {
      key: 'NMB-1',
      content_url: LIVE_CONTENT_URL,
      extra: {
        urls: [
          '/api/v1/issues/NMB-2/attachments/att-2/content',
          'https://nimbus.example.com/browse/NMB-1',
        ],
      },
    },
  ],
}

beforeAll(() => {
  files.set('/api/v1/issues/bootstrap/', () => jsonResponse(NESTED_BOOTSTRAP))
  files.set('/bootstrap.json', () => jsonResponse(NESTED_BOOTSTRAP))
})

beforeEach(() => {
  hosted.on = false
  nativeCalls.length = 0
  attachWindow(stubFetch)
})

afterEach(() => {
  hosted.on = false
  nativeCalls.length = 0
  globalThis.fetch = originalFetch
  Reflect.deleteProperty(globalThis, 'window')
})

describe('installHostedFetch in a non-hosted build', () => {
  test('unit project does not compile with VITE_HOSTED_DEMO=1', () => {
    expect(
      import.meta.env.VITE_HOSTED_DEMO === '1',
      'unit project must compile as a non-hosted build',
    ).toBe(false)
  })

  test('runtime hostedDemo default is off', async () => {
    const { isHostedDemo } = await vi.importActual<typeof import('./config')>('./config')
    expect(isHostedDemo(), 'config DEFAULTS.hostedDemo must stay false').toBe(false)
  })

  test('install leaves globalThis.fetch as the same reference', async () => {
    const installHostedFetch = await loadInstall()
    const before = globalThis.fetch
    expect(before).toBe(stubFetch)
    installHostedFetch()
    expect(
      globalThis.fetch,
      'hosted adapter installed in a non-hosted build',
    ).toBe(before)
    expect(
      win().fetch,
      'hosted adapter installed in a non-hosted build',
    ).toBe(before)
  })

  test('attachment-shaped URL is not rewritten even if hostedDemo is on', async () => {
    hosted.on = true
    const installHostedFetch = await loadInstall()
    installHostedFetch()
    const res = await win().fetch('/api/v1/issues/bootstrap/')
    expect(res.status).toBe(200)
    const body = (await res.json()) as typeof NESTED_BOOTSTRAP
    expect(
      body.issues[0].content_url,
      'hosted adapter installed in a non-hosted build',
    ).toBe(LIVE_CONTENT_URL)
    expect(body.issues[0].extra.urls[0]).toBe('/api/v1/issues/NMB-2/attachments/att-2/content')
    expect(nativeCalls.some((c) => c.href.includes('/api/v1/issues/bootstrap'))).toBe(true)
  })
})
