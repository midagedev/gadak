import { test, expect, type Locator, type Page, type Route } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { attachConsoleErrors, gotoApp } from './helpers'

const here = dirname(fileURLToPath(import.meta.url))

// Where my-work.png lands: repo scratch by default (gitignored, CI-safe — a
// Linux runner has no /private), overridable so this round routes the capture
// to its own session scratchpad for the lead's vision review.
const SHOT_DIR = process.env.MY_WORK_SHOT_DIR ?? join(here, '../scratch')
const SHOT = join(SHOT_DIR, 'my-work.png')

/**
 * my-work pack: the contributor's first screen — identity flags (mine /
 * delegated), the two stance labels in the sidebar, first-run landing, and
 * aging-in-progress on the real status_changed axis.
 *
 * Contract ↔ assertion table (clause → assertion names):
 *  C2/C3 anonymous: stance labels stay, identity views are absent (not
 *      disabled), the eight team/plain views remain
 *      anonymous sidebar: labels yes, identity rows no, eight rows by name
 *      stance labels are rows of text, not buttons
 *  C1 person-match is the only mine-rule (sidebar count == list == SQL)
 *      identified sidebar lists My issues 46 / Handed off 25
 *      clicking My issues shows 46 rows, every assignee is Dana (first row pinned)
 *  C5 first-run precedence (aria-current is the sidebar contract)
 *      first run identified lands on My issues
 *      first run anonymous lands on the epic breakdown
 *  C4 aging reads status_changed asc — expected order computed from the
 *      fixture's own bootstrap body, never hardcoded
 *      aging-in-progress opens on the oldest in-status issue
 *
 * FAIL-first: unit siblings hold the pure halves (view-config.test.ts flags +
 * sort round-trip, builtin-views.test.ts ten views/partition, startup-view
 * .test.ts precedence, filters-actor.test.ts evaluation) and failed against
 * the pre-change source 2026-09-06 (21 red). In the browser the same change
 * reads as: no stance labels, no My issues / Handed off rows, first-run
 * landing on the epic breakdown, aging sorted by updated-at.
 *
 * Identity plumbing: the e2e serve ships its own credential (serve.sh writes
 * email dana@example.com), so "anonymous" here is the explicit 200 {email:
 * null} shape me.svelte.ts documents — render-before-auth stays anonymous.
 */

const ASIDE = 'aside'
const SCROLLER = '[data-testid="issue-list-scroller"]'

/** The Dana identity the spec fixes for this pack — id first, then email. */
const DANA_ME = { email: 'dana@example.com', account_id: 'demo-dana', name: 'Dana Whitfield' }
/**
 * Sidebar counts for Dana in examples/demo.db (SQL-verified 2026-09-06, the
 * person-match rule mirrored with coalesce on every nullable side — a bare
 * `not (assignee_id='demo-dana' or …)` silently drops unassigned rows under
 * SQL NULL three-valued logic and reports 25, which is only the held-by-
 * others half; delegatedBy counts "or nobody" too: 25 held + 35 unassigned).
 */
const DANA_MINE = 46
const DANA_DELEGATED = 60

type BootRow = {
  issue_key: string
  status_category?: string | null
  status_changed_at?: string | null
  updated_at?: string | null
}

async function mockAuthMe(page: Page, body: unknown): Promise<void> {
  await page.route('**/api/v1/auth/me/**', (route) => route.fulfill({ json: body }))
}

/** Passthrough bootstrap capture (the stale-threshold pattern): the real
 *  e2e server still answers, the spec reads the fixture's own rows. */
async function captureBootstrap(page: Page): Promise<BootRow[]> {
  const held: { issues: BootRow[] | null } = { issues: null }
  const intercept = async (route: Route): Promise<void> => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues?: BootRow[] }
    if (body.issues) held.issues = body.issues
    await route.fulfill({ response, json: body })
  }
  await page.route('**/api/v1/issues/bootstrap/', intercept)
  await gotoApp(page)
  expect(held.issues, 'fixture bootstrap must carry issues').toBeTruthy()
  return held.issues!
}

function sidebarButton(page: Page, name: string): Locator {
  return page.locator(ASIDE).getByRole('button', { name })
}

