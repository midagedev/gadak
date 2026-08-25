/**
 * Tokens promo for docs/media/tokens.{gif,mp4} (GDK-795).
 *
 * Left: paper terminal types real `gadak config set` commands.
 * Right: the open tab retints without a reload (500 ms ui-focus poll).
 *
 *   1. accent → #7a4bd0 (~1 s)
 *   2. dataColors label quick-win → #2e7d32
 *   3. locked bg-base warns on stderr AND applies — the tab turns black
 *      (GDK-858: judgment warns, never refuses; scene 4 recovers the page)
 *   4. spacing row → 50px — rows repaint taller in place (dimensions, GDK-842)
 *
 * Gated by GADAK_MEDIA=1. Viewport must stay FRAME_W×FRAME_H (promo-split.ts)
 * or Playwright letterboxes the capture.
 */
import { writeFileSync } from 'node:fs'

import { test, expect } from '@playwright/test'

import { forceLocale, DEMO_ISSUE_COUNT_EN_RE } from '../helpers'
import {
  PROMO_PORT,
  assertFrozen,
  appFrame,
  beat,
  clearTerminal,
  freezePromoHome,
  promoAppOrigin,
  promoFrameHtml,
  runTyped,
  showCli,
  timelinePath,
  typeCommand,
  writeTrimSeconds,
} from './promo-split'

const isMedia = !!process.env.GADAK_MEDIA

const ACCENT_CMD = `gadak config set ui.tokens '{"colors":{"accent":"#7a4bd0"}}'`
const LABEL_CMD = `gadak config set ui.dataColors '{"label":{"quick-win":"#2e7d32"}}'`
const LOCKED_CMD = `gadak config set ui.tokens '{"colors":{"bg-base":"#000000"}}'`
// Scene 4 (dimension axis). The colors ride along because `config set
// ui.tokens` replaces the whole token object — a spacing-only write would
// revert scene 1's accent mid-take, and re-stating accent drops scene 3's
// bg-base, recovering the paper page for the rest of the take. Row is 50px
// because the relation row-excerpt ≥ row + 8px judges the effective set:
// the default row-excerpt of 59px caps row at 51px; 52px still saves under
// GDK-858 but lands with a range warning (GDK-857 ladder teaching).
const DIMS_CMD = `gadak config set ui.tokens '{"colors":{"accent":"#7a4bd0"},"spacing":{"row":"50px"}}'`

const APP_SRC = `${promoAppOrigin()}/#/?lb=quick-win&sc=new%2Cinprogress`

