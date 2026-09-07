import { test, expect } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * The feed's own failure signal (moved from ux-f12.spec.ts in the v0.21 audit
 * ladder round); its /tmp capture was deleted with the move. The docs
 * surface's 503 case lives in docs-ux.spec.ts, the shared strip's in
 * offline-banner.spec.ts.
 */

test.describe('feed load failure (GDK-1066)', () => {
  test('a 503 feed is an error line, never the empty copy', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    // Only the feed endpoint fails — identity and every other surface answer,
    // so this measures the feed's own branch, not the shared offline strip.
    await page.route('**/api/v1/issues/feed/**', (route) =>
      route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: '{"error":"service unavailable"}',
      }),
    )
    // Cold-hash boot into the feed screen (same entry as url-state.spec.ts's
    // gotoParams) — the way a shared feed link arrives at the failure.
    await forceLocale(page, 'en')
    await page.goto('/#/?feed=all')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })

    // The feed's own failure signal, in the DocsView/HistoryView shape.
    await expect(page.getByTestId('feed-load-error')).toBeVisible()
    await expect(page.getByTestId('feed-load-error')).toContainText(en['feed.loadFailed'])
    // Failure is not emptiness: "No new activity" is reserved for a request
    // that succeeded with zero rows.
    await expect(page.getByText(en['feed.empty'], { exact: true })).toHaveCount(0)
    // No banner assertion here, on purpose — same one-owner reasoning as the
    // docs case: everything but feed/ answers, so the pool sync marks
    // the server back up. The feed surface's own signal is the error line.

    await page.unrouteAll({ behavior: 'ignoreErrors' })
    await page.getByRole('button', { name: en['common.retry'] }).click()
    // Recovered: the error line is gone; what follows (rows, or the empty
    // copy now that a request actually succeeded) is fixture data, not this
    // test's axis.
    await expect(page.getByTestId('feed-load-error')).toHaveCount(0)

    expect(
      errors.filter((e) => !e.includes('Failed to load resource')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
