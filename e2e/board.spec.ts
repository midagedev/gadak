import { test, expect, type Page } from '@playwright/test'
import {
  DEMO_ISSUE_COUNT_EN_RE,
  appConsoleErrors,
  attachConsoleErrors,
  forceLocale,
  gotoApp,
} from './helpers'

/*
 * GDK-1175 — the status board, and the asymmetry that is the point of it.
 *
 * Two claims are under test:
 *
 *   1. The board is the same view read across instead of down: the three
 *      status columns are always all three, keyed on status_category and
 *      never on a display name.
 *   2. A move this client did not make flies. A card whose status changed
 *      underneath the tab — a `gadak transition` in another window, an agent,
 *      another client, all of which reach this tab as one thing: a delta on
 *      the mirror tick — lands in its new column carrying `data-moved`, the
 *      marker BoardView sets only on the external-move path.
 *
 * Why a rewritten delta and not a real `gadak transition`: the e2e workspace
 * is the demo.db mirror with no writable origin behind it, so the CLI's write
 * would be refused before it ever reached the mirror. What the CLI actually
 * gives this tab is a delta on the 500ms tick — the exact packet this rig
 * hands it, through `issues.applyDelta`, the one door an outside change comes
 * through. The rig is borrowed from mirror-instant.spec.ts, which pins the
 * tick itself; this spec pins what the board does when the tick lands.
 *
 * `data-moved` is asserted rather than a mid-flight transform: the flight is
 * 260ms of Web Animation and racing it is flake, while the attribute is the
 * decision. The silent half — that this tab's own write never sets it — is
 * pinned in web/src/lib/board-moves.test.ts, where the judgment lives.
 */

type IssueRow = Record<string, unknown> & { issue_key: string; status_category?: string }

interface Rig {
  /** The bootstrap row for a key, so an injected delta is the real shape. */
  rowFor(key: string): IssueRow | null
  /** Move `key` into `category` on every delta from now on, and bump the mirror. */
  move(key: string, category: string): void
}

async function installRig(page: Page): Promise<Rig> {
  const state = {
    version: 'v0',
    rows: new Map<string, IssueRow>(),
    moved: null as { key: string; category: string } | null,
  }

  await page.route('**/api/v1/issues/bootstrap/**', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    for (const row of body.issues) state.rows.set(row.issue_key, row)
    await route.fulfill({ response, json: body })
  })

  await page.route(
    (url) => url.pathname.includes('/ui-focus/'),
    async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as Record<string, unknown>
      body.mirrorVersion = state.version
      await route.fulfill({ response, json: body })
    },
  )

  await page.route(
    (url) => url.pathname.includes('/delta/'),
    async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as { upserted?: IssueRow[] }
      const m = state.moved
      const base = m ? state.rows.get(m.key) : null
      if (m && base) {
        body.upserted = [
          ...(body.upserted ?? []),
          {
            ...base,
            status_category: m.category,
            // A different id is what a real transition changes; the category
            // alone would still pass, but this keeps the id path exercised.
            status_id: `e2e-${m.category}`,
            status: `moved to ${m.category}`,
          },
        ]
      }
      await route.fulfill({ response, json: body })
    },
  )

  return {
    rowFor: (key) => state.rows.get(key) ?? null,
    move: (key, category) => {
      state.moved = { key, category }
      state.version = `v${Number(state.version.slice(1)) + 1}`
    },
  }
}

/**
 * Open a board on the status axis directly, the way a shared link would.
 * The toggle and the grouping menu are driven by hand in the first test;
 * this is the deterministic starting point for the move.
 */
async function gotoBoard(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto('/#/?sc=new,inprogress,done&g=status_category&ly=board')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('board-card').first()).toBeVisible({ timeout: 30_000 })
}

/** The column a card is sitting in, by its `data-board-column` key. */
function columnOf(page: Page, key: string) {
  return page.locator(`[data-board-column]:has([data-board-key="${key}"])`)
}

