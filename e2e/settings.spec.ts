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
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })

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

  test('shows our own Jira call volume, including throttling', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)

    const dialog = page.getByRole('dialog', { name: 'Settings' })
    // The fixture seeds one day of api_usage. The row hides itself when nothing
    // has been counted, so asserting the numbers — not just the label — is what
    // keeps a silently dropped runtime.apiUsage from passing.
    await expect(dialog.getByText('Jira calls')).toBeVisible()
    // Seed is 1204; earlier tests in this process also increment today's
    // counter (write-meta against the fake site), so the exact seed is not
    // stable — but it only ever grows, so require at least the four digits the
    // seed guarantees. \d+ would let "0 today" pass and defeat the guard above.
    await expect(dialog.getByText(/\d{4,} today/)).toBeVisible()
    await expect(dialog.getByText('2 throttled')).toBeVisible()
  })
})

/*
 * Settings-audit contracts (false copy / dead toggle). These two assertions
 * failed against the pre-fix catalogs and Features tab (FAIL-first 2026-08-15):
 * sourcesNoProjects said "no issue is mirrored" / "미러링되는 이슈가 없습니다",
 * and Features rendered a "Web push" checkbox that saved a flag whose
 * endpoints 404.
 */
test.describe('settings copy contracts', () => {
  test('Features tab does not render the web-push toggle', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Features', exact: true }).click()
    await expect(dialog.getByText('Personal feed')).toBeVisible()
    await expect(dialog.getByText('Web push', { exact: true })).toHaveCount(0)
  })

  test('empty project picker label includes every project', async ({ page }) => {
    const API = 'http://127.0.0.1:7877/api/v1/issues/'
    await page.route(`${API}settings/`, (route) =>
      route.fulfill({
        json: { projects: [], staleThresholdHours: 72 },
      }),
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

    const projects = dialog.getByTestId('scope-projects')
    await expect(projects).toBeVisible()
    await expect(projects.getByTestId('scope-empty')).toContainText('every project')
  })
})
