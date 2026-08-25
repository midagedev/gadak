import { expect, test, type Page, type Request } from '@playwright/test'
import { forceLocale, gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

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

// GDK-835: a v2 cached row that predates `labels` must not collapse the list.
// Seeded at version 2 so the v1→v2 wipe cannot be what saves us. Bootstrap
// and delta are aborted so the first paint is the cached row, not a network
// replace.
const LEGACY_KEY = 'LEGACY-835'

async function seedLegacyV2Row(page: Page): Promise<void> {
  await page.evaluate(
    async ({ name, key }) => {
      await new Promise<void>((resolve, reject) => {
        const request = indexedDB.open(name, 2)
        request.onupgradeneeded = () => {
          const db = request.result
          if (!db.objectStoreNames.contains('issues')) {
            db.createObjectStore('issues', { keyPath: 'issue_key' })
          }
          if (!db.objectStoreNames.contains('meta')) {
            db.createObjectStore('meta', { keyPath: 'key' })
          }
        }
        request.onerror = () => reject(request.error ?? new Error('indexeddb-open-failed'))
        request.onsuccess = () => {
          const db = request.result
          const tx = db.transaction(['issues', 'meta'], 'readwrite')
          tx.oncomplete = () => {
            db.close()
            resolve()
          }
          tx.onerror = () => reject(tx.error ?? new Error('indexeddb-seed-failed'))
          tx.objectStore('issues').put({
            issue_key: key,
            summary: 'legacy cached row missing labels',
            status: 'To Do',
            status_category: 'new',
            issue_type: 'Bug',
            priority: null,
            priority_rank: 0,
            severity: null,
            assignee: null,
            assignee_email: null,
            reporter: null,
            reporter_email: null,
            team_group: null,
            epic_key: null,
            parent_key: null,
            source_project: 'NMB',
            created_at: '2026-01-01T00:00:00.000Z',
            updated_at: '2026-08-24T00:00:00.000Z',
            resolved_at: null,
            status_changed_at: null,
            reopen_count: 0,
            reopened_at: null,
            reopen_reason: null,
            comment_count: 0,
            dev_project_number: null,
            related_project_number: null,
            environment: null,
            browser: null,
            found_version: null,
            occurrence: null,
            solution: null,
            critical_phenomenon: null,
            development_area: null,
            cs: null,
            development_test_assignee: null,
            development_test_assignee_email: null,
            development_test_result: null,
            qa_impact_state: '',
            qa_impact_label: '',
            qa_runs: [],
            qa_suites: [],
          })
          tx.objectStore('meta').put({
            key: 'sync',
            server_time: '2026-08-01T00:00:00.000Z',
            sync_version: 1,
            members: [],
          })
        }
      })
    },
    { name: DB_NAME, key: LEGACY_KEY },
  )
}

test.describe('legacy cached row (GDK-835)', () => {
  test('a v2 row missing labels still renders the list; skeleton does not persist', async ({
    page,
  }) => {
    await forceLocale(page, 'en')
    await deleteDatabase(page)
    await seedLegacyV2Row(page)
    await page.route('**/api/v1/issues/bootstrap/**', (route) => route.abort())
    await page.route('**/api/v1/issues/delta/**', (route) => route.abort())

    await page.goto('/')

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 15_000 })
    await expect(page.locator(`[data-issue-key="${LEGACY_KEY}"]`)).toBeVisible({ timeout: 15_000 })
    await expect(page.locator('html')).not.toHaveAttribute('data-skeleton')
  })
})
