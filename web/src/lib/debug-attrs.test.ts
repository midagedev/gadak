/*
 * GDK-1146 / GDK-931: test-observability debug attributes on <html> have a
 * single writer (lib/debug-attrs) and cost nothing when they are off.
 *
 * The defect being pinned: detail-cache's publishDebug wrote
 * dataset.detailCache = [...cache.keys()].join(',') on EVERY cache mutation
 * in the production bundle — a hover-prefetch ran a 50-key join plus an
 * <html> attribute set, and the only consumer was one e2e poll. The half
 * people forget is the join: skipping the attribute write but still
 * computing the string fails contract 2 below, so both are asserted.
 *
 * detail-cache.svelte.ts uses runes and the unit project has no svelte
 * plugin, so it is imported dynamically with $state stubbed to identity —
 * the cache itself is a plain Map, only `epoch` is a rune and no assertion
 * here depends on its reactivity.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test, vi } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const DASH = join(HERE, '..', 'components', 'dashboard', 'DashboardView.svelte')
const CACHE_SRC = join(HERE, 'detail-cache.svelte.ts')

/** Attribute string a real DOM would surface for a dataset camelCase prop. */
function attrName(prop: string): string {
  return `data-${prop.replace(/[A-Z]/g, (m) => `-${m.toLowerCase()}`)}`
}

/**
 * Minimal document fake. dataset is a Proxy doing the camelCase→kebab-case
 * conversion a real DOMStringMap performs, so assertions can speak in
 * attribute spelling (`data-detail-cache`) exactly like the e2e poll does.
 */
function stubDocument(): Record<string, string | undefined> {
  const attrs: Record<string, string | undefined> = {}
  const dataset = new Proxy(
    {},
    {
      get: (_t, prop: string) => attrs[attrName(prop)],
      set: (_t, prop: string, value: string) => {
        attrs[attrName(prop)] = value
        return true
      },
      deleteProperty: (_t, prop: string) => {
        delete attrs[attrName(prop)]
        return true
      },
    },
  )
  vi.stubGlobal('document', {
    documentElement: { dataset },
    querySelector: () => null,
  })
  return attrs
}

function stubLocalStorage(store: Record<string, string>): void {
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => store[k] ?? null,
    setItem: (k: string, v: string) => {
      store[k] = v
    },
  })
}

function stubDetailFetch(): void {
  vi.stubGlobal(
    'fetch',
    async () =>
      new Response(
        JSON.stringify({
          issue_key: 'stub',
          development_opinion: '',
          description_adf: null,
          attachments: [],
          comments: [],
          history: [],
          linked_issues: [],
          linked_prs: [],
          qa_context: null,
        }),
        { headers: { 'Content-Type': 'application/json' } },
      ),
  )
}

/** Import detail-cache fresh, with debug attrs forced OFF (no DEV, no key). */
async function importCacheWithDebugOff() {
  vi.stubEnv('DEV', false)
  vi.stubGlobal('$state', (v: unknown) => v)
  stubLocalStorage({})
  stubDetailFetch()
  const attrs = stubDocument()
  vi.resetModules()
  const dc = await import('./detail-cache.svelte')
  return { dc, attrs }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
})

describe('GDK-1146: cache mutations publish nothing while debug attrs are off', () => {
  test('contract 1 — N mutations never create data-detail-cache on <html>', async () => {
    const { dc, attrs } = await importCacheWithDebugOff()

    await dc.getDetailCached('STD-1')
    await dc.getDetailCached('STD-2')
    await dc.getDetailCached('STD-3')
    dc.invalidate('STD-1')
    dc.invalidateAll()
    await dc.getDetailCached('STD-4')

    expect(attrs['data-detail-cache']).toBeUndefined()
  })

  test('contract 2 — the cache-key join is not even computed', async () => {
    const { dc } = await importCacheWithDebugOff()
    const joinSpy = vi.spyOn(Array.prototype, 'join')

    await dc.getDetailCached('STD-1')
    await dc.getDetailCached('STD-2')
    dc.invalidateAll()

    expect(joinSpy).not.toHaveBeenCalled()
  })
})

/** Import debug-attrs fresh so each test starts with an unmemoized flag. */
async function importModule() {
  vi.resetModules()
  return import('./debug-attrs')
}

