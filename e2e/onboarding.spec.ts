import { test, expect, type Page } from '@playwright/test'
import { en } from '../web/src/lib/i18n/en'
import { attachConsoleErrors, forceLocale } from './helpers'
import { mockBrowseRoutes, type BrowseMock } from './browse-mock'

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

/**
 * F6: while the wizard owns the pane, write/query chrome that duplicates it
 * must be gone. The settings gear stays on every step. The wizard's own
 * Open settings is step 1 only (and the no-projects branch of step 2) —
 * pass `wizardEscape: true` there.
 */
async function assertOnboardingChromeQuiet(
  page: Page,
  opts: { wizardEscape?: boolean } = {},
): Promise<void> {
  const wizard = page.getByTestId('onboarding')
  await expect(wizard).toBeVisible()
  await expect(page.getByRole('button', { name: en['write.newIssue'] })).toHaveCount(0)
  await expect(page.getByRole('button', { name: en['common.setCredentials'], exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: en['settings.localOriginHow'] })).toHaveCount(0)
  await expect(page.getByTestId('search-input')).toHaveCount(0)
  await expect(page.getByTestId('filter-chip')).toHaveCount(0)
  await expect(page.getByTestId('sidebar-jira-filters')).toHaveCount(0)
  if (opts.wizardEscape) {
    await expect(wizard.getByRole('button', { name: en['onboarding.openSettings'] })).toBeVisible()
  }
  await expect(page.getByRole('button', { name: en['sidebar.settings'], exact: true })).toBeVisible()
}

