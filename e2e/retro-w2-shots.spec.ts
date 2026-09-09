/*
 * Capture-only, for the W2 retro round's vision self-verification.
 *
 * Eight frames: the retro screen and the sprint strip, light and dark, en and
 * ko. What each has to show is what this round changed —
 *
 *   · the sprint strip's goal line, which was absent from every recorded
 *     frame because the fixture's sprints had no goal (GDK-1717)
 *   · the retro screen's sprint cut with its board control (GDK-1696/1713)
 *     and the surprises / aging / closed sections under it
 *
 * Set W2_SHOT_DIR to run. Nothing here asserts a look — the frames are read
 * by eye against the round's contract (no clipping, no overlap, no untranslated
 * string, no empty-state sentence where a value belongs).
 */
import { test, expect } from './helpers'
import type { Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

const SHOT_DIR = process.env.W2_SHOT_DIR ?? ''
const VIEW = { width: 1440, height: 900 }

async function prepare(page: Page, locale: string, theme: string): Promise<void> {
  await page.addInitScript(
    ([loc, th]) => {
      try {
        localStorage.setItem('gadak_locale', loc)
        localStorage.setItem('gadak:theme', th)
      } catch {
        /* private mode */
      }
    },
    [locale, theme],
  )
}

/** The theme, stamped after boot: the serve's own settings are light, so a
 *  dark frame asked for through localStorage alone comes back light. */
async function stampTheme(page: Page, theme: string): Promise<void> {
  await page.evaluate((th) => {
    document.documentElement.setAttribute('data-theme', th)
  }, theme)
  // The attribute flips a CSS variable set; one frame is enough for the
  // repaint, and 200ms is that with room on a loaded machine.
  await page.waitForTimeout(200)
}

test.describe('w2 retro captures', () => {
  test('capture', async ({ page }) => {
    test.skip(!process.env.W2_SHOT_DIR, 'capture-only; set W2_SHOT_DIR to run')
    mkdirSync(SHOT_DIR, { recursive: true })

    for (const theme of ['light', 'dark']) {
      for (const locale of ['en', 'ko']) {
        const ctx = await page.context().browser()!.newContext({ viewport: VIEW })
        const p = await ctx.newPage()
        await prepare(p, locale, theme)

        // The sprint strip first: the goal line is the frame's subject, and it
        // only stands on the board under the active-sprint scope.
        await p.goto('/#/?ly=board&sst=active')
        await expect(p.getByTestId('board')).toBeVisible({ timeout: 30_000 })
        await stampTheme(p, theme)
        await expect(p.getByTestId('sprint-strip')).toBeVisible()
        await expect(p.getByTestId('sprint-strip-goal')).toBeVisible()
        // The board's cards fade in; the frame is read for layout, so it waits
        // for that transition to finish rather than catching it mid-way.
        await p.waitForTimeout(400)
        await p.screenshot({ path: join(SHOT_DIR, `strip-${theme}-${locale}.png`) })

        // Then the retro screen, sprint cut, sections unfolded.
        await p.goto('/#/?retro=1')
        await expect(p.getByTestId('retro-view')).toBeVisible({ timeout: 30_000 })
        await stampTheme(p, theme)
        await expect(p.getByTestId('retro-sentence')).toBeVisible()
        const sprintSeg = p.locator('[data-testid="retro-range"][data-range="sprint"]')
        if (await sprintSeg.count()) {
          await sprintSeg.first().click()
          // The sprint cut is a fresh request; 600ms covers the round trip to
          // the local serve and the re-render that follows it.
          await p.waitForTimeout(600)
        }
        // Same repaint settle as the board frame above.
        await p.waitForTimeout(400)
        await p.screenshot({ path: join(SHOT_DIR, `retro-${theme}-${locale}.png`) })

        await ctx.close()
      }
    }
  })
})
