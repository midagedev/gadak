import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * List bulk writes (GDK-331 priority, GDK-332 assignee search).
 *
 * Same write-through grammar as write-through.spec.ts: the fixture token is
 * fake, so the test fulfils GET catalog / GET users / PUT the way write.go
 * answers after a healthy Jira + mirror refresh. Server truth is not the
 * optimistic patch, so a UI that kept only its local guess fails.
 */

const KEY = 'NMB-110'
const PRIORITIES = [
  { id: '1', name: 'Highest' },
  { id: '2', name: 'High' },
  { id: '3', name: 'Medium' },
  { id: '4', name: 'Low' },
  { id: '5', name: 'Lowest' },
] as const
const PICK = PRIORITIES[1]
const SERVER_PRIORITY = 'WT-MIRROR-PRIORITY'
const SEARCH_Q = 'zzq'
const OUTSIDER = {
  account_id: 'wt-e2e-outsider',
  display_name: 'WT Outsider',
  email: 'outsider@example.test',
  avatar_url: '',
  active: true,
} as const
const SERVER_ASSIGNEE = 'WT-MIRROR-ASSIGNEE'

type IssueRow = Record<string, unknown> & { issue_key: string }

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

async function ignoreFixtureDeltas(page: Page): Promise<void> {
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { upserted?: IssueRow[] }
    body.upserted = (body.upserted ?? []).filter((it) => it.issue_key !== KEY)
    await route.fulfill({ response, json: body })
  })
}

async function captureIssue(page: Page): Promise<IssueRow> {
  const held: { issue: IssueRow | null } = { issue: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    held.issue = body.issues.find((it) => it.issue_key === KEY) ?? null
    await route.fulfill({ response, json: body })
  })
  await ignoreFixtureDeltas(page)
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include NMB-110').toBeTruthy()
  return held.issue!
}

async function selectListRow(page: Page, key: string): Promise<void> {
  const input = searchInput(page)
  await input.fill(key)
  const row = page.locator(`[data-testid="issue-list-scroller"] [data-issue-key="${key}"]`)
  await expect(row).toBeVisible()
  await row.locator('button[aria-pressed]').click()
  await expect(page.getByTestId('bulk-bar')).toBeVisible()
}

test.describe('list bulk edits', () => {
  // GDK-602: a consistency sweep once broke the Deselect <button> tag so its
  // attributes rendered as textContent — svelte-check stays silent on that,
  // and only Esc/palette paths were covered. The exact accessible name plus a
  // working click is what catches the class.
  test('the bulk bar Deselect button is a button, and clicking it clears the selection', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await captureIssue(page)
    await selectListRow(page, KEY)

    const bar = page.getByTestId('bulk-bar')
    const deselect = bar.getByRole('button', { name: 'Deselect', exact: true })
    await expect(deselect).toBeVisible()
    await expect(deselect).toBeEnabled()
    await deselect.click()
    await expect(bar).not.toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('p opens the catalog priority menu; picking PUTs the id and updates the row chip', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.route('**/priorities/', (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      return fulfillJSON(route, { priorities: [...PRIORITIES] })
    })
    const issue = await captureIssue(page)

    let posted: { priority_id?: string | null } | null = null
    await page.route(`**/api/v1/issues/${KEY}/priority/`, async (route) => {
      if (route.request().method() !== 'PUT') return route.continue()
      posted = route.request().postDataJSON() as { priority_id?: string | null }
      await fulfillJSON(route, {
        issue: {
          ...issue,
          priority: SERVER_PRIORITY,
          priority_id: PICK.id,
          priority_rank: 2,
        },
      })
    })

    await selectListRow(page, KEY)
    await page.keyboard.press('p')
    const menu = page.getByTestId('bulk-priority-menu')
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('option', { name: 'None' })).toBeVisible()
    await menu.getByRole('option', { name: PICK.name, exact: true }).click()

    await expect.poll(() => posted?.priority_id).toBe(PICK.id)
    expect(posted).not.toHaveProperty('priority')
    const row = page.locator(`[data-testid="issue-list-scroller"] [data-issue-key="${KEY}"]`)
    await expect(row.getByLabel(`Priority ${SERVER_PRIORITY}`)).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a menu finds a user outside members via server search and assigns them', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)

    let searched: string | null = null
    await page.route('**/api/v1/issues/users/**', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      const url = new URL(route.request().url())
      searched = url.searchParams.get('q')
      await fulfillJSON(route, { users: [OUTSIDER] })
    })

    let posted: { account_id?: string | null } | null = null
    await page.route(`**/api/v1/issues/${KEY}/assignee/`, async (route) => {
      if (route.request().method() !== 'PUT') return route.continue()
      posted = route.request().postDataJSON() as { account_id?: string | null }
      await fulfillJSON(route, {
        issue: {
          ...issue,
          assignee: SERVER_ASSIGNEE,
          assignee_id: OUTSIDER.account_id,
          assignee_email: OUTSIDER.email,
        },
      })
    })

    await selectListRow(page, KEY)
    await page.keyboard.press('a')
    const menu = page.getByTestId('bulk-assignee-menu')
    await expect(menu).toBeVisible()
    await expect(menu).toHaveAttribute('data-cand-source', 'local')

    await menu.getByPlaceholder('Search name or email').fill(SEARCH_Q)
    await expect(menu).toHaveAttribute('data-cand-source', 'search')
    const outsider = menu.locator('[data-cand-origin="server"]', { hasText: OUTSIDER.display_name })
    await expect(outsider).toBeVisible()
    await expect.poll(() => searched).toBe(SEARCH_Q)
    await outsider.click()

    await expect.poll(() => posted?.account_id).toBe(OUTSIDER.account_id)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
