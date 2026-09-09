import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { attachConsoleErrors, forceLocale, gotoApp } from './helpers'

/*
 * GDK-1709 / GDK-1711 — the sprint strip and the carry-over mark.
 *
 * The strip is the active sprint given a line of its own between the toolbar
 * and the columns: name, goal, dates, days left, and a two-segment bar in
 * the board's own category colours. It exists only while the board is scoped
 * to exactly one active sprint, which on the demo fixture is Sprint 42
 * (2026-09-02 → 2026-09-16, 20 issues: 6 done, 4 in progress, 10 to do —
 * measured against examples/demo.db, not against the pool this tab loaded).
 *
 * The counts come from GET /api/v1/issues/sprints/, so they describe the whole
 * sprint. The board beside them is the *open* pool narrowed by the scope, and
 * the two numbers are allowed to differ — that difference is the reason the
 * server does the counting.
 *
 * The carry-over mark reads issues_raw.carryover_count (13 of Sprint 42's
 * issues carry one on the fixture). Null, never 0, on an origin with no
 * changelog, so an unmarked card is not a claim.
 */

/* Captures happen only when a round asks for them by naming a directory
 * (GDK-1570, e2e/capture-guard.unit.ts): CI runs this file for its
 * assertions, and nobody consumes PNGs there. */
const SHOTS = process.env.SPRINT_SHOT_DIR ?? ''

/*
 * Board, scoped to the active sprint — the one state the strip renders in.
 *
 * By address, not by clicking: `gotoApp` waits on the English pool count
 * ("534 issues"), which never appears on a ko or ja boot, and this helper is
 * what the three-locale captures below come through. Same hash the CLI's
 * `views open --jql 'sprint in openSprints()'` writes (internal/jql/hash.go).
 */
async function openScopedBoard(page: Page): Promise<void> {
  await page.goto('/#/?ly=board&sst=active')
  await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
  await expect(page).toHaveURL(/sst=active/)
}

test.describe('GDK-1709 sprint strip', () => {
  test('appears only under the active scope and reports the whole sprint', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await gotoApp(page)
    await page.getByTestId('view-settings').click()
    await page.getByTestId('layout-board').click()
    await expect(page.getByTestId('board')).toBeVisible()

    // "All" is not a sprint: nothing to describe, nothing drawn.
    await expect(page.getByTestId('sprint-strip')).toHaveCount(0)

    await page.getByTestId('sprint-scope-active').click()
    const strip = page.getByTestId('sprint-strip')
    await expect(strip).toBeVisible()
    await expect(page.getByTestId('sprint-strip-name')).toHaveText('Sprint 42')

    // The whole sprint, from the server — 20 issues, 6 of them done. The
    // board's own card count is the open pool under the same scope and is
    // deliberately a different number.
    await expect(page.getByTestId('sprint-strip-count')).toHaveText('6 / 20 · 30%')

    // The bar's two filled segments, in the order work moves, widths from
    // the same counts as the text beside them. To do is the bare track (the
    // vision pass read a painted third segment as a blue ribbon before it
    // read the bar as 30% done), so a 'new' segment is a regression here.
    const bar = page.getByTestId('sprint-strip-bar')
    await expect(bar).toHaveAttribute('title', 'done 6 · in progress 4 · to do 10')
    await expect(bar.locator('[data-segment="new"]')).toHaveCount(0)
    for (const [seg, n] of [
      ['done', 6],
      ['inprogress', 4],
    ] as const) {
      const el = bar.locator(`[data-segment="${seg}"]`)
      await expect(el).toBeVisible()
      const width = await el.evaluate((e) => (e as HTMLElement).style.width)
      expect(width).toBe(`${(n / 20) * 100}%`)
    }

    // The dates line carries both bounds and a days phrase, never a "D-n".
    const dates = await page.getByTestId('sprint-strip-dates').textContent()
    expect(dates).toMatch(/2026/)
    expect(dates).not.toMatch(/D-/)

    // Back to All and the strip goes away rather than describing a board
    // that is no longer one sprint.
    await page.getByTestId('sprint-scope-all').click()
    await expect(page.getByTestId('sprint-strip')).toHaveCount(0)

    expect(errors).toEqual([])
  })

  test('the bar wears the status-category tokens, not a palette of its own', async ({ page }) => {
    await forceLocale(page, 'en')
    await openScopedBoard(page)
    await expect(page.getByTestId('sprint-strip')).toBeVisible()

    // The board's own column headers draw no category dots when the board is
    // already grouped by category (BoardView passes showCategoryCounts=false),
    // so there is no sibling element to read the colour off. What can be
    // asserted — and what the contract actually is — is that each segment
    // resolves to the same paint as the status-category token every other
    // category mark on the app uses. A second palette invented for this bar
    // is what this fails on; it does not prove the two elements share a
    // function, which the vision pass checks by eye.
    const tokens = await page.evaluate(() => {
      const read = (v: string) => {
        const probe = document.createElement('span')
        probe.style.background = `var(${v})`
        document.body.append(probe)
        const c = getComputedStyle(probe).backgroundColor
        probe.remove()
        return c
      }
      return {
        done: read('--color-status-done'),
        inprogress: read('--color-status-inprogress'),
      } as Record<string, string>
    })
    const segs = await page
      .getByTestId('sprint-strip-bar')
      .locator('[data-segment]')
      .evaluateAll((els) =>
        Object.fromEntries(
          els.map((e) => [
            (e as HTMLElement).dataset.segment ?? '',
            getComputedStyle(e).backgroundColor,
          ]),
        ),
      )
    expect(Object.keys(segs).sort()).toEqual(['done', 'inprogress'])
    for (const cat of Object.keys(segs)) {
      expect(segs[cat], `${cat} must be the status-category token's own paint`).toBe(tokens[cat])
    }
  })

  test('GDK-1711 the carry-over mark is on the cards that were carried', async ({ page }) => {
    await forceLocale(page, 'en')
    await openScopedBoard(page)
    const marks = page.getByTestId('board-card-carryover')
    // 13 of Sprint 42's 20 issues carry one; the board shows the open subset,
    // so this is "some, not all, and never on every card".
    const n = await marks.count()
    expect(n).toBeGreaterThan(0)
    expect(n).toBeLessThan(await page.getByTestId('board-card').count())
    await expect(marks.first()).toHaveAttribute('title', /Carried over from/)
  })
})

