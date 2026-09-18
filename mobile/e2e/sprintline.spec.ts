// The sprint line (GDK-1867) at 402×874, against `gadak demo` — the same
// fixture every other spec here uses. `examples/demo.db` already holds the
// shape this round needs and was not touched: Sprint 42 is the one active
// sprint (41 closed, 43 future) with 20 issues, 6 of them done.
//
// What this file is for: the numbers and the pick are asserted in
// src/lib/sprint.test.ts, over the same rows, with no browser. This is the
// effect confirmed once — the line is on the screen, it says what the
// snapshot says, and tapping it re-scopes the queue. Nothing here re-measures
// what the pure tests measure, and nothing here asserts the days-left
// sentence: that one moves with the wall clock.
import { expect, test } from './helpers'

const LINE = '[data-testid="sprint-line"]'

test('the active sprint reads as one line above the queue', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  const line = page.locator(`.pane:not(.off) ${LINE}`)
  await expect(line).toHaveCount(1)
  await expect(line).toBeVisible()
  // The sprint's own name, and the snapshot's own arithmetic: 6 of 20 done.
  await expect(line.locator('.name')).toHaveText('Sprint 42')
  await expect(line.locator('.count')).toHaveText('6 / 20 · 30%')
  // The goal has been in the mirror since sprints became rows; this is the
  // first phone surface that reads it.
  await expect(line.locator('.goal')).not.toHaveText('')

  // It sits under the heading and above the first issue row.
  const lineY = (await line.boundingBox())!.y
  const firstRowY = (await page.locator('.pane:not(.off) button.row').first().boundingBox())!.y
  expect(lineY).toBeLessThan(firstRowY)
})

test('tapping the line scopes the queue to that sprint, grouped by category', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  await page.locator(`.pane:not(.off) ${LINE}`).click()

  // The heading is the scope's name and its count — the 20 rows the sprint
  // holds, done ones included: "how far has this come" needs them.
  const heading = page.locator('.pane:not(.off) h1 button.scope')
  await expect(heading.locator('.name')).toHaveText('Active sprint')
  await expect(heading.locator('.count')).toHaveText('·20')

  // Grouped by status category, not by priority. The fixture has rows in all
  // three (10 new · 4 in progress · 6 done).
  //
  // Re-pinned 2026-09-18 (GDK-1993): the header order is the desk's now, not
  // the phone's — in progress first, because the two surfaces stopped keeping
  // separate groupers and the desk's reading of "what is moving, what is
  // left, what landed" leads with what is moving. FAIL-first read
  // `Expected "New" … Received "In progress"` on the first header.
  const groups = page.locator('.pane:not(.off) .section .label')
  await expect(groups).toHaveText(['In progress', 'New', 'Done'])
  await expect(page.locator('.pane:not(.off) .section .n')).toHaveText(['4', '10', '6'])

  // The line is still there, now marked as the current scope.
  await expect(page.locator(`.pane:not(.off) ${LINE}`)).toHaveAttribute('aria-current', 'true')
})

test('the scope picker offers the sprint once, under the built-in section', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  // Wait for the line, not just for a row. `openPicker()` snapshots the row
  // counts once, when the sheet opens — a click that lands before the
  // sprints answer would open a sheet whose sprint row has no count. A
  // painted row already implies it (`issuesBootKind` holds the skeleton
  // until `loaded`, which sync sets after the sprints fetch), but the wait
  // is what makes that ordering the spec's own precondition rather than a
  // fact borrowed from another module on a fast machine.
  await page.locator(`.pane:not(.off) ${LINE}`).waitFor()

  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()

  // One row, wearing the desk's own name for this slice, with the sprint's
  // full count beside it. GDK-1542's defect was two built-in rows answering
  // one question; this one answers a question no other row asks.
  const row = page.locator('button.palette-row', { hasText: 'Active sprint' })
  await expect(row).toHaveCount(1)
  await expect(row.locator('.n')).toHaveText('20')

  await row.click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await expect(page.locator('.pane:not(.off) h1 button.scope .name')).toHaveText('Active sprint')
})

