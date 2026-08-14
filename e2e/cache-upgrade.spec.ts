import { expect, test, type Page, type Request } from '@playwright/test'
import { gotoApp } from './helpers'

// e2e/serve.sh writes site https://nimbus.example.com — see composeCacheScope.
const DB_NAME = 'issue-navigator:site:nimbus.example.com'

interface CacheState {
  version: number
  count: number
  hasLegacyRow: boolean
  hasReporterID: boolean
  syncVersion: number | null
  writeCachedAt: string | null
}

async function deleteDatabase(page: Page): Promise<void> {
  await page.goto('/healthz')
  await page.evaluate(
    (name) =>
      new Promise<void>((resolve, reject) => {
        const request = indexedDB.deleteDatabase(name)
        request.onsuccess = () => resolve()
        request.onerror = () => reject(request.error)
        request.onblocked = () => reject(new Error('indexeddb-delete-blocked'))
      }),
    DB_NAME,
  )
}

async function seedLegacyCache(page: Page): Promise<void> {
  await deleteDatabase(page)
  await page.evaluate(
    (name) =>
      new Promise<void>((resolve, reject) => {
        const request = indexedDB.open(name, 1)
        request.onupgradeneeded = () => {
          request.result.createObjectStore('issues', { keyPath: 'issue_key' })
          request.result.createObjectStore('meta', { keyPath: 'key' })
        }
        request.onerror = () => reject(request.error)
        request.onsuccess = () => {
          const database = request.result
          const transaction = database.transaction(['issues', 'meta'], 'readwrite')
          transaction.objectStore('issues').put({ issue_key: 'LEGACY-1' })
          transaction.objectStore('meta').put({
            key: 'sync',
            server_time: '2000-01-01T00:00:00.000Z',
            sync_version: 999,
            members: [],
          })
          transaction.objectStore('meta').put({
            key: 'write',
            transitions: {},
            projects: [],
            updated_at: null,
            cached_at: 'legacy-write-meta',
          })
          transaction.oncomplete = () => {
            database.close()
            resolve()
          }
          transaction.onerror = () => reject(transaction.error)
          transaction.onabort = () => reject(transaction.error)
        }
      }),
    DB_NAME,
  )
}

async function cacheState(page: Page): Promise<CacheState> {
  return page.evaluate(
    (name) =>
      new Promise<CacheState>((resolve, reject) => {
        const request = indexedDB.open(name)
        request.onerror = () => reject(request.error)
        request.onsuccess = () => {
          const database = request.result
          const transaction = database.transaction(['issues', 'meta'], 'readonly')
          const issuesRequest = transaction.objectStore('issues').getAll()
          const syncRequest = transaction.objectStore('meta').get('sync')
          const writeRequest = transaction.objectStore('meta').get('write')
          transaction.oncomplete = () => {
            const rows = issuesRequest.result as Array<Record<string, unknown>>
            resolve({
              version: database.version,
              count: rows.length,
              hasLegacyRow: rows.some((row) => row.issue_key === 'LEGACY-1'),
              hasReporterID: rows.some((row) => typeof row.reporter_id === 'string' && row.reporter_id !== ''),
              syncVersion:
                typeof syncRequest.result?.sync_version === 'number'
                  ? syncRequest.result.sync_version
                  : null,
              writeCachedAt:
                typeof writeRequest.result?.cached_at === 'string'
                  ? writeRequest.result.cached_at
                  : null,
            })
            database.close()
          }
          transaction.onerror = () => reject(transaction.error)
        }
      }),
    DB_NAME,
  )
}

function issueRequestLog(page: Page): { requests: Request[]; stop: () => void } {
  const requests: Request[] = []
  const listener = (request: Request) => {
    if (request.url().includes('/api/v1/issues/')) requests.push(request)
  }
  page.on('request', listener)
  return { requests, stop: () => page.off('request', listener) }
}

test.describe('IssueLite cache upgrade', () => {
  test('legacy v1 cache invalidates once, then the v2 cache is reused', async ({ page }) => {
    await seedLegacyCache(page)
    await page.route('**/api/v1/issues/meta/write/', (route) => route.abort())
    const log = issueRequestLog(page)

    await gotoApp(page)

    expect(log.requests.some((request) => request.url().includes('/bootstrap/'))).toBe(true)
    expect(log.requests.some((request) => request.url().includes('/delta/'))).toBe(false)
    const bootstrap = log.requests.find((request) => request.url().includes('/bootstrap/'))
    expect(bootstrap?.headers()['if-none-match']).toBeUndefined()
    await expect
      .poll(() => cacheState(page), { timeout: 30_000 })
      .toMatchObject({
        version: 2,
        hasLegacyRow: false,
        hasReporterID: true,
        writeCachedAt: 'legacy-write-meta',
      })
    const upgraded = await cacheState(page)
    expect(upgraded.count).toBeGreaterThan(0)
    expect(upgraded.syncVersion).not.toBe(999)

    log.requests.length = 0

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    await expect
      .poll(() => log.requests.some((request) => request.url().includes('/delta/')))
      .toBe(true)

    expect(log.requests.some((request) => request.url().includes('/bootstrap/'))).toBe(false)
    const reopened = await cacheState(page)
    expect(reopened).toMatchObject({ version: 2, hasReporterID: true })
    expect(reopened.count).toBe(upgraded.count)
    expect(reopened.syncVersion).not.toBe(999)

    log.stop()
  })

  test('fresh install creates a v2 cache and bootstraps normally', async ({ page }) => {
    await deleteDatabase(page)
    const log = issueRequestLog(page)

    await gotoApp(page)

    expect(log.requests.some((request) => request.url().includes('/bootstrap/'))).toBe(true)
    await expect
      .poll(() => cacheState(page), { timeout: 30_000 })
      .toMatchObject({ version: 2, hasLegacyRow: false, hasReporterID: true })
    expect((await cacheState(page)).count).toBeGreaterThan(0)

    log.stop()
  })
})
