import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings } from './helpers'

const SETTINGS_URL = 'http://127.0.0.1:7877/api/v1/issues/settings/'

test.describe('settings dialog', () => {
  test('changes staleThresholdHours, saves, and API reflects the value', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await openServerSettings(page)

    const dialog = page.getByRole('dialog', { name: 'Settings' })
    const stale = dialog.getByLabel('Stale threshold (hours)')
    await expect(stale).toBeVisible()
    await stale.fill('48')

    // Save triggers location.reload() after ~600ms (same hash URL).
    await Promise.all([
      page.waitForEvent('load'),
      dialog.getByRole('button', { name: 'Save', exact: true }).click(),
    ])

    // After reload the list boots again.
    await expect(page.getByText(/519 issues/).first()).toBeVisible({ timeout: 30_000 })

    const res = await request.get(SETTINGS_URL)
    expect(res.ok()).toBeTruthy()
    const body = (await res.json()) as Record<string, unknown>
    expect(body.staleThresholdHours).toBe(48)

    // Restore fixture default so later specs see 72 if they open settings.
    await request.put(SETTINGS_URL, {
      data: { ...body, staleThresholdHours: 72 },
    })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
