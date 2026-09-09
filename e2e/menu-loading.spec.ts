import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'
import { MENU_ORIGIN_TIMEOUT_MS } from '../web/src/lib/menu-loading'

/*
 * GDK-1566: a menu that waits on the origin must not sit on a bare
 * "Loading…". The write-path catalog endpoints proxy straight to the origin,
 * and an unreachable origin holds the socket ~15 s (measured 502 at
 * time_total=15.02) — the audit found the bulk priority menu, the detail
 * priority picker and the status menu all painting {t('common.loading')} at
 * 0 ms with no grace, no cap and no way out.
 *
 * The contract here:
 *  - the wait obeys the shared skeleton grace (nothing in the first 120 ms,
 *    then the compact loading row);
 *  - the wait ends at MENU_ORIGIN_TIMEOUT_MS — imported, never re-derived —
 *    and after it "Loading…" is gone;
 *  - what replaces it is an answer: the cached catalog (with the offline
 *    note) when one exists, otherwise the catalog's failure sentence and a
 *    Retry.
 *
 * The detail-picker half of the contract moved to vitest (GDK-1702 cost
 * ladder): withMenuTimeout's timing and the picker's cached-catalog
 * fallback are web/src/lib/menu-loading.test.ts — the fallback is pinned
 * as a source scan there, because the picker cannot mount in the unit
 * project and the fallback's claims are its source. This spec keeps the
 * real path: one menu opened against a stalled origin, end to end.
 *
 * The stalled routes fulfill after 60 s — far past every assertion — so the
 * pre-fix tree hangs exactly like the unreachable origin did, and the
 * post-fix fallback state stays put for the assertions (no late-arriving
 * catalog can flip it mid-test). The stall is a timer inside the handler:
 * route.fulfill's own `delay` option does not hold on this Playwright build
 * (measured: a 5000 ms delay answered in 8 ms), and the timer is unref'd so
 * it never holds the runner open after the page is gone.
 */

/** Delay that outlives the test on both sides of the fix. */
const LONGER_THAN_THE_TEST_MS = 60_000

/** Hold the route open without fulfilling — the unreachable-origin shape. */
async function stall(): Promise<void> {
  await new Promise<void>((resolve) => {
    const timer = setTimeout(resolve, LONGER_THAN_THE_TEST_MS)
    timer.unref?.()
  })
}

async function selectBulkRows(page: Page, n: number): Promise<void> {
  for (let i = 0; i < n; i++) {
    await page.keyboard.press('j')
    await page.keyboard.press('x')
  }
  const bar = page.getByTestId('bulk-bar')
  await expect(bar).toBeVisible()
  await expect(bar.getByText(`${n} selected`)).toBeVisible()
}

test.describe('menu origin wait is capped (GDK-1566)', () => {
  test.use({ viewport: { width: 1440, height: 900 } })

  test('bulk priority menu: timeout shows the failure sentence and Retry, never a stuck Loading', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.route('**/api/v1/issues/priorities/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await stall()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: { priorities: [{ id: '1', name: 'Highest' }] },
      })
    })
    await gotoApp(page)
    await selectBulkRows(page, 2)

    await page.getByRole('button', { name: en['bulk.changePriority'], exact: true }).click()
    const menu = page.getByTestId('bulk-priority-menu')
    await expect(menu).toBeVisible()

    // Inside the wait: the grace-governed loading row (nothing before
    // 120 ms is its contract, owned by skeleton-grace — here we pin that the
    // wait state is the only thing on screen while the origin stalls).
    await expect(menu.getByText(en['common.loading'])).toBeVisible()

    // At the cap the wait must end: "Loading…" is gone…
    await expect(menu.getByText(en['common.loading'])).toBeHidden({
      timeout: MENU_ORIGIN_TIMEOUT_MS + 4_000,
    })
    // …replaced by the catalog's failure sentence and a Retry.
    await expect(menu.getByTestId('menu-load-error')).toHaveText(en['write.prioritiesFailed'])
    const retry = menu.getByTestId('menu-load-retry')
    await expect(retry).toBeVisible()

    // Retry means the wait again — a second unanswered origin is a second
    // capped wait, not a dead menu.
    await retry.click()
    await expect(menu.getByText(en['common.loading'])).toBeVisible()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