describe('debug-attrs: the switch', () => {
  test('no DEV and no key → off; a blocked localStorage is off, not a throw', async () => {
    vi.stubEnv('DEV', false)
    stubDocument()
    vi.stubGlobal('localStorage', {
      getItem: (): string => {
        throw new Error('blocked')
      },
      setItem: () => {},
    })
    const m = await importModule()
    expect(m.debugAttrsEnabled()).toBe(false)
  })

  test('a DEV build is on regardless of storage', async () => {
    stubDocument()
    stubLocalStorage({})
    const m = await importModule()
    expect(m.debugAttrsEnabled()).toBe(true)
  })

  test("the exported key with value '1' opts a production build in", async () => {
    vi.stubEnv('DEV', false)
    stubDocument()
    const store: Record<string, string> = {}
    stubLocalStorage(store)
    const m = await importModule()
    expect(m.debugAttrsEnabled()).toBe(false)
    store[m.DEBUG_ATTRS_KEY] = '1'
    const on = await importModule()
    expect(on.debugAttrsEnabled()).toBe(true)
  })

  test('the flag is decided once — the hot path never re-reads localStorage', async () => {
    vi.stubEnv('DEV', false)
    stubDocument()
    const getItem = vi.fn(() => '1')
    vi.stubGlobal('localStorage', { getItem, setItem: () => {} })
    const m = await importModule()
    m.publishDebugAttr('detailCache', () => 'a,b')
    m.publishDebugAttr('detailCache', () => 'a')
    expect(getItem).toHaveBeenCalledTimes(1)
  })
})

describe('debug-attrs: publishDebugAttr', () => {
  test('off → the thunk is not evaluated and nothing lands on <html>', async () => {
    vi.stubEnv('DEV', false)
    const attrs = stubDocument()
    stubLocalStorage({})
    const m = await importModule()
    const thunk = vi.fn(() => 'STD-1,STD-2')

    m.publishDebugAttr('detailCache', thunk)

    expect(thunk).not.toHaveBeenCalled()
    expect(attrs['data-detail-cache']).toBeUndefined()
  })

  test('on → the thunk is evaluated once and the attribute uses dataset spelling', async () => {
    vi.stubEnv('DEV', false)
    const attrs = stubDocument()
    stubLocalStorage({ gadak_debug_attrs: '1' })
    const m = await importModule()
    const thunk = vi.fn(() => 'STD-1,STD-2')

    m.publishDebugAttr('detailCache', thunk)
    m.publishDebugAttr('lastDashOpen', () => 'panel')

    expect(thunk).toHaveBeenCalledTimes(1)
    expect(attrs['data-detail-cache']).toBe('STD-1,STD-2')
    expect(attrs['data-last-dash-open']).toBe('panel')
  })

  test('no document (SSR/node) → noop, thunk not evaluated', async () => {
    stubLocalStorage({ gadak_debug_attrs: '1' })
    const m = await importModule()
    const thunk = vi.fn(() => 'x')
    expect(() => m.publishDebugAttr('detailCache', thunk)).not.toThrow()
    expect(thunk).not.toHaveBeenCalled()
  })
})

describe('single ownership (source contract)', () => {
  test('debug-attrs.ts is the dataset writer', async () => {
    const src = readFileSync(join(HERE, 'debug-attrs.ts'), 'utf8')
    expect(src).toMatch(/documentElement\.dataset\[/)
  })

  test('detail-cache has no bare documentElement.dataset write and routes through the owner', () => {
    const src = readFileSync(CACHE_SRC, 'utf8')
    expect(src, 'no direct dataset write may return (GDK-1146)').not.toMatch(
      /documentElement\.dataset/,
    )
    expect(src, 'the cache key join must ride behind the owner thunk').toMatch(
      /publishDebugAttr\('detailCache', \(\) => \[\.\.\.cache\.keys\(\)\]\.join\(','\)\)/,
    )
  })

  test('DashboardView has no bare documentElement.dataset write and routes through the owner', () => {
    const src = readFileSync(DASH, 'utf8')
    expect(src, 'no direct dataset write may return (GDK-931)').not.toMatch(
      /documentElement\.dataset/,
    )
    expect(src, 'the fired rule must still be named through the owner').toMatch(
      /publishDebugAttr\('lastDashOpen', \(\) => rule\)/,
    )
  })
})
