import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('client-side search', () => {
  test('narrows the list immediately with zero /api/ traffic while typing', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Wait for the boot payload to land, plus a short settle for the trailing
    // boot requests. Deliberately not networkidle: the app polls for a delta
    // every 15s, so "no network for 500ms" only becomes true after that poll
    // fires, which added 15s to this test for nothing.
    await expect(page.getByText(/519 issues/).first()).toBeVisible({ timeout: 30_000 })
    await page.waitForTimeout(300)

    const apiDuringType: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (url.includes('/api/')) apiDuringType.push(url)
    })

    const input = searchInput(page)
    await input.click()
    await input.pressSequentially('NMB-110', { delay: 20 })

    // Count updates as the pool filters locally.
    await expect(page.getByText(/1 issues?|1 issue/)).toBeVisible()
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    // Matched substring is highlighted in the surviving row.
    await expect(page.locator('mark', { hasText: /NMB-110/i }).first()).toBeVisible()

    expect(
      apiDuringType,
      `expected no /api/ requests while typing, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
