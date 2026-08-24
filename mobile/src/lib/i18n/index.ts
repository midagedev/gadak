/*
 * Tiny t() — mobile-owned catalogs in the web shape (flat dotted keys, one
 * file per locale). The web catalog machinery (messages/*.ts codegen) is
 * deliberately not shared: mobile strings are few and enumerated by type
 * (MessageKey), so a missing key is a type error on both locales.
 */
import { en, type Messages } from './en'
import { ko } from './ko'

export type MessageKey = keyof Messages

const CATALOGS: Record<Locale, Messages> = { en, ko }

export type Locale = 'en' | 'ko'

export function detectLocale(): Locale {
  const lang = typeof navigator !== 'undefined' ? navigator.language : 'en'
  return lang?.toLowerCase().startsWith('ko') ? 'ko' : 'en'
}

export function t(key: MessageKey, params?: Record<string, string | number>, locale: Locale = detectLocale()): string {
  let s: string = CATALOGS[locale][key] ?? en[key]
  if (params) {
    for (const [k, v] of Object.entries(params)) s = s.replaceAll(`{${k}}`, String(v))
  }
  return s
}

export function categoryLabel(cat: string): MessageKey {
  if (cat === 'new') return 'status.new'
  if (cat === 'inprogress') return 'status.inprogress'
  if (cat === 'done') return 'status.done'
  return 'status.unknown'
}
