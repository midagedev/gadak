import 'fake-indexeddb/auto'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { openDB } from 'idb'
import {
  ISSUE_CACHE_DB_VERSION,
  getAllIssues,
  issueCacheDbName,
  resetIssueCacheConnection,
  upgradeIssueCache,
} from './db'

const NAME = 'issue-navigator:gdk-649-upgrade'
let seq = 0
function uniqueName(): string {
  seq += 1
  return `${NAME}-${seq}`
}

async function seedV1(name: string): Promise<void> {
  const db1 = await openDB(name, 1, {
    upgrade(database) {
      database.createObjectStore('issues', { keyPath: 'issue_key' })
      database.createObjectStore('meta', { keyPath: 'key' })
    },
  })
  await db1.put('issues', { issue_key: 'LEGACY-1' })
  await db1.put('meta', {
    key: 'sync',
    server_time: '2000-01-01T00:00:00.000Z',
    sync_version: 999,
    members: [],
  })
  await db1.put('meta', {
    key: 'write',
    transitions: {},
    projects: [],
    updated_at: null,
    cached_at: 'legacy-write-meta',
  })
  db1.close()
}

async function deleteDb(name: string): Promise<void> {
  resetIssueCacheConnection()
  await Promise.race([
    new Promise<void>((resolve, reject) => {
      const req = indexedDB.deleteDatabase(name)
      req.onsuccess = () => resolve()
      req.onerror = () => reject(req.error ?? new Error('indexeddb-delete-failed'))
      req.onblocked = () => resolve()
    }),
    new Promise<void>((resolve) => setTimeout(resolve, 500)),
  ])
}

beforeEach(() => {
  vi.stubGlobal('window', { location: { pathname: '/' } })
})

afterEach(() => {
  resetIssueCacheConnection()
  vi.unstubAllGlobals()
})

describe('issue cache upgrade (v1 → v2)', () => {
  test('legacy v1 rows and the sync cursor are dropped; write meta stays', async () => {
    const name = uniqueName()
    await seedV1(name)
    const db2 = await openDB(name, ISSUE_CACHE_DB_VERSION, { upgrade: upgradeIssueCache })
    try {
      expect(db2.version).toBe(2)
      const issues = await db2.getAll('issues')
      expect(issues.some((row) => row.issue_key === 'LEGACY-1')).toBe(false)
      expect(issues).toEqual([])
      expect(await db2.get('meta', 'sync')).toBeUndefined()
      expect(await db2.get('meta', 'write')).toMatchObject({ cached_at: 'legacy-write-meta' })
    } finally {
      db2.close()
      await deleteDb(name)
    }
  })

  test('a later v2 reopen does not clear a fresh snapshot', async () => {
    const name = uniqueName()
    const first = await openDB(name, ISSUE_CACHE_DB_VERSION, { upgrade: upgradeIssueCache })
    try {
      await first.put(
        'issues',
        { issue_key: 'NMB-1', reporter_id: 'demo-alex' } as import('./types').IssueLite,
      )
      await first.put('meta', {
        key: 'sync',
        sync_version: 7,
        server_time: '2026-08-01T00:00:00.000Z',
        members: [],
      } as import('./types').CacheMeta)
    } finally {
      first.close()
    }

    const second = await openDB(name, ISSUE_CACHE_DB_VERSION, { upgrade: upgradeIssueCache })
    try {
      expect(second.version).toBe(2)
      expect(await second.get('issues', 'NMB-1')).toMatchObject({ reporter_id: 'demo-alex' })
      expect(await second.get('meta', 'sync')).toMatchObject({ sync_version: 7 })
    } finally {
      second.close()
      await deleteDb(name)
    }
  })

  test('fresh install creates the v2 stores', async () => {
    const name = uniqueName()
    const db2 = await openDB(name, ISSUE_CACHE_DB_VERSION, { upgrade: upgradeIssueCache })
    try {
      expect(db2.version).toBe(2)
      expect([...db2.objectStoreNames].sort()).toEqual(['issues', 'meta'])
      expect(await db2.getAll('issues')).toEqual([])
    } finally {
      db2.close()
      await deleteDb(name)
    }
  })
})

describe('getAllIssues row contract (v2, wipe does not run)', () => {
  test('a legacy v2 row missing labels hydrates with empty arrays', async () => {
    const name = issueCacheDbName()
    await deleteDb(name)
    const conn = await openDB(name, ISSUE_CACHE_DB_VERSION, { upgrade: upgradeIssueCache })
    try {
      expect(conn.version).toBe(2)
      await conn.put(
        'issues',
        { issue_key: 'LEGACY-2', summary: 'cached v2 row' } as import('./types').IssueLite,
      )
    } finally {
      conn.close()
    }
    resetIssueCacheConnection()

    const rows = await getAllIssues()
    expect(rows).toHaveLength(1)
    expect(rows[0].issue_key).toBe('LEGACY-2')
    expect(rows[0].summary).toBe('cached v2 row')
    expect(rows[0].labels).toEqual([])
    expect(rows[0].fix_versions).toEqual([])
    expect(rows[0].components).toEqual([])

    resetIssueCacheConnection()
    await deleteDb(name)
  })
})

describe('issueCacheDbName', () => {
  test('empty scope keeps the historic name', () => {
    expect(issueCacheDbName('')).toBe('issue-navigator')
  })

  test('a site partition is in the name', () => {
    expect(issueCacheDbName('site:nimbus.example.com')).toBe(
      'issue-navigator:site:nimbus.example.com',
    )
  })
})
