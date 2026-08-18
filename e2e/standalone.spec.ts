import { expect, test, type Page } from '@playwright/test'
import { gotoApp, openServerSettings } from './helpers'

/**
 * Track D: a standalone workspace must show its kind indicator; a connected
 * one must not. Key off data-testid + the served kind — not a locale string
 * or a platform command.
 */

async function serveWorkspaceKind(page: Page, kind: 'standalone' | 'connected'): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = (await res.json()) as Record<string, unknown>
    await route.fulfill({
      response: res,
      json: { ...doc, workspaceKind: kind },
    })
  })
}

test.describe('standalone workspace indicator', () => {
  test('standalone workspace shows its indicator; connected does not', async ({ page }) => {
    await serveWorkspaceKind(page, 'standalone')
    await gotoApp(page)
    await openServerSettings(page)

    const indicator = page.getByTestId('workspace-kind')
    await expect(indicator).toBeVisible()
    await expect(indicator).toHaveAttribute('data-kind', 'standalone')
    await expect(page.getByTestId('standalone-init-command')).toBeVisible()

    // The served document is the source of truth — the chip must match it,
    // not a client-side guess from an empty site URL.
    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { workspaceKind?: string }).workspaceKind ?? '' : ''
    })
    expect(served).toBe('standalone')
    await expect(indicator).toHaveAttribute('data-kind', served)
  })

  test('connected workspace does not show the standalone indicator', async ({ page }) => {
    await serveWorkspaceKind(page, 'connected')
    await gotoApp(page)
    await openServerSettings(page)

    await expect(page.getByTestId('workspace-kind')).toHaveCount(0)

    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { workspaceKind?: string }).workspaceKind ?? '' : ''
    })
    expect(served).toBe('connected')
    await expect(page.getByTestId('workspace-kind')).toHaveCount(0)
  })
})
