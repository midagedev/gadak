import { expect, test, type Page, type Request } from '@playwright/test'
import { gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

// e2e/serve.sh writes site https://nimbus.example.com — see composeCacheScope.
//
// GDK-826: the IndexedDB upgrade internals this spec used to re-seed
// (v1→v2, legacy rows dropped, write meta kept, v2 reopen not clearing) are
// owned by web/src/lib/db.test.ts against fake-indexeddb — the export
// exists for exactly that. What only a browser can prove is the network
// wiring around the cache: a cold first visit bootstraps without
// If-None-Match, and a reload asks for a delta instead of re-bootstrapping.

const DB_NAME = 'issue-navigator:site:nimbus.example.com'

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

function issueRequestLog(page: Page): { requests: Request[]; stop: () => void } {
  const requests: Request[] = []
  const listener = (request: Request) => {
    if (request.url().includes('/api/v1/issues/')) requests.push(request)
  }
  page.on('request', listener)
  return { requests, stop: () => page.off('request', listener) }
}

test.describe('IssueLite cache network wiring', () => {
  test('first visit bootstraps with no If-None-Match; reload asks for a delta', async ({ page }) => {
    await deleteDatabase(page)
    await page.route('**/api/v1/issues/meta/write/', (route) => route.abort())
    const log = issueRequestLog(page)

    await gotoApp(page)

    const bootstrap = log.requests.find((request) => request.url().includes('/bootstrap/'))
    expect(bootstrap, 'a cold cache must bootstrap').toBeTruthy()
    expect(bootstrap?.headers()['if-none-match']).toBeUndefined()
    expect(log.requests.some((request) => request.url().includes('/delta/'))).toBe(false)

    log.requests.length = 0

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect
      .poll(() => log.requests.some((request) => request.url().includes('/delta/')))
      .toBe(true)
    expect(log.requests.some((request) => request.url().includes('/bootstrap/'))).toBe(false)

    log.stop()
  })
})
