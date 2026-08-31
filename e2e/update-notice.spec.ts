import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings } from './helpers'
import { en } from '../web/src/lib/i18n/en'

const LATEST = '0.99.0'
const RELEASE = `https://github.com/midagedev/gadak/releases/tag/v${LATEST}`
const NOTES = 'Fixed the flaky upload.\nSecond line.'

async function injectDeltaUpdate(
  page: import('@playwright/test').Page,
  extra: Record<string, unknown> = {},
): Promise<void> {
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as Record<string, unknown>
    await route.fulfill({
      response,
      json: { ...body, latest_version: LATEST, release_url: RELEASE, ...extra },
    })
  })
}

async function flushDelta(page: import('@playwright/test').Page): Promise<void> {
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'visible',
    })
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

test.describe('update notice from delta', () => {
  test('latest_version on delta shows the sidebar notice with that version', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await injectDeltaUpdate(page)
    await gotoApp(page)
    await flushDelta(page)
    const notice = page.getByTestId('update-notice')
    await expect(notice).toBeVisible()
    await expect(notice).toContainText(LATEST)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('delta without latest_version shows no notice', async ({ page }) => {
    await gotoApp(page)
    await expect(page.getByTestId('update-notice')).toHaveCount(0)
  })

  test('Settings Sync shows the tag and release link, and names brew only on macOS', async ({
    page,
  }) => {
    await injectDeltaUpdate(page)
    await gotoApp(page)
    await flushDelta(page)
    await expect(page.getByTestId('update-notice')).toBeVisible()
    await openServerSettings(page)
    const row = page.getByTestId('settings-update')
    await expect(row).toBeVisible()
    await expect(row).toContainText(LATEST)
    await expect(page.getByTestId('settings-update-link')).toHaveAttribute('href', RELEASE)

    // The upgrade command is per-platform, so the expectation has to be too:
    // asserting the brew line unconditionally would pass on a macOS dev box
    // and fail on the Linux CI runner — the local/CI split that hides
    // defects. Ask the served config which machine this is.
    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { os?: string }).os ?? '' : ''
    })
    const brew = page.getByTestId('settings-update-brew')
    if (served === 'darwin') {
      await expect(brew).toHaveText('brew upgrade --cask gadak')
    } else {
      await expect(brew).toHaveCount(0)
    }
  })

  test('release_notes on delta opens the link-first dialog from the banner', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await injectDeltaUpdate(page, { release_notes: NOTES })
    await gotoApp(page)
    await flushDelta(page)
    const notice = page.getByTestId('update-notice')
    await expect(notice).toBeVisible()
    await expect(page.getByTestId('update-notes')).toHaveCount(0)
    await notice.click()
    const notes = page.getByTestId('update-notes')
    await expect(notes).toBeVisible()
    await expect(notes).toContainText(LATEST)
    const link = notes.getByTestId('update-notes-link')
    await expect(link).toHaveAttribute('href', RELEASE)
    await expect(link).toHaveText(en['settings.updateReleaseNotes'])
    // GDK-1246: the release page owns the notes text — the raw body must
    // not be dumped into the dialog.
    await expect(notes.locator('pre')).toHaveCount(0)
    await expect(notes).not.toContainText('Fixed the flaky upload.')
    // The upgrade command is per-platform (same discipline as the Settings
    // row): an OS without one must not end the dialog in an empty bordered
    // footer strip — the footer hides entirely there.
    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { os?: string }).os ?? '' : ''
    })
    const footer = notes.locator('[data-dialog-footer]')
    if (served === 'darwin') {
      await expect(footer).toBeVisible()
      await expect(footer).toContainText('brew upgrade --cask gadak')
    } else {
      await expect(footer).toBeHidden()
    }
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('delta without release_notes still opens the dialog from the banner', async ({ page }) => {
    await injectDeltaUpdate(page)
    await gotoApp(page)
    await flushDelta(page)
    const notice = page.getByTestId('update-notice')
    await expect(notice).toBeVisible()
    await expect(notice).not.toHaveAttribute('href')
    await notice.click()
    const notes = page.getByTestId('update-notes')
    await expect(notes).toBeVisible()
    await expect(notes).toContainText(LATEST)
    await expect(notes.getByTestId('update-notes-link')).toHaveAttribute('href', RELEASE)
  })
})