test.describe('GDK-1175 status board', () => {
  test('opens as three status columns and keeps the layout in the URL', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // The toggle is the door: one click turns the list on screen into a board
    // and the URL says so, which is what makes a board shareable.
    await page.getByTestId('layout-board').click()
    await expect(page.getByTestId('board')).toBeVisible()
    await expect(page).toHaveURL(/ly=board/)

    // Group by progress the way a person does — the axis is the list's own.
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Progress', exact: true }).click()

    const columns = page.getByTestId('board-column')
    await expect(columns).toHaveCount(3)
    // Keyed on status_category, in work order. A display name here would be a
    // different string on a Korean account and this assertion is what says so.
    await expect(columns.nth(0)).toHaveAttribute('data-board-column', 'new')
    await expect(columns.nth(1)).toHaveAttribute('data-board-column', 'inprogress')
    await expect(columns.nth(2)).toHaveAttribute('data-board-column', 'done')

    await expect(page.getByTestId('board-card').first()).toBeVisible()

    // Back to the list, and the param goes with it.
    await page.getByTestId('layout-list').click()
    await expect(page.getByTestId('board')).toHaveCount(0)
    await expect(page).not.toHaveURL(/ly=board/)

    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('a search keeps the board — a filter change is not a view apply (GDK-1247)', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoBoard(page)

    const box = page.getByTestId('search-input')
    await box.click()
    await box.fill('project = NMA AND statusCategory = "In Progress"')
    await box.press('Enter')

    // The filters landed, and the layout did not go with them.
    await expect(page).toHaveURL(/pj=NMA/)
    await expect(page).toHaveURL(/ly=board/)
    await expect(page.getByTestId('board')).toBeVisible()

    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('a write made elsewhere moves the card across columns and marks it as not ours', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const rig = await installRig(page)
    await gotoBoard(page)

    // Take a card that is not already in the column we are going to move it
    // to, so "it moved" cannot be true before the delta lands.
    const sources = page
      .locator('[data-board-column="new"] [data-board-key], [data-board-column="inprogress"] [data-board-key]')
    await expect(sources.first()).toBeVisible()
    const key = (await sources.nth(0).getAttribute('data-board-key')) ?? ''
    const key2 = (await sources.nth(1).getAttribute('data-board-key')) ?? ''
    expect(key, 'a board card must carry its issue key').not.toBe('')
    expect(key2, 'the burst needs a second movable card').not.toBe('')
    expect(rig.rowFor(key), 'the moved row must be a real bootstrap row').not.toBeNull()
    expect(rig.rowFor(key2), 'the second moved row must be a real bootstrap row').not.toBeNull()
    await expect(columnOf(page, key)).toHaveAttribute('data-board-column', /new|inprogress/)

    // GDK-1254: park the Done column somewhere of the person's own choosing
    // before the outside write lands — the landing scroll is borrowed, and
    // this is the position it has to come back to.
    const doneScroller = page.locator('[data-board-column="done"] .scroll-region')
    await doneScroller.evaluate((el) => (el.scrollTop = 400))
    await expect
      .poll(() => doneScroller.evaluate((el) => el.scrollTop))
      .toBeGreaterThan(0)

    rig.move(key, 'done')

    // The card is in the Done column, and it flew there: `data-moved` is set
    // only by the external-move path, and lives for the 1.2s landing ring.
    const card = page.locator(`[data-board-key="${key}"]`)
    await expect(columnOf(page, key), 'the card must land in the Done column').toHaveAttribute(
      'data-board-column',
      'done',
      { timeout: 5_000 },
    )
    await expect(card, 'an outside move must be marked as not this tab’s').toHaveAttribute(
      'data-moved',
      '1',
    )

    // A second write lands while the first ring is still up (GDK-1254's
    // filmed failure was exactly this burst): the loan must extend, not
    // fork — per-run bookkeeping mistook the first nudge's scroll for the
    // person's and never gave anything back.
    rig.move(key2, 'done')
    await expect(columnOf(page, key2), 'the second card must land too').toHaveAttribute(
      'data-board-column',
      'done',
      { timeout: 5_000 },
    )

    // The rings needed the scroll; the person owns it. Once the last ring
    // expires the column returns to where it was parked — not to 400 on the
    // nose, because cards landing above the parked region make the browser's
    // scroll anchoring restate the same view as a larger number (measured
    // 461 for one landing). Without the give-back the column sits at the
    // landing (measured 4), so "at least the parked offset" is the contract.
    await expect
      .poll(() => doneScroller.evaluate((el) => el.scrollTop), { timeout: 8_000 })
      .toBeGreaterThanOrEqual(400)

    expect(appConsoleErrors(errors)).toEqual([])
  })
})
