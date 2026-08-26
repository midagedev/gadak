/*
 * GDK-960: same `at` is applied once. App.svelte cannot be mounted in the
 * vitest unit project (no svelte plugin — skeleton-grace.test.ts /
 * gdk-944.test.ts). The apply decision lives here; the suite also scans
 * App.svelte for the call sites.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  UI_FOCUS_KEY,
  readLastFocusKey,
  rememberFocusKey,
  shouldApplyUIFocus,
  uiFocusKey,
} from './ui-focus'

const HERE = dirname(fileURLToPath(import.meta.url))
const APP = join(HERE, '../App.svelte')

function appSrc(): string {
  return readFileSync(APP, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GDK-960 shouldApplyUIFocus', () => {
  const H = 'pj=NMA'

  test('the same payload is applied only once', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, H, null)).toBe(true)
    expect(shouldApplyUIFocus(at, H, uiFocusKey(at, H))).toBe(false)
  })

  test('a newer at is applied', () => {
    const last = uiFocusKey('2026-08-26T00:00:00Z', H)
    expect(shouldApplyUIFocus('2026-08-26T00:01:00Z', H, last)).toBe(true)
  })

  /*
   * GDK-981: `at` is a wall-clock stamp and can repeat — an older server
   * writes it at second resolution, so `views open A && views open B` gives
   * both the same one. Keyed on `at` alone the tab that applied A would drop
   * B in silence, which is the symptom GDK-960 set out to fix.
   */
  test('a second hash under the same at is still applied', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, 'pj=NMB', uiFocusKey(at, 'pj=NMA'))).toBe(true)
  })

  test('the same hash under the same at is not applied twice', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, 'pj=NMA', uiFocusKey(at, 'pj=NMA'))).toBe(false)
  })

  test('missing at cannot be deduped and is applied', () => {
    const last = uiFocusKey('2026-08-26T00:00:00Z', H)
    expect(shouldApplyUIFocus('', H, last)).toBe(true)
    expect(shouldApplyUIFocus(null, H, last)).toBe(true)
    expect(shouldApplyUIFocus(undefined, H, null)).toBe(true)
  })
})

describe('GDK-960 last-applied key storage', () => {
  const AT = '2026-08-26T00:00:00Z'
  const H = 'pj=NMA'
  const KEY = uiFocusKey(AT, H)

  test('memory is preferred over sessionStorage', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => 'from-store',
      setItem: () => {
        throw new Error('should not write')
      },
    })
    expect(readLastFocusKey('from-memory')).toBe('from-memory')
  })

  test('sessionStorage fills in after a refresh (memory empty)', () => {
    const store: Record<string, string> = {}
    vi.stubGlobal('sessionStorage', {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => {
        store[k] = v
      },
    })
    rememberFocusKey(KEY)
    expect(store[UI_FOCUS_KEY]).toBe(KEY)
    expect(readLastFocusKey(null)).toBe(KEY)
    expect(shouldApplyUIFocus(AT, H, readLastFocusKey(null))).toBe(false)
  })

  test('blocked sessionStorage is treated as not yet applied', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    })
    expect(readLastFocusKey(null)).toBeNull()
    expect(() => rememberFocusKey(KEY)).not.toThrow()
    expect(shouldApplyUIFocus(AT, H, readLastFocusKey(null))).toBe(true)
  })
})

describe('GDK-960 App.svelte calls the apply-once helpers', () => {
  test('applyFocus uses pollUIFocus and the at gate', () => {
    const src = appSrc()
    expect(src).toContain('pollUIFocus')
    expect(src).not.toContain('takeUIFocus')
    expect(src).toContain('shouldApplyUIFocus')
    expect(src).toContain('readLastFocusKey')
    expect(src).toContain('rememberFocusKey')
  })
})
