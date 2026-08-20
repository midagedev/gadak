import { afterEach, beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'
import { installHostedFetch } from './hosted-fetch'

/**
 * Runtime gate. This file runs in the `hosted-adapter` vitest project
 * (VITE_HOSTED_DEMO='1', same as tools/hosted-demo/build.mjs). The
 * compile-time-off branch is hosted-fetch.off.test.ts. config.ts has no
 * setter; this is the isHostedDemo() seam the module already calls per request.
 */
const hosted = vi.hoisted(() => ({ on: true }))

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

function attachWindow(fetchImpl: FetchFn): TestWindow {
  const w: TestWindow = {
    fetch: fetchImpl,
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

const NESTED_BOOTSTRAP = {
  issues: [
    {
      key: 'NMB-1',
      content_url: 'https://nimbus.example.com/api/v1/issues/NMB-1/attachments/att-1/content/',
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
  files.set('/bootstrap.json', () => jsonResponse(NESTED_BOOTSTRAP))
  files.set('/detail/NMB-1.json', () =>
    jsonResponse({
      comments: [
        { body: 'see /api/v1/issues/NMB-1/attachments/att-9/content/' },
      ],
    }),
  )
  files.set(
    '/attachments/10000',
    () =>
      new Response(new Uint8Array([1, 2, 3]), {
        status: 200,
        headers: { 'Content-Type': 'image/png' },
      }),
  )
  attachWindow(stubFetch)

  // Install with no global `navigator`, which is CI's Node and not
  // necessarily yours: Node grew a global `navigator` in 21, so an
  // unguarded `navigator.serviceWorker` in installHostedFetch passes
  // locally and throws in CI. Deleting it here reproduces that on any
  // Node, and `installed` latches, so this is the one install that runs
  // the service-worker cleanup path.
  const hadNavigator = 'navigator' in globalThis
  const savedNavigator = (globalThis as { navigator?: unknown }).navigator
  Reflect.deleteProperty(globalThis, 'navigator')
  try {
    installHostedFetch()
  } finally {
    if (hadNavigator) {
      Object.defineProperty(globalThis, 'navigator', {
        configurable: true,
        writable: true,
        value: savedNavigator,
      })
    }
  }
})

beforeEach(() => {
  hosted.on = true
  nativeCalls.length = 0
})

afterEach(() => {
  hosted.on = true
})

describe('installHostedFetch rewrite table', () => {
  test('install wraps window.fetch (VITE_HOSTED_DEMO=1)', () => {
    expect(win().fetch).not.toBe(stubFetch)
  })

  test('bootstrap serves static JSON and rewrites nested content_url', async () => {
    const res = await win().fetch('/api/v1/issues/bootstrap/')
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({
      issues: [
        {
          key: 'NMB-1',
          content_url: '/attachments/att-1',
          extra: {
            urls: ['/attachments/att-2', 'https://nimbus.example.com/browse/NMB-1'],
          },
        },
      ],
    })
    expect(nativeCalls.map((c) => new URL(c.href).pathname)).toContain('/bootstrap.json')
  })

  test('detail serves static JSON and rewrites nested content_url', async () => {
    const res = await win().fetch('/api/v1/issues/NMB-1/detail/')
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({
      comments: [{ body: 'see /attachments/att-9' }],
    })
    expect(nativeCalls.map((c) => new URL(c.href).pathname)).toContain('/detail/NMB-1.json')
  })

  test('attachments serve the static file (no JSON rewrite)', async () => {
    const res = await win().fetch('/api/v1/issues/NMB-1/attachments/10000/content/')
    expect(res.status).toBe(200)
    expect(res.headers.get('Content-Type')).toBe('image/png')
    expect([...new Uint8Array(await res.arrayBuffer())]).toEqual([1, 2, 3])
    expect(nativeCalls.map((c) => new URL(c.href).pathname)).toContain('/attachments/10000')
  })

  test.each([
    {
      name: 'auth/me',
      url: '/api/v1/auth/me/',
      status: 200,
      body: { email: null },
    },
    {
      name: 'views empty',
      url: '/api/v1/issues/views/',
      status: 200,
      body: { views: [] },
    },
    {
      name: 'watches empty',
      url: '/api/v1/issues/watches/',
      status: 200,
      body: { keys: [] },
    },
    {
      name: 'feed empty',
      url: '/api/v1/issues/feed/',
      status: 200,
      body: {
        items: [],
        unread_counts: { all: 0, mentions: 0, watched: 0, assigned: 0 },
      },
    },
    {
      name: 'search 404',
      url: '/api/v1/issues/search/?q=idempotency',
      status: 404,
      body: { error: 'not_found' },
    },
    {
      name: 'unknown issues path 404',
      url: '/api/v1/issues/nope/',
      status: 404,
      body: { error: 'not_found' },
    },
    {
      name: 'non-issues api 404',
      url: '/api/v1/settings/',
      status: 404,
      body: { error: 'not_found' },
    },
  ])('$name', async ({ url, status, body }) => {
    const res = await win().fetch(url)
    expect(res.status).toBe(status)
    expect(await res.json()).toEqual(body)
    expect(nativeCalls).toEqual([])
  })

  test('delta is 404 — hosted never fabricates a live delta (GDK-440)', async () => {
    const res = await win().fetch('/api/v1/issues/delta/?since=2026-08-04T09:15:00.000Z')
    expect(res.status).toBe(404)
    expect(await res.json()).toEqual({ error: 'not_found' })
    expect(nativeCalls).toEqual([])
  })

  test.each(['POST', 'PUT', 'PATCH', 'DELETE'] as const)('non-GET %s → 501 demo_read_only', async (method) => {
    const res = await win().fetch('/api/v1/issues/bootstrap/', { method })
    expect(res.status).toBe(501)
    expect(await res.json()).toEqual({ error: 'demo_read_only' })
    expect(nativeCalls).toEqual([])
  })

  test('non-api URL passthrough', async () => {
    files.set('/index.html', () => new Response('ok', { status: 200 }))
    const res = await win().fetch('/index.html')
    expect(res.status).toBe(200)
    expect(await res.text()).toBe('ok')
    expect(nativeCalls.map((c) => new URL(c.href, 'http://127.0.0.1/').pathname)).toEqual([
      '/index.html',
    ])
  })

  test('cross-origin passthrough', async () => {
    const res = await win().fetch('https://other.example/api/v1/issues/bootstrap/')
    expect(res.status).toBe(404)
    expect(await res.text()).toBe('missing')
    expect(nativeCalls.map((c) => c.href)).toEqual(['https://other.example/api/v1/issues/bootstrap/'])
  })

  test('hostedDemo off: even an api URL passthroughs', async () => {
    hosted.on = false
    const res = await win().fetch('/api/v1/issues/bootstrap/')
    expect(res.status, 'build flag on + isHostedDemo() false must not intercept').toBe(404)
    expect(await res.text()).toBe('missing')
    expect(
      nativeCalls.some((c) => c.href.includes('/api/v1/issues/bootstrap')),
      'build flag on + isHostedDemo() false must not intercept',
    ).toBe(true)
  })
})
