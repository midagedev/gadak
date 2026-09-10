import { test, expect } from './helpers'
import { apiURL, attachConsoleErrors, gotoApp, openServerSettings, DEMO_ISSUE_COUNT_EN_RE } from './helpers'
import { en } from '../web/src/lib/i18n/en'

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

    for (const tab of [
      'Sources',
      'Features',
      'Terminal',
      'Teams',
      'Members',
      'Fields',
      'About',
    ]) {
      await dialog.getByRole('tab', { name: tab, exact: true }).click()
      await expect(mirror, `mirror must not render on the ${tab} tab`).toHaveCount(0)
    }

    await dialog.getByRole('tab', { name: 'Sync', exact: true }).click()
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
  test('the Teams rule row fits inside the dialog (GDK-1052)', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('tab', { name: 'Teams', exact: true }).click()

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
    // The seed covers today AND tomorrow (e2e/serve.sh, GDK-1592): this figure
    // is the UTC day at request time, the seed ran at serve start, and a shard
    // that crosses midnight would otherwise assert against an empty day.
    // Grouped or not (GDK-1560 made every count go through formatNumber, so
    // en renders the seed as "1,204"): what must not pass is a figure under
    // four digits, which is what a dropped runtime.apiUsage would leave.
    await expect(dialog.getByText(/(?:\d{1,3}(?:,\d{3})+|\d{4,}) today/)).toBeVisible()
    await expect(dialog.getByText('2 throttled')).toBeVisible()
  })
})

/*
 * Settings-audit contracts (false copy / dead toggle) moved off the browser
 * by the GDK-1702 cost ladder: the empty-project-picker label, the GDK-476
 * settings lead and the About-tab hrefs were three app boots asserting
 * strings that live in the catalogs and the components — they are
 * web/src/components/settings/settings-copy.test.ts now (source-scan, the
 * FeaturesTab.test.ts idiom that took the web-push toggle before them).
 * The audit they came from failed pre-fix on: sourcesNoProjects said "no
 * issue is mirrored" / "미러링되는 이슈가 없습니다", and Features rendered a
 * "Web push" checkbox that saved a flag whose endpoints 404.
 */

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
    await dialog.getByRole('tab', { name: 'Sources', exact: true }).click()

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
    await dialog.getByRole('tab', { name: 'Features', exact: true }).click()
    await dialog.getByRole('tab', { name: 'Sources', exact: true }).click()
    await expect(picker).toBeVisible()
    expect(spacesCalls, 'space list requests: 1 failed + 1 retry, no refetch after').toBe(2)
  })
})

/*
 * Moved from ux-f12.spec.ts (v0.21 audit ladder round): the sources tab's
 * hung-list and Confluence-confirm states, beside the retry case above. The
 * /tmp/f12-shots captures were deleted with the move.
 */
test.describe('sources tab failure states (GDK-476)', () => {
  test('a hung sources list leaves Loading for an error + manual keys', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const API = apiURL('/api/v1/issues/')
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
    await page.route(`${API}meta/write/`, async (route) => {
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
    await dialog.getByRole('tab', { name: 'Sources', exact: true }).click()
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

    expect(
      errors.filter((e) => !e.includes('ERR_FAILED') && !e.includes('Failed to load resource')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })

  test('turning Confluence on for every space needs a second click', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const API = apiURL('/api/v1/issues/')
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
    await dialog.getByRole('tab', { name: 'Sources', exact: true }).click()

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

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