/** Drive the three required steps; leaves the wizard on the optional step 4. */
async function runRequiredSteps(page: Page): Promise<void> {
  const wizard = page.getByTestId('onboarding')
  await expect(wizard).toBeVisible({ timeout: 30_000 })
  await expect(wizard.getByText('Step 1 of 4 · Connect')).toBeVisible()
  // GDK-1345: nothing is chosen at first; the Jira form (and its Open
  // settings door) follows the choice.
  await wizard.getByTestId('onboarding-source-jira').click()
  // The escape hatch out of a wizard that blocks the app. polish.spec.ts held
  // its only assertion until GDK-289 deleted that first-run duplicate, so it is
  // asserted here instead — on the step-1 render every onboarding test already
  // performs, which costs no extra page load.
  await expect(wizard.getByRole('button', { name: 'Open settings' })).toBeVisible()
  await assertOnboardingChromeQuiet(page, { wizardEscape: true })
  await expect(wizard).toHaveAttribute('data-onboarding-reason', 'no-credential')
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
    await expect(wizard).toHaveAttribute('data-onboarding-reason', 'hold')
    // F7: sidebar must not paint a zero-count / Syncing… line next to 519.
    await expect(page.getByTestId('sidebar-sync-now')).toHaveCount(0)
    await expect(page.getByText(/^0 issues/)).toHaveCount(0)
    await expect(page.getByText(en['sync.busy'])).toHaveCount(0)
    await assertOnboardingChromeQuiet(page, { wizardEscape: false })
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
    // while the wizard keeps the pane. The sidebar count is hidden during
    // hold (GDK-299 F7) so we wait on the resync itself, not on a
    // contradictory "1 issues" next to "Mirrored 519".
    const resync = page.waitForResponse(
      (res) => /\/api\/v1\/issues\/(bootstrap|delta)\//.test(res.url()) && res.ok(),
    )
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await resync
    await expect(page.getByTestId('onboarding-agent')).toBeVisible()

    // Completing the step enters the app.
    await page.getByTestId('onboarding-finish').click()
    await expect(page.getByTestId('onboarding')).toBeHidden()
    await expect(page.getByText('Ship the first mirrored issue')).toBeVisible()

    // First run (no saved view yet) lands on the epic breakdown, not a bare
    // all-open replica (GDK-100). The view key is in the hash.
    await expect(page).toHaveURL(/g=epic/)

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

  // The CLI and the settings screen have always read an empty project list as
  // "every project this account can see". Only the wizard called zero a
  // mistake, so the one path a first-time user is actually on demanded a
  // decision the product does not require (GDK-99).
  test('picking no projects is a choice: the sync starts and mirrors everything', async ({
    page,
  }) => {
    const flow = await mockWizard(page)

    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    await wizard.getByTestId('onboarding-source-jira').click()
    await wizard.locator('input[name="site"]').fill('https://example.atlassian.net')
    await wizard.locator('input[name="email"]').fill('dana@example.com')
    await wizard.locator('input[name="token"]').fill('super-secret-token')
    await wizard.getByRole('button', { name: 'Connect', exact: true }).click()

    await expect(page.getByTestId('onboarding-projects')).toBeVisible()
    // Deliberately touch nothing — and press the affordance that says so.
    await wizard.getByRole('button', { name: 'Select none' }).click()
    await expect(wizard.getByText('0 selected')).toBeVisible()

    const start = wizard.getByRole('button', { name: 'Start first sync' })
    await expect(start).toBeEnabled()
    await start.click()

    // The empty list has to reach the server as an empty list: a client that
    // "helpfully" substituted every visible key would pin today's catalogue
    // into the config, and a project created next week would never sync.
    await expect(page.getByTestId('onboarding-agent')).toBeVisible({ timeout: 20_000 })
    expect(flow.savedProjects).toEqual([])
  })

  // Jira answers every bad credential with the same 401, and only one of the
  // traps is recognisable from the pasted token — the ATCTT prefix of an org
  // key. So the server sends two codes and the wizard says two different
  // things (GDK-69); asserting only one of them would let the other rot.
  for (const tc of [
    {
      name: 'a scoped or mistyped token gets the check the user can run',
      code: 'credential_rejected',
      token: 'wrong',
      says: 'Scoped tokens',
    },
    {
      name: 'an org key is named outright, because its prefix gives it away',
      code: 'credential_rejected_org_key',
      token: `ATCTT${'x'.repeat(30)}`,
      says: 'Org API keys',
    },
  ]) {
    test(`a rejected credential stays on step 1 — ${tc.name}`, async ({ page }) => {
      await mockFirstRun(page)
      await page.route(`${API}onboarding/connect/`, (route) =>
        route.fulfill({ status: 401, json: { error: tc.code } }),
      )

      await forceLocale(page, 'en')
      await page.goto('/')

      const wizard = page.getByTestId('onboarding')
      await expect(wizard).toBeVisible({ timeout: 30_000 })
      await wizard.getByTestId('onboarding-source-jira').click()
      await wizard.locator('input[name="site"]').fill('https://example.atlassian.net')
      await wizard.locator('input[name="email"]').fill('dana@example.com')
      await wizard.locator('input[name="token"]').fill(tc.token)
      await wizard.getByRole('button', { name: 'Connect', exact: true }).click()

      const err = page.getByTestId('onboarding-error')
      await expect(err).toContainText('Jira rejected')
      await expect(err).toContainText(tc.says)
      await expect(page.getByTestId('onboarding-connect')).toBeVisible()
    })
  }

  /*
   * GDK-71: on the desktop surface the token page opens in the in-app pane
   * and the paste field takes focus. Serve keeps target="_blank". The
   * existing tests above must not change — they are the non-desktop half.
   */
  const TOKEN_PAGE = 'https://id.atlassian.com/manage-profile/security/api-tokens'

  async function bootDesktopOnboarding(page: Page): Promise<BrowseMock> {
    await page.setViewportSize({ width: 1680, height: 1000 })
    await mockFirstRun(page)
    // Last-registered config.json route wins: empty first-run + desktop.
    await page.route('**/config.json', (route) =>
      route.fulfill({
        json: {
          apiBase: '/api/v1/issues/',
          authBase: '/api/v1/auth/',
          jiraBaseUrl: '',
          projects: [],
          features: {},
          desktop: true,
        },
      }),
    )
    const mock = await mockBrowseRoutes(page)
    await forceLocale(page, 'en')
    await page.goto('/')
    return mock
  }

  test('on the desktop surface the token link opens the in-app pane and focuses the paste field', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktopOnboarding(page)

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    // GDK-1345: the credential form follows the Jira choice.
    await wizard.getByTestId('onboarding-source-jira').click()

    const tokenLink = wizard.getByRole('link', { name: en['onboarding.tokenLink'] })
    await expect(tokenLink).toHaveAttribute('href', TOKEN_PAGE)
    await tokenLink.click()

    const pane = page.getByTestId('browse-pane')
    await expect(pane).toBeVisible()
    await expect(page.getByTestId('browse-url')).toHaveText(TOKEN_PAGE)
    expect(mock.tabs().map((t) => t.url)).toEqual([TOKEN_PAGE])
    expect(mock.opened, 'must not fall through to the system browser').toEqual([])
    await expect(wizard.locator('input[name="token"]')).toBeFocused()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('on the desktop surface the hint line keeps a system-browser escape hatch', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktopOnboarding(page)

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    // GDK-1345: the credential form follows the Jira choice.
    await wizard.getByTestId('onboarding-source-jira').click()

    const escape = wizard.getByRole('link', { name: en['browse.openExternal'] })
    await expect(escape).toHaveAttribute('href', TOKEN_PAGE)
    await escape.click()

    await expect.poll(() => mock.opened).toEqual([TOKEN_PAGE])
    expect(mock.tabs()).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('under serve the token link stays a new-tab URL', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockFirstRun(page)
    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    // GDK-1345: the credential form follows the Jira choice.
    await wizard.getByTestId('onboarding-source-jira').click()

    const tokenLink = wizard.getByRole('link', { name: en['onboarding.tokenLink'] })
    await expect(tokenLink).toHaveAttribute('href', TOKEN_PAGE)
    await expect(tokenLink).toHaveAttribute('target', '_blank')
    await expect(wizard.getByRole('link', { name: en['browse.openExternal'] })).toHaveCount(0)
    await expect(page.getByTestId('browse-pane')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
