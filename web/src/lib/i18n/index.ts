/*
 * Thin i18n runtime — no library dependency.
 * Locale: localStorage gadak_locale > navigator.language (ko* / ja*) > en.
 * Changing locale reloads the page (see setLocale).
 */

import { formatAbs } from '../calendar'
import { en, ja, ko, type MessageKey } from './catalog'
import { LOCALES, type Locale } from './types'

export type { Locale, MessageKey }

const STORAGE_KEY = 'gadak_locale'
const catalogs: Record<Locale, Record<MessageKey, string>> = { en, ko, ja }

let current: Locale = 'en'

function detectLocale(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY) ?? localStorage.getItem('scry_locale')
    // GDK-825: the allowlist is LOCALES — the registry that owns the Locale
    // type. This used to restate 'en'||'ko'||'ja' by hand, so a fourth entry
    // in LOCALES was silently ignored here.
    const hit = LOCALES.find((l) => l === stored)
    if (hit) return hit
  } catch {
    /* private mode / SSR-ish */
  }
  if (typeof navigator !== 'undefined') {
    const lang = (navigator.language || '').toLowerCase()
    if (lang === 'ko' || lang.startsWith('ko-')) return 'ko'
    if (lang === 'ja' || lang.startsWith('ja-')) return 'ja'
  }
  return 'en'
}

/** Call once at boot (before first render). Idempotent. */
export function initLocale(): Locale {
  current = detectLocale()
  syncDocumentLang()
  return current
}

/** Keep <html lang> in sync so screen readers pick the right pronunciation. */
function syncDocumentLang(): void {
  if (typeof document !== 'undefined') document.documentElement.lang = localeTag()
}

export function locale(): Locale {
  return current
}

/** Persist and hard-reload so all modules re-read catalogs. */
export function setLocale(next: Locale): void {
  try {
    localStorage.setItem(STORAGE_KEY, next)
  } catch {
    /* ignore */
  }
  if (next === current) return
  current = next
  syncDocumentLang() // reload re-runs initLocale, but don't leave a stale lang mid-teardown
  location.reload()
}

/** BCP 47 tag for Intl APIs. */
function localeTag(): string {
  if (current === 'ko') return 'ko-KR'
  if (current === 'ja') return 'ja-JP'
  return 'en-US'
}

/** Collator for name sorting. */
export function collator(): Intl.Collator {
  return new Intl.Collator(localeTag())
}

export type MessageParams = Record<string, string | number>

/** Translate a catalog key; `{name}` placeholders replaced from params. */
export function t(key: MessageKey, params?: MessageParams): string {
  const table = catalogs[current] ?? en
  let s: string = table[key] ?? en[key] ?? String(key)
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      s = s.replaceAll(`{${k}}`, String(v))
    }
  }
  return s
}

/* ── Field / column / category / deploy label helpers ── */

const FIELD_KEYS = [
  'status_category',
  'status',
  'assignee_email',
  'reporter_email',
  'team_group',
  'labels',
  'priority',
  'severity',
  'issue_type',
  'components',
  'fix_versions',
  'environment',
  'browser',
  'dev_project_number',
  'found_version',
  'occurrence',
  'solution',
  'critical_phenomenon',
  'development_area',
  'development_test_assignee_email',
  'development_test_result',
  'qa_run',
  'qa_suite',
  'qa_impact',
  'deploy_state',
  'cs',
  'jira_project',
  'source_project',
] as const

export type FieldLabelKey = (typeof FIELD_KEYS)[number]

export function fieldLabel(field: string): string {
  const key = `field.${field}` as MessageKey
  if (key in en) return t(key)
  return field
}

export const COLUMN_LABEL_KEYS = [
  'assignee',
  'updated',
  'labels',
  'reopen',
  'stale',
  'qa_impact',
  'deploy',
  'severity',
  'issue_type',
  'status',
  'reporter',
  'comment_count',
  'fix_versions',
  'components',
  'created',
  'due',
  'environment',
  'team_group',
  'dev_test_result',
] as const

export type ColumnLabelKey = (typeof COLUMN_LABEL_KEYS)[number]

export function columnLabel(key: ColumnLabelKey): string {
  const mk = `column.${key}` as MessageKey
  if (mk in en) return t(mk)
  return key
}

// One key per category, and no second parameter that could grow one back.
// Group headers used to ask for a `spaced` variant; the number beside the
// label lives in its own span (GroupHeader.svelte), so the spacing was never
// a layout requirement — it was a Korean-only duplicate string (GDK-298).
export function categoryLabel(cat: 'new' | 'inprogress' | 'done'): string {
  return t(`category.${cat}` as MessageKey)
}

