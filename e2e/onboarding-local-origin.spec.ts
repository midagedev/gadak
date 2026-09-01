import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from './helpers'

/*
 * GDK-247: connecting a Jira site onto a local-origin workspace that holds
 * locally originated issues must be refused until the user opts in.
 * The demo server is a connected fixture, so the empty first-run state and
 * the 409 are both route-mocked — no request reaches Jira or mutates config.
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

const PERSIST = '/tmp/gadak-e2e/origin/issuetap.yaml'

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
  await page.route(`${API}bootstrap/**`, (route) =>
    route.fulfill({ json: { ...EMPTY_BOOTSTRAP, issues: [] } }),
  )
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({
      json: { ...EMPTY_BOOTSTRAP, issues: [], upserted: [], deleted_keys: [] },
    }),
  )
}

test.describe('local-origin onboarding origin guard', () => {
  test('409 standalone_data_present is shown; replace requires a second confirm', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await mockFirstRun(page)

    const connectBodies: Record<string, unknown>[] = []
    await page.route(`${API}onboarding/connect/`, async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      connectBodies.push(body)
      if (body.replace_standalone === true) {
        await route.fulfill({ json: CREDENTIAL })
        return
      }
      await route.fulfill({
        status: 409,
        json: { error: 'standalone_data_present', issues: 3, persist: PERSIST },
      })
    })
    await page.route(`${API}projects/available/`, (route) =>
      route.fulfill({
        json: {
          projects: [{ key: 'NMB', name: 'Nimbus', projectTypeKey: 'software' }],
          truncated: false,
        },
      }),
    )

    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    await wizard.locator('input[name="site"]').fill('https://example.atlassian.net')
    await wizard.locator('input[name="email"]').fill('dana@example.com')
    await wizard.locator('input[name="token"]').fill('super-secret-token')
    await wizard.getByRole('button', { name: 'Connect', exact: true }).click()

    const block = page.getByTestId('onboarding-local-origin-block')
    await expect(block).toBeVisible()
    // GDK-1281: the sentence now says where the issues came from rather
    // than naming a workspace kind. The count and its subject are what this
    // assertion is about, and both are still here.
    await expect(block).toContainText('3 issues or documents that originated here')
    await expect(page.getByTestId('onboarding-local-origin-persist')).toContainText(PERSIST)
    await expect(block).toContainText('separate workspace')
    expect(connectBodies).toHaveLength(1)
    expect(connectBodies[0].replace_standalone).toBeUndefined()

    const replace = wizard.getByRole('button', { name: 'Replace and connect', exact: true })
    await expect(replace).toBeDisabled()

    await page.getByTestId('onboarding-replace-local').check()
    await expect(replace).toBeEnabled()
    await replace.click()

    await expect(page.getByTestId('onboarding-projects')).toBeVisible()
    expect(connectBodies).toHaveLength(2)
    expect(connectBodies[1].replace_standalone).toBe(true)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
