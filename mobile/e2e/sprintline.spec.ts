// The sprint line (GDK-1867) at 402×874, against `gadak demo` — the same
// fixture every other spec here uses. `examples/demo.db` already holds the
// shape this round needs and was not touched: Sprint 42 is the one active
// sprint (41 closed, 43 future) with 20 issues, 6 of them done.
//
// What this file is for: the numbers and the pick are asserted in
// src/lib/sprint.test.ts, over the same rows, with no browser. This is the
// effect confirmed once — the line is on the screen, it says what the
// snapshot says, and tapping it re-scopes the queue. Nothing here re-measures
// what the pure tests measure, and nothing here asserts the days-left
// sentence: that one moves with the wall clock.
import { expect, test } from './helpers'

const LINE = '[data-testid="sprint-line"]'

test('the active sprint reads as one line above the queue', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  const line = page.locator(`.pane:not(.off) ${LINE}`)
  await expect(line).toHaveCount(1)
  await expect(line).toBeVisible()
  // The sprint's own name, and the snapshot's own arithmetic: 6 of 20 done.
  await expect(line.locator('.name')).toHaveText('Sprint 42')
  await expect(line.locator('.count')).toHaveText('6 / 20 · 30%')
  // The goal has been in the mirror since sprints became rows; this is the
  // first phone surface that reads it.
  await expect(line.locator('.goal')).not.toHaveText('')

  // It sits under the heading and above the first issue row.
  const lineY = (await line.boundingBox())!.y
  const firstRowY = (await page.locator('.pane:not(.off) button.row').first().boundingBox())!.y
  expect(lineY).toBeLessThan(firstRowY)
})

test('tapping the line scopes the queue to that sprint, grouped by category', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  await page.locator(`.pane:not(.off) ${LINE}`).click()

  // The heading is the scope's name and its count — the 20 rows the sprint
  // holds, done ones included: "how far has this come" needs them.
  const heading = page.locator('.pane:not(.off) h1 button.scope')
  await expect(heading.locator('.name')).toHaveText('Active sprint')
  await expect(heading.locator('.count')).toHaveText('·20')

  // Grouped in the order work moves, not by priority. The fixture has rows
  // in all three categories (10 new · 4 in progress · 6 done).
  const groups = page.locator('.pane:not(.off) .section .label')
  await expect(groups).toHaveText(['New', 'In progress', 'Done'])
  await expect(page.locator('.pane:not(.off) .section .n')).toHaveText(['10', '4', '6'])

  // The line is still there, now marked as the current scope.
  await expect(page.locator(`.pane:not(.off) ${LINE}`)).toHaveAttribute('aria-current', 'true')
})

test('the scope picker offers the sprint once, under the built-in section', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  // Wait for the line, not just for a row. `openPicker()` snapshots the row
  // counts once, when the sheet opens — a click that lands before the
  // sprints answer would open a sheet whose sprint row has no count. A
  // painted row already implies it (`issuesBootKind` holds the skeleton
  // until `loaded`, which sync sets after the sprints fetch), but the wait
  // is what makes that ordering the spec's own precondition rather than a
  // fact borrowed from another module on a fast machine.
  await page.locator(`.pane:not(.off) ${LINE}`).waitFor()

  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()

  // One row, wearing the desk's own name for this slice, with the sprint's
  // full count beside it. GDK-1542's defect was two built-in rows answering
  // one question; this one answers a question no other row asks.
  const row = page.locator('button.palette-row', { hasText: 'Active sprint' })
  await expect(row).toHaveCount(1)
  await expect(row.locator('.n')).toHaveText('20')

  await row.click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await expect(page.locator('.pane:not(.off) h1 button.scope .name')).toHaveText('Active sprint')
})
