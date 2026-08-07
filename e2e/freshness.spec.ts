import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp } from './helpers'

/*
 * Freshness chip: the mirror↔Jira leg made visible in the list header.
 * The tone cases stub sync_health so the state under test is the one asserted
 * — the committed fixture only ever ages into "stale".
 */

const API = '**/api/v1/issues/'

type Source = {
  key: string
  label: string
  status: string
  synced_at: string | null
  message: string
}

function isoAgo(ms: number): string {
  return new Date(Date.now() - ms).toISOString()
}

function bootstrapWith(sources: Source[]) {
  const overall =
    sources.some((s) => s.status === 'failed')
      ? 'failed'
      : sources.some((s) => s.status === 'stale' || s.status === 'missing')
        ? 'warning'
        : 'healthy'
  return {
    server_time: new Date().toISOString(),
    sync_version: 0,
    members: [],
    members_version: '',
    issues: [],
    sync_health: { overall, checked_at: new Date().toISOString(), sources },
  }
}

/** Identified session over an empty mirror whose sync_health we control. */
async function mockMirror(page: Page, sources: Source[]): Promise<void> {
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
  await page.route(`${API}bootstrap/**`, (route) =>
    route.fulfill({ json: bootstrapWith(sources) }),
  )
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({ json: { ...bootstrapWith(sources), upserted: [], deleted_keys: [] } }),
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
  await forceLocale(page, 'en')
  await page.goto('/')
}

test.describe('freshness chip', () => {
  test('renders the mirror age as relative time', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockMirror(page, [
      { key: 'jira', label: 'Jira', status: 'healthy', synced_at: isoAgo(30_000), message: 'ok' },
    ])

    const chip = page.getByTestId('freshness-chip')
    await expect(chip).toBeVisible({ timeout: 30_000 })
    await expect(chip).toHaveAttribute('data-state', 'fresh')
    // relativeTime is minute-granular below the hour: 30s reads as "just now".
    await expect(chip).toHaveText(/^\s*Synced (just now|\d+m ago)\s*$/)
    await expect(chip).toHaveAttribute('title', /Mirror pulled from Jira/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a behind mirror gets the stale tone, a failed one the error tone', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockMirror(page, [
      {
        key: 'jira',
        label: 'Jira',
        status: 'stale',
        synced_at: isoAgo(4 * 60 * 60_000),
        message: 'last sync is behind',
      },
    ])

    const chip = page.getByTestId('freshness-chip')
    await expect(chip).toBeVisible({ timeout: 30_000 })
    await expect(chip).toHaveAttribute('data-state', 'stale')
    // The verdict now travels with the age. "Synced 4h ago" was true and
    // useless on its own: it never said that four hours behind is a problem
    // here, which is the whole reason this chip has a stale tone at all.
    await expect(chip).toHaveText('Sync delayed · 4h ago')
    await expect(chip).toHaveClass(/text-status-stale/)
    await expect(chip).toHaveAttribute('title', /Mirror is behind/)

    // Same page, failed source: the error tone and the server's message in the tooltip.
    await page.unrouteAll({ behavior: 'ignoreErrors' })
    await mockMirror(page, [
      {
        key: 'jira',
        label: 'Jira',
        status: 'failed',
        synced_at: isoAgo(2 * 60 * 60_000),
        message: 'jira: 401 unauthorized',
      },
    ])
    const failed = page.getByTestId('freshness-chip')
    await expect(failed).toBeVisible({ timeout: 30_000 })
    await expect(failed).toHaveAttribute('data-state', 'failed')
    // Same rule as the stale case: the age rides along, so "failed" is anchored
    // to how old the last good mirror is rather than left as a bare alarm.
    await expect(failed).toHaveText('Sync failed · 2h ago')
    await expect(failed).toHaveClass(/text-status-reopen/)
    await expect(failed).toHaveAttribute('title', /jira: 401 unauthorized/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('clicking the chip POSTs an incremental sync', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

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

    const chip = page.getByTestId('freshness-chip')
    await expect(chip).toBeVisible()
    await chip.click()

    await expect.poll(() => syncBody).toEqual({ mode: 'incremental' })
    await expect(
      page.getByTestId('toast').filter({ hasText: /Sync finished|Starting sync/i }).first(),
    ).toBeVisible({ timeout: 10_000 })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
