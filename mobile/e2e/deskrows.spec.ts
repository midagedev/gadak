/*
 * GDK-1874 end to end: the five things that stay on the desk say so, each
 * where a person would look for them.
 *
 * src/lib/desk.test.ts measures what a browser cannot — that the Fields row
 * is owed by a configured custom field and not by a system one, and that all
 * five call sites draw the one component and no second spelling of the
 * sentence. This file confirms the effect once, at 402×874 against `gadak
 * demo`: the rows are on the screens, they are inert, and they clear the
 * touch floor.
 *
 * The fixture decides which half of the Fields case is reachable here. It
 * configures no custom fields, so the visible assertion is the ABSENCE, and
 * the premise is pinned rather than assumed — if the demo db ever gains a
 * field spec, the first expect below goes red and points at this comment
 * instead of the Fields row quietly changing meaning.
 */
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { SERVE_ORIGIN } from '../playwright.config'

/** The touch floor, read after the sheet's fly has finished: a 44px control
 *  mid-transform at deviceScaleFactor 3 measures 43.99993896484375, which is
 *  a reading of the animation and not of the layout (viewport.spec.ts). */
const TOUCH_FLOOR = 44

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
}

/** Every desk row wears the same three facts, whatever it names. */
async function expectDeskRow(page: Page, testid: string, label: string): Promise<void> {
  const row = page.locator(`[data-testid="${testid}"]`)
  await expect(row).toHaveCount(1)
  await expect(row).toBeVisible()
  await expect(row.locator('.name')).toHaveText(label)
  // The desk sentence is the catalog's, and it is the same one on all five.
  await expect(row.locator('.why')).toHaveText('Open on the desktop')
  await expect(row).toHaveAttribute('aria-disabled', 'true')
  await expect(row).toBeDisabled()
  const box = await row.boundingBox()
  expect(box!.height, `${testid} touch floor`).toBeGreaterThanOrEqual(TOUCH_FLOOR)
}

async function bootIssues(page: Page): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

async function openPicker(page: Page): Promise<void> {
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await settle(page)
}

test('the scope picker names view authoring and dashboards', async ({ page }) => {
  await bootIssues(page)
  await openPicker(page)

  // View authoring, at the end of the saved views — a group the sheet now
  // draws even with nothing saved, because that is exactly when someone
  // goes looking for where a view is made. The fixture saves none, so the
  // heading here stands over this row alone.
  await expect(page.locator('.palette-section', { hasText: 'Saved views' })).toHaveCount(1)
  await expectDeskRow(page, 'desk-row-views', 'View settings')

  // Dashboards, last in the list: the one scope-shaped surface the phone
  // has no plate for at all.
  await expectDeskRow(page, 'desk-row-dashboards', 'Dashboards')

  // Order: the views row sits under its own heading and above the
  // dashboards row, which is the last thing in the list.
  const views = (await page.locator('[data-testid="desk-row-views"]').boundingBox())!
  const dashboards = (await page.locator('[data-testid="desk-row-dashboards"]').boundingBox())!
  expect(views.y).toBeLessThan(dashboards.y)

  // Neither is a way out of the sheet: it is still cancellable, and the
  // scope did not change under them.
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
})

test('the sprint band names the board, on the line that reads it', async ({ page }) => {
  await bootIssues(page)
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()
  await settle(page)

  await expectDeskRow(page, 'desk-row-board', 'Board')

  const line = (await page.locator('.pane:not(.off) [data-testid="sprint-line"]').boundingBox())!
  const board = (await page.locator('[data-testid="desk-row-board"]').boundingBox())!
  // Under the line it belongs to, and above the queue that line describes.
  expect(board.y).toBeGreaterThan(line.y)
  const firstRow = (await page.locator('.pane:not(.off) button.row').first().boundingBox())!
  expect(board.y).toBeLessThan(firstRow.y)

  // On the same left column as the sprint's own name. The desk row carries
  // the scope sheet's 8px inset, so every placement outside that sheet owes
  // a wrapper that makes up the other 8 — this is the assertion that says
  // the wrapper is there (GDK-1874; the alignment a review round would
  // otherwise have to catch by eye).
  const sprintName = (await page
    .locator('.pane:not(.off) [data-testid="sprint-line"] .name')
    .boundingBox())!
  const boardName = (await page.locator('[data-testid="desk-row-board"] .name').boundingBox())!
  expect(boardName.x, 'board row label column').toBeCloseTo(sprintName.x, 1)
})

test('a page detail says its content is edited on the desk', async ({ page }) => {
  await bootIssues(page)
  await openPicker(page)
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  const row = page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first()
  await row.waitFor()
  await row.click()
  await page.locator('.page-detail button.back').waitFor()
  await settle(page)

  await expectDeskRow(page, 'desk-row-page-edit', 'Content')

  // Above the body it describes, and above the comment composer — which is
  // the write this screen does have, and must still be there (GDK-1873).
  const desk = (await page.locator('[data-testid="desk-row-page-edit"]').boundingBox())!
  const body = (await page.locator('.page-detail .body').boundingBox())!
  expect(desk.y).toBeLessThan(body.y)
  await expect(page.locator('.page-detail .composer input')).toHaveCount(1)

  // Same left column as the page title above it.
  const title = (await page.locator('.page-detail h1').boundingBox())!
  const label = (await page.locator('[data-testid="desk-row-page-edit"] .name').boundingBox())!
  expect(label.x, 'page desk row label column').toBeCloseTo(title.x, 1)
})

test('an issue with no configured custom field draws no Fields desk row', async ({ page }) => {
  // The premise, measured rather than assumed: `field_specs` is projected
  // from workspace config (internal/server/read.go fieldSpecsOut) and the
  // demo workspace configures none. The present case lives in
  // src/lib/desk.test.ts, which is the only place it can.
  const boot = (await (await fetch(`${SERVE_ORIGIN}/api/v1/issues/bootstrap/`)).json()) as {
    field_specs?: unknown[]
  }
  expect(boot.field_specs ?? [], 'demo fixture configures no custom field').toEqual([])

  await bootIssues(page)
  await page.locator('button.search').click()
  await page.locator('.pane:not(.off) input').first().fill('NMB-105')
  const hit = page.locator('.pane:not(.off) button.row', { hasText: 'NMB-105' }).first()
  await hit.waitFor()
  await hit.click()
  await page.locator('button.back').waitFor()
  await settle(page)

  // The section itself is on screen — this is an issue that carries fields,
  // so the absence below is the desk row's and not the section's.
  await expect(page.locator('[data-testid="detail-fields"]')).toHaveCount(1)
  await expect(page.locator('[data-testid="desk-row-fields"]')).toHaveCount(0)
})
