import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

/**
 * The stale threshold learns from the workspace: when no setting is given,
 * the p85 cycle time of issues finished in the last 90 days replaces the
 * fixed 72 hours (flow block on bootstrap/delta). The mark itself is
 * unchanged — same glyph, same bands, same ratios (GDK-1336/GDK-766 own
 * that look); what this spec pins is which threshold the mark follows and
 * that the hover names the rule when the rule was learned (G7).
 *
 * Route-mock strategy: bootstrap and delta are passed through to the real
 * e2e server (so its half of the contract keeps running) and only the flow
 * block is set or stripped. Delta is mocked too — the store clears a
 * carried flow when the wire omits it, so an unmocked poll would race the
 * assertion.
 *
 * FAIL-first: against the pre-change client, the flow cases failed — the
 * mark followed the configured 72 regardless of the mocked flow, and the
 * title never contained "85%".
 */

const KEY = 'NMB-5' // fixture: in progress, status_changed_at 2026-06-05 → months old
const LIST_ROW = `[data-testid="issue-list-scroller"] [data-issue-key="${KEY}"]`
const STALE_MARK = `${LIST_ROW} [data-col="stale"] [data-stale-band]`

type IssueRow = Record<string, unknown> & { started_at?: string | null; status_changed_at?: string | null }

/** The clock the row's age reads (view-config workAge): started_at when the
 *  mirror knows when work started, else status_changed_at — and the plain
 *  title names which (2026-09-07). */
function ageClock(row: IssueRow): { stamp: string; phrase: string } {
  return row.started_at
    ? { stamp: row.started_at, phrase: 'days since work started' }
    : { stamp: row.status_changed_at ?? '', phrase: 'days in this status' }
}
type FlowBody = { flow?: { cycle_p85_hours: number; samples: number } }

/** Installs the passthrough mocks and returns the fixture's own row for
 *  KEY, captured off the bootstrap body. Age assertions compute from that
 *  stamp, not from a hardcoded date, so the fixture may age freely. */
async function mockFlow(page: Page, flow: FlowBody['flow']): Promise<IssueRow> {
  const held: { issue: IssueRow | null } = { issue: null }
  const intercept = async (route: Route): Promise<void> => {
    const response = await route.fetch()
    const body = (await response.json()) as {
      issues?: IssueRow[]
      upserted?: IssueRow[]
    } & FlowBody
    if (body.issues) {
      held.issue = body.issues.find((it) => it.issue_key === KEY) ?? null
    }
    if (flow) body.flow = flow
    else delete body.flow
    await route.fulfill({ response, json: body })
  }
  await page.route('**/api/v1/issues/bootstrap/', intercept)
  await page.route((url) => url.pathname.includes('/delta/'), intercept)
  await forceLocale(page, 'en')
  await gotoApp(page)
  expect(held.issue, `fixture bootstrap must include ${KEY}`).toBeTruthy()
  return held.issue!
}

async function openRow(page: Page): Promise<ReturnType<typeof searchInput>> {
  const input = searchInput(page)
  await input.fill(KEY)
  await expect(page.locator(LIST_ROW)).toBeVisible()
  return input
}

test.describe('learned stale threshold', () => {
  test('a learned 1h threshold marks the months-old row stale and the hover names the rule', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await mockFlow(page, { cycle_p85_hours: 1, samples: 20 })
    await openRow(page)

    const mark = page.locator(STALE_MARK)
    await expect(mark).toBeVisible()
    // 1h threshold, ~2200h row: far past every band edge.
    await expect(mark).toHaveAttribute('data-stale-band', 'loud')
    // G7: the title says what the line is and where it came from.
    // The sample count rides in the title beside the number (G7).
    await expect(mark).toHaveAttribute('title', /85% of the 20 issues finished in the last 90 days/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('no flow block: the 72h default holds, band and plain title unchanged', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const row = await mockFlow(page, null)
    await openRow(page)

    // Compute the exact expectation from the fixture's own stamp.
    const { stamp, phrase } = ageClock(row)
    const ageHours = (Date.now() - Date.parse(stamp)) / 3_600_000
    expect(ageHours, 'fixture row must be far past 72h for this assertion').toBeGreaterThan(72)
    const days = Math.max(1, Math.round(ageHours / 24))
    const ratio = ageHours / 72
    const band = ratio <= 2 ? 'quiet' : ratio <= 4 ? 'mid' : 'loud'

    const mark = page.locator(STALE_MARK)
    await expect(mark).toBeVisible()
    await expect(mark).toHaveAttribute('data-stale-band', band)
    // The plain title: the rule is only named when it was learned; the
    // clock (started vs status) is always named.
    await expect(mark).toHaveAttribute('title', `${days} ${phrase}`)
    await expect(mark).not.toHaveAttribute('title', /85%/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a flow below the sample minimum is ignored — 72h behaviour', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const row = await mockFlow(page, { cycle_p85_hours: 1, samples: 5 })
    await openRow(page)

    const { stamp, phrase } = ageClock(row)
    const ageHours = (Date.now() - Date.parse(stamp)) / 3_600_000
    const days = Math.max(1, Math.round(ageHours / 24))
    const ratio = ageHours / 72
    const band = ratio <= 2 ? 'quiet' : ratio <= 4 ? 'mid' : 'loud'

    const mark = page.locator(STALE_MARK)
    await expect(mark).toBeVisible()
    await expect(mark).toHaveAttribute('data-stale-band', band)
    await expect(mark).toHaveAttribute('title', `${days} ${phrase}`)
    await expect(mark).not.toHaveAttribute('title', /85%/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
