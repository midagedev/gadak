// Extra capture for the GDK-1867 review round: the sprint line before and
// after the tap. The walk next door lands on the list but never taps the
// line, so the grouped sprint scope has no picture. Same fixture, same
// 402×874 viewport, its own output dir so the walk's rmSync cannot take it.
//
// The sheet is closed with `button.cancel` (the labelled button inside the
// panel), not with the scrim: the scrim carries aria-label="Cancel" too and
// a role-name click lands on its centre, which the panel covers.
import { test, type Page } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const outDir = join(here, '..', '..', 'scratch', 'mobile-shots', 'sprintline')

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
}

test('sprint line, before and after the tap', async ({ page }) => {
  rmSync(outDir, { recursive: true, force: true })
  mkdirSync(outDir, { recursive: true })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()
  await settle(page)
  await page.screenshot({ path: join(outDir, '01-issues-sprintline.png') })

  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').click()
  await page.locator('.pane:not(.off) .section .label').first().waitFor()
  await settle(page)
  await page.screenshot({ path: join(outDir, '02-sprint-scope.png') })

  // Scrolled to the end, so the Done group header under the sprint scope is
  // in frame.
  await page.locator('.pane:not(.off) main').first().evaluate((el) => {
    el.scrollTop = el.scrollHeight
  })
  await settle(page)
  await page.screenshot({ path: join(outDir, '03-sprint-scope-end.png') })
  await page.locator('.pane:not(.off) main').first().evaluate((el) => {
    el.scrollTop = 0
  })
  await settle(page)

  // The scope picker, so the sprint row can be read beside the built-ins.
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await settle(page)
  await page.screenshot({ path: join(outDir, '04-scope-sheet.png') })
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await settle(page)

  // Dark, still scoped to the sprint.
  await page.emulateMedia({ colorScheme: 'dark' })
  await settle(page)
  await page.screenshot({ path: join(outDir, '05-sprint-scope-dark.png') })

  // And the landing list in dark, for the line's contrast beside the rows.
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'All open' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await settle(page)
  await page.screenshot({ path: join(outDir, '06-issues-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' })
})
