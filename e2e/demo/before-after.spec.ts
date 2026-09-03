/**
 * Before/after take for the 0.20 "read" cut (GDK-1253 follow-up).
 *
 * The same choreography is recorded twice — once against a serve of the
 * previous release, once against this one — on the same demo fixture, the same
 * viewport and the same routes, so the two recordings differ only in what the
 * product draws. camera.mjs then wipes one over the other.
 *
 * Beats (each mark carries the rect the camera frames):
 *   1. detail_hold  — NMB-139 open, its long description in the panel
 *      wide_click   — (0.20 only) the wide-reading toggle; the prose takes the column
 *   2. page_hold    — a wiki page with headings and a list
 *   3. board_hold   — the board; dock_open — the terminal dock opens
 *
 * GADAK_MEDIA=1 gated; GADAK_BA_BASE names the serve (record-before-after.sh).
 */
import { test, expect, type Locator, type Page } from '@playwright/test'
import { appendFileSync } from 'node:fs'
import { forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA
const PROOF = process.env.GADAK_BA_PROOF || ''
const AFTER = process.env.GADAK_BA_AFTER === '1'

function mark(name: string, extra: Record<string, unknown> = {}): void {
  if (!PROOF) return
  appendFileSync(PROOF, `${JSON.stringify({ mark: name, epoch_ms: Date.now(), note: '', ...extra })}\n`)
}
async function markAt(name: string, target: Locator): Promise<void> {
  if (!PROOF) return
  const box = await target.boundingBox()
  mark(name, box ? { rect: { x: box.x, y: box.y, w: box.width, h: box.height } } : {})
}
const beat = (page: Page, ms: number) => page.waitForTimeout(ms)

test.describe('before/after take', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the same screens, one release apart', async ({ page }) => {
    test.setTimeout(180_000)
    await forceLocale(page, 'en')
    const vp = page.viewportSize()!
    mark('start', { viewport: { w: vp.width, h: vp.height } })

    // Beat 1 — the issue body.
    await page.goto('/#/?issue=NMB-139')
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible({ timeout: 30_000 })
    await beat(page, 2000)
    await markAt('detail_hold', page.getByTestId('issue-detail-panel'))
    await beat(page, 2500)
    if (AFTER) {
      const wide = page.getByTestId('issue-detail-wide')
      await expect(wide).toBeVisible()
      await markAt('wide_click', wide)
      await wide.click()
      await beat(page, 900)
      await markAt('wide_hold', page.getByTestId('issue-detail-panel'))
      await beat(page, 3000)
      await wide.click()
      await beat(page, 600)
    }

    // Beat 2 — a wiki page.
    await page.goto('/#/?doc=622657')
    await beat(page, 2500)
    await markAt('page_hold', page.locator('main'))
    await beat(page, 2500)

    // Beat 3 — the board, then the dock.
    await page.goto('/#/?sc=new,inprogress,done&g=status_category&ly=board')
    await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
    await beat(page, 1500)
    await markAt('board_hold', page.getByTestId('board'))
    await beat(page, 1200)
    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('terminal', { delay: 40 })
    await beat(page, 300)
    await page.keyboard.press('Enter')
    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    mark('dock_open')
    await beat(page, 1500)
    await markAt('dock_hold', pane)
    await beat(page, 3000)
    mark('end')
  })
})
