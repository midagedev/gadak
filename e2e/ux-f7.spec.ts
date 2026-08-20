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
    await expect(input).toHaveAttribute('placeholder', en['list.searchPlaceholderShort'])
    const fits = await input.evaluate((el: HTMLInputElement) => {
      const cs = getComputedStyle(el)
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d')
      if (!ctx) return false
      ctx.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`.trim()
      return ctx.measureText(el.placeholder).width <= el.clientWidth + 0.5
    })
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
    await expect(searchInput(page)).toHaveAttribute('placeholder', en['list.searchPlaceholder'])
    const docked = await openIssue(page)
    await expect(docked.getByTestId('issue-detail-back')).toHaveCount(0)
    await page.screenshot({ path: '/tmp/f7-shots/1280-list.png' })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
