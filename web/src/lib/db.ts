/*
 * Issue Navigator — IndexedDB cache (idb)
 *
 * Persistent layer for the Linear-style "browser as DB" strategy. On revisit,
 * hydrate the memory pool from this cache before the network to kill cold start.
 *
 * Stores:
 *  - issues: key = issue_key (full IssueLite)
 *  - meta:   key = 'sync' (single record: server_time / sync_version / members)
 */

import { openDB, type DBSchema, type IDBPDatabase } from 'idb'
import { workspaceName } from './config'
import type { CacheMeta, IssueLite, WriteMetaCache } from './types'

// Workspace mounts get their own database: two mirrors on one origin would
// otherwise overwrite each other's cached issue pool.
const DB_NAME = workspaceName() ? `issue-navigator:ws:${workspaceName()}` : 'issue-navigator'
// v2 invalidates IssueLite rows written before reporter_id joined the row
// contract. SQLite already has the IDs, so one fresh bootstrap is sufficient;
// write metadata is independent and remains warm.
const DB_VERSION = 2

interface IssueDB extends DBSchema {
  issues: {
    key: string // issue_key
    value: IssueLite
  }
  meta: {
    key: string // 'sync' | 'write'
    value: CacheMeta | WriteMetaCache
  }
}

/**
 * Treat open as failed if it does not finish within this window.
 * IndexedDB open can queue silently behind another tab's pending delete/upgrade
 * (no error, no event, waits forever) — without a timeout the whole app stays on skeleton.
 * On failure the caller (store) catches and runs network-only (memory pool).
 */
const OPEN_TIMEOUT_MS = 2_000

let dbPromise: Promise<IDBPDatabase<IssueDB>> | null = null

function db(): Promise<IDBPDatabase<IssueDB>> {
  if (!dbPromise) {
    const opening = openDB<IssueDB>(DB_NAME, DB_VERSION, {
      upgrade(database, oldVersion, _newVersion, transaction) {
        if (!database.objectStoreNames.contains('issues')) {
          database.createObjectStore('issues', { keyPath: 'issue_key' })
        }
        if (!database.objectStoreNames.contains('meta')) {
          database.createObjectStore('meta', { keyPath: 'key' })
        }
        // Existing v1 rows may not carry reporter_id. Clear only that legacy
        // issue snapshot and its sync cursor: keeping the cursor could make an
        // empty cache send If-None-Match and accept a 304. Fresh v2 databases
        // and every later v2 reopen skip this branch entirely.
        if (oldVersion > 0 && oldVersion < 2) {
          void transaction.objectStore('issues').clear()
          void transaction.objectStore('meta').delete('sync')
        }
      },
      // Yield the connection when another tab wants a higher-version upgrade (or delete).
      // Without this, bumping DB_VERSION on deploy freezes every new tab for users who
      // still have an old-version tab open — infinite skeleton waiting on the upgrade.
      blocking() {
        void opening.then((conn) => conn.close()).catch(() => {})
        dbPromise = null // This tab runs memory-only after; reload picks up the new schema
      },
      terminated() {
        dbPromise = null // Browser forcibly closed the connection — reopen on next call
      },
    })
    dbPromise = Promise.race([
      opening,
      new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error('indexeddb-open-timeout')), OPEN_TIMEOUT_MS),
      ),
    ])
    // Even after a timeout failure, if open eventually succeeds, swap it in for the
    // next call; on a real failure, reset so a later call can retry.
    dbPromise.catch(() => {
      opening.then(
        (conn) => {
          dbPromise = Promise.resolve(conn)
        },
        () => {
          dbPromise = null
        },
      )
    })
  }
  return dbPromise
}

/** Load all cached issues (for hydration). */
export async function getAllIssues(): Promise<IssueLite[]> {
  return (await db()).getAll('issues')
}

/** Bulk issue upsert (full bootstrap or delta upserted). One transaction. */
export async function putIssues(issues: IssueLite[]): Promise<void> {
  if (issues.length === 0) return
  const conn = await db()
  const tx = conn.transaction('issues', 'readwrite')
  const store = tx.objectStore('issues')
  for (const issue of issues) store.put(issue)
  await tx.done
}

/** Bulk issue delete (delta deleted_keys). */
export async function deleteIssues(keys: string[]): Promise<void> {
  if (keys.length === 0) return
  const conn = await db()
  const tx = conn.transaction('issues', 'readwrite')
  const store = tx.objectStore('issues')
  for (const key of keys) store.delete(key)
  await tx.done
}

/**
 * Chunk size for replaceAllIssues write transactions.
 * A single clear()+10k put() txn completed ~690ms after interactive on the
 * cold path; closing the tab mid-flight rolled back the whole txn (clear
 * included) so the next visit was cold again. 1k-row commits land durable
 * progress without one multi-second exclusive txn.
 */
const REPLACE_CHUNK = 1_000

/**
 * Replace the store with a full bootstrap (clears leftover tombstones).
 *
 * Writes in REPLACE_CHUNK-sized transactions. clear() runs only in the first
 * chunk so later chunks only put.
 *
 * Ordering contract (do not invert at the call site): issues.svelte.ts
 * #bootstrap awaits this fully, then #persistMeta() (lastSync / sync_version).
 * lastSync is the signal that the cache is a complete base for delta. If we
 * die mid-chunk, meta is not advanced, so a partial issues store is never
 * trusted as a finished bootstrap — no partial-cache + delta accident.
 */
export async function replaceAllIssues(issues: IssueLite[]): Promise<void> {
  const conn = await db()
  if (issues.length === 0) {
    const tx = conn.transaction('issues', 'readwrite')
    await tx.objectStore('issues').clear()
    await tx.done
    return
  }
  for (let i = 0; i < issues.length; i += REPLACE_CHUNK) {
    const chunk = issues.slice(i, i + REPLACE_CHUNK)
    const tx = conn.transaction('issues', 'readwrite')
    const store = tx.objectStore('issues')
    if (i === 0) await store.clear()
    for (const issue of chunk) store.put(issue)
    await tx.done
  }
}

export async function getMeta(): Promise<CacheMeta | undefined> {
  return (await db()).get('meta', 'sync') as Promise<CacheMeta | undefined>
}

export async function putMeta(meta: CacheMeta): Promise<void> {
  await (await db()).put('meta', meta)
}

/** Cache write meta (transition map / create-meta) — load at boot for 0ms dropdowns. */
export async function getWriteMeta(): Promise<WriteMetaCache | undefined> {
  return (await db()).get('meta', 'write') as Promise<WriteMetaCache | undefined>
}

export async function putWriteMeta(meta: WriteMetaCache): Promise<void> {
  await (await db()).put('meta', meta)
}

/** Clear everything (manual reset / debug). */
export async function clearAll(): Promise<void> {
  const conn = await db()
  await conn.clear('issues')
  await conn.clear('meta')
}
