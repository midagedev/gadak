import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * The search field's placeholder: scope named, never clipped (moved from
 * ux-f7.spec.ts in the v0.21 audit ladder round — the audit-placement ux-fNN
 * files were dissolved by surface). GDK-463's overlay-detail half and
 * GDK-1056's width switch share one invariant, so they live together. The
 * /tmp/f7-shots captures were deleted with the move.
 */

const KEY = 'NMB-110'

async function openIssue(page: import('@playwright/test').Page) {
  const input = searchInput(page)
  await input.fill(KEY)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: KEY })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

/** The placeholder must fit the input's client box — the invariant behind
 *  both the GDK-463 and GDK-1056 switches: never a clipped intermediate. */
async function placeholderFits(page: import('@playwright/test').Page) {
  const input = searchInput(page)
  return input.evaluate((el: HTMLInputElement) => {
    const cs = getComputedStyle(el)
    const canvas = document.createElement('canvas')
    const ctx = canvas.getContext('2d')
    if (!ctx) return false
    ctx.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`.trim()
    return ctx.measureText(el.placeholder).width <= el.clientWidth + 0.5
  })
}

test.describe('search placeholder', () => {
  test('GDK-463: 900px placeholder fits; overlay detail has a back control', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 900, height: 800 })
    await gotoApp(page)

    const input = searchInput(page)
    // GDK-1336: at 900 the one-row toolbar wraps and the field gets a line of
    // its own, so the long string fits here now; which string shows follows
    // the field's width (GDK-1056). The invariant is scope named + unclipped.
    await expect(input).toHaveAttribute('placeholder', /search this list/i)
    const fits = await placeholderFits(page)
    expect(fits, '900px placeholder must render without clipping').toBe(true)

    const panel = await openIssue(page)
    const back = panel.getByTestId('issue-detail-back')
    await expect(back).toBeVisible()
    await expect(back).toHaveAttribute('aria-label', en['feed.backToList'])

    await expect(page.getByTestId('issue-scrim')).toBeVisible()
    await back.click()
    await expect(panel).toBeHidden()

    const again = await openIssue(page)
    await page.getByTestId('issue-scrim').click({ position: { x: 300, y: 400 } })
    await expect(again).toBeHidden()

    await page.setViewportSize({ width: 1280, height: 800 })
    // GDK-1336: the toolbar is one row, so the field is narrower at 1280 than
    // the long placeholder. The contract is that the placeholder names its
    // scope and fits — which of the two strings that is follows the width.
    await expect(searchInput(page)).toHaveAttribute('placeholder', /search this list/i)
    expect(await placeholderFits(page), '1280px placeholder must render without clipping').toBe(true)
    const docked = await openIssue(page)
    await expect(docked.getByTestId('issue-detail-back')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-1056: the placeholder names its scope and never clips, docked or not (1440)', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)

    const input = searchInput(page)
    // The switch is measured on the field, not the viewport (GDK-1056). Since
    // GDK-1336 the one-row toolbar already leaves the 1440 field below the long
    // string, so the switch cannot be observed as long→short here any more;
    // what remains observable is the invariant it protected: at every width,
    // docked or not, the placeholder names the scope and renders unclipped.
    await expect(input).toHaveAttribute('placeholder', /search this list/i)
    expect(await placeholderFits(page), '1440 placeholder must render without clipping').toBe(true)

    const panel = await openIssue(page)
    // 1440 docks the panel (no overlay back control) — the input shrinks
    // while the viewport does not.
    await expect(panel.getByTestId('issue-detail-back')).toHaveCount(0)
    await expect(input).toHaveAttribute('placeholder', en['list.searchPlaceholderShort'])
    const fits = await placeholderFits(page)
    expect(fits, '1440+docked-panel placeholder must render without clipping').toBe(true)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
