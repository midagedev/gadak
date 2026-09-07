/**
 * Unified-search palette promo for docs/media/search.{gif,mp4}.
 *
 * Paper list with one project chip, then ⌘K. The typed token is the
 * usearch.spec.ts comment-only fixture (`workaround`) — local title matching
 * cannot see it, so All search is what fills in, with a
 * Comment match snippet. Enter opens the issue. No DOM caption, no app edits.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1024×640
 * (see search.config.ts) or Playwright letterboxes the capture.
 */
import { test, expect, type Page, type TestInfo } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { catalogFor, forceLocale, mediaLocale, DEMO_ISSUE_COUNT_RE, MEDIA_LOCALE_STAMP } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/**
 * The UI language this take records in (GADAK_MEDIA_LOCALE, default en).
 * The palette chrome below is read out of that locale's catalog; the typed
 * token and the issue it lands on stay English because they are fixture
 * data, not UI copy.
 */
const LOCALE = mediaLocale()
const t = catalogFor(LOCALE)

/** usearch.spec.ts: 31 comments / 2 issue bodies; 0 titles, 0 pages. */
const TOKEN = 'workaround'
const LOCAL_PREFIX = 'work'
const TOKEN_REST = 'around'

/**
 * Stamp the locale beside the take.
 *
 * Measured 2026-09-07: running `bash e2e/demo/export-search.sh` with
 * GADAK_MEDIA_LOCALE=ja over a results directory left by an earlier English
 * take produced search.ja.mp4 with English pixels in it, and nothing said so
 * — the export script only ever looked for a video.webm. `make media-search`
 * happens to rm -rf the directory first, so the failure only reaches you
 * when you run the export by hand, which is exactly what MEDIA.md documents.
 * The take now says which language it is, and the export refuses a mismatch.
 */
async function stampLocale(testInfo: TestInfo): Promise<void> {
  await writeFile(join(testInfo.project.outputDir, MEDIA_LOCALE_STAMP), `${LOCALE}\n`, 'utf8')
}

/** Pause between beats so a human can read the UI. Same default as web-demo. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

test.describe('unified search demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('⌘K all-search finds a comment-only token past the chip', async ({ page }, testInfo) => {
    await stampLocale(testInfo)
    await forceLocale(page, LOCALE)
    // One chip (`Project: NMS`). emptyFilters + pj, not the All-open preset
    // (that preset is two category chips).
    await page.goto('/#/?pj=NMS')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'NMS' })).toBeVisible()
    await expect(page.getByTestId('palette-open')).toBeVisible()
    await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_RE)
    await beat(page, 900)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: t['palette.title'] })
    await expect(palette).toBeVisible()
    // GDK-472: empty palette is the placeholder, not a second hint line.
    await expect(palette.getByTestId('palette-empty-hint')).toHaveCount(0)
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
    await expect(
      palette.getByTestId('palette-section').filter({ hasText: t['palette.sectionUnified'] }),
    ).toBeVisible()
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
