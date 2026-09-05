import { expect, test, type Page, type Route } from '@playwright/test'
import { apiURL, attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

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
 *
 * GitHub discussion #80 (GDK-1292 / GDK-1293): the label of a linked row is
 * the catalog phrase for its direction ("is blocked by"), never the mirror's
 * raw `inward`/`outward` token; and following a link is a navigation, so the
 * back button returns to the issue it was followed from.
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

// A fixture pair linked both ways: NMA-104 blocks NMB-104 (outward), so
// NMB-104's row for NMA-104 is the inward side.
const BLOCKED = 'NMB-104'
const BLOCKER = 'NMA-104'

async function gotoIssue(page: Page, key: string) {
  await forceLocale(page, 'en')
  await page.goto(`/#/?issue=${key}`)
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toHaveClass(/is-open/, { timeout: 30_000 })
  await expect(panel.getByRole('link', { name: key, exact: true })).toBeVisible()
  return panel
}

test.describe('linked issues', () => {
  test('label is the catalog phrase for the direction, not the raw token (GDK-1293)', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.route(`**/api/v1/issues/${BLOCKED}/linktypes/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, CATALOG)
    })
    const panel = await gotoIssue(page, BLOCKED)
    const links = panel.getByTestId('linked-issues')
    const row = links.getByRole('button').filter({ hasText: BLOCKER })
    await expect(row).toContainText('is blocked by')
    await expect(links.getByText('inward', { exact: true })).toHaveCount(0)
    await expect(links.getByText('outward', { exact: true })).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('without a catalog the label falls back to the type name, never the token', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.route(`**/api/v1/issues/${BLOCKED}/linktypes/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { link_types: [] })
    })
    const panel = await gotoIssue(page, BLOCKED)
    const links = panel.getByTestId('linked-issues')
    const row = links.getByRole('button').filter({ hasText: BLOCKER })
    await expect(row).toContainText('Blocks')
    await expect(links.getByText('inward', { exact: true })).toHaveCount(0)
    await expect(links.getByText('outward', { exact: true })).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the link-type catalog is fetched once per workspace, not once per issue (GDK-1297)', async ({
    page,
  }) => {
    // The catalog is a workspace asset. Refetching it on every detail open
    // emptied the rows first, so each open flashed the bare type name
    // ("Blocks") before the direction phrase ("is blocked by") came back.
    // One fetch for the pair below; the second open reads what the first
    // one already holds and never shows the bare name.
    const errors = attachConsoleErrors(page)
    let fetches = 0
    await page.route('**/api/v1/issues/*/linktypes/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      fetches++
      await fulfillJSON(route, CATALOG)
    })
    const panel = await gotoIssue(page, BLOCKED)
    const links = panel.getByTestId('linked-issues')
    await expect(links.getByRole('button').filter({ hasText: BLOCKER })).toContainText(
      'is blocked by',
    )
    expect(fetches).toBe(1)

    await links.getByRole('button').filter({ hasText: BLOCKER }).click()
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toBeVisible()
    await expect(links.getByRole('button').filter({ hasText: BLOCKED })).toContainText('blocks')
    expect(fetches, 'the second detail open must not refetch the catalog').toBe(1)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('back returns to the issue the link was followed from (GDK-1292)', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.route('**/api/v1/issues/*/linktypes/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, CATALOG)
    })
    const panel = await gotoIssue(page, BLOCKER)

    await panel.getByTestId('linked-issues').getByRole('button').filter({ hasText: BLOCKED }).click()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKED}`))
    await expect(panel.getByRole('link', { name: BLOCKED, exact: true })).toBeVisible()

    // Following a link left an entry, so back reopens the issue it came from.
    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}`), { timeout: 5_000 })
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toBeVisible()

    // Closing is a move too: back after close reopens what was closed.
    await panel.getByTestId('issue-detail-close').click()
    await expect(page).not.toHaveURL(/issue=/)
    await expect(panel).not.toHaveClass(/is-open/)
    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}`), { timeout: 5_000 })
    await expect(panel).toHaveClass(/is-open/)
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an issue opened over a document takes the panel, and back gives the document back', async ({
    page,
    request,
  }) => {
    // The right panel is one union: opening the issue pushes `issue=` while the
    // document's binding drops `doc=` in the same flush. The second write must
    // build on the first (setParams syncs the router on push), or the queued
    // hashchange reads a hash with no `issue=` and closes the panel it just
    // opened — which is what the full suite showed, and this spec alone did not.
    const errors = attachConsoleErrors(page)
    const res = await request.get(apiURL('/api/v1/issues/pages/'))
    const body = (await res.json()) as { pages: { key: string }[] }
    const doc = [...body.pages].sort((a, b) => a.key.localeCompare(b.key))[0]

    await forceLocale(page, 'en')
    await page.goto(`/#/?doc=${encodeURIComponent(doc.key)}`)
    await expect(page.getByTestId('doc-panel')).toBeVisible({ timeout: 30_000 })
    // The document link restores the documents screen in the main column; the
    // issue list comes back through the sidebar, with the panel still open.
    await page.getByRole('button', { name: /All open/ }).click()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page.getByTestId('doc-panel')).toBeVisible()

    // From the list, not the address bar: it is the state moving first that
    // makes the two bindings write in one flush.
    await searchInput(page).fill(BLOCKER)
    await searchInput(page).press('Enter')
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toHaveClass(/is-open/)
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toBeVisible()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}`))
    await expect(page).not.toHaveURL(/doc=/)
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)

    // And the other way round: a document opened over the issue, from the
    // sidebar's recent list.
    await page.getByTestId(`recent-doc-${doc.key}`).getByRole('button').first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page).toHaveURL(/doc=/)
    await expect(page).not.toHaveURL(/issue=/)
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toHaveCount(0)

    // Each open was a move, so back walks them in reverse.
    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}`), { timeout: 5_000 })
    await expect(page).not.toHaveURL(/doc=/)
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toBeVisible()
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)

    await page.goBack()
    await expect(page).toHaveURL(/doc=/, { timeout: 5_000 })
    await expect(page).not.toHaveURL(/issue=/)
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

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
