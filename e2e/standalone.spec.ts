import { expect, test, type Page } from '@playwright/test'
import { gotoApp, openServerSettings } from './helpers'

/**
 * Track D: a standalone workspace must show its kind indicator; a connected
 * one must not. Key off data-testid + the served kind — not a locale string
 * or a platform command.
 *
 * The create path is the CLI (this round does not add a write endpoint).
 * Copy is asserted against the real clipboard, not the "Copied" label
 * (GDK-178: a toast that lies is worse than a button that fails aloud).
 */

const STANDALONE_INIT_COMMAND = 'gadak --workspace <name> init --standalone'

async function serveWorkspaceKind(
  page: Page,
  kind: 'standalone' | 'connected',
  extra: Record<string, unknown> = {},
): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = (await res.json()) as Record<string, unknown>
    await route.fulfill({
      response: res,
      json: { ...doc, ...extra, workspaceKind: kind },
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
    await expect(indicator).toHaveAttribute('aria-label', /issuetap persist/)
    await expect(page.getByTestId('standalone-init-command')).toHaveText(STANDALONE_INIT_COMMAND)

    // The served document is the source of truth — the chip must match it,
    // not a client-side guess from an empty site URL.
    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { workspaceKind?: string }).workspaceKind ?? '' : ''
    })
    expect(served).toBe('standalone')
    await expect(indicator).toHaveAttribute('data-kind', served)
  })

  test('copy writes the init command to the clipboard', async ({ page }) => {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await serveWorkspaceKind(page, 'standalone')
    await gotoApp(page)
    await openServerSettings(page)

    await page.getByTestId('standalone-init-copy').click()
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(
      STANDALONE_INIT_COMMAND,
    )
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

  test('empty site + connected (hosted-demo shape) does not show the badge', async ({ page }) => {
    await serveWorkspaceKind(page, 'connected', { jiraBaseUrl: '' })
    await gotoApp(page)
    await openServerSettings(page)

    await expect(page.getByTestId('workspace-kind')).toHaveCount(0)
    await expect(page.getByTestId('standalone-init-command')).toHaveText(STANDALONE_INIT_COMMAND)
  })

  test('sidebar create control reveals the init command', async ({ page }) => {
    await serveWorkspaceKind(page, 'connected')
    await gotoApp(page)

    const create = page.getByTestId('standalone-create')
    await expect(create).toBeVisible()
    await create.click()
    await expect(page.getByText(STANDALONE_INIT_COMMAND, { exact: true })).toBeVisible()
  })
})
