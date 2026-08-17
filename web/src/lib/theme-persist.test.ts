import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

const hosted = vi.hoisted(() => ({ on: false }))
const toast = vi.hoisted(() => vi.fn())

vi.mock('./config', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./config')>()
  return {
    ...actual,
    isHostedDemo: () => hosted.on,
  }
})

vi.mock('../stores/write.svelte', () => ({
  write: { toast },
}))

import { THEME_STORAGE_KEY } from './storage'
import { hydrateThemeFromServer, persistThemePreference, readThemePreference } from './theme'
import { en } from './i18n/en'

type FetchCall = { method: string; url: string; body: unknown }

const store = new Map<string, string>()
const calls: FetchCall[] = []
const originalFetch = globalThis.fetch
const originalDocument = globalThis.document
const originalLocalStorage = globalThis.localStorage
const originalWarn = console.warn

let getStatus = 200
let putStatus = 200
let getBody: Record<string, unknown> = {
  projects: ['NMB'],
  staleThresholdHours: 72,
  appearance: { theme: 'system' },
}

function jsonResponse(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const stubFetch: typeof fetch = async (input, init) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  const method = (init?.method ?? 'GET').toUpperCase()
  const body = init?.body ? JSON.parse(String(init.body)) : undefined
  calls.push({ method, url, body })
  if (method === 'GET') return jsonResponse(getBody, getStatus)
  if (method === 'PUT') return jsonResponse(body ?? {}, putStatus)
  return new Response('no', { status: 405 })
}

function installDom(): void {
  const attr = { value: null as string | null }
  globalThis.document = {
    documentElement: {
      setAttribute(_name: string, value: string) {
        attr.value = value
      },
      removeAttribute() {
        attr.value = null
      },
      getAttribute() {
        return attr.value
      },
    },
  } as unknown as Document
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem(key: string) {
        return store.has(key) ? store.get(key)! : null
      },
      setItem(key: string, value: string) {
        store.set(key, value)
      },
      removeItem(key: string) {
        store.delete(key)
      },
    },
  })
}

beforeEach(() => {
  hosted.on = false
  toast.mockReset()
  calls.length = 0
  store.clear()
  getStatus = 200
  putStatus = 200
  getBody = {
    projects: ['NMB'],
    staleThresholdHours: 72,
    appearance: { theme: 'system' },
  }
  globalThis.fetch = stubFetch
  installDom()
})

afterEach(() => {
  globalThis.fetch = originalFetch
  globalThis.document = originalDocument
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: originalLocalStorage,
  })
  console.warn = originalWarn
})

describe('persistThemePreference', () => {
  test('GET then PUT, appearance only, other settings preserved', async () => {
    await persistThemePreference('ink')
    expect(store.get(THEME_STORAGE_KEY)).toBe('ink')
    expect(calls.map((c) => c.method)).toEqual(['GET', 'PUT'])
    expect(calls[0].url).toContain('settings/')
    expect(calls[1].url).toContain('settings/')
    expect(calls[1].body).toMatchObject({
      projects: ['NMB'],
      staleThresholdHours: 72,
      appearance: { theme: 'ink' },
    })
    expect(toast).not.toHaveBeenCalled()
  })

  test('PUT failure keeps the local mirror and toasts saved-locally', async () => {
    putStatus = 500
    await persistThemePreference('dark')
    expect(store.get(THEME_STORAGE_KEY)).toBe('dark')
    expect(readThemePreference()).toBe('dark')
    expect(calls.map((c) => c.method)).toEqual(['GET', 'PUT'])
    expect(toast).toHaveBeenCalledTimes(1)
    expect(toast.mock.calls[0][0]).toBe(en['theme.savedLocally'])
  })

  test('GET failure toasts and does not PUT', async () => {
    getStatus = 502
    await persistThemePreference('ember')
    expect(store.get(THEME_STORAGE_KEY)).toBe('ember')
    expect(calls.map((c) => c.method)).toEqual(['GET'])
    expect(toast).toHaveBeenCalledTimes(1)
    expect(toast.mock.calls[0][0]).toBe(en['theme.savedLocally'])
  })

  test('hosted demo skips write-through', async () => {
    hosted.on = true
    await persistThemePreference('light')
    expect(store.get(THEME_STORAGE_KEY)).toBe('light')
    expect(calls).toEqual([])
    expect(toast).not.toHaveBeenCalled()
  })
})

describe('hydrateThemeFromServer', () => {
  test('applies a server theme that differs from the local mirror', async () => {
    getBody = { appearance: { theme: 'ink' } }
    await hydrateThemeFromServer()
    expect(store.get(THEME_STORAGE_KEY)).toBe('ink')
    expect(readThemePreference()).toBe('ink')
    expect(calls.map((c) => c.method)).toEqual(['GET'])
  })

  test('server system wins over a local palette', async () => {
    store.set(THEME_STORAGE_KEY, 'dark')
    getBody = { appearance: { theme: 'system' } }
    await hydrateThemeFromServer()
    expect(readThemePreference()).toBe('system')
  })

  test('missing appearance (old server) keeps local', async () => {
    store.set(THEME_STORAGE_KEY, 'ember')
    getBody = { projects: ['NMB'] }
    await hydrateThemeFromServer()
    expect(readThemePreference()).toBe('ember')
  })

  test('GET failure keeps local and warns once', async () => {
    store.set(THEME_STORAGE_KEY, 'ink')
    getStatus = 404
    const warn = vi.fn()
    console.warn = warn
    await hydrateThemeFromServer()
    expect(readThemePreference()).toBe('ink')
    expect(warn).toHaveBeenCalledTimes(1)
    expect(String(warn.mock.calls[0][0])).toMatch(/keeping local preference/)
  })

  test('hosted demo does not fetch', async () => {
    hosted.on = true
    await hydrateThemeFromServer()
    expect(calls).toEqual([])
  })
})
