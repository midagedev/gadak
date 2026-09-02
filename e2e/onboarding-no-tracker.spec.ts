import { test, expect, type Page } from '@playwright/test'
import { en } from '../web/src/lib/i18n/en'
import { attachConsoleErrors, forceLocale } from './helpers'

/*
 * GDK-377: the no-tracker front door. One click under the credential form
 * seeds a local-origin workspace and lands on the first-issue composer — the
 * whole first session with no terminal and no token. The demo server is a
 * connected fixture, so both the empty first-run state and the seeded state
 * are route-mocked: config.json flips to workspaceKind local-origin only after
 * POST onboarding/standalone answers, which is what the real handler swaps
 * in. No request reaches Jira and nothing mutates the fixture's config.
 */

const API = '**/api/v1/issues/'

const EMPTY_BOOTSTRAP = {
  server_time: '2026-08-26T00:00:00.000Z',
  sync_version: 0,
  members: [],
  members_version: '',
  issues: [],
  sync_health: { overall: 'healthy', checked_at: '2026-08-26T00:00:00.000Z', sources: [] },
}

const SEED_RESPONSE = { workspace_kind: 'standalone', default_project: 'STD' }

const STD_CREATE_META = {
  projects: [
    {
      key: 'STD',
      name: 'Standard',
      issue_types: [
        { id: '10001', name: 'Task' },
        { id: '10002', name: 'Bug' },
        { id: '10003', name: 'Story' },
      ],
    },
  ],
}

// Same complete-row discipline as onboarding.spec.ts's MIRRORED_ISSUE: every
// field the client reads is present, since a missing one shows up as a
// console error rather than a failed assertion.
const FIRST_ISSUE = {
  issue_key: 'STD-1',
  summary: 'Ship the first no-tracker issue',
  status: 'To Do',
  status_category: 'new',
  issue_type: 'Task',
  priority: 'Medium',
  priority_rank: 3,
  severity: null,
  assignee: null,
  assignee_email: null,
  reporter: null,
  reporter_email: null,
  labels: [],
  fix_versions: [],
  components: [],
  team_group: null,
  epic_key: null,
  parent_key: null,
  source_project: 'STD',
  created_at: '2026-08-26T00:00:00.000Z',
  updated_at: '2026-08-26T00:00:00.000Z',
  resolved_at: null,
  status_changed_at: '2026-08-26T00:00:00.000Z',
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

interface Flow {
  /** True once the mocked verb answered — flips config/credential/meta mocks. */
  seeded: boolean
  /** True once the composer submitted — flips the mocked mirror. */
  created: boolean
  localOriginBody: Record<string, unknown> | null
  createBody: Record<string, unknown> | null
}

async function mockNoTracker(page: Page): Promise<Flow> {
  const flow: Flow = { seeded: false, created: false, localOriginBody: null, createBody: null }

  // config.json is the boot document AND the post-seed refetch: the same
  // route answers both, statefully, the way the real config file changes.
  await page.route('**/config.json', (route) =>
    route.fulfill({
      json: {
        apiBase: '/api/v1/issues/',
        authBase: '/api/v1/auth/',
        jiraBaseUrl: '',
        projects: flow.seeded ? ['STD'] : [],
        features: {},
        ...(flow.seeded ? { workspaceKind: 'standalone' } : {}),
      },
    }),
  )
  await page.route('**/api/v1/auth/me/', (route) => route.fulfill({ json: { email: null } }))

  const mirror = () => (flow.created ? [FIRST_ISSUE] : [])
  await page.route(`${API}bootstrap/**`, (route) =>
    route.fulfill({ json: { ...EMPTY_BOOTSTRAP, issues: mirror() } }),
  )
  await page.route(`${API}delta/**`, (route) =>
    route.fulfill({
      json: { ...EMPTY_BOOTSTRAP, issues: mirror(), upserted: mirror(), deleted_keys: [] },
    }),
  )

  await page.route(`${API}onboarding/standalone/`, async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    flow.localOriginBody = route.request().postDataJSON() as Record<string, unknown>
    flow.seeded = true
    await route.fulfill({ json: SEED_RESPONSE })
  })

  // The write gate: closed at boot, open once the workspace is seeded.
  await page.route(`${API}credential/`, (route) =>
    route.fulfill({
      json: {
        configured: flow.seeded,
        jira_email: '',
        display_name: '',
        verified_at: '',
        token_hint: '',
      },
    }),
  )

  // meta/write: empty at boot (no credential); the seeded project after —
  // this is the refetch startLocalOrigin does, without which the composer
  // would open on the boot-cached empty answer.
  await page.route(`${API}meta/write/`, (route) =>
    route.fulfill({
      json: flow.seeded
        ? {
            transitions: {},
            create_meta: STD_CREATE_META,
            updated_at: '2026-08-26T00:00:00.000Z',
          }
        : { transitions: {}, create_meta: { projects: [] }, updated_at: null },
    }),
  )
  await page.route(`${API}priorities/`, (route) => route.fulfill({ json: { priorities: [] } }))
  await page.route((url) => url.pathname.includes('/create-meta/fields'), (route) =>
    route.fulfill({ json: { fields: [] } }),
  )

  await page.route(`${API}create/`, async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    flow.createBody = route.request().postDataJSON() as Record<string, unknown>
    flow.created = true
    await route.fulfill({ json: { issue: FIRST_ISSUE } })
  })

  // Selecting the new row opens the detail pane, which reads this. The demo
  // server behind everything else has no STD-1 — without the mock the 404 is
  // a console error, which this suite treats as a failure.
  await page.route(`${API}STD-1/detail/`, (route) =>
    route.fulfill({
      json: {
        issue_key: 'STD-1',
        development_opinion: '',
        description_adf: null,
        attachments: [],
        comments: [],
        history: [],
        linked_issues: [],
        linked_prs: [],
        qa_context: null,
      },
    }),
  )
  return flow
}

