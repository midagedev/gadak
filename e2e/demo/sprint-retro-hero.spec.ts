/**
 * The 0.22 sprint + retro hero — one take for the release tweet.
 *
 * Two claims in one cut. The board, scoped to the active sprint, now says
 * what that sprint is in a line of its own: name, dates, days left, a
 * progress bar counted over the whole sprint (GDK-1709). Then the retro,
 * from the palette: the running bucket summarised above the table, a
 * sparkline on every row, the cut switched to sprints, and one number
 * clicked open into the issues behind it (GDK-1712, GDK-1693).
 *
 * No hover beats. Playwright's recording carries no cursor, so a beat
 * whose whole content is a tooltip records as a still — the first take
 * spent three seconds that way (vision pass, 2026-09-09). The carry-over
 * mark (GDK-1711) is in frame on the cards but not narrated for the same
 * reason. And the board stands on the whole sprint, not the open pool: the
 * strip counts done issues, so a Done column emptied by the open-only view
 * read as a contradiction beside "6 / 20 · 30%".
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
    await beat(page, 900)

    // The whole pool as a board, not "My issues" — a sprint board is a team
    // surface, and the strip counts done issues, so the Done column has to
    // hold them. A built-in view's own filters are not chips (GDK-1336), so
    // there is nothing to clear; the view-less board is a route, and the
    // recording never shows the address bar.
    await page.goto('/#/?ly=board')
    await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
    const scope = page.getByTestId('sprint-scope')
    await expect(scope).toBeVisible()
    await expect(scope).toContainText(T['board.scopeAxis'])
    await beat(page, 1500)

    // Beat 1 — the sprint gets its line. Scope to the active sprint and the
    // strip stands between the toolbar and the columns: name, dates, days
    // left, the bar — and the Done column is full beside it.
    await page.getByTestId('sprint-scope-active').click()
    await expect(page).toHaveURL(/sst=active/)
    const strip = page.getByTestId('sprint-strip')
    await expect(strip).toBeVisible()
    await expect(page.getByTestId('sprint-strip-name')).toHaveText(/\S/)
    await expect(page.getByTestId('sprint-strip-count')).toHaveText(/\d+ \/ \d+ · \d+%/)
    await expect(page.getByTestId('board-card').first()).toBeVisible()
    await expect(page.getByTestId('board-card-carryover').first()).toBeVisible()
    await beat(page, 2800)

    // Beat 2 — the retro, from the palette, the way it is reached.
    await page.keyboard.press('ControlOrMeta+k')
    await beat(page, 1000)
    await page.getByTestId('palette-action-retro').click()
    const view = page.getByTestId('retro-view')
    await expect(view).toBeVisible()
    await expect(view).toContainText(T['retro.title'])
    await expect(page.getByTestId('retro-summary')).toBeVisible()
    await expect(page.getByTestId('retro-table')).toBeVisible()
    await expect(page.getByTestId('retro-sparkline').first()).toBeVisible()
    await beat(page, 1600)

    // Beat 3 — the same report cut by sprint: two named columns, the
    // running one marked, the summary retitled to the sprint.
    await page.getByTestId('retro-range').filter({ hasText: T['retro.bySprint'] }).click()
    await expect(page.getByTestId('retro-week')).toHaveCount(2)
    await expect(page.getByTestId('retro-week').last()).toContainText(T['retro.thisSprint'])
    await expect(page.getByTestId('retro-summary-title')).toContainText(T['retro.thisSprint'])
    await beat(page, 1800)

    // Beat 4 — a number is a door. The in-progress cell of the running
    // sprint opens the issues behind it, as a keys view (the fuller list of
    // the two doors in the demo mirror).
    const cell = page.locator('[data-testid="retro-cell"][data-metric="in progress"]').last()
    await expect(cell).toBeVisible()
    await cell.click()
    await expect(view).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect.poll(() => page.url()).toContain('ks=')
    await expect(page.getByTestId('issue-row-trail').first()).toBeVisible()
    await beat(page, 1800)

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })
})
