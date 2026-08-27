import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * GDK-128: trailing list-row fields must share an x so the eye can scan a
 * column. The unmodified row is one inline flex after the summary — elapsed
 * and parent-key x wandered by hundreds of pixels. This spec reads real
 * geometry (getBoundingClientRect().x), not appearance.
 *
 * FAIL-first: the wide-viewport spread assertion is red on the inline-flow
 * row. The narrow-width case asserts a field is dropped (display:none), not
 * squeezed — that hide already existed (epic went at 1024px).
 *
 * What drops epic changed in GDK-1046: the row's own width, not the
 * viewport's (`.trail-break-epic`, row ≤749). The assertions below are
 * mechanism-blind and stayed green through that move — they ask whether the
 * field is gone, never how. Containment is a different axis and lives in
 * e2e/list-row-overflow.spec.ts: a column can share an x with every other
 * row and still be painted outside the scroller, which is what this spec
 * passed through for months.
 */

const SPREAD_MAX_PX = 2

/** Visible issue rows only — virtual list, group headers have no key. */
async function fieldXs(page: Page, field: string): Promise<number[]> {
  return page.evaluate((name) => {
    const scroller = document.querySelector('[data-testid="issue-list-scroller"]')
    if (!scroller) return []
    const xs: number[] = []
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const box = row.getBoundingClientRect()
      if (box.bottom < 0 || box.top > innerHeight) continue

      const slotted = row.querySelector<HTMLElement>(`[data-col="${name}"]`)
      let el: HTMLElement | null = null
      if (slotted) {
        el = slotted
      } else if (name === 'updated') {
        // Unmodified row: updated is the last w-10 (created is off by default).
        const times = row.querySelectorAll<HTMLElement>('.w-10')
        el = times[times.length - 1] ?? null
      } else if (name === 'epic') {
        el = row.querySelector<HTMLElement>('[data-testid="epic-chip"]')
      }
      if (!el) continue
      const style = getComputedStyle(el)
      if (style.display === 'none' || style.visibility === 'hidden') continue
      xs.push(el.getBoundingClientRect().x)
    }
    return xs
  }, field)
}

function spreadPx(xs: number[]): number {
  if (xs.length === 0) return 0
  return Math.max(...xs) - Math.min(...xs)
}

test.describe('list row trailing columns', () => {
  test('updated (and epic, when present) share an x across rows', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1600, height: 900 })
    await gotoApp(page)
    await expect(
      page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
    ).toBeVisible()

    const updated = await fieldXs(page, 'updated')
    expect(updated.length, 'need many rendered updated cells').toBeGreaterThan(8)
    const updatedSpread = spreadPx(updated)
    expect(
      updatedSpread,
      `updated x spread ${updatedSpread.toFixed(2)}px (n=${updated.length}) xs=${updated.map((x) => x.toFixed(1)).join(',')}`,
    ).toBeLessThanOrEqual(SPREAD_MAX_PX)

    const epic = await fieldXs(page, 'epic')
    if (epic.length >= 2) {
      const epicSpread = spreadPx(epic)
      expect(
        epicSpread,
        `epic x spread ${epicSpread.toFixed(2)}px (n=${epic.length}) xs=${epic.map((x) => x.toFixed(1)).join(',')}`,
      ).toBeLessThanOrEqual(SPREAD_MAX_PX)
    }

    const labels = await fieldXs(page, 'labels')
    if (labels.length >= 2) {
      const labelsSpread = spreadPx(labels)
      expect(
        labelsSpread,
        `labels x spread ${labelsSpread.toFixed(2)}px (n=${labels.length})`,
      ).toBeLessThanOrEqual(SPREAD_MAX_PX)
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a narrow viewport drops the lg-only epic field instead of squeezing it', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 900, height: 800 })
    await gotoApp(page)
    await expect(
      page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
    ).toBeVisible()

    // Epic drops below a row width of 750 (GDK-1046; it was viewport
    // `lg:`/1024 when this was written). 900 must drop the column, not
    // squeeze it. Painted rects, not the chip's own display: the button no
    // longer carries the breakpoint (the slot does).
    const visibleEpic = await page.evaluate(() => {
      const chips = [...document.querySelectorAll<HTMLElement>('[data-testid="epic-chip"]')].filter(
        (el) => el.getClientRects().length > 0,
      ).length
      const slots = [...document.querySelectorAll<HTMLElement>('[data-col="epic"]')].filter((el) => {
        const s = getComputedStyle(el)
        return s.display !== 'none' && s.visibility !== 'hidden'
      }).length
      return { chips, slots }
    })
    expect(visibleEpic.chips, 'epic chip must not paint below lg').toBe(0)
    expect(visibleEpic.slots, 'epic slot must be display:none below lg').toBe(0)

    const updated = await fieldXs(page, 'updated')
    expect(updated.length, 'updated remains at 900px').toBeGreaterThan(4)
    const updatedSpread = spreadPx(updated)
    expect(
      updatedSpread,
      `updated x spread at 900px ${updatedSpread.toFixed(2)}px (n=${updated.length})`,
    ).toBeLessThanOrEqual(SPREAD_MAX_PX)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
