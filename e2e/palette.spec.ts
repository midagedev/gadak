import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('command palette', () => {
  test('Cmd+K opens it, typing stays local, Enter opens the issue detail', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Let the post-boot API chatter (bootstrap/write-meta) settle first.
    // Wait for the boot payload to land, plus a short settle for the trailing
    // boot requests. Deliberately not networkidle: the app polls for a delta
    // every 15s, so "no network for 500ms" only becomes true after that poll
    // fires, which added 15s to this test for nothing.
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    await page.waitForTimeout(300)

    const apiDuringType: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (url.includes('/api/') && !url.includes('/ui-focus/')) apiDuringType.push(url)
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('NMB-110', { delay: 20 })
    const first = palette.getByRole('option').first()
    await expect(first).toContainText('NMB-110')
    await expect(first).toHaveAttribute('aria-selected', 'true')

    expect(
      apiDuringType,
      `expected no /api/ requests while typing, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('opens while a text input has focus, and Esc closes it', async ({ page }) => {
    await gotoApp(page)

    await searchInput(page).click()
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // Empty query lists views and actions (no query typed yet).
    await expect(palette.getByRole('option', { name: /New issue/ })).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(palette).toBeHidden()
  })
})
