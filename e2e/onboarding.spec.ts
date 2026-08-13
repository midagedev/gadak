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

/*
 * One complete row, so a finished wizard can hand the pane to a real list —
 * every field the client reads is present, since a missing one shows up as a
 * console error rather than a failed assertion.
 */
const MIRRORED_ISSUE = {
  issue_key: 'NMB-1',
  summary: 'Ship the first mirrored issue',
  status: 'In Progress',
  status_category: 'indeterminate',
  issue_type: 'Task',
  priority: 'Medium',
  priority_rank: 3,
  severity: null,
  assignee: 'Dana Scully',
  assignee_email: 'dana@example.com',
  reporter: 'Dana Scully',
  reporter_email: 'dana@example.com',
  labels: [],
  fix_versions: [],
  components: [],
  team_group: null,
  epic_key: null,
  parent_key: null,
  source_project: 'NMB',
  created_at: '2026-08-01T00:00:00.000Z',
  updated_at: '2026-08-03T00:00:00.000Z',
  resolved_at: null,
  status_changed_at: '2026-08-03T00:00:00.000Z',
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
  qa_impact_state: 'none',
  qa_impact_label: '',
  qa_runs: [],
  qa_suites: [],
}

/**
 * Fresh instance: empty mirror, no identity, no projects. `mirror()` is what the
 * pool reads on every (re)sync — empty until the caller says the first sync
 * landed, which is how the mirror filling under the wizard gets exercised.
 */
async function mockFirstRun(page: Page, mirror: () => unknown[] = () => []): Promise<void> {
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
    route.fulfill({ json: { ...EMPTY_BOOTSTRAP, issues: mirror() } }),
  )
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({
      json: { ...EMPTY_BOOTSTRAP, issues: mirror(), upserted: mirror(), deleted_keys: [] },
    }),
  )
}

interface Flow {
  /** True once the mocked sync has reported done — flips what the mirror serves. */
  synced: boolean
  connectBody: Record<string, unknown>
  savedProjects: unknown
}

/** Routes for the three required steps, plus the state the assertions read back. */
async function mockWizard(page: Page): Promise<Flow> {
  const flow: Flow = { synced: false, connectBody: {}, savedProjects: null }
  await mockFirstRun(page, () => (flow.synced ? [MIRRORED_ISSUE] : []))

  await page.route(`${API}onboarding/connect/`, async (route) => {
    flow.connectBody = route.request().postDataJSON() as Record<string, unknown>
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
  await page.route(`${API}settings/`, async (route) => {
    if (route.request().method() === 'PUT') {
      flow.savedProjects = (route.request().postDataJSON() as { projects: string[] }).projects
      await route.fulfill({ json: { projects: flow.savedProjects } })
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
    if (!running) flow.synced = true
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
  return flow
}

/** Drive the three required steps; leaves the wizard on the optional step 4. */
async function runRequiredSteps(page: Page): Promise<void> {
  const wizard = page.getByTestId('onboarding')
  await expect(wizard).toBeVisible({ timeout: 30_000 })
  await expect(wizard.getByText('Step 1 of 4 · Connect')).toBeVisible()
  await wizard.locator('input[name="site"]').fill('https://example.atlassian.net')
  await wizard.locator('input[name="email"]').fill('dana@example.com')
  await wizard.locator('input[name="token"]').fill('super-secret-token')
  await wizard.getByRole('button', { name: 'Connect', exact: true }).click()

  await expect(page.getByTestId('onboarding-projects')).toBeVisible()
  await expect(wizard.getByText('Step 2 of 4 · Projects')).toBeVisible()
  await expect(wizard.getByText('Connected as Dana Scully')).toBeVisible()
  await expect(wizard.getByText('Nimbus')).toBeVisible()
  await expect(wizard.getByText('Operations')).toBeVisible()
  await wizard.getByRole('checkbox').first().check()
  await expect(wizard.getByText('1 selected')).toBeVisible()
  await wizard.getByRole('button', { name: 'Start first sync' }).click()

  await expect(page.getByTestId('onboarding-agent')).toBeVisible({ timeout: 20_000 })
}

test.describe('first-run onboarding', () => {
  test('connect → projects → live first sync → the optional agent step', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    const flow = await mockWizard(page)

    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await runRequiredSteps(page)

    expect(flow.connectBody).toMatchObject({
      site: 'https://example.atlassian.net',
      jira_email: 'dana@example.com',
      api_token: 'super-secret-token',
    })
    expect(flow.savedProjects).toEqual(['NMB'])

    // Step 4 — the sync result carries over, and the step says it is optional.
    await expect(wizard.getByText('Optional · Connect an agent')).toBeVisible()
    await expect(page.getByTestId('onboarding-sync-done')).toContainText('Mirrored 519 issues')
    await expect(page.getByTestId('onboarding-cmd-claude')).toContainText('gadak mcp install claude')
    await expect(wizard.getByText('gadak mcp install cursor')).toBeVisible()
    await expect(wizard.getByText('gadak mcp install codex')).toBeVisible()
    await expect(wizard.getByRole('link', { name: 'Agent setup' })).toBeVisible()
    await expect(wizard.getByRole('link', { name: 'Query recipes' })).toBeVisible()

    // The command is copied, not run: nothing here posts to the server.
    await page.getByTestId('onboarding-copy-claude').click()
    await expect(page.getByTestId('onboarding-copy-claude')).toHaveText('Copied')
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('gadak mcp install claude')

    // The mirror filling underneath must not yank the step away: a background
    // resync (the store's own visibilitychange path) lands rows in the pool
    // while the wizard keeps the pane.
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(page.getByText('1 issues').first()).toBeVisible()
    await expect(page.getByTestId('onboarding-agent')).toBeVisible()

    // Completing the step enters the app.
    await page.getByTestId('onboarding-finish').click()
    await expect(page.getByTestId('onboarding')).toBeHidden()
    await expect(page.getByText('Ship the first mirrored issue')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the agent step is optional — Skip enters the app just the same', async ({ page }) => {
    await mockWizard(page)
    await forceLocale(page, 'en')
    await page.goto('/')

    await runRequiredSteps(page)
    await page.getByTestId('onboarding-skip').click()

    await expect(page.getByTestId('onboarding')).toBeHidden()
    await expect(page.getByText('Ship the first mirrored issue')).toBeVisible()
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
    await expect(page.getByTestId('onboarding-error')).toContainText('Org API keys')
    await expect(page.getByTestId('onboarding-connect')).toBeVisible()
  })
})