export type DeployStateKey = 'none' | 'merged' | 'dev' | 'qa_preview' | 'qa' | 'prod'

export function deployStateLabel(state: DeployStateKey | string): string {
  const mk = `deploy.${state}` as MessageKey
  if (mk in en) return t(mk)
  return state
}

/* ── Relative time (single implementation) ── */

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR

export type RelativeKind = 'just_now' | 'duration'

export interface RelativeParts {
  kind: RelativeKind
  /** Present when kind === 'duration'. */
  n?: number
  unit?: 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year'
}

/**
 * Structured relative time — FavoritesNav and others must not string-match output.
 * Returns null when iso is missing/invalid.
 */
function relativeTimeParts(iso: string | null | undefined): RelativeParts | null {
  if (!iso) return null
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return null
  const diff = Date.now() - ts
  if (diff < MIN) return { kind: 'just_now' }
  if (diff < HOUR) return { kind: 'duration', n: Math.floor(diff / MIN), unit: 'minute' }
  if (diff < DAY) return { kind: 'duration', n: Math.floor(diff / HOUR), unit: 'hour' }
  const days = Math.floor(diff / DAY)
  if (days < 7) return { kind: 'duration', n: days, unit: 'day' }
  if (days < 30) return { kind: 'duration', n: Math.floor(days / 7), unit: 'week' }
  if (days < 365) return { kind: 'duration', n: Math.floor(days / 30), unit: 'month' }
  return { kind: 'duration', n: Math.floor(days / 365), unit: 'year' }
}

export type RelativeStyle = 'compact' | 'long'

/** Format structured parts. compact: "3m"; long: "3m ago" (+ "yesterday" for 1 day). */
function formatRelativeParts(
  parts: RelativeParts,
  style: RelativeStyle = 'compact',
): string {
  if (parts.kind === 'just_now') return t('time.justNow')
  const n = parts.n ?? 0
  const unit = parts.unit ?? 'minute'
  if (style === 'long' && unit === 'day' && n === 1) return t('time.yesterday')
  const keyMap: Record<string, { compact: MessageKey; long: MessageKey }> = {
    minute: { compact: 'time.minute', long: 'time.minuteAgo' },
    hour: { compact: 'time.hour', long: 'time.hourAgo' },
    day: { compact: 'time.day', long: 'time.dayAgo' },
    week: { compact: 'time.week', long: 'time.weekAgo' },
    month: { compact: 'time.month', long: 'time.monthAgo' },
    year: { compact: 'time.year', long: 'time.yearAgo' },
  }
  const keys = keyMap[unit]
  return t(style === 'long' ? keys.long : keys.compact, { n })
}

/**
 * Unified relative time.
 * - compact (list rows): "just now" / "3m" / "2h"
 * - long (detail / sync): "just now" / "3m ago" / "yesterday"
 */
export function relativeTime(
  iso: string | null | undefined,
  style: RelativeStyle = 'compact',
): string {
  const parts = relativeTimeParts(iso)
  if (!parts) {
    if (iso == null || iso === '') return ''
    // detail format historically returned raw iso on parse failure
    return style === 'long' ? String(iso) : ''
  }
  return formatRelativeParts(parts, style)
}

/** FavoritesNav "seen X ago" without string-matching relative output. */
export function relativeSeenLabel(iso: string | null | undefined): string {
  const parts = relativeTimeParts(iso)
  if (!parts) return ''
  if (parts.kind === 'just_now') return t('time.seenJustNow')
  const relative = formatRelativeParts(parts, 'compact')
  return t('time.seenAgo', { relative })
}

/** Absolute datetime for tooltips. Date-only strings stay on their calendar day. */
export function absTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const kind = /^\d{4}-\d{2}-\d{2}$/.test(iso.trim()) ? 'date' : 'instant'
  return formatAbs(iso, kind, undefined, localeTag())
}

/** Locale-aware number formatting. */
export function formatNumber(n: number): string {
  return n.toLocaleString(localeTag())
}

/** Locale-aware time-of-day. */
export function formatTimeOfDay(isoOrDate: string | Date): string {
  const d = typeof isoOrDate === 'string' ? new Date(isoOrDate) : isoOrDate
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString(localeTag(), { hour: '2-digit', minute: '2-digit' })
}

// Initialize eagerly so modules that call t() at import time get the right locale.
initLocale()
