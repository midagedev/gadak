import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * Successful write-through (and one failure) against the demo fixture.
 *
 * The fixture credential is fake — a real POST/PUT would 409/502 at Jira —
 * so these tests fulfil the write endpoints the way write.go does after a
 * healthy Jira + mirror refresh: `{"issue": <IssueLite>}` (+ `comment` on
 * POST comment/). The Go suite already covers "Jira first, then SyncIssue";
 * this file covers the journey the user performs.
 *
 * Server truth is deliberately not the optimistic patch (a distinct status /
 * assignee / comment body) so a UI that kept only its local guess fails.
 */

const KEY = 'NMB-110'
const TRANSITION = {
  id: '21',
  name: 'Start work',
  to_status: 'In Progress',
  to_category: 'indeterminate',
} as const
const SERVER_STATUS = 'WT-MIRROR-STATUS'
const SERVER_ASSIGNEE = 'WT-MIRROR-ASSIGNEE'
const TYPED_COMMENT = 'wt-e2e typed comment'
const SERVER_COMMENT = 'wt-e2e mirrored comment'

type IssueRow = Record<string, unknown> & { issue_key: string }
type MemberRow = {
  email: string
  name: string
  display_name?: string | null
  jira_account_id?: string | null
}

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Drop fixture-mirror upserts of KEY so a 15s delta cannot overwrite the write. */
async function ignoreFixtureDeltas(page: Page): Promise<void> {
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { upserted?: IssueRow[] }
    body.upserted = (body.upserted ?? []).filter((it) => it.issue_key !== KEY)
    await route.fulfill({ response, json: body })
  })
}

async function captureIssue(page: Page): Promise<{ issue: IssueRow; members: MemberRow[] }> {
  const held: { issue: IssueRow | null; members: MemberRow[] } = { issue: null, members: [] }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[]; members: MemberRow[] }
    held.issue = body.issues.find((it) => it.issue_key === KEY) ?? null
    held.members = body.members ?? []
    await route.fulfill({ response, json: body })
  })
  await ignoreFixtureDeltas(page)
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include NMB-110').toBeTruthy()
  return { issue: held.issue!, members: held.members }
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

test.describe('write-through', () => {
  test('comment: request carries the typed body; thread shows the refreshed comment', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const { issue } = await captureIssue(page)
    const panel = await openIssue(page)

    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    const composer = panel.getByTestId('comment-composer')
    await expect(composer).toBeVisible()

    let posted: { text?: string } | null = null
    await page.route(`**/api/v1/issues/${KEY}/comment/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as { text?: string }
      const count = typeof issue.comment_count === 'number' ? issue.comment_count : 0
      await fulfillJSON(route, {
        issue: { ...issue, comment_count: count + 1 },
        comment: {
          comment_id: 'wt-e2e-c1',
          author: 'Dana Whitfield',
          body: SERVER_COMMENT,
          created_at: '2026-08-15T12:00:00.000Z',
        },
      })
    })

    await composer.fill(TYPED_COMMENT)
    await composer.press('Meta+Enter')

    await expect.poll(() => posted?.text).toBe(TYPED_COMMENT)
    await expect(composer).toHaveValue('')
    await expect(panel.getByText(SERVER_COMMENT)).toBeVisible()
    await expect(panel.getByText(TYPED_COMMENT)).toHaveCount(0)

    // GDK-301: comment success is the same class as create — the result can
    // leave the viewport (QuickComment closes; the thread may be below the
    // fold), so a success toast is the honest feedback.
    const toast = page.getByTestId('toast').and(page.getByRole('status'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText('Posted comment on NMB-110.')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('transition: request names the chosen id; panel shows server status', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { issue } = await captureIssue(page)
    const panel = await openIssue(page)
    const chip = panel.getByTestId('status-transition')
    await expect(chip).toBeVisible()
    const before = ((await chip.innerText()) ?? '').trim()
    expect(before.length, 'status chip must show the current status').toBeGreaterThan(0)

    await page.route(`**/api/v1/issues/${KEY}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })

    let posted: { transition_id?: string } | null = null
    await page.route(`**/api/v1/issues/${KEY}/transition/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as { transition_id?: string }
      await fulfillJSON(route, {
        issue: {
          ...issue,
          status: SERVER_STATUS,
          status_category: TRANSITION.to_category,
        },
      })
    })

    await chip.click()
    const option = page.getByRole('option', { name: TRANSITION.name })
    await expect(option).toBeVisible()
    await option.click()

    await expect.poll(() => posted?.transition_id).toBe(TRANSITION.id)
    await expect(chip).toContainText(SERVER_STATUS)
    await expect(chip).not.toContainText(TRANSITION.to_status)
    await expect(chip).not.toContainText(before)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('assign: request names the chosen account; panel shows server assignee', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const { issue, members } = await captureIssue(page)
    const me = members.find((m) => m.email === 'dana@example.com' && m.jira_account_id)
    expect(me?.jira_account_id, 'fixture must expose Dana as an assignable member').toBeTruthy()

    const panel = await openIssue(page)
    const picker = panel.getByTestId('assignee-picker')
    await expect(picker).toBeVisible()

    let posted: { account_id?: string | null } | null = null
    await page.route(`**/api/v1/issues/${KEY}/assignee/`, async (route) => {
      if (route.request().method() !== 'PUT') return route.continue()
      posted = route.request().postDataJSON() as { account_id?: string | null }
      await fulfillJSON(route, {
        issue: {
          ...issue,
          assignee: SERVER_ASSIGNEE,
          assignee_id: me!.jira_account_id,
          assignee_email: me!.email,
        },
      })
    })

    await picker.click()
    const dialog = page.getByRole('dialog', { name: 'Choose assignee' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: /Assign to me/ }).click()

    await expect.poll(() => posted?.account_id).toBe(me!.jira_account_id)
    await expect(picker).toContainText(SERVER_ASSIGNEE)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('failed transition rolls back the chip and surfaces an error toast', async ({ page }) => {
    // 502 {error: jira_unavailable} is write.go failJira's default for an
    // unexpected Jira error. writeErrorMessage maps that code to
    // write.jiraUnavailable — never the raw code, never the operation
    // fallback unless the code is unknown.
    const { issue } = await captureIssue(page)
    const panel = await openIssue(page)
    const chip = panel.getByTestId('status-transition')
    await expect(chip).toBeVisible()
    const before = ((await chip.innerText()) ?? '').trim()

    await page.route(`**/api/v1/issues/${KEY}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })

    let posted = false
    await page.route(`**/api/v1/issues/${KEY}/transition/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = true
      await fulfillJSON(route, { error: 'jira_unavailable' }, 502)
    })

    await chip.click()
    await page.getByRole('option', { name: TRANSITION.name }).click()

    await expect.poll(() => posted).toBe(true)
    const toast = page.getByTestId('toast').and(page.getByRole('alert'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText('Could not reach Jira.')
    await expect(chip).toContainText(before)
    await expect(chip).not.toContainText(TRANSITION.to_status)
    await expect(chip).not.toContainText(SERVER_STATUS)
    // Snapshot was restored; the write response never landed.
    expect(issue.status, 'precondition: fixture status is what the chip rolled back to').toBe(
      before,
    )
  })
})
