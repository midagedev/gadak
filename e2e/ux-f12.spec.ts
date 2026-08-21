import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'
import { ko } from '../web/src/lib/i18n/ko'

/*
 * F12 UX defects (GDK-475, GDK-476, GDK-477).
 *
 * Each test waits on the state it names — not a proxy flag. Captures for the
 * visual pass land in /tmp/f12-shots/ after the assertions hold.
 */

const API = 'http://127.0.0.1:7877/api/v1/issues/'

async function openIssue(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  // Exact row via data-issue-key: hasText('NMA-1') also matches NMA-10/-100,
  // and which one comes first depends on the boot view's ordering (GDK-100
  // made that epic-grouped).
  await page
    .locator(`[data-testid="issue-list-scroller"] [data-issue-key="${key}"]`)
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('F12 detail / settings / server-down', () => {
  test('GDK-475: zero comments are counted once; shortcut lives on one kbd', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)
    // Flat view: the epic-grouped boot default (GDK-100) can leave the
    // searched row outside the virtual scroller, so the click never lands.
    await page.getByRole('button', { name: /All open/ }).click()
    const panel = await openIssue(page, 'NMA-1')

    const comments = panel.getByRole('heading', { name: 'Comments' })
    await expect(comments).toBeVisible()
    await expect(comments).toHaveText(/Comments\s*0/)
    await expect(panel.getByText('No comments', { exact: true })).toHaveCount(0)

    const composer = panel.getByTestId('comment-composer')
    await expect(composer).toHaveAttribute('placeholder', en['write.commentPlaceholder'])
    expect(en['write.commentPlaceholder']).not.toMatch(/⌘Enter|@mention/)

    const shortcut = panel.getByTestId('comment-shortcut')
    await expect(shortcut).toHaveCount(1)
    // GDK-354 / F-1: kbd is the platform modifier + Enter, not the catalog
    // string (which used to hard-code ⌘Enter on every OS). Same platform
    // test as modifierSymbol() in web/src/lib/unified-search.ts.
    expect(en['write.commentShortcut']).not.toMatch(/⌘/)
    expect(ko['write.commentShortcut']).not.toMatch(/⌘/)
    const mod = await page.evaluate(() =>
      /Mac|iP(hone|ad)/.test(navigator.platform) ? '⌘' : 'Ctrl',
    )
    const label = `${mod}Enter`
    await expect(shortcut).toHaveText(label)
    await expect(panel.getByText(label)).toHaveCount(1)

    await page.screenshot({ path: '/tmp/f12-shots/475-detail-nma1.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-476: settings lead names the job, not a file path', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)
    await openServerSettings(page)

    const dialog = page.getByRole('dialog', { name: 'Settings' })
    const lead = dialog.getByTestId('settings-intro')
    await expect(lead).toBeVisible()
    await expect(lead).toHaveText(en['settings.intro'])
    await expect(lead).not.toContainText('config.json')
    await expect(lead).not.toContainText('~/.gadak')

    await page.screenshot({ path: '/tmp/f12-shots/476-settings-intro.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-476: a hung sources list leaves Loading for an error + manual keys', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.route(`${API}projects/available/`, async (route) => {
      await new Promise((r) => setTimeout(r, 60_000))
      await route.abort()
    })
    await page.route(`${API}settings/spaces/`, async (route) => {
      await new Promise((r) => setTimeout(r, 60_000))
      await route.abort()
    })

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()

    const sources = dialog.getByTestId('settings-sources')
    await expect(sources.getByTestId('scope-projects-fallback')).toBeVisible()
    await expect(sources.getByText(en['settings.projectsManual'])).toBeVisible({
      timeout: 12_000,
    })
    await expect(sources.getByText(en['settings.scopeLoading'])).toHaveCount(0)
    await expect(sources.getByTestId('scope-spaces-error')).toBeVisible()
    await expect(sources.getByTestId('scope-spaces-error')).toHaveText(
      en['settings.spacesUnavailable'],
    )
    // Client timeout on the site list is not "gadak serve is gone".
    await expect(page.getByTestId('offline-banner')).toHaveCount(0)

    await page.screenshot({ path: '/tmp/f12-shots/476-sources-error.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-476: turning Confluence on for every space needs a second click', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.route(`${API}settings/`, (route) =>
      route.fulfill({ json: { projects: ['NMB'], staleThresholdHours: 72 } }),
    )
    await page.route(`${API}projects/available/`, (route) =>
      route.fulfill({
        json: {
          projects: [{ key: 'NMB', name: 'Nimbus Backend', projectTypeKey: 'software' }],
          truncated: false,
        },
      }),
    )
    await page.route(`${API}settings/spaces/`, (route) =>
      route.fulfill({
        json: { spaces: [], all_global_when_empty: false, enabled: false },
      }),
    )

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()

    const confluence = dialog.getByTestId('sources-confluence')
    const turnOn = confluence.getByTestId('confluence-turn-on')
    await expect(turnOn).toHaveText(en['settings.confluenceTurnOnAll'])

    await turnOn.click()
    await expect(turnOn).toHaveText(en['settings.confluenceTurnOnAllConfirm'])
    await expect(confluence.getByTestId('confluence-all-warning')).toHaveCount(0)
    await expect(confluence.getByTestId('confluence-turn-off')).toHaveCount(0)

    await turnOn.click()
    await expect(confluence.getByTestId('confluence-all-warning')).toBeVisible()
    await expect(confluence.getByTestId('confluence-turn-off')).toBeVisible()

    await page.screenshot({ path: '/tmp/f12-shots/476-confluence-confirm.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-477: a failed request raises the banner now; recovery retries detail', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)
    const panel = await openIssue(page, 'NMB-110')
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()

    let down = true
    await page.route('**/api/v1/issues/**', (route) => {
      if (down) return route.abort('failed')
      return route.continue()
    })

    const input = searchInput(page)
    await input.fill('NMA-1')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMA-1' })
      .first()
      .click()

    await expect(panel.getByTestId('detail-load-error')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('offline-banner')).toBeVisible({ timeout: 3_000 })

    await page.getByTestId('sidebar-sync-now').click()
    const popover = page.getByTestId('sync-history-popover')
    await expect(popover).toBeVisible()
    const offlineLine = popover.getByTestId('sync-history-offline')
    await expect(offlineLine).toBeVisible()
    await expect(offlineLine).toContainText(en['sidebar.serverUnreachable'])
    await expect(popover.getByTestId('sync-history-retry')).toBeVisible()

    await page.screenshot({ path: '/tmp/f12-shots/477-offline-detail.png' })

    down = false
    await popover.getByTestId('sync-history-retry').click()
    await expect(page.getByTestId('offline-banner')).toHaveCount(0)
    await expect(panel.getByTestId('detail-load-error')).toHaveCount(0)
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await expect(panel.getByRole('button', { name: en['common.retry'] })).toHaveCount(0)

    await page.screenshot({ path: '/tmp/f12-shots/477-recovered.png' })
    // route.abort() logs net::ERR_FAILED; that is the down signal under test.
    expect(
      errors.filter((e) => !e.includes('ERR_FAILED') && !e.includes('Failed to load resource')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
