/*
 * Token storage tests. The contract under pin: settings.ts is the only
 * reader/writer of the pairing token; in a Tauri webview the token lives
 * in the secure store behind token_get/token_set/token_del (never in
 * localStorage), and the legacy localStorage token migrates exactly once —
 * deleted only after the secure write succeeded. Outside Tauri (browser
 * dev, vitest) the localStorage fallback is the DEV-only path.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { invoke } = vi.hoisted(() => ({ invoke: vi.fn() }))
vi.mock('@tauri-apps/api/core', () => ({ invoke }))

import { clearPairing, inTauriApp, readPairing, readToken, savePairing } from './settings'

/** Minimal localStorage for the node test environment. */
class MemStorage {
  private m = new Map<string, string>()
  getItem(k: string): string | null {
    return this.m.has(k) ? (this.m.get(k) as string) : null
  }
  setItem(k: string, v: string): void {
    this.m.set(k, v)
  }
  removeItem(k: string): void {
    this.m.delete(k)
  }
  clear(): void {
    this.m.clear()
  }
}

const PAIRING_KEY = 'gadak-mobile.pairing'
const DEV_TOKEN_KEY = 'gadak-mobile.token'

let storage: MemStorage

beforeEach(() => {
  storage = new MemStorage()
  vi.stubGlobal('localStorage', storage)
  invoke.mockReset()
  delete (globalThis as Record<string, unknown>)._TAURI_INTERNALS_
})

const asTauri = (): void => {
  ;(globalThis as Record<string, unknown>)._TAURI_INTERNALS_ = {}
}
const unsetTauri = (): void => {
  delete (globalThis as Record<string, unknown>)._TAURI_INTERNALS_
}

describe('DEV fallback (no Tauri runtime)', () => {
  it('stores the token in the DEV localStorage key, not in the pairing entry', async () => {
    unsetTauri()
    await savePairing({ endpoint: 'https://home.example.ts.net', token: 'tok-a', label: 'phone' })
    expect(storage.getItem(DEV_TOKEN_KEY)).toBe('tok-a')
    // The pairing entry carries meta only — the token must not leak into it.
    expect(storage.getItem(PAIRING_KEY)).not.toContain('tok-a')
    expect(await readToken()).toBe('tok-a')
  })

  it('clearPairing removes meta and DEV token', async () => {
    unsetTauri()
    await savePairing({ endpoint: 'https://home.example.ts.net', token: 'tok-a', label: '' })
    await clearPairing()
    expect(storage.getItem(PAIRING_KEY)).toBeNull()
    expect(storage.getItem(DEV_TOKEN_KEY)).toBeNull()
    expect(readPairing()).toBeNull()
  })
})

describe('secure store (Tauri webview)', () => {
  it('reads the token from token_get and never touches localStorage', async () => {
    asTauri()
    invoke.mockResolvedValueOnce('tok-secure')
    expect(await readToken()).toBe('tok-secure')
    expect(invoke).toHaveBeenCalledWith('token_get')
    expect(storage.getItem(DEV_TOKEN_KEY)).toBeNull()
  })

  it('savePairing writes the token through token_set; meta stays local', async () => {
    asTauri()
    invoke.mockResolvedValue(undefined)
    await savePairing({ endpoint: 'https://home.example.ts.net', token: 'tok-b', label: 'l' })
    expect(invoke).toHaveBeenCalledWith('token_set', { token: 'tok-b' })
    expect(storage.getItem(PAIRING_KEY)).not.toContain('tok-b')
    const p = readPairing()
    expect(p?.endpoint).toBe('https://home.example.ts.net')
    expect(p?.label).toBe('l')
  })

  it('savePairing rejects when the secure write fails', async () => {
    asTauri()
    invoke.mockRejectedValueOnce(new Error('keychain locked'))
    await expect(
      savePairing({ endpoint: 'https://h', token: 'tok-c', label: '' }),
    ).rejects.toThrow('keychain locked')
  })

  it('clearPairing calls token_del and swallows its failure', async () => {
    asTauri()
    invoke.mockRejectedValueOnce(new Error('gone'))
    await expect(clearPairing()).resolves.toBeUndefined()
    expect(invoke).toHaveBeenCalledWith('token_del')
    expect(storage.getItem(PAIRING_KEY)).toBeNull()
  })
})

describe('one-time migration out of localStorage', () => {
  const seedLegacy = (): void => {
    storage.setItem(
      PAIRING_KEY,
      JSON.stringify({
        endpoint: 'https://home.example.ts.net',
        token: 'tok-legacy',
        label: 'old',
        savedAt: '2026-08-01T00:00:00Z',
      }),
    )
  }

  it('moves the legacy token to the secure store and strips it from localStorage', async () => {
    asTauri()
    seedLegacy()
    invoke.mockResolvedValueOnce(null).mockResolvedValueOnce(undefined) // token_get miss, token_set ok
    expect(await readToken()).toBe('tok-legacy')
    expect(invoke).toHaveBeenCalledWith('token_set', { token: 'tok-legacy' })
    const entry = storage.getItem(PAIRING_KEY) ?? ''
    expect(entry).not.toContain('tok-legacy')
    expect(JSON.parse(entry)).toEqual({
      endpoint: 'https://home.example.ts.net',
      label: 'old',
      savedAt: '2026-08-01T00:00:00Z',
    })
  })

  it('keeps the localStorage copy when the secure write fails (retry next read)', async () => {
    asTauri()
    seedLegacy()
    invoke.mockResolvedValueOnce(null).mockRejectedValueOnce(new Error('store down'))
    expect(await readToken()).toBe('tok-legacy')
    expect(storage.getItem(PAIRING_KEY)).toContain('tok-legacy')
  })

  it('does not migrate when the secure store already has a token', async () => {
    asTauri()
    seedLegacy()
    invoke.mockResolvedValueOnce('tok-newer')
    expect(await readToken()).toBe('tok-newer')
    expect(invoke).not.toHaveBeenCalledWith('token_set', expect.anything())
  })

  it('returns empty when nothing is stored anywhere', async () => {
    asTauri()
    invoke.mockResolvedValueOnce(null)
    expect(await readToken()).toBe('')
  })
})

describe('runtime flag', () => {
  it('tracks _TAURI_INTERNALS_ presence', () => {
    unsetTauri()
    expect(inTauriApp()).toBe(false)
    asTauri()
    expect(inTauriApp()).toBe(true)
  })
})
