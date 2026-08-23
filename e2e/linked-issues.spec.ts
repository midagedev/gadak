import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * Issue-link write-through from the detail panel (GDK-85).
 *
 * The fixture credential is fake — a real POST would 502 at Jira — so this
 * fulfils GET linktypes + POST link the way write.go does after a healthy
 * origin + mirror refresh. The list is the GET detail body after the write
 * (not the typed key), matching e2e/write-through.spec.ts.
 *
 * Delete is not on origin.IssueLinker (IssueLinkTypes + LinkIssues only);
 * this spec covers add → appears.
 */

const KEY = 'NMB-110'
const TYPED_KEY = 'NMA-9'
const SERVER_KEY = 'NMS-1'
const CATALOG = {
  link_types: [
    { id: '10000', name: 'Blocks', outward: 'blocks', inward: 'is blocked by' },
  ],
} as const

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

async function openIssue(page: Page) {
  const input = searchInput(page)
  await input.fill(KEY)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: KEY })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('linked issues', () => {
  test('add: request carries type+key; list shows the refreshed link, not the typed guess', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)

    await page.route(`**/api/v1/issues/${KEY}/linktypes/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, CATALOG)
    })

    let created = false
    let posted: { type?: string; key?: string } | null = null
    await page.route(`**/api/v1/issues/${KEY}/link/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as { type?: string; key?: string }
      created = true
      await fulfillJSON(route, { issue })
    })

    await page.route(`**/api/v1/issues/${KEY}/detail/`, async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        linked_issues?: { key: string; type: string; direction: string; summary: string | null }[]
      }
      if (created) {
        body.linked_issues = [
          ...(body.linked_issues ?? []).filter((l) => l.key !== SERVER_KEY && l.key !== TYPED_KEY),
          {
            key: SERVER_KEY,
            type: 'Blocks',
            direction: 'blocks',
            summary: 'wt-e2e mirrored link',
          },
        ]
      }
      await route.fulfill({ response, json: body })
    })

    const panel = await openIssue(page)
    const links = panel.getByTestId('linked-issues')
    await expect(links).toBeVisible()
    const form = panel.getByTestId('linked-issues-add')
    await expect(form).toBeVisible()

    const typeSelect = form.getByTestId('linked-issues-type')
    await expect(typeSelect).toBeEnabled()
    await typeSelect.selectOption('blocks')
    await form.getByTestId('linked-issues-key').fill(TYPED_KEY)
    await form.getByTestId('linked-issues-submit').click()

    await expect.poll(() => posted?.type).toBe('blocks')
    await expect.poll(() => posted?.key).toBe(TYPED_KEY)
    await expect(links.getByText(SERVER_KEY)).toBeVisible()
    await expect(links.getByText(TYPED_KEY)).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
