import { afterEach, describe, expect, test } from 'vitest'
import { THEME_STORAGE_KEY } from './storage'
import {
  THEME_MODES,
  THEMES,
  applyThemePreference,
  dataThemeAttr,
  isThemePreference,
  parseThemePreference,
  readThemePreference,
  setThemePreference,
} from './theme'

describe('theme registry', () => {
  test('THEMES is the palette catalog; system is a mode, not a palette', () => {
    expect(THEMES.map((t) => t.name)).toEqual(['light', 'dark'])
    expect(THEMES.map((t) => t.labelKey)).toEqual(['theme.light', 'theme.dark'])
    expect(THEME_MODES.map((m) => m.name)).toEqual(['system', 'light', 'dark'])
    expect(THEME_MODES.map((m) => m.labelKey)).toEqual([
      'theme.system',
      'theme.light',
      'theme.dark',
    ])
  })

  test('adding a theme is an entry in THEMES (picker reads THEME_MODES)', () => {
    const names = new Set(THEME_MODES.map((m) => m.name))
    for (const theme of THEMES) {
      expect(names.has(theme.name)).toBe(true)
    }
    expect(names.has('system')).toBe(true)
  })
})

describe('parseThemePreference', () => {
  test('default and garbage are system', () => {
    expect(parseThemePreference(null)).toBe('system')
    expect(parseThemePreference(undefined)).toBe('system')
    expect(parseThemePreference('')).toBe('system')
    expect(parseThemePreference('auto')).toBe('system')
    expect(parseThemePreference('DARK')).toBe('system')
  })

  test('accepts the three stored values', () => {
    expect(parseThemePreference('system')).toBe('system')
    expect(parseThemePreference('light')).toBe('light')
    expect(parseThemePreference('dark')).toBe('dark')
    expect(isThemePreference('dark')).toBe(true)
    expect(isThemePreference('sepia')).toBe(false)
  })
})

describe('dataThemeAttr', () => {
  test('system clears the attribute so the media query can apply', () => {
    expect(dataThemeAttr('system')).toBeNull()
    expect(dataThemeAttr('light')).toBe('light')
    expect(dataThemeAttr('dark')).toBe('dark')
  })
})

describe('applyThemePreference', () => {
  const store = new Map<string, string>()
  const originalDocument = globalThis.document
  const originalLocalStorage = globalThis.localStorage

  afterEach(() => {
    globalThis.document = originalDocument
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: originalLocalStorage,
    })
    store.clear()
  })

  function installMocks(initialAttr: string | null = 'light'): { attr: { value: string | null } } {
    const attr = { value: initialAttr }
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
    return { attr }
  }

  test('dark sets data-theme; system removes the boot-time light attribute', () => {
    const { attr } = installMocks('light')
    applyThemePreference('dark')
    expect(attr.value).toBe('dark')
    applyThemePreference('system')
    expect(attr.value).toBeNull()
    applyThemePreference('light')
    expect(attr.value).toBe('light')
  })

  test('setThemePreference persists under the storage key and applies', () => {
    const { attr } = installMocks(null)
    setThemePreference('dark')
    expect(store.get(THEME_STORAGE_KEY)).toBe('dark')
    expect(attr.value).toBe('dark')
    expect(readThemePreference()).toBe('dark')
  })

  test('readThemePreference defaults to system when unset', () => {
    installMocks(null)
    expect(readThemePreference()).toBe('system')
  })
})
