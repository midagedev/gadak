import { test, expect, type Page } from '@playwright/test'

/**
 * GDK-335 — hosted demo lands on the SPA with no intermediate overlay.
 * GitHub / About live in the demo chrome (hosted-links).
 *
 * Hosted-config only — not part of the CI e2e set.
 */

const DEMO = '/gadak/'

test.use({
  viewport: { width: 1280, height: 900 },
})

async function gotoDemo(page: Page): Promise<void> {
  await page.goto(DEMO, { waitUntil: 'domcontentloaded' })
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
}

test.describe('hosted demo entry', () => {
  test('paints issue-layout with zero overlay clicks', async ({ page }) => {
    await page.goto(DEMO, { waitUntil: 'domcontentloaded' })
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByTestId('hosted-links')).toBeVisible()
    await expect(page.getByTestId('demo-banner')).toBeVisible()
  })

  test('GitHub and About popover links keep their hrefs', async ({ page }) => {
    await gotoDemo(page)

    const github = page.getByTestId('hosted-links-github')
    await expect(github).toHaveAttribute('href', 'https://github.com/midagedev/gadak')
    await expect(github).toHaveAttribute('target', '_blank')
    await expect(github).toHaveAttribute('rel', /noopener/)

    await page.getByTestId('hosted-links-about').click()
    const popover = page.getByTestId('hosted-links-popover')
    await expect(popover).toBeVisible()
    await expect(popover).toContainText(
      'A local SQLite file of your Jira — so "which epic is stuck?" is one query, not an unaskable one.',
    )
    const brew = popover.locator('pre')
    await expect(brew).toHaveText('brew install midagedev/tap/gadak')
    await expect(brew).toHaveCSS('user-select', 'all')

    await expect(popover.getByRole('link', { name: 'Watch the 60s demo' })).toHaveAttribute(
      'href',
      /web-demo\.mp4$/,
    )
    await expect(popover.getByRole('link', { name: 'Report an issue' })).toHaveAttribute(
      'href',
      'https://github.com/midagedev/gadak/issues',
    )
    await expect(popover.getByRole('link', { name: 'midagedev@gmail.com' })).toHaveAttribute(
      'href',
      'mailto:midagedev@gmail.com',
    )
    await expect(popover.getByRole('link', { name: '@midagedev' })).toHaveAttribute(
      'href',
      'https://x.com/midagedev',
    )
  })

  test('About popover toggles, closes on Escape, and closes on outside click', async ({ page }) => {
    await gotoDemo(page)
    const about = page.getByTestId('hosted-links-about')
    const popover = page.getByTestId('hosted-links-popover')

    await about.click()
    await expect(popover).toBeVisible()
    await about.click()
    await expect(popover).toHaveCount(0)

    await about.click()
    await expect(popover).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(popover).toHaveCount(0)

    await about.click()
    await expect(popover).toBeVisible()
    await page.getByTestId('demo-banner').click({ position: { x: 16, y: 8 } })
    await expect(popover).toHaveCount(0)
  })
})
