import { type Page, type Route } from '@playwright/test'
import { expect, test } from './helpers'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/**
 * GDK-254 / GDK-533: create dialog vs. create-meta/fields/.
 *  - The endpoint missing (404, older server) is terminal-but-quiet: no
 *    warning, no blocking — create submits exactly as before.
 *  - Extra required fields split by whether this dialog can fill them
 *    (GDK-533): fillable ones render editors and their raw values ride the
 *    create POST as custom_fields; unfillable ones are named in the footer
 *    sentence and keep Create disabled — a submit the origin is known to
 *    reject must never be sent.
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

/** Capture the create POST; answer with a key the fixture holds so the
 *  post-create detail GET does not 404 (GDK-1295 note in test 1). */
async function captureCreate(page: Page, template: IssueRow, key: string) {
  let posted: CreateBody | null = null
  await page.route('**/api/v1/issues/create/', async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    posted = route.request().postDataJSON() as CreateBody
    await fulfillJSON(route, {
      issue: {
        ...template,
        issue_key: key,
        summary: posted.summary,
        issue_type: 'Task',
        source_project: 'NMB',
      },
    })
  })
  return () => posted
}

test.describe('create fields (GDK-254/GDK-533)', () => {
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

    const getPosted = await captureCreate(page, template, 'NMB-1')

    const dialog = await openNewIssue(page)
    await expect(dialog.getByTestId('new-issue-required-warn')).toHaveCount(0)
    await expect(dialog.getByTestId('new-issue-duedate')).toBeVisible()
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-254 no fields meta')
    await dialog.getByRole('button', { name: en['common.create'] }).click()

    await expect.poll(() => getPosted()).not.toBeNull()
    const posted = getPosted()!
    expect(posted.summary).toBe('gdk-254 no fields meta')
    // No fields meta → no custom_fields key at all; the request is
    // byte-identical to a pre-GDK-533 create.
    expect('custom_fields' in posted).toBe(false)
    await expect(dialog).toHaveCount(0)

    expect(
      errors.filter((e) => !e.includes('404')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('unfillable required fields keep Create disabled; fillable ones get editors', async ({ page }) => {
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
            // Fillable (kind+options): renders an editor, stays out of the sentence.
            {
              field_id: 'customfield_10092', name: 'Solution', required: true, has_default: false,
              type: 'option', kind: 'option', options: [{ id: '10160', value: 'Fixed' }],
            },
            // Unfillable (no kind on an older/mapped-out server): the sentence.
            { field_id: 'customfield_10050', name: 'Sprint', required: true, has_default: false, type: 'array' },
          ],
        })
      },
    )

    const getPosted = await captureCreate(page, template, 'NMB-10')

    const dialog = await openNewIssue(page)

    // The fillable one renders an editor instead of being named.
    const solution = dialog.getByTestId('create-custom-customfield_10092')
    await expect(solution).toBeVisible()
    await expect(solution.locator('option', { hasText: 'Fixed' })).toHaveCount(1)

    // The sentence names only what cannot be filled here.
    const warn = dialog.getByTestId('new-issue-required-warn')
    await expect(warn).toBeVisible()
    await expect(warn).toContainText('Sprint')
    await expect(warn).not.toContainText('Solution')
    await expect(warn).not.toContainText('Reporter')

    // Filling everything fillable (and the title) is not enough: Sprint has
    // no editor, so the doomed submit never goes out.
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-533 never sent')
    await solution.selectOption('10160')
    const create = dialog.getByRole('button', { name: en['common.create'] })
    await expect(create).toBeDisabled()
    await expect.poll(() => getPosted(), { timeout: 1500 }).toBeNull()

    expect(
      errors.filter((e) => !e.includes('404')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('filled custom fields ride the create POST as raw editor values', async ({ page }) => {
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
            {
              field_id: 'customfield_10092', name: 'Solution', required: true, has_default: false,
              type: 'option', kind: 'option',
              options: [
                { id: '10160', value: 'Fixed' },
                { id: '10161', value: "Won't Fix" },
              ],
            },
            { field_id: 'customfield_10030', name: 'Customer', required: true, has_default: false, type: 'string', kind: 'text' },
            { field_id: 'customfield_10040', name: 'Renewal', required: true, has_default: false, type: 'date', kind: 'date' },
            { field_id: 'customfield_10035', name: 'Score', required: true, has_default: false, type: 'number', kind: 'number' },
          ],
        })
      },
    )

    const getPosted = await captureCreate(page, template, 'NMB-11')

    const dialog = await openNewIssue(page)

    // No unfillable field → no footer sentence, only editors.
    await expect(dialog.getByTestId('new-issue-required-warn')).toHaveCount(0)
    const solution = dialog.getByTestId('create-custom-customfield_10092')
    const customer = dialog.getByTestId('create-custom-customfield_10030')
    const renewal = dialog.getByTestId('create-custom-customfield_10040')
    const score = dialog.getByTestId('create-custom-customfield_10035')
    await expect(solution).toBeVisible()
    await expect(customer).toBeVisible()
    await expect(renewal).toBeVisible()
    await expect(score).toBeVisible()
    // The option editor leads with the placeholder, not a value.
    await expect(solution).toHaveValue('')

    const create = dialog.getByRole('button', { name: en['common.create'] })
    await dialog.getByPlaceholder(en['write.issueTitle']).fill('gdk-533 fill path')
    // Empty fillable required fields block just like unfillable ones.
    await expect(create).toBeDisabled()

    await solution.selectOption('10160')
    await customer.fill('Acme')
    await expect(create).toBeDisabled() // date + score still missing
    await renewal.fill('2026-10-01')
    await score.fill('42')
    await expect(create).toBeEnabled()

    await create.click()
    await expect.poll(() => getPosted()).not.toBeNull()
    // Raw editor values, keyed by field_id: the option sends its id, not the
    // label — the server resolves kind and wraps (same encoder as the CLI).
    expect(getPosted()!.custom_fields).toEqual({
      customfield_10092: '10160',
      customfield_10030: 'Acme',
      customfield_10040: '2026-10-01',
      customfield_10035: '42',
    })

    expect(
      errors.filter((e) => !e.includes('404')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