/*
 * Captures. Six frames (three locales × two themes) plus a card close-up, for
 * the visual contract: does the strip read as a line of the board's chrome,
 * do ko and ja fit without truncation, does the bar hold contrast in dark.
 */
test.describe('sprint strip captures', () => {
  test('capture the list row mark', async ({ page }) => {
    test.skip(!process.env.SPRINT_SHOT_DIR, 'capture-only; set SPRINT_SHOT_DIR to shoot')
    mkdirSync(SHOTS, { recursive: true })
    // The row is the mark's other surface, and it folds where `assignee` and
    // `updated` do — this frame is the wide row, where it paints.
    await page.setViewportSize({ width: 1440, height: 900 })
    await forceLocale(page, 'en')
    await page.goto('/#/?sst=active')
    await expect(page.getByTestId('issue-row-carryover').first()).toBeVisible({ timeout: 30_000 })
    await page.screenshot({ path: join(SHOTS, 'list-carryover.png') })
  })

  for (const locale of ['en', 'ko', 'ja'] as const) {
    for (const scheme of ['light', 'dark'] as const) {
      test(`capture ${locale} ${scheme}`, async ({ page }) => {
        test.skip(!process.env.SPRINT_SHOT_DIR, 'capture-only; set SPRINT_SHOT_DIR to shoot')
        mkdirSync(SHOTS, { recursive: true })
        await page.emulateMedia({ colorScheme: scheme })
        await forceLocale(page, locale)
        await openScopedBoard(page)
        await expect(page.getByTestId('sprint-strip')).toBeVisible()
        await page.screenshot({ path: join(SHOTS, `board-${locale}-${scheme}.png`) })
        await page
          .getByTestId('sprint-strip')
          .screenshot({ path: join(SHOTS, `strip-${locale}-${scheme}.png`) })
        if (locale === 'en' && scheme === 'light') {
          const card = page
            .getByTestId('board-card')
            .filter({ has: page.getByTestId('board-card-carryover') })
            .first()
          await card.screenshot({ path: join(SHOTS, 'card-carryover.png') })
        }
      })
    }
  }
})
