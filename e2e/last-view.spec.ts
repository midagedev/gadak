import { expect, test } from './helpers'
import { DEMO_ISSUE_COUNT_EN_RE, attachConsoleErrors, forceLocale } from './helpers'

/**
 * GDK-1586: a view that arrives by URL is the view this tab last showed, and
 * the storage key is literally named lastView — so a keep-url boot saves it.
 * The persist effect only fires on a viewKey *change* and keep-url never
 * changes viewKey (applyStartupView applies nothing), so the write belongs
 * to the startup path itself. FAIL-first: pre-change nothing was saved, and
 * the next boot forgot the link's view.
 */

const KEY = 'NMB-5' // fixture issue; any key a ks view can hold

test('a view that arrives by URL is saved as the last view', async ({ page }) => {
  const errors = attachConsoleErrors(page)
  await forceLocale(page, 'en')
  await page.goto(`/#/?ks=${KEY}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  // The arrived view committed: the keyed row is on screen.
  await expect(
    page.locator(`[data-testid="issue-list-scroller"] [data-issue-key="${KEY}"]`),
  ).toBeVisible()

  // The saved value is filters.viewKey's own format — view params only,
  // sorted, k=v joined; with one param, that is the whole string. The
  // startup effect waits for auth before saving, so poll on the write
  // itself rather than a proxy condition.
  await expect
    .poll(() =>
      page.evaluate(() => {
        const key = Object.keys(localStorage).find((k) => k.endsWith('last-view'))
        return key === undefined ? null : localStorage.getItem(key)
      }),
    )
    .toBe(`ks=${KEY}`)

  expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
})
