import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/**
 * GDK-254: create dialog degrades when create-meta/fields/ is missing,
 * and still submits when it names extra required fields (warning, not a block).
 */

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

const PRIORITY_CATALOG = [{ id: '3', name: '보통' }]

function isCreateMetaFields(url: string): boolean {
  return url.includes('/create-meta/fields')
}

async function stubWriteMeta(page: Page): Promise<void> {
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
    if (isCreateMetaFields(route.request().url())) return route.continue()
    await fulfillJSON(route, { projects: CREATE_PROJECTS })
  })
  await page.route('**/priorities/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await fulfillJSON(route, { priorities: PRIORITY_CATALOG })
  })
}

async function boot(page: Page): Promise<IssueRow> {
  await stubWriteMeta(page)
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

test.describe('create fields (GDK-254)', () => {
  test('submit still works when create-meta/fields/ is unavailable', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const template = await boot(page)

    await page.route(
      (url) => url.pathname.includes('/create-meta/fields'),
      async (route) => {
        if (route.request().method() !== 'GET') return route.continue()
        await fulfillJSON(route, { error: 'not_found' }, 404)
      },
    )

    let posted: CreateBody | null = null
    // The mocked create answers with a key the fixture holds: the dialog
    // opens the created issue afterwards (selection.select → detail GET),
    // and a key the server does not know 404s in the console — a race the
    // CI runner lost twice (GDK-1295). Write-through puts the real issue in
    // the mirror before it answers, so an existing key is the honest shape.
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, {
        issue: {
          ...template,
          issue_key: 'NMB-1',
          summary: posted.summary,
          issue_type: 'Task',
          source_project: 'NMB',
        },
      })
    })

    const dialog = await openNewIssue(page)
    await expect(dialog.getByTestId('new-issue-required-warn')).toHaveCount(0)
    await expect(dialog.getByTestId('new-issue-duedate')).toBeVisible()
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-254 no fields meta')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    await expect.poll(() => posted).not.toBeNull()
    expect(posted!.summary).toBe('gdk-254 no fields meta')
    await expect(dialog).toHaveCount(0)

    expect(
      errors.filter((e) => !e.includes('404')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('warns extra required fields but does not block create', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const template = await boot(page)

    await page.route(
      (url) => url.pathname.includes('/create-meta/fields'),
      async (route) => {
        if (route.request().method() !== 'GET') return route.continue()
        await fulfillJSON(route, {
          fields: [
            { field_id: 'issuetype', name: 'Issue Type', required: true, has_default: false, type: 'issuetype' },
            { field_id: 'project', name: 'Project', required: true, has_default: false, type: 'project' },
            { field_id: 'reporter', name: 'Reporter', required: true, has_default: true, type: 'user' },
            { field_id: 'summary', name: 'Summary', required: true, has_default: false, type: 'string' },
            { field_id: 'customfield_10050', name: 'Sprint', required: true, has_default: false, type: 'array' },
          ],
        })
      },
    )

    let posted: CreateBody | null = null
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, {
        issue: {
          ...template,
          issue_key: 'NMB-10',
          summary: posted.summary,
          issue_type: 'Task',
          source_project: 'NMB',
        },
      })
    })

    const dialog = await openNewIssue(page)
    const warn = dialog.getByTestId('new-issue-required-warn')
    await expect(warn).toBeVisible()
    await expect(warn).toContainText('Sprint')
    await expect(warn).not.toContainText('Reporter')
    await expect(dialog.getByRole('button', { name: en['common.create'] })).toBeEnabled()

    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-254 warn still creates')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    await expect.poll(() => posted).not.toBeNull()
    expect(posted!.summary).toBe('gdk-254 warn still creates')

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
