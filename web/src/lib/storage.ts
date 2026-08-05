/*
 * localStorage key helpers + one-shot migration.
 * Old prefix `issue-nav:` → `scry:`. Call once at app boot.
 */

import { workspaceName } from './config'

// Workspace mounts (/w/<name>/) share the origin with the primary, so their
// keys carry the workspace name — favorites or a last view from one mirror
// must never leak into another.
const WS = workspaceName() ? `ws:${workspaceName()}:` : ''

/** Keys in active use. */
export const STORAGE_KEYS = {
  /** Fallback only — holds the set when the server does not accept favorites (hosted demo). */
  favorites: `scry:${WS}favorites`,
  /**
   * Favorites display order. The set itself is owned by the server (`favorites` table),
   * but that table has no order column and returns insert order only. Drag-made order is
   * this browser's display preference, so it lives here — intentional split of ownership
   * between set (server) and order (local).
   */
  favoritesOrder: `scry:${WS}favorites-order`,
  recent: `scry:${WS}recent`,
  personalViews: `scry:${WS}personal-views`,
  lastView: `scry:${WS}last-view`,
} as const

/** Prefix for recency.ts (`scry:recent:` + kind). */
export const RECENT_KIND_PREFIX = `scry:${WS}recent:`

/** Migration only — runtime reads/writes all use scry:. */
const LEGACY = {
  favorites: 'issue-nav:favorites',
  recent: 'issue-nav:recent',
  personalViews: 'issue-nav:personal-views',
  lastView: 'issue-nav:last-view',
  recentKindPrefix: 'issue-nav:recent:',
} as const

const EXACT_MIGRATIONS: [string, string][] = [
  [LEGACY.favorites, STORAGE_KEYS.favorites],
  [LEGACY.recent, STORAGE_KEYS.recent],
  [LEGACY.personalViews, STORAGE_KEYS.personalViews],
  [LEGACY.lastView, STORAGE_KEYS.lastView],
]

/**
 * One-shot migrate of old `issue-nav:*` keys to `scry:*`.
 * If the new key already exists, leave its value and only drop the old key. Not reversible.
 * Quietly ignore localStorage exceptions (private mode, etc.) — same as other call sites.
 */
export function migrateStorageKeys(): void {
  // Workspace mounts must not adopt the primary's legacy keys as their own —
  // migration belongs to the primary page alone.
  if (WS !== '') return
  try {
    for (const [oldKey, newKey] of EXACT_MIGRATIONS) {
      const oldVal = localStorage.getItem(oldKey)
      if (oldVal === null) continue
      if (localStorage.getItem(newKey) === null) {
        localStorage.setItem(newKey, oldVal)
      }
      localStorage.removeItem(oldKey)
    }

    // Recency prefix scan: issue-nav:recent:<kind> → scry:recent:<kind>
    // (exact `issue-nav:recent` already handled above — match colon prefix only)
    const legacyRecentKeys: string[] = []
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i)
      if (k && k.startsWith(LEGACY.recentKindPrefix)) legacyRecentKeys.push(k)
    }
    for (const oldKey of legacyRecentKeys) {
      const kind = oldKey.slice(LEGACY.recentKindPrefix.length)
      const newKey = RECENT_KIND_PREFIX + kind
      const oldVal = localStorage.getItem(oldKey)
      if (oldVal !== null && localStorage.getItem(newKey) === null) {
        localStorage.setItem(newKey, oldVal)
      }
      localStorage.removeItem(oldKey)
    }
  } catch {
    /* private mode / unavailable — ignore */
  }
}