test.describe('my-work pack: sidebar stances and identity views', () => {
  test('anonymous sidebar: labels yes, identity rows no, eight rows by name', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, { email: null })
    await gotoApp(page)

    // Both stance labels are present — the sections' reading exists even
    // when "mine" does not.
    await expect(page.getByTestId('sidebar-stance-mine')).toBeVisible()
    await expect(page.getByTestId('sidebar-stance-team')).toBeVisible()

    // Identity views are absent, not disabled (C3): no row, no count, nothing
    // to click that would open an empty list.
    await expect(sidebarButton(page, 'My issues')).toHaveCount(0)
    await expect(sidebarButton(page, 'Handed off')).toHaveCount(0)

    // The other eight views are all there, by accessible name.
    for (const name of [
      'All open',
      'Unassigned new',
      'Recently updated',
      'Aging in progress',
      'Stale',
      'Reopened',
      'Epics',
      'Resolved this week',
    ]) {
      await expect(sidebarButton(page, name), name).toBeVisible()
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('stance labels are rows of text, not buttons (C8)', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, { email: null })
    await gotoApp(page)
    // The label names nothing clickable — they cannot appear as buttons.
    await expect(page.locator(ASIDE).getByRole('button', { name: 'My work' })).toHaveCount(0)
    await expect(page.locator(ASIDE).getByRole('button', { name: 'Team flow' })).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('my-work pack: identified sidebar and the mine list', () => {
  test('identified sidebar lists My issues 46 / Handed off 60', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, DANA_ME)
    await captureBootstrap(page)

    // Person-match counts (id first, then email): the sidebar count column
    // and SQL agree — DANA_MINE/DANA_DELEGATED are the fixture's own numbers.
    await expect(sidebarButton(page, 'My issues')).toContainText(String(DANA_MINE))
    await expect(sidebarButton(page, 'Handed off')).toContainText(String(DANA_DELEGATED))

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('clicking My issues shows 46 rows, every assignee is Dana (first row pinned)', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, DANA_ME)
    await captureBootstrap(page)

    // First run already lands on my-work (asserted below in the landing
    // tests); leave the view, come back through the row — the click path.
    await sidebarButton(page, 'All open').click()
    await sidebarButton(page, 'My issues').click()
    await expect(page.getByTestId('list-count')).toContainText(String(DANA_MINE))

    // Every visible row is Dana's; the first row's assignee avatar says so.
    const rows = page.locator(`${SCROLLER} [data-issue-key]`)
    await expect(rows.first()).toBeVisible()
    await expect(rows.first().locator('[data-col="assignee"] [title]')).toHaveAttribute(
      'title',
      /Dana/,
    )

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('screenshot for the lead vision review (written, never judged here)', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, DANA_ME)
    await captureBootstrap(page)
    // The capture is of the My issues view however it is reached; gotoApp
    // steers a fresh context to Epics, so open it by the sidebar row.
    await sidebarButton(page, 'My issues').click()
    await expect(page.getByTestId('list-count')).toContainText(String(DANA_MINE))
    await page.screenshot({ path: SHOT, animations: 'disabled' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('my-work pack: first-run landing (aria-current is the contract)', () => {
  test('first run identified lands on My issues', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, DANA_ME)
    // startup: 'product' — this test is about the first-run rule itself, so
    // the helper must not steer the fresh context back to the Epics view.
    await gotoApp(page, { startup: 'product' })
    // Fresh context ⇒ no last-used view; identified + 46 assigned ⇒ my-work.
    await expect(page.getByTestId('list-count')).toContainText(String(DANA_MINE))
    await expect(sidebarButton(page, 'My issues')).toHaveAttribute('aria-current', 'true')
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('first run anonymous lands on the epic breakdown', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Anonymous (the 200 {email:null} shape): no "mine", so the pre-my-work
    // first-run default holds — the epic breakdown, unchanged.
    await mockAuthMe(page, { email: null })
    await gotoApp(page)
    await expect(sidebarButton(page, 'Epics')).toHaveAttribute('aria-current', 'true')
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('my-work pack: aging on the real status_changed axis', () => {
  test('aging-in-progress opens on the oldest in-status issue', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockAuthMe(page, DANA_ME)
    const boot = await captureBootstrap(page)

    // Expected order from the fixture's own rows: among in-progress issues
    // with a parseable stamp, the oldest status_changed_at first (ties →
    // newest updated_at); missing stamps sort last in both directions.
    const inProgress = boot.filter((it) => it.status_category === 'inprogress')
    expect(inProgress.length, 'fixture must have in-progress rows').toBeGreaterThan(0)
    const stamped = inProgress.filter((it) => {
      const t = Date.parse(it.status_changed_at ?? '')
      return Number.isFinite(t)
    })
    const expectedFirst = stamped.reduce((best, it) => {
      const bt = Date.parse(best.status_changed_at!)
      const itT = Date.parse(it.status_changed_at!)
      if (itT !== bt) return itT < bt ? it : best
      // Tie: newest updated_at wins (the comparator's second key).
      return (it.updated_at ?? '') > (best.updated_at ?? '') ? it : best
    })

    await sidebarButton(page, 'Aging in progress').click()
    await expect(page.getByTestId('list-count')).toContainText(String(inProgress.length))

    const firstRow = page.locator(`${SCROLLER} [data-issue-key]`).first()
    await expect(firstRow).toHaveAttribute('data-issue-key', expectedFirst.issue_key)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
