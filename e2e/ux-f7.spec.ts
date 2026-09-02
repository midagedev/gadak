import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * F7 UX defects (GDK-460, GDK-461, GDK-462, GDK-463).
 *
 * Each test waits on the state it names — not a proxy flag. Captures for the
 * visual pass land in /tmp/f7-shots/ after the assertions hold.
 */

const NONSENSE = 'zzzznotanissue999'
const DRAFT = 'f7-esc-draft-must-survive'
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

test.describe('F7 UX defects', () => {
  test('GDK-461: zero palette matches do not default Enter to create', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page.getByTestId('freshness-chip')).not.toHaveAttribute('data-state', 'syncing', {
      timeout: 30_000,
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await palette.getByRole('combobox').fill(NONSENSE)

    await expect(palette.getByTestId('palette-unified-empty')).toBeVisible()
    const createNow = palette.getByTestId('palette-create-now')
    await expect(createNow).toBeVisible()
    await expect(createNow).toHaveAttribute('aria-selected', 'false')
    await expect(palette.locator('[role="option"][aria-selected="true"]')).toHaveCount(0)
    // One create entry: the typed-summary row. New issue is the empty-query
    // action and must not be force-appended beside it.
    await expect(palette.getByTestId('palette-new-issue')).toHaveCount(0)

    await page.keyboard.press('Enter')
    await expect(palette).toBeVisible()
    await expect(page.getByRole('dialog', { name: /new issue/i })).toHaveCount(0)

    await page.keyboard.press('ArrowDown')
    await expect(createNow).toHaveAttribute('aria-selected', 'true')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-462: Esc in the comment composer blurs; the next Esc closes', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const panel = await openIssue(page)

    const composer = panel.getByTestId('comment-composer')
    await expect(composer).toBeVisible()
    await composer.fill(DRAFT)
    await expect(composer).toBeFocused()

    await page.keyboard.press('Escape')
    await expect(panel).toBeVisible()
    await expect(composer).not.toBeFocused()
    await expect(composer).toHaveValue(DRAFT)

    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()

    const reopened = await openIssue(page)
    await expect(reopened.getByTestId('comment-composer')).toHaveValue(DRAFT)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-460: freshness copy lives on the chip; the sidebar is Sync history', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const chip = page.getByTestId('freshness-chip')
    const row = page.getByTestId('sidebar-sync-now')
    await expect(chip).not.toHaveAttribute('data-state', 'syncing', { timeout: 30_000 })

    await expect(row).toContainText(en['sidebar.syncHistory'])
    await expect(row).not.toContainText(/Sync delayed|Synced |ago/)
    const chipText = ((await chip.textContent()) ?? '').trim()
    expect(chipText.length, 'the chip still names the mirror age').toBeGreaterThan(0)
    expect(chipText).not.toBe(en['sidebar.syncHistory'])
    // Chip click is sync (now, or retry after a failed pass). Either sentence
    // is the chip's job; the sidebar's is the history popover.
    await expect(chip).toHaveAttribute('title', /Click to (sync now|retry)/)

    await row.click()
    const popover = page.getByTestId('sync-history-popover')
    await expect(popover).toBeVisible()
    await expect(popover).toContainText(en['sidebar.syncHistory'])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

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
    await page.screenshot({ path: '/tmp/f7-shots/900-list.png' })

    const panel = await openIssue(page)
    const back = panel.getByTestId('issue-detail-back')
    await expect(back).toBeVisible()
    await expect(back).toHaveAttribute('aria-label', en['feed.backToList'])
    await page.screenshot({ path: '/tmp/f7-shots/900-detail.png' })

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
    await page.screenshot({ path: '/tmp/f7-shots/1280-list.png' })

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
