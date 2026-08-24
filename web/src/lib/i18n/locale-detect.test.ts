import { expect, test, vi } from 'vitest'
import { LOCALES } from './types'
import { initLocale, locale } from './index'

/*
 * GDK-825: detectLocale used to restate the locale union by hand
 * ('en'||'ko'||'ja'), so a fourth LOCALES entry was honored by the type
 * system and ignored by the runtime. The guard now reads LOCALES itself;
 * this pins "every shipped locale is bootable from storage" so the two can
 * never split again. Storage stub follows sync-now-error.test.ts.
 */

test('every LOCALES member is honored from the stored key', () => {
  const mem = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => {
      mem.set(k, v)
    },
    removeItem: (k: string) => {
      mem.delete(k)
    },
    clear: () => mem.clear(),
    key: (i: number) => [...mem.keys()][i] ?? null,
    get length() {
      return mem.size
    },
  })
  vi.stubGlobal('navigator', { language: 'en-US' })

  for (const l of LOCALES) {
    mem.set('gadak_locale', l)
    initLocale()
    expect(locale(), `stored ${l} must boot ${l}`).toBe(l)
  }

  // Outside the registry: ignored, falls through to navigator/default.
  mem.set('gadak_locale', 'xx')
  initLocale()
  expect(locale()).toBe('en')
})
