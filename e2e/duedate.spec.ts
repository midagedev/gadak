import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/**
 * GDK-223 / GDK-251: create can send duedate, and Jira field rejections
 * (jira_errors + Message()) render in the create dialog and write toasts.
 */

const KEY = 'NMB-110'
const TRANSITION = {
  id: '21',
  name: 'Start work',
  to_status: 'In Progress',
  to_category: 'indeterminate',
} as const
const CREATE_PROJECTS = [
  {
    key: 'NMB',
    name: 'Numbers',
    issue_types: [
      { id: '10001', name: 'Task' },
      { id: '10004', name: 'Bug' },
    ],
  },
]

type CreateBody = Record<string, unknown>
type IssueRow = Record<string, unknown> & { issue_key: string }

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Fixture Jira is fake — create-meta never returns. Seed local write-meta. */
const PRIORITY_CATALOG = [
  { id: '1', name: '가장 높음' },
  { id: '2', name: '높음' },
  { id: '3', name: '보통' },
]

async function stubCreateMeta(page: Page): Promise<void> {
  await page.route('**/api/v1/issues/meta/write/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await fulfillJSON(route, {
      transitions: {},
      create_meta: { projects: CREATE_PROJECTS },
      updated_at: '2026-08-18T00:00:00.000Z',
    })
  })
  await page.route('**/api/v1/issues/create-meta/**', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    if (route.request().url().includes('/create-meta/fields')) {
      // Empty success: the dialog degrades (no stars, no warning) without a
      // console 404 the attachConsoleErrors assertions would fail on.
      await fulfillJSON(route, { fields: [] })
      return
    }
    await fulfillJSON(route, { projects: CREATE_PROJECTS })
  })
  await page.route('**/priorities/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await fulfillJSON(route, { priorities: PRIORITY_CATALOG })
  })
}

async function bootWithCreateMeta(page: Page): Promise<IssueRow> {
  await stubCreateMeta(page)
  const held: { issue: IssueRow | null } = { issue: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    held.issue = body.issues[0] ?? null
    await route.fulfill({ response, json: body })
  })
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include an issue').toBeTruthy()
  return held.issue!
}

async function openNewIssue(page: Page) {
  await page.getByRole('button', { name: en['write.newIssue'], exact: true }).click()
  const dialog = page.getByRole('dialog', { name: en['write.newIssue'] })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByPlaceholder(en['write.issueTitle'])).toBeVisible()
  return dialog
}

