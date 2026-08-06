/*
 * Issue Navigator — display helpers ([explore])
 *  Pure functions: relative time / initials / priority·status meta. Reused across components.
 */

import type { IssueLite } from './types'
import { effectiveCategory, type StatusCategory } from './view-config'
import {
  absTime as i18nAbsTime,
  categoryLabel,
  relativeTime as i18nRelativeTime,
  t,
} from './i18n'

/* ── Relative time (single i18n impl, compact) ── */

/** "just now · 3m · 2h · 1d …" compact relative time. */
export function relativeTime(iso: string | null): string {
  return i18nRelativeTime(iso, 'compact')
}

/** Absolute time (for tooltips). */
export function absTime(iso: string | null): string {
  return i18nAbsTime(iso)
}

/* ── Search-term highlighting ── */

/** Case-insensitive substring split, one span per query word. Chosung /
 *  key-shortcut hits have no slices → no highlight.
 *
 *  Words are highlighted separately because that is how the query was answered:
 *  the search matches each word on its own, so "webhook replay" returns a title
 *  reading "replaying failed webhook deliveries". Treating the query as one
 *  literal string highlighted nothing there, and a row that matched while
 *  showing no reason reads as a bug in the search (vision verdict 2026-08-06). */
export function highlightSegments(text: string, q: string): { text: string; hit: boolean }[] {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return [{ text, hit: false }]
  const lower = text.toLowerCase()
  const out: { text: string; hit: boolean }[] = []
  let i = 0
  for (;;) {
    // Earliest word wins; at a tie the longer one, so "run runbook" marks the
    // whole word rather than leaving a stub behind.
    let at = -1
    let len = 0
    for (const w of words) {
      const p = lower.indexOf(w, i)
      if (p < 0) continue
      if (at < 0 || p < at || (p === at && w.length > len)) {
        at = p
        len = w.length
      }
    }
    if (at < 0) break
    if (at > i) out.push({ text: text.slice(i, at), hit: false })
    out.push({ text: text.slice(at, at + len), hit: true })
    i = at + len
  }
  if (out.length === 0) return [{ text, hit: false }]
  if (i < text.length) out.push({ text: text.slice(i), hit: false })
  return out
}

/* ── Initials (avatar fallback) ── */

export function initials(name: string | null | undefined, email?: string | null): string {
  const raw = (name || email || '').trim()
  if (!raw) return '?'
  // Emails: use local part only. Tokenizing the domain turns marco@x.io into
  // 'MX' (or 'MI' if the TLD sneaks in) — avatars should point at a person.
  const at = raw.indexOf('@')
  const src = at > 0 ? raw.slice(0, at) : raw
  // Hangul: strip spaces, last two chars (given name, drop family name). Latin: first letter of first two tokens.
  if (/[가-힣]/.test(src)) {
    const nm = src.replace(/\s+/g, '')
    return nm.length >= 2 ? nm.slice(-2) : nm
  }
  const parts = src.split(/[\s._-]+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return src.slice(0, 2).toUpperCase()
}

/** Stable color index from email/name string (avatar background). */
export function colorIndex(seed: string | null | undefined, buckets = 8): number {
  const s = seed ?? ''
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h) % buckets
}

/* ── Priority meta ── */

export interface PriorityMeta {
  label: string
  /** Semantic color (hex for Tailwind arbitrary values). */
  color: string
  /** Bar fill level 1–5 (icon render). */
  level: number
}

/** priority string → display meta. Matches both KO and EN names. */
export function priorityMeta(priority: string | null): PriorityMeta {
  const p = (priority ?? '').toLowerCase()
  if (/highest|긴급|가장 높음|blocker/.test(p)) return { label: priority ?? '', color: '#ef4444', level: 5 }
  if (/high|높음|major/.test(p)) return { label: priority ?? '', color: '#f97316', level: 4 }
  if (/medium|보통|normal/.test(p)) return { label: priority ?? '', color: '#eab308', level: 3 }
  if (/lowest|가장 낮음|매우 ?낮음|trivial/.test(p)) return { label: priority ?? '', color: '#64748b', level: 1 }
  if (/low|낮음|minor/.test(p)) return { label: priority ?? '', color: '#3b82f6', level: 2 }
  return { label: priority ?? t('common.none'), color: '#64748b', level: 0 }
}

/* ── Status category meta ── */

export function categoryMetaOf(cat: StatusCategory): { label: string; color: string } {
  const color =
    cat === 'new'
      ? 'var(--color-status-new)'
      : cat === 'inprogress'
        ? 'var(--color-status-inprogress)'
        : 'var(--color-status-done)'
  return { label: categoryLabel(cat), color }
}

/** @deprecated prefer categoryMetaOf — kept as getter-style for call sites. */
export const CATEGORY_META: Record<StatusCategory, { get label(): string; color: string }> = {
  new: {
    get label() {
      return categoryLabel('new')
    },
    color: 'var(--color-status-new)',
  },
  inprogress: {
    get label() {
      return categoryLabel('inprogress')
    },
    color: 'var(--color-status-inprogress)',
  },
  done: {
    get label() {
      return categoryLabel('done')
    },
    color: 'var(--color-status-done)',
  },
}

export function categoryOf(issue: IssueLite): StatusCategory {
  return effectiveCategory(issue)
}
