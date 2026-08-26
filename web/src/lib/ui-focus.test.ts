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
  UI_FOCUS_AT_KEY,
  readLastFocusAt,
  rememberFocusAt,
  shouldApplyUIFocus,
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
  test('the same at is applied only once', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, null)).toBe(true)
    expect(shouldApplyUIFocus(at, at)).toBe(false)
  })

  test('a newer at is applied', () => {
    expect(shouldApplyUIFocus('2026-08-26T00:01:00Z', '2026-08-26T00:00:00Z')).toBe(true)
  })

  test('missing at cannot be deduped and is applied', () => {
    expect(shouldApplyUIFocus('', '2026-08-26T00:00:00Z')).toBe(true)
    expect(shouldApplyUIFocus(null, '2026-08-26T00:00:00Z')).toBe(true)
    expect(shouldApplyUIFocus(undefined, null)).toBe(true)
  })
})

describe('GDK-960 last-applied at storage', () => {
  test('memory is preferred over sessionStorage', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => 'from-store',
      setItem: () => {
        throw new Error('should not write')
      },
    })
    expect(readLastFocusAt('from-memory')).toBe('from-memory')
  })

  test('sessionStorage fills in after a refresh (memory empty)', () => {
    const store: Record<string, string> = {}
    vi.stubGlobal('sessionStorage', {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => {
        store[k] = v
      },
    })
    rememberFocusAt('2026-08-26T00:00:00Z')
    expect(store[UI_FOCUS_AT_KEY]).toBe('2026-08-26T00:00:00Z')
    expect(readLastFocusAt(null)).toBe('2026-08-26T00:00:00Z')
    expect(shouldApplyUIFocus('2026-08-26T00:00:00Z', readLastFocusAt(null))).toBe(false)
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
    expect(readLastFocusAt(null)).toBeNull()
    expect(() => rememberFocusAt('2026-08-26T00:00:00Z')).not.toThrow()
    expect(shouldApplyUIFocus('2026-08-26T00:00:00Z', readLastFocusAt(null))).toBe(true)
  })
})

describe('GDK-960 App.svelte calls the apply-once helpers', () => {
  test('applyFocus uses pollUIFocus and the at gate', () => {
    const src = appSrc()
    expect(src).toContain('pollUIFocus')
    expect(src).not.toContain('takeUIFocus')
    expect(src).toContain('shouldApplyUIFocus')
    expect(src).toContain('readLastFocusAt')
    expect(src).toContain('rememberFocusAt')
  })
})
