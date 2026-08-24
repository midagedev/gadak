/**
 * Right-hand half of the claude-drive flagship clip.
 *
 * Loads the promo-split chrome (paper bar + empty terminal | real app iframe)
 * and records until e2e/.tmp/claude-drive/web-stop exists. The terminal pane
 * is discarded at composite time; VHS supplies the left 720×640.
 *
 * Gated by GADAK_MEDIA=1. Does not spawn gadak children — Claude Code in
 * the VHS half is the only writer. The serve tab follows via ui-focus.
 */
import { existsSync, mkdirSync, writeFileSync, appendFileSync } from 'node:fs'
import path from 'node:path'

import { test, expect } from '@playwright/test'

import { forceLocale, DEMO_ISSUE_COUNT_EN_RE } from '../helpers'
import { E2E_DIR, appFrame, promoAppOrigin, promoFrameHtml } from './promo-split'

const isMedia = !!process.env.GADAK_MEDIA

const DIR = path.join(E2E_DIR, '.tmp', 'claude-drive')
const STOP = path.join(DIR, 'web-stop')
const READY = path.join(DIR, 'web-ready-epoch')
const TIMELINE = path.join(DIR, 'web-timeline.jsonl')
const START_EPOCH = path.join(DIR, 'web-start-epoch')

const APP_SRC = `${promoAppOrigin()}/#/?lb=quick-win&sc=new%2Cinprogress`
const FRAME_SEL = 'iframe[data-testid="dashboard-frame"]'

function beat(label: string, extra: Record<string, unknown> = {}): void {
  mkdirSync(DIR, { recursive: true })
  appendFileSync(TIMELINE, JSON.stringify({ t: Date.now(), label, ...extra }) + '\n')
}

test.describe('claude-drive web', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')
  test.describe.configure({ timeout: 600_000 })

  test('record serve tab until VHS stop file', async ({ page }) => {
    mkdirSync(DIR, { recursive: true })
    writeFileSync(TIMELINE, '')
    writeFileSync(START_EPOCH, `${Date.now() / 1000}\n`)

    await forceLocale(page, 'en')
    await page.emulateMedia({ colorScheme: 'light' })
    await page.setContent(promoFrameHtml(APP_SRC), { waitUntil: 'domcontentloaded' })

    const app = page.frameLocator('#app')
    await expect(app.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect(app.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(app.getByTestId('filter-chip').filter({ hasText: /quick-win/ })).toBeVisible({
      timeout: 15_000,
    })

    writeFileSync(READY, `${Date.now() / 1000}\n`)
    beat('web_ready')
    await page.waitForTimeout(800)

    const t0 = Date.now()
    let lastAccent = ''
    let lastChip = ''
    let sawDash = false
    const deadline = t0 + 9 * 60 * 1000

    while (!existsSync(STOP) && Date.now() < deadline) {
      let frame
      try {
        frame = appFrame(page)
      } catch {
        frame = null
      }
      if (frame) {
        try {
          const accent = (
            await frame.evaluate(() =>
              getComputedStyle(document.documentElement).getPropertyValue('--color-accent').trim().toLowerCase(),
            )
          ).trim()
          if (accent && accent !== lastAccent) {
            if (lastAccent) beat('accent_changed', { from: lastAccent, to: accent })
            lastAccent = accent
          }
        } catch {
          /* frame navigated */
        }
        try {
          const chip = app.locator('[data-col="labels"] button').filter({ hasText: 'quick-win' }).first()
          if (await chip.count()) {
            const bg = await chip.evaluate((el) => getComputedStyle(el).backgroundColor)
            if (bg && bg !== lastChip) {
              if (lastChip) beat('label_changed', { from: lastChip, to: bg })
              lastChip = bg
            }
          }
        } catch {
          /* list not in view */
        }
        try {
          const dash = app.locator(FRAME_SEL)
          const n = await dash.count()
          if (n > 0 && !sawDash) {
            sawDash = true
            beat('dashboard_open')
          }
        } catch {
          /* still the list */
        }
      }
      await page.waitForTimeout(400)
    }

    beat('web_stop', { sawDash, lastAccent, lastChip, waitedMs: Date.now() - t0 })
    await page.waitForTimeout(1500)
  })
})
