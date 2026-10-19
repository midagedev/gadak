import { afterEach, expect, test, vi } from 'vitest'
import { ko } from './catalog'
import { initLocale, locale, localeTag, setLocale, t } from './index'

/*
 * GDK-2034 (2026-10-19): setLocale used to hard-reload the page, which threw
 * away every unpersisted edit. It now switches in place: t() answers in the
 * new language immediately and location.reload is never called.
 */

afterEach(() => {
  vi.unstubAllGlobals()
})

test('setLocale switches t() in place without reloading', () => {
  const mem = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => void mem.set(k, v),
  })
  vi.stubGlobal('navigator', { language: 'en-US' })
  const reload = vi.fn()
  vi.stubGlobal('location', { reload })

  expect(initLocale()).toBe('en')
  expect(t('write.newIssue')).toBe('New issue')

  setLocale('ko')

  expect(locale()).toBe('ko')
  expect(localeTag()).toBe('ko-KR')
  expect(t('write.newIssue')).toBe(ko['write.newIssue'])
  expect(mem.get('gadak_locale')).toBe('ko')
  expect(reload).not.toHaveBeenCalled()
})
