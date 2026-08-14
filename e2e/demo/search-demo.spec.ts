/**
 * Unified-search palette promo for docs/media/search.{gif,mp4}.
 *
 * Paper list with one project chip, then ⌘K. The typed token is the
 * usearch.spec.ts comment-only fixture (`workaround`) — local title matching
 * cannot see it, so All search (ignores filters) is what fills in, with a
 * Comment match snippet. Enter opens the issue. No DOM caption, no app edits.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1024×640
 * (see search.config.ts) or Playwright letterboxes the capture.
 */
import { test, expect, type Page } from '@playwright/test'
import { forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** usearch.spec.ts: 31 comments / 2 issue bodies; 0 titles, 0 pages. */
const TOKEN = 'workaround'
const LOCAL_PREFIX = 'work'
const TOKEN_REST = 'around'

/** Pause between beats so a human can read the UI. Same default as web-demo. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

test.describe('unified search demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('⌘K all-search finds a comment-only token past the chip', async ({ page }) => {
    await forceLocale(page, 'en')
    // One chip (`Project: NMS`). emptyFilters + pj, not the All-open preset
    // (that preset is two category chips).
    await page.goto('/#/?pj=NMS')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'NMS' })).toBeVisible()
    await expect(page.getByTestId('palette-open')).toBeVisible()
    await expect(page.getByTestId('list-count')).not.toHaveText(/534/)
    await beat(page, 900)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await expect(palette.getByTestId('palette-empty-hint')).toBeVisible()
    await beat(page, 700)

    // Prefix first: local title matching still has "work" (103 titles in the
    // snapshot). The rest of the token clears that section.
    await page.keyboard.type(LOCAL_PREFIX, { delay: 140 })
    await expect(palette.getByTestId('palette-section').first()).toBeVisible()
    await beat(page, 700)

    await page.keyboard.type(TOKEN_REST, { delay: 140 })
    const unified = palette.getByTestId('palette-unified-issue').first()
    await expect(unified).toBeVisible({ timeout: 10_000 })
    const snippet = unified.getByTestId('palette-unified-snippet')
    await expect(snippet).toBeVisible()
    await expect(snippet).toHaveAttribute('data-match-field', 'comment')
    await expect(snippet).toContainText(new RegExp(TOKEN, 'i'))
    await expect(palette.getByTestId('palette-section').filter({ hasText: /all search/i })).toBeVisible()
    // First hit is NMA-36 (not in the NMS chip). Enter that one so the last
    // frame is an NMA issue beside Project: NMS — the ignore-filters claim.
    await beat(page, 1400)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByTestId('title-editor')).toBeVisible()
    await beat(page, 1800)
  })
})
