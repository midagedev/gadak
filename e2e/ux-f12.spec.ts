import { test, expect, type Page } from '@playwright/test'
import { apiURL, attachConsoleErrors, gotoApp, openServerSettings, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'
import { ko } from '../web/src/lib/i18n/ko'

/*
 * F12 UX defects (GDK-475, GDK-476, GDK-477).
 *
 * Each test waits on the state it names — not a proxy flag. Captures for the
 * visual pass land in /tmp/f12-shots/ after the assertions hold.
 */

const API = apiURL('/api/v1/issues/')

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
    // GDK-354 / F-1: kbd is the platform modifier + the ↵ glyph the cheat
    // sheet prints (GDK-621) — not a catalog string hard-coding ⌘Enter on
    // every OS. Same platform test as modifierSymbol() in
    // web/src/lib/unified-search.ts.
    expect(en['write.commentShortcut']).not.toMatch(/⌘/)
    expect(ko['write.commentShortcut']).not.toMatch(/⌘/)
    const mod = await page.evaluate(() =>
      /Mac|iP(hone|ad)/.test(navigator.platform) ? '⌘' : 'Ctrl',
    )
    const label = `${mod} ↵`
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
    /*
     * The two numbers below are the contract, not tuning. The site lists are
     * held for HOLD_MS while the client gives up after TIMEOUT_MS, and the
     * error UI has to arrive inside that gap. The eventual 5xx renders the
     * same UI, so the gap is measured (below) rather than left to an expect
     * budget — assertion order decides when a budget starts, and a budget
     * that starts after HOLD_MS has already elapsed proves nothing.
     */
    const TIMEOUT_MS = 250
    const HOLD_MS = 3_000
    // Test-only: production SCOPE_LIST_MS is 8_000 (SettingsDialog.svelte).
    await page.addInitScript((ms) => {
      ;(window as unknown as { __gadakTestFetchTimeoutMs?: number }).__gadakTestFetchTimeoutMs = ms
    }, TIMEOUT_MS)
    // Fixture Jira is fake — GET meta/write/ otherwise holds teardown on a
    // createmeta DNS miss (~15s). Same fulfill shape as duedate.spec.ts.
    await page.route('**/api/v1/issues/meta/write/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await route.fulfill({
        json: {
          transitions: {},
          create_meta: { projects: [] },
          updated_at: '2026-08-18T00:00:00.000Z',
        },
      })
    })
    await gotoApp(page)
    // Hang the two site lists, then answer — that is the GDK-476 shape, and
    // the only way scopeListSignal() actually fires. Not route.abort(): a
    // network throw is what GDK-477 reads as "gadak serve is gone", which
    // raises offline-banner and pins teardown ~15s waiting for it to clear.
    // The hold is longer than the 250ms hook above and short enough that
    // teardown does not wait on it.
    await page.route(
      (url) =>
        url.pathname.includes('/projects/available') || url.pathname.includes('/settings/spaces'),
      async (route) => {
        await new Promise((r) => setTimeout(r, HOLD_MS))
        await route.fulfill({ status: 500, json: { error: 'unavailable' } })
      },
    )
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    const sources = dialog.getByTestId('settings-sources')
    const clickedAt = Date.now()
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()
    await expect(sources.getByTestId('scope-spaces-error')).toBeVisible()
    const errorAfterMs = Date.now() - clickedAt

    await expect(sources.getByTestId('scope-projects-fallback')).toBeVisible()
    await expect(sources.getByText(en['settings.projectsManual'])).toBeVisible({
      timeout: 12_000,
    })
    // GDK-476 itself: "Loading the list…" has to go away without the site.
    await expect(sources.getByText(en['settings.scopeLoading'])).toHaveCount(0)
    // The client timeout is what cleared it, not the request finally answering.
    expect(
      errorAfterMs,
      `sources error took ${errorAfterMs}ms; the lists were held ${HOLD_MS}ms, so anything at or past that is the response, not the ${TIMEOUT_MS}ms client timeout`,
    ).toBeLessThan(HOLD_MS)
    await expect(sources.getByTestId('scope-spaces-error')).toHaveText(
      en['settings.spacesUnavailable'],
    )
    // Client timeout on the site list is not "gadak serve is gone".
    await expect(page.getByTestId('offline-banner')).toHaveCount(0)

    await page.screenshot({ path: '/tmp/f12-shots/476-sources-error.png' })
    expect(
      errors.filter((e) => !e.includes('ERR_FAILED') && !e.includes('Failed to load resource')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
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
