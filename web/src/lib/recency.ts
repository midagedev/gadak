/*
 * Issue Navigator — recent-use history (for personalized ranking)
 *
 * localStorage helper that quietly ranks choices in every picker UI
 * (assignee / transition / new issue).
 *  - Per kind: recent values newest-first, de-duped, cap 10.
 *  - Order itself is the suggestion (no badge). Record only successful actions
 *    (caller's responsibility).
 *
 * kind examples: 'assignee', 'transition:<project>', 'create-project',
 *          'create-type:<project>', 'label'
 */

import { RECENT_KIND_PREFIX } from './storage'

const PREFIX = RECENT_KIND_PREFIX
const MAX = 10

/** Recent values for a kind (newest first). Empty array if none. */
export function recentOf(kind: string): string[] {
  try {
    const raw = localStorage.getItem(PREFIX + kind)
    if (!raw) return []
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return []
  }
}

/** Record a value at the front (de-dup, cap MAX). Empty values ignored. */
export function recordRecent(kind: string, value: string): void {
  if (!value) return
  try {
    const next = [value, ...recentOf(kind).filter((v) => v !== value)].slice(0, MAX)
    localStorage.setItem(PREFIX + kind, JSON.stringify(next))
  } catch {
    /* localStorage unavailable — ignore */
  }
}

/**
 * Recent rank index (0 = most recent, missing → Infinity). Drop into a sort comparator.
 */
export function recentRank(kind: string, value: string): number {
  const i = recentOf(kind).indexOf(value)
  return i === -1 ? Infinity : i
}
