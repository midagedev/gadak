/*
 * localStorage key helpers + one-shot migration.
 * Old prefixes `issue-nav:` and `scry:` → `gadak:`. Call once at app boot.
 */

import { composeCacheScope, config, workspaceName } from './config'

/**
 * Partition prefix. Read at access time so site from config.json (loaded
 * before mount) is included — a module-level capture would freeze DEFAULTS.
 */
function keyPrefix(): string {
  const scope = composeCacheScope(workspaceName(), config().jiraBaseUrl)
  return scope ? `${scope}:` : ''
}

/** Keys in active use. Getters so a site loaded after import still partitions. */
export const STORAGE_KEYS = {
  /** Fallback only — holds the set when the server does not accept favorites (hosted demo). */
  get favorites() {
    return `gadak:${keyPrefix()}favorites`
  },
  /**
   * Favorites display order. The set itself is owned by the server (`favorites` table),
   * but that table has no order column and returns insert order only. Drag-made order is
   * this browser's display preference, so it lives here — intentional split of ownership
   * between set (server) and order (local).
   */
  get favoritesOrder() {
    return `gadak:${keyPrefix()}favorites-order`
  },
  get recent() {
    return `gadak:${keyPrefix()}recent`
  },
  get personalViews() {
    return `gadak:${keyPrefix()}personal-views`
  },
  get lastView() {
    return `gadak:${keyPrefix()}last-view`
  },
  /** Last document-view tab (viewed / updated / author). */
  get docsTab() {
    return `gadak:${keyPrefix()}docs-tab`
  },
}

/**
 * Appearance boot-mirror prefix. Workspace-scoped (`/w/<name>` →
 * `gadak:theme:<name>`), not site-partitioned — the boot script in
 * index.html has no config.json, so the path is the only partition it
 * can see. Default mount (`/`) keeps the unscoped key.
 */
export const THEME_STORAGE_KEY = 'gadak:theme'

/** Same path rule as `workspaceName()` — kept here so the boot script can match it. */
export const THEME_WORKSPACE_PATH_RE = /^\/w\/([A-Za-z0-9_-]+)(\/|$)/

/** Derive the boot-mirror key from a pathname. `/` and `/w/oss` stay distinct. */
export function themeStorageKeyFromPath(pathname: string): string {
  const m = pathname.match(THEME_WORKSPACE_PATH_RE)
  return m ? `${THEME_STORAGE_KEY}:${m[1]}` : THEME_STORAGE_KEY
}

/** Active boot-mirror key for this page. Safe when `window` is missing (tests). */
export function themeStorageKey(): string {
  const path =
    typeof window !== 'undefined' && window.location ? window.location.pathname : '/'
  return themeStorageKeyFromPath(path)
}

/** Prefix for recency.ts (`gadak:recent:` + kind). */
export function recentKindPrefix(): string {
  return `gadak:${keyPrefix()}recent:`
}

/**
 * Comment-draft localStorage key. Workspace + site so a draft cannot post to
 * the wrong mirror after a re-init or /w/ switch.
 */
export function composeCommentDraftKey(workspace: string, siteUrl: string, issueKey: string): string {
  const scope = composeCacheScope(workspace, siteUrl)
  return scope ? `gadak:comment-draft:${scope}:${issueKey}` : `gadak:comment-draft:${issueKey}`
}

export function commentDraftKey(issueKey: string): string {
  return composeCommentDraftKey(workspaceName(), config().jiraBaseUrl, issueKey)
}

/** Migration only — runtime reads/writes all use gadak:. */
const LEGACY = {
  favorites: 'issue-nav:favorites',
  recent: 'issue-nav:recent',
  personalViews: 'issue-nav:personal-views',
  lastView: 'issue-nav:last-view',
  recentKindPrefix: 'issue-nav:recent:',
} as const

function exactMigrations(): [string, string][] {
  const ws = workspaceName() ? `ws:${workspaceName()}:` : ''
  return [
    [LEGACY.favorites, STORAGE_KEYS.favorites],
    [LEGACY.recent, STORAGE_KEYS.recent],
    [LEGACY.personalViews, STORAGE_KEYS.personalViews],
    [LEGACY.lastView, STORAGE_KEYS.lastView],
    [`scry:${ws}favorites`, STORAGE_KEYS.favorites],
    [`scry:${ws}favorites-order`, STORAGE_KEYS.favoritesOrder],
    [`scry:${ws}recent`, STORAGE_KEYS.recent],
    [`scry:${ws}personal-views`, STORAGE_KEYS.personalViews],
    [`scry:${ws}last-view`, STORAGE_KEYS.lastView],
    [`scry:${ws}docs-tab`, STORAGE_KEYS.docsTab],
  ]
}

/**
 * One-shot migrate of old `issue-nav:*` and `scry:*` keys to `gadak:*`.
 * If the new key already exists, leave its value and only drop the old key. Not reversible.
 * Quietly ignore localStorage exceptions (private mode, etc.) — same as other call sites.
 */
export function migrateStorageKeys(): void {
  // Workspace mounts must not adopt the primary's legacy keys as their own —
  // migration belongs to the primary page alone.
  if (workspaceName() !== '') return
  try {
    for (const [oldKey, newKey] of exactMigrations()) {
      const oldVal = localStorage.getItem(oldKey)
      if (oldVal === null) continue
      if (localStorage.getItem(newKey) === null) {
        localStorage.setItem(newKey, oldVal)
      }
      localStorage.removeItem(oldKey)
    }

    migratePrefix(LEGACY.recentKindPrefix, recentKindPrefix())
    migratePrefix('scry:recent:', recentKindPrefix())
    migratePrefix('scry:comment-draft:', 'gadak:comment-draft:')
    migrateUnscopedIntoScope()
  } catch {
    /* private mode / unavailable — ignore */
  }
}

function migratePrefix(oldPrefix: string, newPrefix: string): void {
  if (!oldPrefix || oldPrefix === newPrefix) return
  const oldKeys: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i)
    if (k && k.startsWith(oldPrefix) && !k.startsWith(newPrefix)) oldKeys.push(k)
  }
  for (const oldKey of oldKeys) {
    const rest = oldKey.slice(oldPrefix.length)
    // Already partitioned (site:/ws:) — do not double-prefix.
    if (rest.startsWith('site:') || rest.startsWith('ws:')) continue
    const newKey = newPrefix + rest
    const oldVal = localStorage.getItem(oldKey)
    if (oldVal !== null && localStorage.getItem(newKey) === null) {
      localStorage.setItem(newKey, oldVal)
    }
    localStorage.removeItem(oldKey)
  }
}

/** Move workspace-only gadak:* keys into the site-partitioned names. */
function migrateUnscopedIntoScope(): void {
  const prefix = keyPrefix()
  if (!prefix) return
  const names = ['favorites', 'favorites-order', 'recent', 'personal-views', 'last-view', 'docs-tab']
  for (const name of names) {
    const oldKey = `gadak:${name}`
    const newKey = `gadak:${prefix}${name}`
    const oldVal = localStorage.getItem(oldKey)
    if (oldVal === null) continue
    if (localStorage.getItem(newKey) === null) localStorage.setItem(newKey, oldVal)
    localStorage.removeItem(oldKey)
  }
  migratePrefix('gadak:comment-draft:', `gadak:comment-draft:${prefix}`)
  migratePrefix('gadak:recent:', recentKindPrefix())
}
