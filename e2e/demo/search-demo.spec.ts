/**
 * Unified-search palette promo for docs/media/search.{gif,mp4}.
 *
 * Paper list with one project chip, then ⌘K. The typed token is a
 * comment-only one in this take's language (helpers.ts MEDIA_COMMENT_TOKEN;
 * en is the usearch.spec.ts fixture `workaround`) — local title matching
 * cannot see it, so All search is what fills in, with a
 * Comment match snippet. Enter opens the issue. No DOM caption, no app edits.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1024×640
 * (see search.config.ts) or Playwright letterboxes the capture.
 */
import { type Page, type TestInfo } from '@playwright/test'
import { test, expect } from '../helpers'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import {
  catalogFor,
  forceLocale,
  mediaLocale,
  DEMO_ISSUE_COUNT_RE,
  MEDIA_COMMENT_TOKEN,
  MEDIA_LOCALE_STAMP,
} from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/**
 * The UI language this take records in (GADAK_MEDIA_LOCALE, default en).
 * The palette chrome below is read out of that locale's catalog, and the
 * mirror under it is the translated copy the Makefile applied for this locale
 * (GDK-1556) — so the typed token is that locale's word, not an English one
 * left standing in a Korean frame.
 */
const LOCALE = mediaLocale()
const t = catalogFor(LOCALE)

/**
 * The comment-only token, in two halves (helpers.ts MEDIA_COMMENT_TOKEN).
 * Measured on the translated mirrors: en `workaround` — 0 titles, 2 bodies,
 * 31 comments (usearch.spec.ts); ko `임시 방편` — 0 titles, 0 bodies, 15
 * comments; ja `回避策` — 0 titles, 2 bodies, 31 comments. 0 titles is the
 * property the clip depends on: local title matching cannot see the token, so
 * All search is what fills the palette.
 */
const TOKEN = MEDIA_COMMENT_TOKEN[LOCALE].token
const LOCAL_PREFIX = MEDIA_COMMENT_TOKEN[LOCALE].prefix
const TOKEN_REST = TOKEN.slice(LOCAL_PREFIX.length)

/** A fixture string as a literal pattern — ja and ko text is not regex. */
const literal = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

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

    // Prefix first: local title matching still has it (en "work" — 103 titles
    // in the snapshot; ko "임시" — 20; ja "回" — 44). The rest of the token
    // clears that section, which is the beat this clip exists for.
    await page.keyboard.type(LOCAL_PREFIX, { delay: 140 })
    await expect(palette.getByTestId('palette-section').first()).toBeVisible()
    await beat(page, 700)

    await page.keyboard.type(TOKEN_REST, { delay: 140 })
    const unified = palette.getByTestId('palette-unified-issue').first()
    await expect(unified).toBeVisible({ timeout: 10_000 })
    const snippet = unified.getByTestId('palette-unified-snippet')
    await expect(snippet).toBeVisible()
    await expect(snippet).toHaveAttribute('data-match-field', 'comment')
    await expect(snippet).toContainText(new RegExp(literal(TOKEN), 'i'))
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
