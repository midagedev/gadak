import { test, expect, type Page } from '@playwright/test'
import { dismissHostedFirstFrame } from './helpers'

/**
 * GDK-440 — a returning visitor must receive a newly published snapshot.
 *
 * Hosted used to take the live delta path whenever IndexedDB had a cache, and
 * the in-page adapter answered every delta with an empty upsert plus
 * server_time=now. A republished bootstrap.json never reached the pool.
 */

const DEMO = '/gadak/'
const NEW_KEY = 'SNAP-440'
const NEW_SUMMARY = 'Returning visitor must see this snapshot row'
const SNAPSHOT_TIME = '2099-01-15T12:00:00.000Z'
const IDB_NAME = 'issue-navigator'

function searchInput(page: Page) {
  return page.getByTestId('search-input')
}

async function applyAllOpen(page: Page): Promise<void> {
  await page.getByRole('button', { name: /All open/ }).click()
  await expect(page).not.toHaveURL(/[?&]g=epic(?:&|$)/)
}

async function forceLocale(page: Page, locale: 'en' | 'ko' = 'en'): Promise<void> {
  await page.addInitScript((loc) => {
    try {
      if (!localStorage.getItem('gadak_locale')) {
        localStorage.setItem('gadak_locale', loc)
      }
    } catch {
      /* ignore */
    }
  }, locale)
}

interface SyncMeta {
  issueCount: number
  serverTime: string | null
}

async function readSyncMeta(page: Page): Promise<SyncMeta> {
  return page.evaluate(
    (name) =>
      new Promise<SyncMeta>((resolve, reject) => {
        const request = indexedDB.open(name)
        request.onerror = () => reject(request.error ?? new Error('idb-open'))
        request.onsuccess = () => {
          const database = request.result
          if (
            !database.objectStoreNames.contains('issues') ||
            !database.objectStoreNames.contains('meta')
          ) {
            database.close()
            resolve({ issueCount: 0, serverTime: null })
            return
          }
          const tx = database.transaction(['issues', 'meta'], 'readonly')
          const issuesReq = tx.objectStore('issues').count()
          const metaReq = tx.objectStore('meta').get('sync')
          tx.oncomplete = () => {
            const meta = metaReq.result as { server_time?: string } | undefined
            resolve({
              issueCount: issuesReq.result,
              serverTime: typeof meta?.server_time === 'string' ? meta.server_time : null,
            })
            database.close()
          }
          tx.onerror = () => reject(tx.error)
        }
      }),
    IDB_NAME,
  )
}

test.describe('hosted demo returning visitor', () => {
  test('reload after a new bootstrap.json shows the new issue (GDK-440)', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 60_000 })

    // Returning-visitor means IndexedDB is a complete cache, not a cold boot.
    await expect
      .poll(async () => (await readSyncMeta(page)).issueCount, { timeout: 30_000 })
      .toBeGreaterThan(500)

    await applyAllOpen(page)
    await searchInput(page).fill(NEW_KEY)
    await expect(page.getByText(NEW_KEY)).toHaveCount(0)

    await page.route('**/gadak/bootstrap.json', async (route) => {
      const res = await route.fetch()
      const body = (await res.json()) as {
        server_time?: string
        issues: Array<Record<string, unknown>>
      }
      const template = body.issues[0]
      if (!template) {
        await route.fulfill({ status: 500, body: 'empty bootstrap' })
        return
      }
      body.issues = [
        ...body.issues,
        {
          ...template,
          issue_key: NEW_KEY,
          summary: NEW_SUMMARY,
          status_category: 'new',
          resolved_at: null,
          epic_key: null,
          parent_key: null,
          updated_at: SNAPSHOT_TIME,
        },
      ]
      body.server_time = SNAPSHOT_TIME
      await route.fulfill({ json: body })
    })

    await page.reload({ waitUntil: 'domcontentloaded' })
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await applyAllOpen(page)
    await searchInput(page).fill(NEW_KEY)
    await expect(page.getByTestId('list-count')).toContainText('1', { timeout: 15_000 })
    await expect(page.getByText(NEW_KEY).first()).toBeVisible()
    await expect(page.getByText(NEW_SUMMARY).first()).toBeVisible()

    await expect
      .poll(async () => (await readSyncMeta(page)).serverTime, { timeout: 15_000 })
      .toBe(SNAPSHOT_TIME)
  })
})
