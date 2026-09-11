/*
 * GDK-734: the comment Reply button was hover-only.
 *
 * `opacity-0 … group-hover:opacity-100` with no focus variant means the
 * control is in the tab order and paints nothing when it gets there — a
 * keyboard user tabs onto an invisible button. The fix adds
 * `focus-within:opacity-100`, so the two halves of the contract are:
 *
 *   1. it is still quiet at rest (opacity 0 with neither hover nor focus), and
 *   2. focus alone — no pointer anywhere near it — reveals it.
 *
 * FAIL-first: assertion 2 fails against the pre-change component (opacity
 * stays "0" while the button holds focus); assertion 1 passes before and
 * after and is the regression pin that keeps the fix from turning the row
 * into permanently visible chrome.
 *
 * Mock strategy follows detail-coaching.spec.ts: bootstrap and delta pass
 * through to the real e2e server, only the detail's comment list is appended
 * to and auth/me is pinned — the Reply button renders on the origin's
 * issueWrite capability and the comment's account id (GDK-1152 moved it off
 * `me.identified`, which is empty on workspaces that reply fine).
 */
import { type Page, type Route } from '@playwright/test'
import { expect, test } from './helpers'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

const KEY = 'NMB-5' // fixture: In Progress, 0 comments of its own
const DANA_ME = { email: 'dana@example.com', account_id: 'demo-dana', name: 'Dana Whitfield' }

/** A comment by someone with an account id — the Reply button's condition. */
const COMMENT = {
  comment_id: 'e2e-reply-1',
  author: 'Priya Raman',
  author_account_id: 'demo-priya',
  body: 'Reply should be reachable without a mouse.',
  created_at: new Date().toISOString(),
}

async function appendDetailComment(page: Page, key: string): Promise<void> {
  await page.route(`**/api/v1/issues/${key}/detail/`, async (route: Route) => {
    if (route.request().method() !== 'GET') return route.continue()
    try {
      const response = await route.fetch()
      const body = (await response.json()) as { comments?: unknown[] }
      await route.fulfill({ response, json: { ...body, comments: [...(body.comments ?? []), COMMENT] } })
    } catch {
      // Detail request still in flight at teardown (detail-coaching.spec.ts
      // hit this twice on CI) — aborting keeps a passed test passed.
      await route.abort().catch(() => {})
    }
  })
}

async function openDetail(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  const jump = page.getByRole('button', { name: new RegExp(`^${key} .+Open with Enter`) })
  await expect(jump).toBeVisible()
  await jump.click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('GDK-734 hover-only comment actions reach the keyboard', () => {
  test('the Reply button stays quiet at rest and appears on focus alone', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.route('**/api/v1/auth/me/**', (route) => route.fulfill({ status: 200, json: DANA_ME }))
    await appendDetailComment(page, KEY)
    await forceLocale(page, 'en')
    await gotoApp(page)

    const panel = await openDetail(page, KEY)
    await expect(panel).toContainText('Reply should be reachable without a mouse.')

    const reply = page.getByTestId('comment-reply')
    await expect(reply).toHaveCount(1)

    // Rest: the pointer is parked far from the comment row, so no :hover.
    await page.mouse.move(0, 0)
    await expect(reply).toHaveCSS('opacity', '0')

    // The button must be in the tab order at all — an invisible control that
    // is also unfocusable would pass the opacity assertion below vacuously.
    const tabbable = await reply.evaluate((el) => {
      const t = (el as HTMLElement).tabIndex
      return { tabIndex: t, disabled: (el as HTMLButtonElement).disabled }
    })
    expect(tabbable.disabled, 'Reply must not be disabled').toBe(false)
    expect(tabbable.tabIndex, 'Reply must be in the natural tab order').toBeGreaterThanOrEqual(0)

    // Focus, still no pointer: this is what tabbing onto the row produces.
    await reply.focus()
    await expect(reply).toBeFocused()
    await expect(reply).toHaveCSS('opacity', '1')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
