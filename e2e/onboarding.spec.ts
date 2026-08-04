import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from './helpers'

/*
 * First-run onboarding against a mocked-empty instance. The demo server is fully
 * set up, so the empty first-run state is built with route mocks: no credential,
 * no projects, an empty mirror — and no request ever reaches Jira or mutates the
 * fixture's config.
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

const CREDENTIAL = {
  configured: true,
  jira_email: 'dana@example.com',
  display_name: 'Dana Scully',
  verified_at: '2026-08-04T00:00:00.000Z',
  token_hint: '…9876',
}

/** Fresh instance: empty mirror, no identity, no projects. */
async function mockFirstRun(page: Page): Promise<void> {
  await page.route('**/config.json', (route) =>
    route.fulfill({
      json: {
        apiBase: '/api/v1/issues/',
        authBase: '/api/v1/auth/',
        jiraBaseUrl: '',
        projects: [],
        features: {},
      },
    }),
  )
  await page.route('**/api/v1/auth/me/', (route) => route.fulfill({ json: { email: null } }))
  await page.route(`${API}bootstrap/**`, (route) => route.fulfill({ json: EMPTY_BOOTSTRAP }))
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({
      json: { ...EMPTY_BOOTSTRAP, upserted: [], deleted_keys: [] },
    }),
  )
}

test.describe('first-run onboarding', () => {
  test('connect → projects → live first sync, without touching the CLI', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockFirstRun(page)

    let connectBody: Record<string, unknown> = {}
    await page.route(`${API}onboarding/connect/`, async (route) => {
      connectBody = route.request().postDataJSON() as Record<string, unknown>
      await route.fulfill({ json: CREDENTIAL })
    })
    await page.route(`${API}projects/available/`, (route) =>
      route.fulfill({
        json: {
          projects: [
            { key: 'NMB', name: 'Nimbus', projectTypeKey: 'software' },
            { key: 'OPS', name: 'Operations', projectTypeKey: 'service_desk' },
          ],
          truncated: false,
        },
      }),
    )
    let savedProjects: unknown = null
    await page.route(`${API}settings/`, async (route) => {
      if (route.request().method() === 'PUT') {
        savedProjects = (route.request().postDataJSON() as { projects: string[] }).projects
        await route.fulfill({ json: { projects: savedProjects } })
        return
      }
      await route.fulfill({ json: { projects: [], staleThresholdHours: 72 } })
    })

    // Progress: two polled snapshots — still fetching, then finished.
    let polls = 0
    await page.route(`${API}sync/`, (route) =>
      route.fulfill({ json: { running: true, phase: 'syncing', fetched: 0, changed: 0, deleted: 0, done: false, error: '', started_at: 'now', finished_at: '' } }),
    )
    await page.route(`${API}sync/progress/`, (route) => {
      polls += 1
      const running = polls < 2
      route.fulfill({
        json: {
          running,
          phase: running ? 'syncing' : 'done',
          fetched: running ? 240 : 519,
          changed: running ? 240 : 519,
          deleted: 0,
          done: !running,
          error: '',
          started_at: 'now',
          finished_at: running ? '' : 'now',
        },
      })
    })

    await forceLocale(page, 'en')
    await page.goto('/')

    // Step 1 — connect.
    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    await expect(wizard.getByText('Step 1 of 3 · Connect')).toBeVisible()
    await wizard.locator('input[name="site"]').fill('https://example.atlassian.net')
    await wizard.locator('input[name="email"]').fill('dana@example.com')
    await wizard.locator('input[name="token"]').fill('super-secret-token')
    await wizard.getByRole('button', { name: 'Connect', exact: true }).click()

    // Step 2 — the site's real project list, checkboxes.
    await expect(page.getByTestId('onboarding-projects')).toBeVisible()
    await expect(wizard.getByText('Step 2 of 3 · Projects')).toBeVisible()
    await expect(wizard.getByText('Connected as Dana Scully')).toBeVisible()
    expect(connectBody).toMatchObject({
      site: 'https://example.atlassian.net',
      jira_email: 'dana@example.com',
      api_token: 'super-secret-token',
    })
    await expect(wizard.getByText('Nimbus')).toBeVisible()
    await expect(wizard.getByText('Operations')).toBeVisible()
    await wizard.getByRole('checkbox').first().check()
    await expect(wizard.getByText('1 selected')).toBeVisible()
    await wizard.getByRole('button', { name: 'Start first sync' }).click()

    // Step 3 — counts move, then the sync reports done. No reload anywhere.
    await expect(page.getByTestId('onboarding-sync')).toBeVisible()
    await expect(page.getByTestId('onboarding-sync-done')).toContainText('Mirrored 519 issues', {
      timeout: 20_000,
    })
    expect(savedProjects).toEqual(['NMB'])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a rejected credential says so and stays on step 1', async ({ page }) => {
    await mockFirstRun(page)
    await page.route(`${API}onboarding/connect/`, (route) =>
      route.fulfill({ status: 401, json: { error: 'credential_rejected' } }),
    )

    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    await wizard.locator('input[name="site"]').fill('https://example.atlassian.net')
    await wizard.locator('input[name="email"]').fill('dana@example.com')
    await wizard.locator('input[name="token"]').fill('wrong')
    await wizard.getByRole('button', { name: 'Connect', exact: true }).click()

    await expect(page.getByTestId('onboarding-error')).toContainText('Jira rejected')
    await expect(page.getByTestId('onboarding-connect')).toBeVisible()
  })
})
