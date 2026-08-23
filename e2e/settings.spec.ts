import { test, expect } from '@playwright/test'
import { apiURL, attachConsoleErrors, gotoApp, openServerSettings, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

const SETTINGS_URL = apiURL('/api/v1/issues/settings/')

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
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })

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

  /*
   * GDK-188: the runtime mirror is a fact about the mirror's sync state, so it
   * belongs to the Sync tab, once — repeated above every tab it pushed each
   * tab's own subject down for facts that tab was not about.
   */
  test('the runtime mirror renders once, at the bottom of the Sync tab', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)

    const dialog = page.getByRole('dialog', { name: 'Settings' })
    const mirror = dialog.getByRole('region', { name: 'This mirror' })

    // Sync is the default tab.
    await expect(mirror).toHaveCount(1)

    // Below the tab's own controls, not above them: the intervals are the
    // subject of the tab, the mirror is the reference under it.
    const order = await dialog.evaluate((root) => {
      const region = root.querySelector('section[aria-label="This mirror"]')
      const token = Array.from(root.querySelectorAll('button')).find((b) =>
        (b.textContent ?? '').includes('Personal Jira API token'),
      )
      if (!region || !token) return 'missing'
      return region.compareDocumentPosition(token) & Node.DOCUMENT_POSITION_PRECEDING
        ? 'after-sync-controls'
        : 'before-sync-controls'
    })
    expect(order).toBe('after-sync-controls')

    for (const tab of ['Sources', 'Features', 'Teams / groups', 'Members', 'Field mapping', 'About']) {
      await dialog.getByRole('button', { name: tab, exact: true }).click()
      await expect(mirror, `mirror must not render on the ${tab} tab`).toHaveCount(0)
    }

    await dialog.getByRole('button', { name: 'Sync', exact: true }).click()
    await expect(mirror).toHaveCount(1)
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
 * endpoints 404. The web-push toggle is a source-scan now
 * (web/src/components/settings/FeaturesTab.test.ts) — it cannot fail in a
 * browser once the checkbox is gone from the source.
 */
test.describe('settings copy contracts', () => {
  test('empty project picker label includes every project', async ({ page }) => {
    const API = apiURL('/api/v1/issues/')
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

test.describe('settings about tab', () => {
  test('lists the four feedback channel hrefs', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'About', exact: true }).click()
    await expect(page.getByTestId('settings-about')).toBeVisible()
    await expect(page.getByTestId('about-link-github')).toHaveAttribute(
      'href',
      'https://github.com/midagedev/gadak',
    )
    await expect(page.getByTestId('about-link-issues')).toHaveAttribute(
      'href',
      'https://github.com/midagedev/gadak/issues',
    )
    await expect(page.getByTestId('about-link-email')).toHaveAttribute(
      'href',
      'mailto:midagedev@gmail.com',
    )
    await expect(page.getByTestId('about-link-x')).toHaveAttribute('href', 'https://x.com/midagedev')
  })
})