test.describe('tokens promo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('accent, label chip, locked refusal, row height — open tab follows the CLI', async ({ page }) => {
    writeFileSync(timelinePath('tokens'), '')
    const t0 = Date.now()
    freezePromoHome()
    assertFrozen()
    beat('tokens', 'frozen')

    await forceLocale(page, 'en')
    await page.emulateMedia({ colorScheme: 'light' })
    await page.setContent(promoFrameHtml(APP_SRC), { waitUntil: 'domcontentloaded' })

    const app = page.frameLocator('#app')
    await expect(app.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect(app.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(app.getByTestId('filter-chip').filter({ hasText: /quick-win/ })).toBeVisible({
      timeout: 15_000,
    })
    writeTrimSeconds('tokens', Math.max(0.16, (Date.now() - t0) / 1000 - 0.45))
    beat('tokens', 'web_ready')
    await page.waitForTimeout(1400)

    await typeCommand(page, ACCENT_CMD, 42)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)
    beat('tokens', 'enter_accent')
    const accentRan = runTyped(ACCENT_CMD)
    if (accentRan.status !== 0) {
      throw new Error(`accent set failed: ${accentRan.stderr || accentRan.stdout}`)
    }
    await showCli(page, accentRan.stdout, accentRan.stderr)

    const frame = appFrame(page)
    await expect
      .poll(
        async () =>
          frame.evaluate(() =>
            getComputedStyle(document.documentElement).getPropertyValue('--color-accent').trim().toLowerCase(),
          ),
        { timeout: 2_500 },
      )
      .toBe('#7a4bd0')
    beat('tokens', 'accent_reflected')
    await page.waitForTimeout(2000)

    await clearTerminal(page)
    await page.waitForTimeout(280)
    await typeCommand(page, LABEL_CMD, 40)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)
    beat('tokens', 'enter_label')
    const labelRan = runTyped(LABEL_CMD)
    if (labelRan.status !== 0) {
      throw new Error(`dataColors set failed: ${labelRan.stderr || labelRan.stdout}`)
    }
    await showCli(page, labelRan.stdout, labelRan.stderr)

    const chip = app.locator('[data-col="labels"] button').filter({ hasText: 'quick-win' }).first()
    await expect(chip).toBeVisible()
    await expect
      .poll(async () => chip.evaluate((el) => getComputedStyle(el).backgroundColor), { timeout: 2_500 })
      .toMatch(/rgba?\(\s*46,\s*125,\s*50/)
    beat('tokens', 'label_reflected')
    await page.waitForTimeout(2000)

    await clearTerminal(page)
    await page.waitForTimeout(280)
    await typeCommand(page, LOCKED_CMD, 40)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)
    beat('tokens', 'enter_locked')
    // GDK-858: the locked tier is a judgment — exit 0, the warning on stderr
    // keeps the measured diagnostics, and the value APPLIES: the open tab's
    // ground turns black on the next poll.
    const lockedRan = runTyped(LOCKED_CMD)
    if (lockedRan.status !== 0) {
      throw new Error(`locked bg-base should warn and save, got: ${lockedRan.stderr || lockedRan.stdout}`)
    }
    await showCli(page, lockedRan.stdout, lockedRan.stderr)
    expect(`${lockedRan.stderr}\n${lockedRan.stdout}`).toMatch(/locked/i)

    await expect
      .poll(
        async () =>
          frame.evaluate(() =>
            getComputedStyle(document.documentElement).getPropertyValue('--color-bg-base').trim().toLowerCase(),
          ),
        { timeout: 2_500 },
      )
      .toBe('#000000')
    beat('tokens', 'locked_shown', { port: PROMO_PORT })
    await page.waitForTimeout(3200)

    // Scene 4 — dimension axis (GDK-842): spacing.row 50px. The CSS var
    // moves AND the row box itself grows — the virtual list reads the row
    // tokens (rowMetrics), so paint and scroll geometry move together.
    const issueRow = app.locator('[data-testid="issue-list-scroller"] [role="button"]').first()
    await expect(issueRow).toBeVisible()
    expect(
      await frame.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--spacing-row').trim(),
      ),
    ).toBe('42px')
    expect(Math.round(await issueRow.evaluate((el) => el.getBoundingClientRect().height))).toBe(42)

    await clearTerminal(page)
    await page.waitForTimeout(280)
    await typeCommand(page, DIMS_CMD, 36)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)
    beat('tokens', 'enter_dims')
    const dimsRan = runTyped(DIMS_CMD)
    if (dimsRan.status !== 0) {
      throw new Error(`dims set failed: ${dimsRan.stderr || dimsRan.stdout}`)
    }
    await showCli(page, dimsRan.stdout, dimsRan.stderr)

    await expect
      .poll(
        async () =>
          frame.evaluate(() =>
            getComputedStyle(document.documentElement).getPropertyValue('--spacing-row').trim(),
          ),
        { timeout: 2_500 },
      )
      .toBe('50px')
    await expect
      .poll(
        async () =>
          Math.round(await issueRow.evaluate((el) => el.getBoundingClientRect().height)),
        { timeout: 2_500 },
      )
      .toBe(50)
    beat('tokens', 'dims_reflected')
    await page.waitForTimeout(2000)
  })
})
