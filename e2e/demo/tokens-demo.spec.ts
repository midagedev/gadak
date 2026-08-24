/**
 * Tokens promo for docs/media/tokens.{gif,mp4} (GDK-795).
 *
 * Left: paper terminal types real `gadak config set` commands.
 * Right: the open tab retints without a reload (500 ms ui-focus poll).
 *
 *   1. accent → #7a4bd0 (~1 s)
 *   2. dataColors label quick-win → #2e7d32
 *   3. locked bg-base refused with the measured error; the tab is untouched
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

const APP_SRC = `${promoAppOrigin()}/#/?lb=quick-win&sc=new%2Cinprogress`

test.describe('tokens promo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('accent, label chip, locked refusal — open tab follows the CLI', async ({ page }) => {
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
    const lockedRan = runTyped(LOCKED_CMD)
    if (lockedRan.status === 0) {
      throw new Error(`locked bg-base should have been refused, got: ${lockedRan.stdout}`)
    }
    await showCli(page, lockedRan.stdout, lockedRan.stderr)
    expect(`${lockedRan.stderr}\n${lockedRan.stdout}`).toMatch(/locked/i)

    await page.waitForTimeout(650)
    expect(
      await frame.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--color-accent').trim().toLowerCase(),
      ),
    ).toBe('#7a4bd0')
    beat('tokens', 'locked_shown', { port: PROMO_PORT })
    await page.waitForTimeout(3200)
  })
})