test.describe('no-tracker onboarding (GDK-377)', () => {
  test('the Built-in answer seeds the workspace and lands on the composer', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const flow = await mockNoTracker(page)

    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    await expect(wizard).toHaveAttribute('data-onboarding-reason', 'no-credential')

    // GDK-1287: step 1 opens on the source question with three answers at one
    // layer. GDK-1345: none is preselected — a form open under one card read
    // as that card being required. Choosing Built-in reveals its door.
    const chooser = page.getByTestId('onboarding-source')
    await expect(chooser.getByRole('radio')).toHaveCount(3)
    await expect(chooser.getByRole('radio', { checked: true })).toHaveCount(0)
    await expect(page.getByTestId('onboarding-connect')).toHaveCount(0)
    await expect(page.getByTestId('onboarding-local-origin')).toHaveCount(0)
    await page.getByTestId('onboarding-source-builtin').click()
    const door = page.getByTestId('onboarding-local-origin')
    await expect(door).toBeVisible()
    await expect(page.getByTestId('onboarding-connect')).toHaveCount(0)
    await expect(door.getByRole('button', { name: en['onboarding.localOriginStart'] })).toBeVisible()

    await door.getByRole('button', { name: en['onboarding.localOriginStart'] }).click()

    // The verb fired, and the wizard left — not for step 2, for good: the
    // workspace kind flipped, so the gate's local-origin clause took over.
    await expect
      .poll(() => flow.localOriginBody)
      .toEqual({})
    await expect(wizard).toBeHidden({ timeout: 10_000 })

    // The landing is the first-issue composer, not an empty list.
    const dialog = page.getByTestId('new-issue-dialog')
    await expect(dialog).toBeVisible({ timeout: 10_000 })
    const title = dialog.getByPlaceholder(en['write.issueTitle'])
    await expect(title).toBeVisible({ timeout: 10_000 })
    await title.fill('Ship the first no-tracker issue')
    await dialog.getByRole('button', { name: en['common.create'], exact: true }).click()

    // The composer prefilled the seeded project and its first type — the
    // person typed one sentence and pressed one button.
    await expect.poll(() => flow.createBody).not.toBeNull()
    expect(flow.createBody).toMatchObject({
      project_key: 'STD',
      issue_type: '10001',
      summary: 'Ship the first no-tracker issue',
    })

    await expect(dialog).toBeHidden()
    // And the first issue is in the list — the workspace is alive end to end.
    // Scoped to the scroller: the same sentence also reaches the detail pane
    // and its title editor once the row is selected.
    await expect(
      page.getByTestId('issue-list-scroller').getByText('Ship the first no-tracker issue'),
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('onboarding')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a connected workspace refusal stays on step 1 with the new-workspace sentence', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await mockNoTracker(page)
    await page.route(`${API}onboarding/standalone/`, (route) =>
      route.fulfill({
        status: 409,
        json: { error: 'workspace_connected', site: 'https://example.atlassian.net' },
      }),
    )

    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })

    await page.getByTestId('onboarding-source-builtin').click()
    await page.getByTestId('onboarding-start-local-origin').click()

    const err = page.getByTestId('onboarding-error')
    await expect(err).toContainText('already connected to a Jira site')
    // The wizard is still step 1 — this path neither converts nor exits.
    await expect(page.getByTestId('onboarding-local-origin')).toBeVisible()
    await expect(page.getByTestId('new-issue-dialog')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the Paired answer opens Settings on the Workspaces tab (GDK-1287)', async ({ page }) => {
    // No route binds an unconfigured workspace to a remote gadak, so the
    // paired door is the existing Settings → Workspaces pairing form.
    const errors = attachConsoleErrors(page)
    await mockNoTracker(page)
    await forceLocale(page, 'en')
    await page.goto('/')

    const wizard = page.getByTestId('onboarding')
    await expect(wizard).toBeVisible({ timeout: 30_000 })
    await page.getByTestId('onboarding-source-paired').click()
    await expect(page.getByTestId('onboarding-connect')).toHaveCount(0)
    await page.getByTestId('onboarding-paired-open').click()

    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await expect(dialog.getByTestId('workspaces-tab')).toBeVisible()
    await expect(dialog.getByTestId('workspaces-mode-paired')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
