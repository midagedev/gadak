import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * GDK-1344: a:visited for issue rows. An opened issue's key goes quiet
 * (data-seen), and one whose updated_at is newer than the visit carries the
 * accent dot a document row uses for the same fact (data-changed). The set
 * comes from local.db once at boot (history/visited/); the store keeps it
 * current as rows are opened.
 */
test.describe('visited rows', () => {
  test('opening a row marks it seen without reloading', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const rows = page.getByTestId('issue-list-scroller').locator('[data-issue-key]')
    await expect(rows.first()).toBeVisible()
    // Some row the shared e2e home has not recorded yet — the suite's other
    // specs leave visits behind, so the first unseen row is the subject.
    const target = rows.locator(':scope:not([data-seen])').first()
    await expect(target).toBeVisible()
    const key = (await target.getAttribute('data-issue-key')) as string

    await target.click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    const row = page.locator(`[data-issue-key="${key}"]`)
    await expect(row).toHaveAttribute('data-seen', 'true')
    // Just opened, so nothing changed since: no dot.
    await expect(row).not.toHaveAttribute('data-changed', 'true')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an issue edited after the visit carries the changed dot', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Every fixture key was "opened" in 2020: each row is seen, and each has
    // been edited since — the dot's whole condition, from the server's answer.
    const items: { key: string; viewed_at: string }[] = []
    for (const p of ['NMA', 'NMB', 'NMS']) {
      for (let i = 1; i <= 300; i++) items.push({ key: `${p}-${i}`, viewed_at: '2020-01-01T00:00:00.000Z' })
    }
    await page.route('**/history/visited/**', (route) => route.fulfill({ json: { items, truncated: false } }))
    await gotoApp(page)

    const first = page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first()
    await expect(first).toHaveAttribute('data-seen', 'true')
    await expect(first).toHaveAttribute('data-changed', 'true')
    await expect(first.getByText('Changed since you opened it')).toHaveCount(1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
