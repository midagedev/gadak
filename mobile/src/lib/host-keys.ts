// Host-scoped cache keys (GDK-1097 B2). The ten localStorage documents a
// paired session owns — the pairing metas, the issue snapshot, views,
// pages, sprints, the scope, the composer drafts (GDK-1863), the discovered
// field specs (GDK-1870), the declared name (GDK-1973) — are namespaced
// per roster host so two hosts never read each other's cache:
// 'gadak.snapshot' becomes 'gadak.snapshot@<hostId>'
// once a host is active. A null host id keeps the bare key, which is the
// address every pre-B2 phone's data lives at (and what an unmigrated phone
// keeps reading). The namespaced shape matches the dev-slot composition in
// secure.ts devSlot — one spelling, two stores.
//
// The base-key names live here, not in store.svelte.ts: the module that
// namespaces a key owns its name, so the store's constants and the
// migration list below cannot drift apart.

export const META_KEY = 'gadak.pairing.meta'
export const TERM_META_KEY = 'gadak.pairing.meta.terminal'
export const CACHE_KEY = 'gadak.snapshot'
export const VIEWS_KEY = 'gadak.views'
export const PAGES_KEY = 'gadak.pages'
/** The mirror's sprint rows (GDK-1867) — the sprint line's own document. */
export const SPRINTS_KEY = 'gadak.sprints'
export const SCOPE_KEY = 'gadak.issues.scope'
export const DRAFTS_KEY = 'gadak.drafts.v1'
/** The site's discovered custom fields (GDK-1870) — the Fields section's
 *  labels and their order. A host document: two sites configure different
 *  fields, and the aliases of one mean nothing on the other. */
export const FIELD_SPECS_KEY = 'gadak.field-specs'
/**
 * The declared name a person typed in Settings (GDK-1973). A host document
 * for the same reason the scope is: the name that attributes this phone's
 * writes belongs to the serve it was declared against, and a second host's
 * name would attribute to the wrong tracker. On a hosted page (no roster)
 * it stays at the bare key — the browser's own localStorage at the serve
 * origin, the one place a hosted page may keep anything.
 */
export const ACTOR_NAME_KEY = 'gadak.actorName'

/**
 * Every key that is namespaced per host. The unpaired marker is a
 * device-wide verdict, not a host document, and deliberately stays out.
 */
export const HOST_SCOPED_KEYS = [
  META_KEY,
  TERM_META_KEY,
  CACHE_KEY,
  VIEWS_KEY,
  PAGES_KEY,
  SPRINTS_KEY,
  SCOPE_KEY,
  DRAFTS_KEY,
  FIELD_SPECS_KEY,
  ACTOR_NAME_KEY,
] as const

/** Namespaced form of a base key: the bare key for a null host (legacy). */
export function hostKey(base: string, hostId: string | null): string {
  return hostId === null ? base : `${base}@${hostId}`
}

/* Guarded storage, the same quiet-cache stance as store.svelte.ts and
   hosts.ts: a rename is a convenience, never a requirement. Byte-level,
   not JSON-level, so the verification below compares exactly what was
   written. */

function readRaw(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeRaw(key: string, value: string): boolean {
  try {
    localStorage.setItem(key, value)
    return true
  } catch {
    return false
  }
}

function dropRaw(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch {
    /* ignore */
  }
}

/**
 * One-time legacy→namespaced rename (GDK-1097 B2), run in boot right after
 * the B1 roster migration. Per key, and only while a host is active: copy
 * the legacy bytes to the namespaced address, read them back, and only a
 * verified copy may drop the legacy key. A failed verification keeps the
 * legacy key and clears the bad shadow, so the next boot can retry instead
 * of being blocked by a corrupt namespaced value. The same no-loss shape
 * as B1's migrateLegacyPairing: every risky step happens before anything
 * observable changes, and no failure path deletes user data.
 *
 * Returns the base keys renamed, for tests.
 */
export function migrateHostKeys(hostId: string | null): string[] {
  if (hostId === null) return []
  const renamed: string[] = []
  for (const base of HOST_SCOPED_KEYS) {
    const scoped = hostKey(base, hostId)
    if (readRaw(scoped) !== null) continue // already namespaced — idempotent
    const legacy = readRaw(base)
    if (legacy === null) continue
    if (!writeRaw(scoped, legacy)) continue
    if (readRaw(scoped) !== legacy) {
      dropRaw(scoped) // a corrupt shadow must not block the next boot's retry
      continue
    }
    dropRaw(base)
    renamed.push(base)
  }
  return renamed
}