test.describe('duedate write + jira_errors', () => {
  test('new-issue dialog POSTs YYYY-MM-DD duedate when set', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const template = await bootWithCreateMeta(page)

    let posted: CreateBody | null = null
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, {
        issue: {
          ...template,
          issue_key: 'GDK-223',
          summary: posted.summary,
          issue_type: 'Task',
          source_project: 'NMB',
        },
        resolved: {
          project: { value: 'NMB', source: 'flag' },
          issue_type: { value: '10004', source: 'flag' },
        },
      })
    })

    const dialog = await openNewIssue(page)
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-223 due set')
    await dialog.getByTestId('new-issue-duedate').fill('2026-09-01')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    await expect.poll(() => posted).not.toBeNull()
    expect(posted!.duedate).toBe('2026-09-01')
    expect(posted!.summary).toBe('gdk-223 due set')
    expect(JSON.stringify(posted)).not.toMatch(/ATATT|Bearer |api_token/i)

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('new-issue dialog POSTs catalog priority_id, not the display name', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const template = await bootWithCreateMeta(page)

    let posted: CreateBody | null = null
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, {
        issue: {
          ...template,
          issue_key: 'GDK-248',
          summary: posted.summary,
          issue_type: 'Task',
          source_project: 'NMB',
        },
      })
    })

    const dialog = await openNewIssue(page)
    const select = dialog.getByTestId('new-issue-priority')
    const named = select.locator('option[value="2"]')
    await expect(named).toHaveText('높음')
    await expect(select.locator('option[value="1"]')).toHaveText('가장 높음')
    await expect(select.locator('option', { hasText: /^Highest$/ })).toHaveCount(0)

    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-248 priority id')
    await select.selectOption('2')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    await expect.poll(() => posted).not.toBeNull()
    expect(posted!.priority_id).toBe('2')
    expect(posted).not.toHaveProperty('priority')
    expect(posted!.summary).toBe('gdk-248 priority id')

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('empty duedate is omitted from the create payload', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const template = await bootWithCreateMeta(page)

    let posted: CreateBody | null = null
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, {
        issue: {
          ...template,
          issue_key: 'GDK-223b',
          summary: posted.summary,
          issue_type: 'Task',
          source_project: 'NMB',
        },
      })
    })

    const dialog = await openNewIssue(page)
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-223 due omitted')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    await expect.poll(() => posted).not.toBeNull()
    expect(posted).not.toHaveProperty('duedate')
    expect(posted!.summary).toBe('gdk-223 due omitted')

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('create dialog shows Jira Message() and jira_errors as given', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await stubCreateMeta(page)
    await gotoApp(page)

    const jiraMessage = "We can't create this issue for you right now."
    const dueErr = 'Due date is required.'
    const sprintErr = 'Sprint is required.'

    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      await fulfillJSON(
        route,
        {
          error: jiraMessage,
          jira_errors: { duedate: dueErr, customfield_10050: sprintErr },
        },
        400,
      )
    })

    const dialog = await openNewIssue(page)
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-223 blocked')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    const shown = dialog.getByTestId('new-issue-error')
    await expect(shown).toBeVisible()
    await expect(shown).toContainText(jiraMessage)
    await expect(shown).toContainText(dueErr)
    await expect(shown).toContainText(sprintErr)
    await expect(shown).not.toContainText(/ATATT|Bearer |api_token|atlassian\.net/i)
    await expect(dialog).toBeVisible()

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('write toast shows jira_errors from a failed transition', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill(KEY)
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: KEY })
      .first()
      .click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()

    const chip = panel.getByTestId('status-transition')
    await expect(chip).toBeVisible()

    await page.route(`**/api/v1/issues/${KEY}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })

    const jiraMessage = 'Field is required'
    const dueErr = 'Due date is required.'
    await page.route(`**/api/v1/issues/${KEY}/transition/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      await fulfillJSON(route, { error: jiraMessage, jira_errors: { duedate: dueErr } }, 400)
    })

    await chip.click()
    await page.getByRole('option', { name: TRANSITION.name }).click()

    const toast = page.getByTestId('toast').and(page.getByRole('alert'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(jiraMessage)
    await expect(toast).toContainText(dueErr)
    await expect(toast).not.toContainText(/ATATT|Bearer |api_token|atlassian\.net/i)

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})

/*
 * GDK-302: the create dialog must not spin on Loading when write-meta has
 * already settled empty. The e2e fixture stores a fake token (so
 * ensureWritable lets the dialog open) and write-meta degrades to empty
 * projects; GET create-meta/ talks to nimbus.example.com and never returns.
 *
 * Evidence is the e2e fixture, not a real Jira site.
 */
test.describe('create dialog unwritable (GDK-302)', () => {
  test('empty write-meta reaches a terminal message and does not GET create-meta', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const createMetaGets: string[] = []
    // Settled empty — the audit's condition. Do not let the fixture's fake
    // origin hang write-meta; that would keep the dialog in a real load.
    await page.route('**/api/v1/issues/meta/write/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, {
        transitions: {},
        create_meta: { projects: [] },
        updated_at: null,
      })
    })
    await page.route('**/api/v1/issues/create-meta/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      createMetaGets.push(route.request().url())
      // Park: a late body would hide a spinner. Wait on dialog state, not time.
      await new Promise<never>(() => {})
    })
    await gotoApp(page)
    await page.getByRole('button', { name: en['write.newIssue'], exact: true }).click()
    const dialog = page.getByRole('dialog', { name: en['write.newIssue'] })
    await expect(dialog).toBeVisible()
    // Fixture credential is present; write-meta settled empty → meta-failed.
    await expect(dialog.getByText(en['write.metaFailed'])).toBeVisible()
    await expect(dialog.getByText(en['common.loading'])).toHaveCount(0)
    await expect(dialog).toHaveAttribute('data-write-state', 'meta-failed')
    expect(
      createMetaGets,
      'create-meta GET must not run when write-meta already settled empty',
    ).toEqual([])
    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
