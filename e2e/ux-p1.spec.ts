import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

/*
 * UX P1 coverage: Sync now (palette), empty-mirror Sync now, credential copy split.
 * Demo fixture is fully set up; empty/credential paths use route mocks.
 */

const API = '**/api/v1/issues/'

const EMPTY_BOOTSTRAP = {
  server_time: '2026-08-04T00:00:00.000Z',
  sync_version: 0,
  members: [],
  members_version: '',
  issues: [],
  sync_health: { overall: 'healthy', checked_at: '2026-08-04T00:00:00.000Z', sources: [] },
}

async function mockIdentifiedEmptyMirror(page: Page): Promise<void> {
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
    route.fulfill({
      json: { email: 'dana@example.com', name: 'Dana Scully', group: null },
    }),
  )
  await page.route(`${API}bootstrap/**`, (route) => route.fulfill({ json: EMPTY_BOOTSTRAP }))
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({
      json: { ...EMPTY_BOOTSTRAP, upserted: [], deleted_keys: [] },
    }),
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
}

test.describe('UX P1', () => {
  test('command palette offers Sync now and POSTs incremental sync', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    // The focus-time mirror pull fires on a stale fixture, and runSyncNow is
    // single-flight: clicking while it runs joins that run instead of starting
    // one, so the POST this test is about never happens. Waiting on the chip
    // waits for the state the assertion depends on, not for a fixed delay —
    // under full-suite load 300ms was not enough and this failed as a flake.
    await expect(page.getByTestId('freshness-chip')).not.toHaveAttribute('data-state', 'syncing', {
      timeout: 30_000,
    })

    let syncBody: unknown = null
    await page.route(`${API}sync/`, async (route) => {
      if (route.request().method() === 'POST') {
        try {
          syncBody = route.request().postDataJSON()
        } catch {
          syncBody = null
        }
        await route.fulfill({
          status: 202,
          json: {
            running: true,
            phase: 'syncing',
            fetched: 0,
            changed: 0,
            deleted: 0,
            done: false,
            error: '',
            started_at: 'now',
            finished_at: '',
          },
        })
        return
      }
      await route.continue()
    })
    let polls = 0
    await page.route(`${API}sync/progress/`, (route) => {
      polls += 1
      const running = polls < 2
      route.fulfill({
        json: {
          running,
          phase: running ? 'syncing' : 'done',
          fetched: running ? 10 : 20,
          changed: running ? 1 : 2,
          deleted: 0,
          done: !running,
          error: '',
          started_at: 'now',
          finished_at: running ? '' : 'now',
        },
      })
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('Sync now', { delay: 15 })
    // exact: the palette's instant-create row (GDK-217) echoes the query, so
    // any typed text also appears inside `Create "Sync now"`.
    const option = palette.getByRole('option', { name: 'Sync now', exact: true })
    await expect(option).toBeVisible()
    await option.click()

    await expect.poll(() => syncBody).toEqual({ mode: 'incremental' })
    await expect(page.getByTestId('toast').filter({ hasText: /Sync finished|Starting sync/i }).first()).toBeVisible({
      timeout: 10_000,
    })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('empty mirror shows Sync now and starts a full sync', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockIdentifiedEmptyMirror(page)

    let syncMode: unknown = null
    await page.route(`${API}sync/`, async (route) => {
      if (route.request().method() === 'POST') {
        try {
          syncMode = (route.request().postDataJSON() as { mode?: string }).mode
        } catch {
          syncMode = null
        }
        await route.fulfill({
          status: 202,
          json: {
            running: false,
            phase: 'done',
            fetched: 0,
            changed: 0,
            deleted: 0,
            done: true,
            error: '',
            started_at: 'now',
            finished_at: 'now',
          },
        })
        return
      }
      await route.continue()
    })
    await page.route(`${API}sync/progress/`, (route) =>
      route.fulfill({
        json: {
          running: false,
          phase: 'done',
          fetched: 0,
          changed: 0,
          deleted: 0,
          done: true,
          error: '',
          started_at: 'now',
          finished_at: 'now',
        },
      }),
    )

    await forceLocale(page, 'en')
    await page.goto('/')

    await expect(page.getByText('No issues')).toBeVisible({ timeout: 30_000 })
    const runSync = page.getByTestId('empty-state-action')
    await expect(runSync).toHaveText('Sync now')
    await runSync.click()

    await expect.poll(() => syncMode).toBe('full')
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('expired token toast says replace, not set-first', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()

    await page.route(`${API}**/comment/`, (route) => {
      if (route.request().method() === 'POST') {
        route.fulfill({ status: 409, json: { error: 'credential_rejected' } })
        return
      }
      route.continue()
    })

    const composer = page.getByTestId('comment-composer')
    await composer.fill('ux-p1 credential rejected probe')
    await composer.press('Meta+Enter')

    await expect(page.getByTestId('toast').filter({ hasText: /rejected|replace/i })).toBeVisible({
      timeout: 8_000,
    })
    await expect(page.getByText(/Set your personal Jira API token first/i)).toHaveCount(0)

    // Intentional 409 surfaces as a browser network console line — ignore those.
    const real = errors.filter((e) => !/status of 409/.test(e))
    expect(real, `console errors:\n${real.join('\n')}`).toEqual([])
  })
})
