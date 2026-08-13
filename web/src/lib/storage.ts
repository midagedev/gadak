/*
 * localStorage key helpers + one-shot migration.
 * Old prefixes `issue-nav:` and `scry:` → `gadak:`. Call once at app boot.
 */

import { workspaceName } from './config'

// Workspace mounts (/w/<name>/) share the origin with the primary, so their
// keys carry the workspace name — favorites or a last view from one mirror
// must never leak into another.
const WS = workspaceName() ? `ws:${workspaceName()}:` : ''

/** Keys in active use. */
export const STORAGE_KEYS = {
  /** Fallback only — holds the set when the server does not accept favorites (hosted demo). */
  favorites: `gadak:${WS}favorites`,
  /**
   * Favorites display order. The set itself is owned by the server (`favorites` table),
   * but that table has no order column and returns insert order only. Drag-made order is
   * this browser's display preference, so it lives here — intentional split of ownership
   * between set (server) and order (local).
   */
  favoritesOrder: `gadak:${WS}favorites-order`,
  recent: `gadak:${WS}recent`,
  personalViews: `gadak:${WS}personal-views`,
  lastView: `gadak:${WS}last-view`,
  /** Last document-view tab (viewed / updated / author). */
  docsTab: `gadak:${WS}docs-tab`,
} as const

/** Prefix for recency.ts (`gadak:recent:` + kind). */
export const RECENT_KIND_PREFIX = `gadak:${WS}recent:`

/** Migration only — runtime reads/writes all use gadak:. */
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
  [`scry:${WS}favorites`, STORAGE_KEYS.favorites],
  [`scry:${WS}favorites-order`, STORAGE_KEYS.favoritesOrder],
  [`scry:${WS}recent`, STORAGE_KEYS.recent],
  [`scry:${WS}personal-views`, STORAGE_KEYS.personalViews],
  [`scry:${WS}last-view`, STORAGE_KEYS.lastView],
  [`scry:${WS}docs-tab`, STORAGE_KEYS.docsTab],
]

/**
 * One-shot migrate of old `issue-nav:*` and `scry:*` keys to `gadak:*`.
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

    migratePrefix(LEGACY.recentKindPrefix, RECENT_KIND_PREFIX)
    migratePrefix('scry:recent:', RECENT_KIND_PREFIX)
    migratePrefix('scry:comment-draft:', 'gadak:comment-draft:')
  } catch {
    /* private mode / unavailable — ignore */
  }
}

function migratePrefix(oldPrefix: string, newPrefix: string): void {
  const oldKeys: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i)
    if (k && k.startsWith(oldPrefix)) oldKeys.push(k)
  }
  for (const oldKey of oldKeys) {
    const newKey = newPrefix + oldKey.slice(oldPrefix.length)
    const oldVal = localStorage.getItem(oldKey)
    if (oldVal !== null && localStorage.getItem(newKey) === null) {
      localStorage.setItem(newKey, oldVal)
    }
    localStorage.removeItem(oldKey)
  }
}
