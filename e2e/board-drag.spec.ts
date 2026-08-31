import { test, expect, type Page, type Route } from '@playwright/test'
import {
  DEMO_ISSUE_COUNT_EN_RE,
  appConsoleErrors,
  attachConsoleErrors,
  forceLocale,
} from './helpers'

/*
 * GDK-1176 — drag a card, drop it on another status column, and the drop is
 * the same optimistic transition write the detail panel makes. Two claims:
 *
 *   1. A cross-column drop POSTs the chosen transition and the card stays
 *      where it was dropped — through the origin confirm AND through the
 *      mirror's echo of that write, which must not read as an outside move
 *      (no `data-moved`, no rollback). The echo is served while the POST is
 *      still held open: that is exactly the race the self-echo guard in
 *      board-moves exists for (the optimistic row keeps the old status_id,
 *      so an early echo compares old id → new id), made deterministic.
 *   2. A plain click is still a click — the detail panel opens, nothing arms,
 *      no ghost is ever on the page.
 *
 * mouse.down()/move(…, {steps})/up() by hand throughout: the 5px arming
 * threshold is part of the mechanism under test, and dragTo would jump it.
 * Legality preview is asserted as column `data-drop` attributes — `illegal`
 * on the card's own column and on one no routed transition reaches, `over`
 * on the target — the observable the mechanism leaves for exactly this.
 */

const TRANSITION = {
  id: '21',
  name: 'Start work',
  to_status: 'In Progress',
  // Jira's REST key on purpose: the fold onto the `inprogress` column key is
  // part of what the drop has to get right.
  to_category: 'indeterminate',
} as const

type IssueRow = Record<string, unknown> & { issue_key: string; status_category?: string }

interface Rig {
  rowFor(key: string): IssueRow | null
  keys(): string[]
  /** Serve `rows` on every delta from now on, and bump the mirror version. */
  echo(rows: IssueRow[]): void
}

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Bootstrap capture + mirror bump + delta injection (the board.spec.ts rig). */
async function installRig(page: Page): Promise<Rig> {
  const state = {
    version: 'v0',
    rows: new Map<string, IssueRow>(),
    inject: [] as IssueRow[],
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
      if (state.inject.length) body.upserted = [...(body.upserted ?? []), ...state.inject]
      await route.fulfill({ response, json: body })
    },
  )

  return {
    rowFor: (key) => state.rows.get(key) ?? null,
    keys: () => [...state.rows.keys()],
    echo: (rows) => {
      state.inject = rows
      state.version = `v${Number(state.version.slice(1)) + 1}`
    },
  }
}

/** Open the board on the status axis directly, the way a shared link would. */
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

test.describe('GDK-1176 board drag', () => {
  test('a drop on another column is the transition write, and the echo does not bounce it', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    // Empty local map so the routed lazy GET below is the one legality source.
    await page.route('**/api/v1/meta/write/', (route) =>
      fulfillJSON(route, { transitions: {}, create_meta: { projects: [] }, updated_at: null }),
    )
    const rig = await installRig(page)
    await gotoBoard(page)

    const source = page.locator('[data-board-column="new"] [data-board-key]').first()
    await expect(source).toBeVisible()
    const key = (await source.getAttribute('data-board-key')) ?? ''
    expect(key, 'a board card must carry its issue key').not.toBe('')
    const row = rig.rowFor(key)
    expect(row, 'the dragged row must be a real bootstrap row').not.toBeNull()

    await page.route(`**/api/v1/issues/${key}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })

    // The mirror row as write.go would return it after Jira + refresh.
    const confirmed: IssueRow = {
      ...row!,
      status: TRANSITION.to_status,
      status_id: 'e2e-drag-inprogress',
      status_category: TRANSITION.to_category,
    }

    // Hold the POST open so the mirror echo can land *before* the confirm.
    let posted: { transition_id?: string } | null = null
    let releasePost = () => {}
    const postGate = new Promise<void>((resolve) => (releasePost = resolve))
    await page.route(`**/api/v1/issues/${key}/transition/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as { transition_id?: string }
      await postGate
      await fulfillJSON(route, { issue: confirmed })
    })

    // Drag by hand: press, cross the 5px threshold, land on the column.
    const box = (await source.boundingBox())!
    const target = (await page.locator('[data-board-column="inprogress"]').boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width / 2 + 12, box.y + box.height / 2 + 4, { steps: 3 })

    const card = page.locator(`[data-board-key="${key}"]`)
    await expect(card).toHaveAttribute('data-dragging', '1')

    await page.mouse.move(target.x + target.width / 2, target.y + target.height / 2, { steps: 6 })
    // The preview: own column and the unreachable one dim, the target says over.
    await expect(page.locator('[data-board-column="inprogress"]')).toHaveAttribute('data-drop', 'over')
    await expect(page.locator('[data-board-column="new"]')).toHaveAttribute('data-drop', 'illegal')
    await expect(page.locator('[data-board-column="done"]')).toHaveAttribute('data-drop', 'illegal')

    await page.mouse.up()

    // The drop is the write: the POST names the transition, the card has
    // already crossed on the optimistic patch, and the gesture is over.
    await expect.poll(() => posted?.transition_id).toBe(TRANSITION.id)
    await expect(columnOf(page, key)).toHaveAttribute('data-board-column', 'inprogress')
    await expect(card).not.toHaveAttribute('data-dragging', '1')
    await expect(page.locator('.board-drag-ghost')).toHaveCount(0)

    // The race, made deterministic: the POST is still held, so the pool still
    // holds the optimistic row (old status_id) when the echo lands. A second
    // row's summary edit rides along as the sentinel that the delta applied.
    const sentinelKey = rig.keys().find((k) => k !== key)!
    const sentinel = { ...rig.rowFor(sentinelKey)!, summary: 'e2e echo sentinel' }
    rig.echo([confirmed, sentinel])
    await expect(page.locator(`[data-board-key="${sentinelKey}"]`)).toContainText(
      'e2e echo sentinel',
      { timeout: 10_000 },
    )
    // One-shot sample, not a retrying matcher: the ring lives 1.2s, and a
    // negated auto-retry would simply wait it out and go green on the bug.
    expect(
      await card.getAttribute('data-moved'),
      'this tab’s own echo must not fly as an outside move',
    ).toBeNull()
    await expect(columnOf(page, key)).toHaveAttribute('data-board-column', 'inprogress')

    // Let the origin confirm land: the card stays put, nothing rolled back.
    releasePost()
    await expect(columnOf(page, key)).toHaveAttribute('data-board-column', 'inprogress')
    await expect(card).not.toHaveAttribute('data-moved', '1')
    await expect(page.getByTestId('toast').and(page.getByRole('alert'))).toHaveCount(0)

    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('a plain click is still a click — select, not drag', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoBoard(page)

    const card = page.getByTestId('board-card').first()
    const key = (await card.getAttribute('data-board-key')) ?? ''
    await card.click()

    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(card).not.toHaveAttribute('data-dragging', '1')
    await expect(page.locator('.board-drag-ghost')).toHaveCount(0)
    expect(key).not.toBe('')

    expect(appConsoleErrors(errors)).toEqual([])
  })
})
