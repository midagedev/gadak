import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'
import { staleBandFor } from '../web/src/lib/view-config'

/**
 * The stale threshold learns from the workspace: when no setting is given,
 * the p85 cycle time of issues finished in the last 90 days replaces the
 * fixed 72 hours (flow block on bootstrap/delta). The mark itself is
 * unchanged — same glyph, same bands, same ratios (GDK-1336/GDK-766 own
 * that look).
 *
 * GDK-1571: this spec is one test because it has exactly one question the
 * unit project cannot answer — does the mark on screen render the value the
 * PRODUCTION function computes? The threshold decision (learned vs set vs
 * default, sample minimum) and the band boundaries (exactly 2×, exactly 4×)
 * are unit-pinned in view-config.test.ts; this file used to re-write the
 * band formula beside IssueRow's copy, which agreed by construction and
 * asserted the edges by nobody. It now imports staleBandFor and holds no
 * ratios of its own.
 *
 * Route-mock strategy: bootstrap and delta are passed through to the real
 * e2e server (so its half of the contract keeps running) and only the flow
 * block is set. Delta is mocked too — the store clears a carried flow when
 * the wire omits it, so an unmocked poll would race the assertion.
 *
 * FAIL-first: against the pre-change client, the flow cases failed — the
 * mark followed the configured 72 regardless of the mocked flow, and the
 * title never contained "85%".
 */

const KEY = 'NMB-5' // fixture: in progress, status_changed_at 2026-06-05 → months old
const LIST_ROW = `[data-testid="issue-list-scroller"] [data-issue-key="${KEY}"]`
const STALE_MARK = `${LIST_ROW} [data-stale-band]`

type IssueRow = Record<string, unknown> & { started_at?: string | null; status_changed_at?: string | null }

/** The clock the row's age reads (view-config workAge): started_at when the
 *  mirror knows when work started, else status_changed_at. */
function ageClock(row: IssueRow): string {
  return row.started_at ?? row.status_changed_at ?? ''
}
type FlowBody = { flow?: { cycle_p85_hours: number; samples: number } }

/** Installs the passthrough mocks and returns the fixture's own row for
 * KEY, captured off the bootstrap body. Age assertions compute from that
 * stamp, not from a hardcoded date, so the fixture may age freely. */
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

test.describe('learned stale threshold', () => {
  test('the mark renders the band the shared function computes, and a learned rule names itself', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    // A learned 1h with 20 samples: every threshold decision the units pin
    // (flow over setting, sample minimum met) is exercised on the wire here.
    const row = await mockFlow(page, { cycle_p85_hours: 1, samples: 20 })
    const input = searchInput(page)
    await input.fill(KEY)
    await expect(page.locator(LIST_ROW)).toBeVisible()

    // The expectation IS the production function (GDK-1571): age off the
    // fixture's own stamp, threshold off the mocked flow. No second copy.
    const stamp = ageClock(row)
    const ageHours = (Date.now() - Date.parse(stamp)) / 3_600_000
    const band = staleBandFor(true, ageHours, 1)

    const mark = page.locator(STALE_MARK)
    await expect(mark).toBeVisible()
    await expect(mark).toHaveAttribute('data-stale-band', band)
    // G7: when the rule was learned, the title says what the line is and
    // where it came from, sample count beside the number.
    await expect(mark).toHaveAttribute('title', /85% of the 20 issues finished in the last 90 days/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