/*
 * The goal reads whole, in a language with no spaces (GDK-1977).
 *
 * The defect was found in the 2026-09-17 vision pass over the ja clip: the
 * line came out `…エスカレーションは SLA 内で対応す…`, cut mid-word, while
 * the same frame in en and ko was not cut. The cause was not the translation
 * — it was that `.goal` clamped to one line. The three goals are the same
 * sentence: 48 Latin characters in English, 32 in Korean of which 22 are
 * full-width, and 34 in Japanese of which 29 are. One line of
 * `--text-micro` at 402px is 370px, which is 30 full-width characters, so
 * the first two fit and the third is two characters over.
 *
 * This asserts the rule and not the string. The served fixture is
 * `examples/demo.db`, whose goal is English and fits on one line either way,
 * so the Japanese goal is injected into the one response that carries it —
 * which also means this keeps measuring after the fixture's prose changes.
 *
 * Two things had to be reproduced, not one, and the first is the reason a
 * first attempt at this test passed on the pre-fix source. A Japanese
 * sentence is only as wide as the font it is drawn in: with the UI in
 * English the CJK falls back to whatever the Latin stack ends in and the
 * same 34 characters measured 333px inside a 370px box — no clipping, no
 * defect. `web/src/lib/i18n/index.ts` sets `documentElement.lang` from the
 * chosen locale and `web/src/app.css` hangs the Japanese font stack off
 * `:lang(ja)`, which draws every full-width glyph at a full em: the same
 * string becomes 13% wider and 29 full-width characters no longer fit in
 * 370px. So the locale travels the road the app reads it by — the same
 * `gadak_locale` init script the clip camera uses — and the `<html lang>`
 * assertion below is the proof it arrived, because a locale that never
 * reached the app reads back as the en-US default and this test would go
 * back to measuring the wrong font.
 *
 * The second is the axis, and it is the reason the height alone will not do.
 * The clamp this replaces was `white-space: nowrap`: it lays the sentence on
 * one line however wide that has to be and hides what does not fit
 * sideways, so on the defect `scrollHeight` EQUALS `clientHeight` (17 and
 * 17) while the width tells (377 against 370). The clamp that replaces it
 * wraps instead, so what a too-long sentence would overflow there is the
 * third line and then the height is what tells. Both are measured, because
 * the two ways of cutting a sentence cut it on different axes. The upper
 * bound is the other half of the contract — two lines, never a block that
 * grows until it pushes the queue off the screen.
 */
const JA_GOAL = 'トリアージの滞留を減らし、エスカレーションは SLA 内で対応する。'

test('a Japanese sprint goal reads whole, in at most two lines', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('gadak_locale', 'ja'))
  await page.route('**/api/v1/issues/sprints/', async (route) => {
    const res = await route.fetch()
    if (res.status() !== 200) {
      await route.fulfill({ response: res })
      return
    }
    const doc = (await res.json()) as { sprints?: { state?: string; goal?: string }[] }
    for (const s of doc.sprints ?? []) {
      if (s.state === 'active') s.goal = JA_GOAL
    }
    await route.fulfill({ response: res, json: doc })
  })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // The font this test is about. Without it the CJK is drawn in a fallback
  // narrow enough that the defect does not reproduce.
  await expect
    .poll(() => page.evaluate(() => document.documentElement.lang))
    .toBe('ja-JP')

  const goal = page.locator(`.pane:not(.off) ${LINE} .goal`)
  await expect(goal).toHaveText(JA_GOAL)

  const box = await goal.evaluate((el) => {
    const line = parseFloat(getComputedStyle(el).lineHeight)
    return {
      scrollW: el.scrollWidth,
      clientW: el.clientWidth,
      scrollH: el.scrollHeight,
      clientH: el.clientHeight,
      line,
    }
  })
  // Nothing is clipped, on either axis.
  //
  // FAIL-first (measured 2026-09-18 at 402px on the pre-fix source, this
  // spec alone):
  //
  //	Error: goal is clipped sideways: 377 > 370
  //
  // and after the fix, 370 of 370 wide and 35 of 35 tall over a 17.4px
  // line — two lines, with the ceiling at 35.8.
  expect(
    box.scrollW,
    `goal is clipped sideways: ${box.scrollW} > ${box.clientW}`,
  ).toBeLessThanOrEqual(box.clientW)
  expect(box.scrollH, `goal is clipped below: ${box.scrollH} > ${box.clientH}`).toBeLessThanOrEqual(
    box.clientH,
  )
  // And the ceiling holds: two lines of chrome, not a paragraph.
  expect(box.clientH).toBeLessThanOrEqual(box.line * 2 + 1)
})
