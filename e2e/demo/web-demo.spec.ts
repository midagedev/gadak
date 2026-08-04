/**
 * Web UI demo recording for docs/media/web-demo.{gif,mp4}.
 *
 * Gated by SCRY_MEDIA=1 so the main e2e suite (which discovers subdirs under
 * e2e/) skips this file and stays at its 10-test baseline. Timing targets
 * ~20–30s of readable motion — no DOM caption overlays (app code stays clean).
 */
import { test, expect, type Page } from '@playwright/test'
import { gotoApp, searchInput } from '../helpers'

const isMedia = !!process.env.SCRY_MEDIA

/** Pause between beats so a human can read the UI. */
async function beat(page: Page, ms = 900): Promise<void> {
  await page.waitForTimeout(ms)
}

/** List toolbar count ("N issues") — not the sidebar pool total. */
function listCount(page: Page) {
  return page.locator('span').filter({ hasText: /^\d+ issues?$/ })
}

test.describe('web UI demo', () => {
  test.skip(!isMedia, 'SCRY_MEDIA=1 only — media pipeline recording')

  test('instant search, filters, palette, detail, saved view', async ({ page }) => {
    // ── 1. Boot: full issue list ──────────────────────────────────────────
    await gotoApp(page)
    await expect(page.getByText('519 issues')).toBeVisible()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await page.waitForLoadState('networkidle')
    await beat(page, 1200)

    // ── 2. Instant search (zero network) + highlight ──────────────────────
    const input = searchInput(page)
    await input.click()
    await beat(page, 300)
    // Type slowly so the list narrowing and <mark> highlights are visible.
    await input.pressSequentially('pagination', { delay: 110 })
    await expect(listCount(page)).toBeVisible()
    await expect(page.locator('mark').first()).toBeVisible({ timeout: 10_000 })
    await beat(page, 1400)

    // Clear search to reset the list for the filter beat.
    await input.fill('')
    await expect(listCount(page)).toBeVisible()
    await beat(page, 500)

    // ── 3. Filter chips → count changes ───────────────────────────────────
    // Boot applies "All open" (Category: New + In progress). Reset, then rebuild
    // filters so the count change is obvious. Demo DB mostly lacks assignee
    // emails, so prefer Priority + Unassigned (stable facet values).
    await page.getByRole('button', { name: 'Reset' }).click()
    await beat(page, 500)
    const countAll = await listCount(page).innerText()

    await page.getByRole('button', { name: '+ Filter' }).click()
    await page.getByRole('button', { name: 'Priority ›', exact: true }).click()
    // First facet row is the most common priority (label + count).
    await page
      .locator('div.absolute button')
      .filter({ has: page.locator('span.text-\\[11px\\]') })
      .first()
      .click()
    await beat(page, 700)
    // Dismiss the floating menu (FilterBar has no Esc handler).
    await input.click()
    await expect(listCount(page)).not.toHaveText(countAll)
    await beat(page, 600)

    await page.getByRole('button', { name: '+ Filter' }).click()
    await page.getByRole('button', { name: 'Unassigned', exact: true }).click()
    await beat(page, 700)
    await input.click()
    await expect(listCount(page)).toBeVisible()
    await beat(page, 1000)

    // Reset so the palette opens against a wide pool again.
    await page.getByRole('button', { name: 'Reset' }).click()
    await beat(page, 400)

    // ── 4. ⌘K command palette → issue detail ──────────────────────────────
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 400)
    await page.keyboard.type('NMB-110', { delay: 90 })
    const first = palette.getByRole('option').first()
    await expect(first).toContainText('NMB-110')
    await beat(page, 700)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    await beat(page, 1000)

    // ── 5. Scroll detail (comments / history) ─────────────────────────────
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await panel.evaluate((el) => {
      const scroller = el.querySelector('.overflow-y-auto') ?? el
      scroller.scrollTo({ top: scroller.scrollHeight * 0.55, behavior: 'smooth' })
    })
    await beat(page, 1000)
    await panel.evaluate((el) => {
      const scroller = el.querySelector('.overflow-y-auto') ?? el
      scroller.scrollTo({ top: scroller.scrollHeight, behavior: 'smooth' })
    })
    await beat(page, 1100)

    // ── 6. Built-in saved view from the sidebar ───────────────────────────
    // Close detail first so the list + count change is the focus.
    await page.keyboard.press('Escape')
    await beat(page, 400)

    // "Stale" is empty on the current demo seed — use Reopened (non-zero).
    const reopenedView = page.getByRole('button', { name: /Reopened/i }).first()
    await reopenedView.click()
    await expect(listCount(page)).toBeVisible()
    await beat(page, 1500)

    // Final rest frame for loop-friendly endings.
    await beat(page, 800)
  })
})
