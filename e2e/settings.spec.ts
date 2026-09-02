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
    const mirror = dialog.getByRole('region', { name: 'This local copy' })

    // Sync is the default tab.
    await expect(mirror).toHaveCount(1)

    // Below the tab's own controls, not above them: the intervals are the
    // subject of the tab, the mirror is the reference under it.
    const order = await dialog.evaluate((root) => {
      const region = root.querySelector('section[aria-label="This local copy"]')
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

  /*
   * GDK-1052: INPUT/SELECT used to carry w-full, which beat the rule row's
   * w-24 by Tailwind emission order (class order in the attribute decides
   * nothing). Measured: the group input rendered ~726px, its three flex-1
   * siblings collapsed to ~18px, and the last input ended past the dialog
   * edge. The unit lint (controls.test.ts) guards the source; this pins the
   * visible geometry the lint cannot see.
   */
  test('the Teams / groups rule row fits inside the dialog (GDK-1052)', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Teams / groups', exact: true }).click()

    // Rule rows only: their group input is the tab's sole input.w-24 (the
    // group-label and product rows above it use flex-1 inputs).
    const group = dialog.locator('input.w-24').first()
    await expect(group, 'fixture seeds group rules; a rule row must render').toBeVisible()
    const row = group.locator('xpath=ancestor::div[1]')
    const inputs = row.locator('input')
    await expect(inputs).toHaveCount(4)

    const [dlg, g, flex1, flex2, flex3, last] = await Promise.all([
      dialog.boundingBox(),
      group.boundingBox(),
      inputs.nth(1).boundingBox(),
      inputs.nth(2).boundingBox(),
      inputs.nth(3).boundingBox(),
      inputs.nth(3).boundingBox(),
    ])
    if (!dlg || !g || !flex1 || !flex2 || !flex3 || !last) throw new Error('boundingBox vanished')

    expect(g.width, 'group input must take its w-24 (6rem), not the base w-full').toBeLessThanOrEqual(96.5)
    for (const [name, box] of [
      ['projects', flex1],
      ['labels', flex2],
      ['components', flex3],
    ] as const) {
      expect(box.width, `flex-1 ${name} input collapsed`).toBeGreaterThanOrEqual(100)
    }
    expect(last.x + last.width, 'last rule input must end inside the dialog').toBeLessThanOrEqual(
      dlg.x + dlg.width + 0.5,
    )
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

/*
 * GDK-1061: the Sources-tab scope lists load once per dialog (the guard
 * that keeps a Jira/Confluence round-trip off every tab switch), which made
 * a failed space list a dead end — the only retry was closing and reopening
 * the whole dialog. The failed state now carries a Retry that re-arms this
 * list's guard only; the success path still loads exactly once, which the
 * request counter at the bottom pins (tab switches must not refetch).
 * Same route-mock pattern as the copy-contract test above.
 */
test.describe('sources tab scope-list retry (GDK-1061)', () => {
  test('a failed space list shows Retry which reloads it; success still loads once', async ({
    page,
  }) => {
    const API = apiURL('/api/v1/issues/')
    await page.route(`${API}settings/`, (route) =>
      route.fulfill({
        json: { projects: [], staleThresholdHours: 72 },
      }),
    )
    await page.route(`${API}projects/available/`, (route) =>
      route.fulfill({
        json: { projects: [], truncated: false },
      }),
    )
    let spacesCalls = 0
    await page.route(`${API}settings/spaces/`, (route) => {
      spacesCalls++
      if (spacesCalls === 1) return route.fulfill({ status: 500, json: { error: 'unavailable' } })
      return route.fulfill({
        json: {
          spaces: [{ key: 'ENG', name: 'Engineering', type: 'team' }],
          all_global_when_empty: false,
        },
      })
    })

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()

    // The failed state: the existing unavailable copy, plus the Retry the
    // issue asked for.
    await expect(dialog.getByTestId('scope-spaces-error')).toContainText(
      'Could not read the space list',
    )
    await dialog.getByTestId('scope-spaces-retry').click()

    // The retry re-requested and the picker is back, with the loaded list:
    // focus the combobox, the option row is the proof the list landed.
    const picker = dialog.getByTestId('scope-spaces')
    await expect(picker).toBeVisible()
    await picker.getByTestId('scope-input').click()
    await expect(picker.getByTestId('scope-option')).toContainText('Engineering')

    // The once-guard survived the retry: leaving and re-entering the tab
    // does not refetch (a successful load is still exactly-once).
    await dialog.getByRole('button', { name: 'Features', exact: true }).click()
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()
    await expect(picker).toBeVisible()
    expect(spacesCalls, 'space list requests: 1 failed + 1 retry, no refetch after').toBe(2)
  })
})
