import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * F13 UX defects (GDK-486, GDK-483).
 *
 * Each test waits on the state it names — not a proxy flag. Captures for the
 * visual pass land in /tmp/f13-shots/ after the assertions hold.
 */

async function openSyncHistory(page: Page) {
  const row = page.getByTestId('sidebar-sync-now')
  await expect(row).toBeVisible()
  await row.click()
  const popover = page.getByTestId('sync-history-popover')
  await expect(popover).toBeVisible()
  return popover
}

test.describe('F13 sidebar freshness and section affordances', () => {
  test('GDK-486: popover Last checked reads the same origin as the chip', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    const chip = page.getByTestId('freshness-chip')
    const row = page.getByTestId('sidebar-sync-now')
    await expect(chip).not.toHaveAttribute('data-state', 'syncing', { timeout: 30_000 })
    // F7: the row stays the history entry; Last checked lives in the popover.
    await expect(row).toContainText(en['sidebar.syncHistory'])
    await expect(row).not.toContainText(/Last checked/)

    const popover = await openSyncHistory(page)
    const line = popover.getByTestId('sync-history-last-checked')
    await expect(line).toBeVisible()
    const prefix = en['sidebar.syncLastChecked'].split('{when}')[0]
    await expect(line).toContainText(prefix)
    await expect(line).toHaveText(/Last checked .+/)

    await page.screenshot({ path: '/tmp/f13-shots/486-popover-last-checked.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-486: a server without last_checked_at hides the line', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    await page.route('**/api/v1/issues/sync/runs/**', async (route) => {
      if (route.request().url().includes('source=confluence')) return route.continue()
      const res = await route.fetch()
      const doc = (await res.json()) as { last_checked_at?: unknown; runs?: unknown }
      delete doc.last_checked_at
      return route.fulfill({ json: doc })
    })

    const popover = await openSyncHistory(page)
    await expect(popover.getByTestId('sync-history-last-checked')).toHaveCount(0)
    await expect(popover).toContainText(en['sidebar.syncHistory'])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-483: reorderable section headers show a grip on hover', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    const header = page.getByTestId('sidebar-section-header-builtin')
    await expect(header).toBeVisible()
    const grip = header.getByTestId('sidebar-section-grip')
    await expect(grip).toBeAttached()
    await expect(grip).toHaveCSS('opacity', '0')

    await header.hover()
    await expect(grip).toHaveCSS('opacity', '1')
    await expect(header).toHaveAttribute('title', en['sidebar.sectionReorderHint'])

    // Non-collapsible personalization blocks are not SidebarSection: no grip.
    // The Feed row carries the same clause the removed "My Issues" heading
    // did (2026-09-07 sidebar subtraction): it lives in the personalization
    // block, which must not grow a grip. Aside-scoped and role-matched —
    // not the "My issues" built-in view button below it (my-work pack),
    // which lives inside the builtin SidebarSection.
    const feedRow = page.locator('aside').getByRole('button', { name: 'Feed' })
    await expect(feedRow).toBeVisible()
    await expect(
      feedRow.locator('xpath=ancestor::div[1]').getByTestId('sidebar-section-grip'),
    ).toHaveCount(0)

    await page.locator('aside.issue-sidebar').screenshot({ path: '/tmp/f13-shots/483-sidebar-hover-grip.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
