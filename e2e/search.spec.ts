import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('client-side search', () => {
  test('narrows the list immediately with zero /api/ traffic while typing', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Wait until the post-boot API chatter (bootstrap/write-meta) has finished.
    await page.waitForLoadState('networkidle')

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

    expect(
      apiDuringType,
      `expected no /api/ requests while typing, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
