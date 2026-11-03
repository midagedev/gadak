import { type Page, type Route } from '@playwright/test'
import { expect, test } from './helpers'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * The stale-assignee refusal on the web (GDK-2073): a 409 assignee_changed
 * from PUT <key>/assignee/ must apply the refreshed row the refusal carries
 * — so the pool and the panel name the real holder, not the optimistic pick
 * and not the stale row the person was looking at — and say who took the
 * issue. The person picking again after the refresh is the deliberate
 * replace; a confirm dialog or a silent overwrite would both hide the holder
 * the refusal exists to name. Same wire the phone's pickAssignee answers
 * (mobile/src/ui/detail/AssigneeSheet.svelte); the verdict itself is
 * internal/origin/assign_check.go's, tabled in the Go suites.
 */

const KEY = 'NMB-110'
const HOLDER = { account_id: 'std-e2e-holder', display_name: 'Sora Holder' } as const

type IssueRow = Record<string, unknown> & { issue_key: string }
type MemberRow = { email: string; jira_account_id?: string | null }

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Drop fixture-mirror upserts of KEY so a 15s delta cannot overwrite the refusal. */
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

test.describe('stale assignee (GDK-2073)', () => {
  test('a 409 assignee_changed names the holder, applies their row, and keeps no confirm dialog', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const { issue, members } = await captureIssue(page)
    const me = members.find((m) => m.email === 'dana@example.com' && m.jira_account_id)
    expect(me?.jira_account_id, 'fixture must expose Dana as an assignable member').toBeTruthy()

    const panel = await openIssue(page)
    const picker = panel.getByTestId('assignee-picker')
    await expect(picker).toBeVisible()

    let posted: { account_id?: string | null; force?: boolean } | null = null
    await page.route(`**/api/v1/issues/${KEY}/assignee/`, async (route) => {
      if (route.request().method() !== 'PUT') return route.continue()
      posted = route.request().postDataJSON() as { account_id?: string | null }
      // The server's refusal (internal/server/write_issue.go): the holder
      // the cache did not show, plus the refreshed row naming them.
      await fulfillJSON(
        route,
        {
          error: 'assignee_changed',
          current_assignee: { account_id: HOLDER.account_id, display_name: HOLDER.display_name },
          issue: {
            ...issue,
            assignee: HOLDER.display_name,
            assignee_id: HOLDER.account_id,
            assignee_email: null,
          },
        },
        409,
      )
    })

    await picker.click()
    const dialog = page.getByRole('dialog', { name: 'Choose assignee' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: /Assign to me/ }).click()

    await expect.poll(() => posted?.account_id).toBe(me!.jira_account_id)
    // The web never sends force: the second pick is the deliberate replace.
    expect(posted).not.toHaveProperty('force')

    // The refusal names the holder and the issue, not a generic failure.
    const toast = page.getByTestId('toast').and(page.getByRole('alert'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(`${KEY} was assigned to ${HOLDER.display_name}`)

    // The pool took the refusal's row: the panel shows the holder, not the
    // optimistic pick (Dana) and not the stale row the fixture had.
    await expect(picker).toContainText(HOLDER.display_name)
    await expect(picker).not.toContainText('Dana Whitfield')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an unassign refusal names nobody with the Unassigned label', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { issue, members } = await captureIssue(page)
    expect(members.length, 'fixture members').toBeGreaterThan(0)

    const panel = await openIssue(page)
    const picker = panel.getByTestId('assignee-picker')

    let posted: { account_id?: string | null } | null = null
    await page.route(`**/api/v1/issues/${KEY}/assignee/`, async (route) => {
      if (route.request().method() !== 'PUT') return route.continue()
      posted = route.request().postDataJSON() as { account_id?: string | null }
      // Nobody holds it on the origin: current_assignee is null, and the
      // toast must fall back to the assignee UI's own Unassigned label.
      await fulfillJSON(
        route,
        {
          error: 'assignee_changed',
          current_assignee: null,
          issue: { ...issue, assignee: null, assignee_id: null, assignee_email: null },
        },
        409,
      )
    })

    // Unassign: the picker's clearing row ("Unassigned" / me-null). Not an
    // exact name match — the row's accessible name carries its – glyph.
    await picker.click()
    const dialog = page.getByRole('dialog', { name: 'Choose assignee' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Unassigned' }).click()

    await expect.poll(() => posted?.account_id).toBe(null)
    const toast = page.getByTestId('toast').and(page.getByRole('alert'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(`${KEY} was assigned to Unassigned`)
    // The applied row shows no holder.
    await expect(picker).toContainText('Unassigned')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
