import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from './helpers'
import { en } from '../web/src/lib/i18n/en'
import type { Locale } from '../web/src/lib/i18n/types'

/*
 * First-sync band (GDK-1677): while a first sync fills an empty mirror the
 * list grows page by page, and a half-filled list must not read as "few
 * issues". The band is driven solely by sync/progress's first_sync field —
 * these specs stub that endpoint over an empty mocked mirror (the freshness
 * chip's mockMirror pattern), because the committed fixture mirror is full
 * and no first sync ever runs against it.
 */

const API = '**/api/v1/issues/'

/** Mirror of api.ts SyncProgress.first_sync — the contract under test. */
type FirstSyncFixture = {
  in_progress: boolean
  phase: 'issues' | 'documents'
  fetched: number
  total?: number
  wiki_pending?: boolean
  started_at: string
}

function bootstrapWith() {
  return {
    server_time: new Date().toISOString(),
    sync_version: 0,
    members: [],
    members_version: '',
    issues: [],
    sync_health: {
      overall: 'healthy',
      checked_at: new Date().toISOString(),
      sources: [{ key: 'jira', label: 'Jira', status: 'healthy', synced_at: new Date().toISOString(), message: 'ok' }],
    },
  }
}

/**
 * Empty mirror + a sync/progress answer whose first_sync comes from a
 * queue: each poll shifts one entry, and the last entry is held forever
 * (the poller keeps polling after the sequence ends). `null` fulfills
 * without the field — absent is the "no first sync" contract, not a 0-value.
 */
async function mockFirstSync(
  page: Page,
  locale: Locale,
  queue: (FirstSyncFixture | null)[],
): Promise<() => number> {
  let polls = 0
  await page.route('**/config.json', (route) =>
    route.fulfill({
      json: {
        apiBase: '/api/v1/issues/',
        authBase: '/api/v1/auth/',
        jiraBaseUrl: 'https://example.atlassian.net',
        projects: ['NMB'],
        features: {},
      },
    }),
  )
  await page.route('**/api/v1/auth/me/', (route) =>
    route.fulfill({ json: { email: 'dana@example.com', name: 'Dana Scully', group: null } }),
  )
  await page.route(`${API}bootstrap/**`, (route) => route.fulfill({ json: bootstrapWith() }))
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({ json: { ...bootstrapWith(), upserted: [], deleted_keys: [] } }),
  )
  await page.route(`${API}credential/`, (route) =>
    route.fulfill({
      json: {
        configured: true,
        jira_email: 'dana@example.com',
        display_name: 'Dana Scully',
        verified_at: '2026-08-04T00:00:00.000Z',
        token_hint: '…9876',
      },
    }),
  )
  await page.route(`${API}meta/write/`, (route) =>
    route.fulfill({ json: { transitions: {}, projects: [], updated_at: null } }),
  )
  await page.route(`${API}sync/progress/`, (route) => {
    polls += 1
    const first = queue.length > 1 ? queue.shift() : queue[0]
    route.fulfill({
      json: {
        running: false,
        phase: 'idle',
        fetched: 0,
        changed: 0,
        deleted: 0,
        done: true,
        error: '',
        started_at: 'now',
        finished_at: 'now',
        first_sync: first ?? undefined,
      },
    })
  })
  await forceLocale(page, locale)
  await page.goto('/')
  return () => polls
}

const ISSUES_TOTAL: FirstSyncFixture = {
  in_progress: true,
  phase: 'issues',
  fetched: 1200,
  total: 3514,
  wiki_pending: true,
  started_at: '2026-09-09T01:13:31Z',
}

test.describe('first-sync band', () => {
  test('first_sync present: the band carries the exact line above the rows', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockFirstSync(page, 'en', [ISSUES_TOTAL])

    const band = page.getByTestId('first-sync-band')
    await expect(band).toBeVisible({ timeout: 30_000 })
    await expect(band).toHaveText('Recent issues first · 1,200 / 3,514 · wiki next')
    await expect(band).toHaveAttribute('role', 'status')
    await expect(band).toHaveAttribute('aria-live', 'polite')
    // The band is list chrome, not header chrome: toolbar and count stay.
    await expect(page.getByTestId('list-toolbar')).toBeVisible()
    await expect(page.getByTestId('list-count')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the poll sequence issues → documents → absent swaps the line, then removes the band', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const DOCS_TOTAL: FirstSyncFixture = {
      in_progress: true,
      phase: 'documents',
      fetched: 120,
      total: 462,
      started_at: '2026-09-09T01:13:31Z',
    }
    // Each poll shifts one answer; the busy cadence (2s) drives the pace.
    // The issues line is queued twice so the first assertion has a full
    // cadence window to sample it before the swap lands.
    await mockFirstSync(page, 'en', [ISSUES_TOTAL, ISSUES_TOTAL, DOCS_TOTAL, null])

    const band = page.getByTestId('first-sync-band')
    await expect(band).toBeVisible({ timeout: 30_000 })
    await expect(band).toHaveText('Recent issues first · 1,200 / 3,514 · wiki next')
    // Documents phase: issues are done, the wiki counts, no "next" tail.
    await expect(band).toHaveText('Issues done · wiki 120 / 462')
    await expect(band).toHaveAttribute('data-phase', 'documents')
    // Absent on the next poll: no done state, no toast — the band is gone.
    await expect(band).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('first_sync absent on every poll: the band never appears', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const polls = await mockFirstSync(page, 'en', [null])

    await expect(page.getByTestId('list-toolbar')).toBeVisible({ timeout: 30_000 })
    // Two polls came back without the field before "never" is a claim: the
    // first lands at once, the second proves the cadence re-asked.
    await expect.poll(() => polls()).toBeGreaterThanOrEqual(2)
    await expect(page.getByTestId('first-sync-band')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Korean locale renders the ko line verbatim', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockFirstSync(page, 'ko', [ISSUES_TOTAL])

    const band = page.getByTestId('first-sync-band')
    await expect(band).toBeVisible({ timeout: 30_000 })
    await expect(band).toHaveText('최근 이슈부터 채우는 중 · 1,200 / 3,514 · 다음은 위키')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an empty mirror with the band live shows no empty-state copy', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockFirstSync(page, 'en', [ISSUES_TOTAL])

    const band = page.getByTestId('first-sync-band')
    await expect(band).toBeVisible({ timeout: 30_000 })
    // The pool is empty and filling: "No issues" (and its Run-sync action)
    // would describe issues that are arriving, not absent ones.
    await expect(page.getByText(en['list.emptyTitle'], { exact: true })).toHaveCount(0)
    await expect(page.getByTestId('empty-state-action')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
