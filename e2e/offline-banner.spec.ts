import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * The offline strip: server-reachability made visible (moved from
 * ux-f12.spec.ts in the v0.21 audit ladder round — the audit-placement
 * ux-fNN files were dissolved by surface). Two down classes:
 *  - GDK-477: the network throw (route.abort) while a detail is open.
 *  - GDK-1054: a server ANSWERING 503 to everything, which stayed silent
 *    before (GDK-1025 audit).
 * The /tmp/f12-shots captures both tests carried were deleted with the move —
 * captures are env-gated now, not inline writes.
 */

async function openIssue(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  // Exact row via data-issue-key: hasText('NMA-1') also matches NMA-10/-100,
  // and which one comes first depends on the boot view's ordering (GDK-100
  // made that epic-grouped).
  await page
    .locator(`[data-testid="issue-list-scroller"] [data-issue-key="${key}"]`)
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('offline banner', () => {
  test('GDK-477: a failed request raises the banner now; recovery retries detail', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)
    const panel = await openIssue(page, 'NMB-110')
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()

    let down = true
    await page.route('**/api/v1/issues/**', (route) => {
      if (down) return route.abort('failed')
      return route.continue()
    })

    const input = searchInput(page)
    await input.fill('NMA-1')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMA-1' })
      .first()
      .click()

    await expect(panel.getByTestId('detail-load-error')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('offline-banner')).toBeVisible({ timeout: 3_000 })

    await page.getByTestId('sidebar-sync-now').click()
    const popover = page.getByTestId('sync-history-popover')
    await expect(popover).toBeVisible()
    const offlineLine = popover.getByTestId('sync-history-offline')
    await expect(offlineLine).toBeVisible()
    await expect(offlineLine).toContainText(en['sidebar.serverUnreachable'])
    await expect(popover.getByTestId('sync-history-retry')).toBeVisible()

    down = false
    await popover.getByTestId('sync-history-retry').click()
    await expect(page.getByTestId('offline-banner')).toHaveCount(0)
    await expect(panel.getByTestId('detail-load-error')).toHaveCount(0)
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await expect(panel.getByRole('button', { name: en['common.retry'] })).toHaveCount(0)

    // route.abort() logs net::ERR_FAILED; that is the down signal under test.
    expect(
      errors.filter((e) => !e.includes('ERR_FAILED') && !e.includes('Failed to load resource')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('GDK-1054: an answered 503 raises the offline strip; recovery clears it', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    /*
     * GDK-477's down class was the network throw only. A server ANSWERING
     * 503 to everything stayed silent — no strip on any screen (GDK-1025
     * audit). No click here: the 500ms ui-focus poll carries a raw() 503
     * within seconds, which is exactly the passive path that used to be
     * silent. 4xx is deliberately not routed: those are answers, not
     * outages, and stay per-surface.
     */
    let down = true
    await page.route('**/api/v1/issues/**', (route) => {
      if (!down) return route.continue()
      return route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: '{"error":"service unavailable"}',
      })
    })

    await expect(page.getByTestId('offline-banner')).toBeVisible({ timeout: 10_000 })
    // Debug axis: one DOM inspection names the state (reachability publish).
    await expect(page.locator('html')).toHaveAttribute('data-server-reachability', 'down')

    // The sync popover's offline line lights up off the same signal.
    await page.getByTestId('sidebar-sync-now').click()
    const popover = page.getByTestId('sync-history-popover')
    await expect(popover).toBeVisible()
    await expect(popover.getByTestId('sync-history-offline')).toBeVisible()

    down = false
    await popover.getByTestId('sync-history-retry').click()
    await expect(page.getByTestId('offline-banner')).toHaveCount(0)
    await expect(page.locator('html')).toHaveAttribute('data-server-reachability', 'up')

    // The intercepted 503s log "Failed to load resource"; that is the regime.
    expect(
      errors.filter((e) => !e.includes('Failed to load resource')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
