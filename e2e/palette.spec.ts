import { test, expect } from '@playwright/test'
import { apiURL, attachConsoleErrors, gotoApp, searchInput, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

test.describe('command palette', () => {
  test('Cmd+K opens it, typing stays local, Enter opens the issue detail', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Let the post-boot API chatter (bootstrap/write-meta) settle first.
    // Wait for the boot payload to land, plus trailing boot / focus-time
    // pull to finish. Deliberately not networkidle: the app polls for a delta
    // every 15s, so "no network for 500ms" only becomes true after that poll
    // fires, which added 15s to this test for nothing.
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    // Same observable ux-p1.spec.ts uses: chip data-state=syncing is
    // mirrorBusy (focus-time pull or background pass), not a duration.
    await expect(page.getByTestId('freshness-chip')).not.toHaveAttribute('data-state', 'syncing', {
      timeout: 30_000,
    })

    const apiDuringType: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (!url.includes('/api/') || url.includes('/ui-focus/')) return
      let path = url
      try {
        path = new URL(url).pathname
      } catch {
        /* keep raw */
      }
      apiDuringType.push(`${req.method()} ${path}`)
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('NMB-110', { delay: 20 })
    const first = palette.getByRole('option').first()
    await expect(first).toContainText('NMB-110')
    await expect(first).toHaveAttribute('aria-selected', 'true')

    expect(
      apiDuringType,
      `in-flight /api/ while typing (must be none): ${apiDuringType.join(', ') || '(none)'}`,
    ).toEqual([])

    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('opens while a text input has focus, and Esc closes it', async ({ page }) => {
    await gotoApp(page)

    await searchInput(page).click()
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // Empty query lists views and actions (no query typed yet).
    await expect(palette.getByRole('option', { name: /New issue/ })).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(palette).toBeHidden()
  })

  test('empty query with no visits still lists recently updated issues', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.addInitScript(() => {
      localStorage.setItem('gadak:recent', '[]')
    })
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // GDK-184: a brand-new profile has no visits, but the home must still
    // be a list — recently-updated issues from the already-loaded pool.
    const options = palette.getByRole('option')
    await expect(options.first()).toBeVisible()
    expect(await options.count()).toBeGreaterThan(0)

    await expect(palette.locator('[data-section="updated"]')).toBeVisible()
    await expect(palette.locator('[data-section="updated"]')).toHaveText(/recently updated/i)

    const updated = palette.getByTestId('palette-updated-row')
    await expect(updated.first()).toBeVisible()
    const n = await updated.count()
    expect(n).toBeGreaterThan(0)
    expect(n).toBeLessThanOrEqual(5)

    await expect(palette.locator('[data-section="recent"]')).toHaveCount(0)

    // ↑↓ walks the flat item list across sections (no extra nav state).
    await page.keyboard.press('ArrowDown')
    await expect(options.nth(1)).toHaveAttribute('aria-selected', 'true')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-191: the Views section mixed row shapes — built-ins had a glyph and the
   * saved kinds had none, and every sub line was a single kind word, so the
   * rows said what they were and never what they would open. Both halves are
   * read off data the palette already holds: the built-in hint and the saved
   * view's own config. A clue that cost a request would break the zero-network
   * rule this palette is built on (file header), so the request count is part
   * of the same assertion.
   */
  test('every view row carries a kind icon and a clue from its own config', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const API = apiURL('/api/v1/issues/')

    // A personal view lives in localStorage; the fixture ships the Jira filter
    // ("Open in NMA", NMA + statusCategory). The team layer has none, so this
    // adds one to the real response rather than replacing it.
    // The GDK-437 one-shot would absorb this fixture's localStorage view into
    // the server on boot — the personal row would come back as a server row
    // and pollute the shared local.db. A failed absorb keeps it personal (the
    // store retries next boot), which is the shape this spec is about. The
    // failure is an unparseable 200, not an error status: the browser logs
    // every non-ok response to the console, and this spec asserts none.
    await page.route(`${API}views/absorb/`, (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: 'absorb-disabled-for-this-spec' }),
    )
    await page.addInitScript(() => {
      localStorage.setItem(
        'gadak:personal-views',
        JSON.stringify([
          {
            id: 'e2e-personal',
            name: 'Nimbus blocked',
            created_at: new Date().toISOString(),
            config: {
              filters: {
                jira_project: ['NMB'],
                status_category: ['inprogress'],
                labels: ['api'],
              },
              display: { group_by: 'status_category', sort: 'updated', dir: 'desc' },
            },
          },
        ]),
      )
    })
    await page.route(`${API}views/`, async (route) => {
      if (route.request().method() !== 'GET') return route.fallback()
      const res = await route.fetch()
      const body = (await res.json()) as { views?: unknown[] }
      body.views = [
        ...(body.views ?? []),
        {
          id: 'e2e-team',
          name: 'Release triage',
          owner_email: 'dana@example.com',
          owner_name: 'Dana',
          created_at: null,
          updated_at: null,
          config: {
            filters: { jira_project: ['NMB', 'NMA', 'NMS'], stale: true },
            display: { group_by: 'status_category', sort: 'updated', dir: 'desc' },
          },
        },
      ]
      await route.fulfill({ response: res, json: body })
    })

    await gotoApp(page)

    const apiAfterOpen: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (url.includes('/api/') && !url.includes('/ui-focus/')) apiAfterOpen.push(url)
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // Empty query: the saved kinds take the section (builtin-views.ts keeps its
    // own glyph, so the built-in row is asserted with a query below).
    const rows = {
      personal: palette.getByRole('option', { name: /My view/ }),
      team: palette.getByRole('option', { name: /Team view/ }),
      source: palette.getByRole('option', { name: /Jira filter/ }),
    }
    for (const [kind, row] of Object.entries(rows)) {
      await expect(row, `${kind} row`).toHaveCount(1)
      await expect(row.locator('svg'), `${kind} row icon`).toHaveCount(1)
    }

    // The clue is what the view opens, off the config the row already holds.
    await expect(rows.personal).toContainText('NMB')
    await expect(rows.personal).toContainText('2 filters')
    // Three keys stop being a clue and become a list, so they collapse to a count.
    await expect(rows.team).toContainText('3 projects')
    await expect(rows.team).toContainText('1 filter')
    await expect(rows.source).toContainText('NMA')

    // A built-in answers with its written hint instead of a filter count.
    await page.keyboard.type('Stale', { delay: 20 })
    const builtin = palette.getByRole('option', { name: /Built-in view/ }).first()
    await expect(builtin).toContainText('Stuck in one status too long')
    await expect(builtin.locator('svg')).toHaveCount(1)

    expect(
      apiAfterOpen.filter((u) => !u.includes('/search/')),
      `expected no /api/ requests from the palette, got:\n${apiAfterOpen.join('\n')}`,
    ).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('empty query lists recently viewed first and keeps those keys out of updated', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.addInitScript(() => {
      localStorage.setItem(
        'gadak:recent',
        JSON.stringify([{ key: 'NMB-110', viewed_at: new Date().toISOString(), kind: 'issue' }]),
      )
    })
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await expect(palette.locator('[data-section="recent"]')).toBeVisible()
    await expect(palette.getByRole('option').first()).toContainText('NMB-110')

    const updated = palette.getByTestId('palette-updated-row')
    await expect(updated.first()).toBeVisible()
    await expect(updated.filter({ hasText: /^NMB-110\b/ })).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-300: typing the Settings action's distinctive word must open Settings
   * in en. The label is "Open settings"; an issue title that merely contains
   * "settings" used to win the ranking, so the same keystroke was reachable
   * in ko (설정) and not in en.
   */
  test('typing settings + Enter opens the Settings dialog', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('settings', { delay: 20 })
    const first = palette.getByRole('option').first()
    await expect(first).toContainText('Open settings')
    await expect(first).toHaveAttribute('aria-selected', 'true')

    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()
    await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Open in Jira is an action when the list has a cursor', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('Open in Jira', { delay: 15 })
    const first = palette.getByRole('option').first()
    await expect(first).toContainText('Open in Jira')
    await expect(first).toHaveAttribute('aria-selected', 'true')
    await expect(first.locator('kbd')).toHaveText('o')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test(', opens the Settings dialog', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press(',')
    await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-618: the palette footer advertises `?` as the way to the shortcuts
   * sheet, but while the palette owns the screen the global keymap ignores
   * every key (keymap.svelte.ts), and the palette's own handler never
   * claimed it — the advertised key just typed into the query. The palette
   * owns `?` now, but only on an empty query: mid-query `?` is a search
   * character, and the sheet must not open over the palette.
   */
  test('? on an empty query swaps the palette for the advertised shortcuts sheet', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // Empty query: the advertised key is honored, and handing the screen
    // over means the palette closes for the sheet.
    await page.keyboard.press('?')
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('shortcuts-dialog')).toBeVisible()

    // Mid-query `?` stays an ordinary character — "a?" must be searchable.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('shortcuts-dialog')).toBeHidden()
    await page.keyboard.press('ControlOrMeta+k')
    await expect(palette).toBeVisible()
    await page.keyboard.type('a', { delay: 15 })
    await page.keyboard.press('?')
    await expect(palette.getByRole('combobox')).toHaveValue('a?')
    await expect(page.getByTestId('shortcuts-dialog')).toHaveCount(0)
    await expect(palette).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
