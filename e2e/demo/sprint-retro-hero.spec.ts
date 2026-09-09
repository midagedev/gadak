/**
 * The 0.22 sprint + retro hero — one take for the release tweet.
 *
 * Two claims in one cut. The board, scoped to the active sprint, now says
 * what that sprint is in a line of its own: name, dates, days left, a
 * progress bar counted over the whole sprint (GDK-1709), and the cards
 * carry how many sprints they have already been through (GDK-1711). Then
 * the retro, from the palette: the running bucket summarised above the
 * table, a sparkline on every row, the cut switched to sprints, and one
 * number clicked open into the issues behind it (GDK-1712, GDK-1693).
 *
 * No terminal, no CLI, no DOM caption. Gated by GADAK_MEDIA=1. Viewport and
 * video size must stay 1280×800 (sprint-retro-hero.config.ts) or Playwright
 * letterboxes the capture. The sibling single-claim takes
 * (sprint-demo.spec.ts, retro-demo.spec.ts) stay as the landing halves.
 */
import { test, expect, type Page, type TestInfo } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import {
  attachConsoleErrors,
  catalogFor,
  forceLocale,
  mediaLocale,
  MEDIA_LOCALE_STAMP,
} from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** The UI language this take records in. Same contract as sprint-demo. */
const LOCALE = mediaLocale()
const T = catalogFor(LOCALE)

/** The take says which language it is; the export refuses a mismatch (GDK-1501). */
async function stampLocale(testInfo: TestInfo): Promise<void> {
  await writeFile(join(testInfo.project.outputDir, MEDIA_LOCALE_STAMP), `${LOCALE}\n`, 'utf8')
}

/** Pause between beats so a human can read the UI. Same default as web-demo. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

test.describe('sprint + retro hero', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the sprint has a line of its own, and the retro reads as a report', async ({
    page,
  }, testInfo) => {
    await stampLocale(testInfo)
    const errors = attachConsoleErrors(page)
    await forceLocale(page, LOCALE)
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
    await beat(page, 600)

    // The team's whole open pool, not "My issues" — a sprint board is a
    // team surface (same reason as sprint-demo.spec.ts).
    await page
      .getByRole('button', { name: new RegExp('^' + T['view.allOpen.name']) })
      .first()
      .click()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await beat(page, 700)

    // Into the board the way a person does.
    await page.getByTestId('view-settings').click()
    await beat(page, 400)
    await page.getByTestId('layout-board').click()
    await expect(page.getByTestId('board')).toBeVisible()
    await page.keyboard.press('Escape')
    const scope = page.getByTestId('sprint-scope')
    await expect(scope).toBeVisible()
    await expect(scope).toContainText(T['board.scopeAxis'])
    await beat(page, 1000)

    // Beat 1 — the sprint gets its line. Scope to the active sprint and the
    // strip stands between the toolbar and the columns: name, dates, days
    // left, the bar.
    await page.getByTestId('sprint-scope-active').click()
    await expect(page).toHaveURL(/sst=active/)
    const strip = page.getByTestId('sprint-strip')
    await expect(strip).toBeVisible()
    await expect(page.getByTestId('sprint-strip-name')).toHaveText(/\S/)
    await expect(page.getByTestId('sprint-strip-count')).toHaveText(/\d+ \/ \d+ · \d+%/)
    await expect(page.getByTestId('board-card').first()).toBeVisible()
    await beat(page, 1800)
    // The bar's basis, on hover (G7) — the cursor rests on it long enough to read.
    await page.getByTestId('sprint-strip-bar').hover()
    await beat(page, 1500)

    // Beat 2 — a card that has been carried. The mark is the last thing on
    // the card's meta line; hovering says how many sprints it has sat in.
    const carried = page.getByTestId('board-card-carryover').first()
    await expect(carried).toBeVisible()
    await carried.hover()
    await beat(page, 1600)

    // Beat 3 — the retro, from the palette, the way it is reached.
    await page.keyboard.press('ControlOrMeta+k')
    await beat(page, 500)
    await page.getByTestId('palette-action-retro').click()
    const view = page.getByTestId('retro-view')
    await expect(view).toBeVisible()
    await expect(view).toContainText(T['retro.title'])
    await expect(page.getByTestId('retro-summary')).toBeVisible()
    await expect(page.getByTestId('retro-table')).toBeVisible()
    await expect(page.getByTestId('retro-sparkline').first()).toBeVisible()
    await beat(page, 2400)

    // Beat 4 — the same report cut by sprint: two named columns, the
    // running one marked, the summary retitled to the sprint.
    await page.getByTestId('retro-range').filter({ hasText: T['retro.bySprint'] }).click()
    await expect(page.getByTestId('retro-week')).toHaveCount(2)
    await expect(page.getByTestId('retro-week').last()).toContainText(T['retro.thisSprint'])
    await expect(page.getByTestId('retro-summary-title')).toContainText(T['retro.thisSprint'])
    await beat(page, 2400)

    // Beat 5 — a number is a door. The closed cell of the running sprint
    // opens the issues that closed in it, as a keys view.
    const cell = page.locator('[data-testid="retro-cell"][data-metric="closed"]').last()
    await expect(cell).toBeVisible()
    await cell.hover()
    await beat(page, 800)
    await cell.click()
    await expect(view).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect.poll(() => page.url()).toContain('ks=')
    await expect(page.getByTestId('issue-row-trail').first()).toBeVisible()
    await beat(page, 2200)

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })
})
