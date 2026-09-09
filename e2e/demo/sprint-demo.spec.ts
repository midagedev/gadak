/**
 * Sprint promo for docs/media/sprint.{gif,mp4} — the 0.22 release cut.
 *
 * The release's claim is that a sprint is an object now, on every origin,
 * and that the board can be scoped to one. So the take is that sentence and
 * nothing else: the board opens, the scope narrows it to the active sprint,
 * then to the backlog — which is `sprint is EMPTY`, the slice that had no
 * name before — then back to all. Every beat goes through the affordance a
 * person uses, and the URL follows on its own, which is the point being
 * made: the scope is a filter, not board state.
 *
 * No terminal, no CLI, no DOM caption. Gated by GADAK_MEDIA=1. Viewport and
 * video size must stay 1280×800 (sprint.config.ts) or Playwright letterboxes
 * the capture.
 */
import { type Page, type TestInfo } from '@playwright/test'
import { test, expect } from '../helpers'
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

/**
 * The UI language this take records in (GADAK_MEDIA_LOCALE, default en). The
 * chrome asserted below is read out of that locale's catalog rather than
 * restated in English, so the take cannot pass on a frame where the control
 * never switched language. The mirror under it is the copy the Makefile
 * translated for this locale (GDK-1556).
 */
const LOCALE = mediaLocale()
const T = catalogFor(LOCALE)

/**
 * The take says which language it is, and the export refuses a mismatch —
 * otherwise a hand-run export over an older take names English pixels
 * `sprint.ko.mp4` and nothing says so (GDK-1501, measured 2026-09-07).
 */
async function stampLocale(testInfo: TestInfo): Promise<void> {
  await writeFile(join(testInfo.project.outputDir, MEDIA_LOCALE_STAMP), `${LOCALE}\n`, 'utf8')
}

/** Pause between beats so a human can read the UI. Same default as web-demo. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

test.describe('sprint demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the board scopes to a sprint, to the backlog, and back', async ({ page }, testInfo) => {
    await stampLocale(testInfo)
    const errors = attachConsoleErrors(page)
    await forceLocale(page, LOCALE)
    // Not gotoApp(): it pins the locale to en and waits on the English
    // sidebar count, so it can only ever boot an English frame. Boot the way
    // the sibling ko-capable take does (search-demo.spec.ts) and wait on the
    // startup view's own commit instead.
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
    await beat(page, 700)

    // The startup view is "My issues" — dana's own open work, which in this
    // mirror leaves exactly 2 issues inside the active sprint, so the board
    // beat below rendered two cards and two "empty" columns (measured on the
    // first ko take). A sprint board is a team surface: stand on the team's
    // whole open pool first, the way the sidebar offers it.
    // The row's accessible name carries its count too ("전체 미해결 368"), so
    // this matches the label at the start rather than the whole string.
    await page
      .getByRole('button', { name: new RegExp('^' + T['view.allOpen.name']) })
      .first()
      .click()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await beat(page, 900)

    // Into the board the way a person does — the layout switch, not a URL.
    await page.getByTestId('view-settings').click()
    await beat(page, 500)
    await page.getByTestId('layout-board').click()
    await expect(page.getByTestId('board')).toBeVisible()
    // Close the menu so the toolbar (and the scope) is what the frame holds.
    await page.keyboard.press('Escape')
    const scope = page.getByTestId('sprint-scope')
    await expect(scope).toBeVisible()
    // The axis word is this locale's, not English standing in a Korean frame.
    await expect(scope).toContainText(T['board.scopeAxis'])
    await expect(page.getByTestId('sprint-scope-backlog')).toHaveText(T['board.scopeBacklog'])
    await beat(page, 1400)

    // The active segment names the sprint. That is the whole first claim:
    // the mirror knows the sprint by name, not as a string on an issue.
    const active = page.getByTestId('sprint-scope-active')
    await expect(active).toHaveText(/\S/)
    await active.click()
    await expect(page).toHaveURL(/sst=active/)
    await expect(page.getByTestId('board-card').first()).toBeVisible()
    await beat(page, 1800)

    // The backlog: `sprint is EMPTY`, which had no name in the view grammar
    // until this release.
    await page.getByTestId('sprint-scope-backlog').click()
    await expect(page).toHaveURL(/sst=none/)
    await expect(page.getByTestId('board-card').first()).toBeVisible()
    await beat(page, 1800)

    // A filter like any other, so the back button undoes it.
    await page.goBack()
    await expect(page).toHaveURL(/sst=active/)
    await beat(page, 1200)

    await page.getByTestId('sprint-scope-all').click()
    await expect(page.getByTestId('sprint-scope-all')).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByTestId('board-card').first()).toBeVisible()
    await beat(page, 1600)

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })
})
