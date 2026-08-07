import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings, walkRows } from './helpers'

const PAGES_URL = 'http://127.0.0.1:7877/api/v1/issues/pages/'

/** Open PROD's space screen → Tree → Feature Specs → the named page. */
async function openDocFromTree(page: Page, title: string): Promise<void> {
  await page.getByTestId('docs-spaces').click()
  await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'PROD' }).click()
  const view = page.getByTestId('space-docs-view')
  await view.getByTestId('space-tree-toggle').click()
  await view
    .getByTestId('doc-tree-node')
    .filter({ hasText: 'Feature Specs' })
    .getByTestId('doc-tree-toggle')
    .click()
  await view
    .getByTestId('doc-tree-node')
    .filter({ hasText: title })
    .getByRole('button', { name: title, exact: true })
    .click()
  await expect(page.getByTestId('doc-title')).toHaveText(title)
}

/** Open the tabbed Documents view from the sidebar. */
async function openDocuments(page: Page): Promise<void> {
  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible()
}

test.describe('documents in the daily loop', () => {
  test('a document just read shows up in the palette next to recent issues', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await openDocFromTree(page, 'Billing Settings Spec')

    // Close the panel so the palette is the only thing on screen.
    await page.keyboard.press('x')
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // Empty query = the recent list, and the page sits in it as a DOC row
    // carrying its space — not as a bare key like an issue.
    const docRow = palette.getByTestId('palette-doc-row')
    await expect(docRow).toHaveCount(1)
    await expect(docRow).toContainText('Billing Settings Spec')
    await expect(docRow).toContainText('Doc')
    await expect(docRow).toContainText('PROD')

    // Choosing it reopens the document rather than an issue.
    await docRow.click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText('Billing Settings Spec')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Documents opens on Viewed, and a read page lands there as one sentence', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Nothing read yet: the default tab says so rather than falling back to
    // someone else's activity (UX_PRINCIPLES §6 — viewed wins the default).
    await openDocuments(page)
    const view = page.getByTestId('docs-view')
    await expect(view.getByTestId('docs-tab').filter({ hasText: 'Viewed' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    await expect(view).toContainText('Documents you open will appear here')
    // It replaces the issue list in the main column.
    await expect(page.getByTestId('issue-list-scroller')).toHaveCount(0)

    await openDocFromTree(page, 'Billing Settings Spec')
    await openDocuments(page)

    const rows = view.getByTestId('doc-row')
    await expect(rows).toHaveCount(1)
    // One sentence: who, when, where. Every mirrored page in the snapshot has an
    // author, so the dropped-author case is covered by the component's guard
    // rather than by fixture data.
    await expect(rows.first()).toContainText('Billing Settings Spec')
    await expect(rows.first()).toContainText('Alex Kim')
    await expect(rows.first()).toContainText('in PROD')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Updated keeps the mirror sort, and the chosen tab survives a reload', async ({
    page,
    request,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    const view = page.getByTestId('docs-view')
    await view.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()

    // Expected order is computed from the mirror itself, so the assertion tests
    // the client sort rather than restating it.
    const res = await request.get(PAGES_URL)
    expect(res.ok()).toBeTruthy()
    const body = (await res.json()) as {
      pages: { title: string; updated_at: string | null }[]
    }
    const expected = [...body.pages]
      .sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''))
      .map((p) => p.title)

    // Walked rather than counted: the list is windowed, so the DOM holds a
    // screenful. Walking asserts the same thing the count used to — every page,
    // in the mirror's order.
    const walked = await walkRows(view.getByTestId('docs-scroll'))
    expect(walked.map((r) => r.title)).toEqual(expected)

    const rows = view.getByTestId('doc-row')
    // The list is a window, and the scrollbar still describes the whole thing.
    // Both halves matter: rendering every row is the freeze this replaced, and
    // a scroll height that only covers the window is a list you cannot reach
    // the end of. 30 is well above a viewport's worth and well below 71.
    expect(await rows.count()).toBeLessThan(30)
    const geometry = await view.getByTestId('docs-scroll').evaluate((el) => ({
      scrollHeight: el.scrollHeight,
      clientHeight: el.clientHeight,
    }))
    expect(geometry.scrollHeight).toBeGreaterThan(geometry.clientHeight * 3)

    // A row opens the document beside the list, which stays put.
    await rows.first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(expected[0])
    await expect(view).toBeVisible()

    // The sidebar entry toggles back to the issue list.
    await page.getByTestId('docs-documents').click()
    await expect(view).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()

    // After a reload the tab is still the one that was left open.
    //
    // The screen comes back on its own: the panel was never closed, so the URL
    // still carries `doc=`, and a page in the address restores the screen that
    // page lives on (2026-08-07 — docs-deeplink.spec.ts owns that rule). The
    // sidebar entry is deliberately not clicked here; it is a toggle, and on a
    // screen that is already open it would close it.
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await expect(view).toBeVisible()
    await expect(view.getByTestId('docs-tab').filter({ hasText: 'Updated' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('By author groups the whole mirror, newest edit first inside each group', async ({
    page,
    request,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    const view = page.getByTestId('docs-view')
    await view.getByTestId('docs-tab').filter({ hasText: 'By author' }).click()

    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as {
      pages: { title: string; author: string | null; updated_at: string | null }[]
    }
    // Groups ordered by each author's newest edit (pages.svelte byAuthor), not
    // API insertion order — matches the Updated tab's recency-first rule.
    const byAuthor = new Map<string, typeof body.pages>()
    for (const p of body.pages) {
      const a = p.author ?? ''
      const list = byAuthor.get(a)
      if (list) list.push(p)
      else byAuthor.set(a, [p])
    }
    const authorGroups = [...byAuthor.entries()]
      .map(([author, list]) => ({
        author,
        pages: [...list].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? '')),
      }))
      .sort((a, b) => (b.pages[0]?.updated_at ?? '').localeCompare(a.pages[0]?.updated_at ?? ''))
    const authors = authorGroups.map((g) => g.author)

    const scroller = view.getByTestId('docs-scroll')
    await expect(view.getByTestId('docs-author-group').first()).toContainText(authors[0])
    const walkedGroups = await walkRows(scroller, {
      rowTestId: 'docs-author-group',
      keyAttr: 'data-author',
    })
    expect(walkedGroups.map((g) => g.key)).toEqual(authors)

    // Every page is still listed — grouping regroups, it does not filter.
    const titles = (await walkRows(scroller)).map((r) => r.title)
    expect(titles.length).toBe(body.pages.length)

    // First group's pages appear first (newest edit inside the group); later
    // groups follow. Assert the leading slice, not the whole list (multi-author).
    const first = authorGroups[0].pages.map((p) => p.title)
    expect(titles.slice(0, first.length)).toEqual(first)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the activity lists carry one line of the body; navigation surfaces do not', async ({
    page,
    request,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    // Read one page first, so the Viewed assertion below runs against a row that
    // exists rather than against an empty tab.
    await openDocFromTree(page, 'Billing Settings Spec')
    await openDocuments(page)

    const view = page.getByTestId('docs-view')
    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as {
      pages: { title: string; updated_at: string | null; excerpt?: string }[]
    }
    const newest = [...body.pages].sort((a, b) =>
      (b.updated_at ?? '').localeCompare(a.updated_at ?? ''),
    )[0]
    // Guard the fixture, not the UI: an all-empty snapshot would let every
    // assertion below pass against a row that renders nothing.
    expect(newest.excerpt, 'demo snapshot has no excerpts to show').toBeTruthy()

    // Updated is someone else's activity — the title alone often cannot say
    // whether the edit is worth opening, so the row shows the body's first line.
    await view.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    const first = view.getByTestId('doc-row').first()
    await expect(first.getByTestId('doc-excerpt')).toHaveText(newest.excerpt!)

    // Exactly one line, whatever the excerpt's length: density is the point, and
    // a wrapping preview would quietly become a second row height.
    const lines = await first.getByTestId('doc-excerpt').evaluate((el) => {
      const lineHeight = parseFloat(getComputedStyle(el).lineHeight)
      return el.getBoundingClientRect().height / lineHeight
    })
    expect(lines).toBeCloseTo(1, 1)

    // It never outranks the meta line it sits under (UX_PRINCIPLES §5).
    const weight = await first.evaluate((row) => {
      const px = (el: Element) => parseFloat(getComputedStyle(el).fontSize)
      const color = (el: Element) => getComputedStyle(el).color
      const meta = row.children[1]
      const excerpt = row.querySelector('[data-testid="doc-excerpt"]')!
      return {
        sameOrSmaller: px(excerpt) <= px(meta),
        sameOrQuieter: color(excerpt) === color(meta),
      }
    })
    expect(weight).toEqual({ sameOrSmaller: true, sameOrQuieter: true })

    // By author asks the same question of the same mirror, so it shows the same.
    await view.getByTestId('docs-tab').filter({ hasText: 'By author' }).click()
    await expect(view.getByTestId('doc-excerpt').first()).toBeVisible()

    // Viewed is your own return path: you have read these, so a preview is
    // noise. (Every fixture page has a body, so the empty-excerpt case is left
    // to the component's guard rather than to snapshot data.)
    await view.getByTestId('docs-tab').filter({ hasText: 'Viewed' }).click()
    await expect(view.getByTestId('doc-row')).toHaveCount(1)
    await expect(view.getByTestId('doc-excerpt')).toHaveCount(0)

    // A space is reached to get somewhere, not to browse — neither its flat list
    // nor its tree becomes a discovery surface.
    // The Spaces disclosure is still open from the tree step above, so the space
    // is clicked directly — toggling it again would close the list.
    await page
      .getByTestId('docs-section')
      .getByTestId('docs-space')
      .filter({ hasText: 'ENG' })
      .click()
    const spaceView = page.getByTestId('space-docs-view')
    await expect(spaceView.getByTestId('doc-row').first()).toBeVisible()
    await expect(spaceView.getByTestId('doc-excerpt')).toHaveCount(0)

    await spaceView.getByTestId('space-tree-toggle').click()
    await expect(spaceView.getByTestId('doc-tree-node').first()).toBeVisible()
    await expect(spaceView.getByTestId('doc-excerpt')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a document edited since the last visit is marked unread', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)

    // A visit older than the page's last edit — the one thing the local mirror
    // can compute that the originals cannot (UX_PRINCIPLES §6).
    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as { pages: { key: string; title: string }[] }
    const stale = body.pages[0]
    await page.addInitScript((key: string) => {
      localStorage.setItem(
        'scry:recent',
        JSON.stringify([{ key, viewed_at: '2000-01-01T00:00:00.000Z', kind: 'doc' }]),
      )
    }, stale.key)

    await gotoApp(page)
    await openDocuments(page)

    const rows = page.getByTestId('docs-view').getByTestId('doc-row')
    await expect(rows).toHaveCount(1)
    await expect(rows.first()).toHaveAttribute('data-unread', 'true')
    await expect(rows.first()).toContainText(stale.title)

    // Opening it clears the mark: the visit is now newer than the edit.
    await rows.first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(rows.first()).not.toHaveAttribute('data-unread', 'true')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a space opens flat from the disclosure and drops the repeated space suffix', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('docs-spaces').click()
    await page
      .getByTestId('docs-section')
      .getByTestId('docs-space')
      .filter({ hasText: 'ENG' })
      .click()

    const view = page.getByTestId('space-docs-view')
    await expect(view).toBeVisible()
    expect((await walkRows(view.getByTestId('space-list-scroll'))).length).toBe(43)
    // The space is the screen, so the row's "in ENG" clause would be noise.
    await expect(view.getByTestId('doc-row').first()).not.toContainText('in ENG')
    await expect(view.getByTestId('doc-row').first()).toContainText('Alex Kim')

    // Tree is available on the same screen and gives the hierarchy back.
    await view.getByTestId('space-tree-toggle').click()
    await expect(view.getByTestId('doc-tree-node').first()).toBeVisible()
    await expect(view.getByTestId('doc-row')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Sources tab picks Jira projects and offers to turn Confluence on', async ({
    page,
  }) => {
    // No console-error assertion here on purpose: the fixture credential is a
    // fake, so the live project list (projects/available/) answers with an
    // error the browser logs. That failure is the path under test.
    await gotoApp(page)
    await openServerSettings(page)

    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()

    const sources = dialog.getByTestId('settings-sources')
    await expect(sources).toBeVisible()

    // The fixture profile has no Confluence block. The section is still here:
    // while it was hidden, this screen was the only place the source could have
    // been turned on and it did not offer to — so an unconfigured source and a
    // missing feature looked the same. It now says which, and offers the switch.
    const confluence = sources.getByTestId('sources-confluence')
    await expect(confluence).toBeVisible()
    await expect(confluence).toContainText('Off for this profile')
    await expect(confluence.getByTestId('confluence-turn-off')).toHaveCount(0)
    const turnOn = confluence.getByTestId('confluence-turn-on')
    await expect(turnOn).toBeVisible()
    // Nothing selected yet, so the button states the consequence rather than
    // leaving "every space you can read" as a side effect of a generic verb.
    // "team space", not "every space": an empty scope mirrors global spaces
    // only — personal ones are opt-in by name (internal/sync/confluence.go).
    await expect(turnOn).toHaveText('Turn on for every team space')

    // Turning it on flips the control and raises the unscoped warning.
    await turnOn.click()
    await expect(confluence.getByTestId('confluence-all-warning')).toBeVisible()
    await expect(confluence.getByTestId('confluence-turn-off')).toBeVisible()
    // Turning it back off drops the scope with it — a stored selection under an
    // off source is a promise nothing keeps.
    await confluence.getByTestId('confluence-turn-off').click()
    await expect(confluence.getByTestId('confluence-turn-on')).toBeVisible()
    await expect(confluence.getByTestId('confluence-all-warning')).toHaveCount(0)

    // Nothing is asserted about the space picker here: this fixture's site does
    // not resolve, so whether the list lands as empty, as an error or as still
    // loading is a property of a socket timing out, not of the app. The two
    // stubbed tests below own the picker's states.

    // The site list is unreachable, so the picker falls back to manual keys and
    // still shows what is configured.
    const fallback = sources.getByTestId('scope-projects-fallback')
    await expect(fallback).toBeVisible()
    await expect(fallback.locator('input')).toHaveValue('NMB, NMA, NMS')
    await expect(sources.getByTestId('scope-projects')).toHaveCount(0)

    // Scope edits land on the next sync, and the tab says so.
    await expect(sources).toContainText('next sync')

    await dialog.getByRole('button', { name: 'Close' }).click()
    await expect(dialog).toHaveCount(0)
  })

  test('with a reachable site, both scopes are typeahead pickers', async ({ page }) => {
    // The fixture credential cannot reach Jira or Confluence, so the two source
    // lists are stubbed here. Nothing is saved: this covers the picker itself,
    // not the write path.
    const API = 'http://127.0.0.1:7877/api/v1/issues/'
    await page.route(`${API}settings/`, (route) =>
      route.fulfill({
        json: {
          projects: ['NMB'],
          // Empty list with the source on = "every global space", which is the
          // state the picker has to explain rather than show as blank.
          confluence: { spaces: [] },
          staleThresholdHours: 72,
        },
      }),
    )
    await page.route(`${API}settings/spaces/`, (route) =>
      route.fulfill({
        json: {
          spaces: [
            { key: 'ENG', name: 'Engineering', type: 'global', selected: false },
            { key: 'PROD', name: 'Nimbus Product', type: 'global', selected: false },
            { key: '~dana', name: 'Dana Kim', type: 'personal', selected: false },
          ],
          all_global_when_empty: true,
          enabled: true,
        },
      }),
    )
    await page.route(`${API}projects/available/`, (route) =>
      route.fulfill({
        json: {
          projects: [
            { key: 'NMB', name: 'Nimbus Backend', projectTypeKey: 'software' },
            { key: 'NMA', name: 'Nimbus App', projectTypeKey: 'software' },
          ],
          truncated: false,
        },
      }),
    )

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()

    // Projects: the configured key arrives as a chip, and typing narrows the list.
    const projects = dialog.getByTestId('scope-projects')
    await expect(projects).toBeVisible()
    await expect(projects.getByTestId('scope-chip')).toHaveText([/NMB/])

    await projects.getByTestId('scope-input').fill('app')
    const options = projects.getByTestId('scope-option')
    await expect(options).toHaveCount(1)
    await expect(options.first()).toContainText('Nimbus App')
    await page.keyboard.press('Enter')
    await expect(projects.getByTestId('scope-chip')).toHaveText([/NMB/, /NMA/])

    // Confluence is on here, so the section offers to turn it off rather than on,
    // and names the consequence of the unscoped state it is currently in.
    const confluence = dialog.getByTestId('sources-confluence')
    await expect(confluence.getByTestId('confluence-turn-off')).toBeVisible()
    await expect(confluence.getByTestId('confluence-turn-on')).toHaveCount(0)
    await expect(confluence.getByTestId('confluence-all-warning')).toBeVisible()

    // Spaces: no selection is a meaning, not a blank.
    const spaces = dialog.getByTestId('scope-spaces')
    await expect(spaces).toBeVisible()
    await expect(spaces.getByTestId('scope-empty')).toContainText('every team (global) space')

    // Personal spaces are one per colleague — hidden until asked for.
    await spaces.getByTestId('scope-input').click()
    await expect(spaces.getByTestId('scope-option')).toHaveCount(2)
    await dialog.getByLabel('Show personal spaces').check()
    await expect(spaces.getByTestId('scope-option')).toHaveCount(3)

    // Picking one replaces the "all global" state with an explicit scope.
    await spaces.getByTestId('scope-input').fill('Engineering')
    await spaces.getByTestId('scope-option').first().click()
    await expect(spaces.getByTestId('scope-chip')).toHaveText([/ENG/])
    await expect(spaces.getByTestId('scope-empty')).toHaveCount(0)
  })

  test('choosing a space while Confluence is off saves it as on', async ({ page }) => {
    /*
     * The write path, which the picker test above deliberately does not cover —
     * and which is where this broke: a space was picked, the chip appeared, and
     * Save sent {enabled:false, spaces:[]}, discarding it without a word. The
     * assertion is the PUT body, because the screen looked correct throughout.
     */
    const API = 'http://127.0.0.1:7877/api/v1/issues/'
    await page.route(`${API}settings/`, async (route) => {
      if (route.request().method() !== 'PUT') {
        // Confluence off: no `confluence` key at all.
        await route.fulfill({ json: { projects: ['NMB'], staleThresholdHours: 72 } })
        return
      }
      await route.fulfill({ json: JSON.parse(route.request().postData() ?? '{}') })
    })
    await page.route(`${API}settings/spaces/`, (route) =>
      route.fulfill({
        json: {
          spaces: [{ key: 'ENG', name: 'Engineering', type: 'global', selected: false }],
          all_global_when_empty: false,
          enabled: false,
        },
      }),
    )

    const puts: string[] = []
    page.on('request', (r) => {
      if (r.method() === 'PUT' && r.url().includes('settings/')) puts.push(r.postData() ?? '')
    })

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Sources', exact: true }).click()

    const confluence = dialog.getByTestId('sources-confluence')
    await expect(confluence.getByTestId('confluence-turn-on')).toBeVisible()

    const spaces = dialog.getByTestId('scope-spaces')
    // Click first: the list opens on focus, and fill() alone leaves it closed.
    await spaces.getByTestId('scope-input').click()
    await spaces.getByTestId('scope-input').fill('Engineering')
    await spaces.getByTestId('scope-option').first().click()
    await expect(spaces.getByTestId('scope-chip')).toHaveText([/ENG/])

    // Visible before it is saved: choosing a space is the request to mirror it,
    // so the control flips without a second click to confirm what was asked.
    await expect(confluence.getByTestId('confluence-turn-off')).toBeVisible()
    await expect(confluence.getByTestId('confluence-turn-on')).toHaveCount(0)

    await dialog.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => puts.length).toBeGreaterThan(0)
    expect(JSON.parse(puts[0]).confluence).toEqual({ enabled: true, spaces: ['ENG'] })
  })
})

/*
 * Why a row is on screen, and what else is like it.
 *
 * The filter used to remove rows and say nothing about the ones it kept, and
 * the labels the server has always sent were dropped by the client entirely —
 * so a document list could neither explain itself nor be narrowed by the axis
 * Confluence actually files pages under.
 */
test.describe('the document lists explain themselves', () => {
  test('a label on a row narrows the screen to that label', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    // Expected from the mirror, not restated here: this also pins that the
    // client keeps the labels the server has been sending all along.
    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as {
      pages: { key: string; labels?: string[]; updated_at: string | null }[]
    }
    const labelled = body.pages.filter((p) => (p.labels ?? []).includes('runbook'))
    expect(labelled.length, 'fixture must carry the label under test').toBe(6)

    await gotoApp(page)
    await openDocuments(page)

    const view = page.getByTestId('docs-view')
    await view.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    await expect(view.getByTestId('docs-count')).toHaveText('71')

    // Typed first only to bring a labelled row into the window — the list is
    // windowed, so "the chip is somewhere below" is not something to click.
    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    await filter.fill('Rate Limit Storm')
    const chip = view.getByTestId('doc-label').filter({ hasText: 'runbook' }).first()
    await expect(chip).toBeVisible()
    await chip.click()

    // Both narrowings are on, and they are AND-ed: one page is both.
    await expect(view.getByTestId('docs-label-chip')).toHaveAttribute('data-label', 'runbook')
    await expect(view.getByTestId('docs-count')).toHaveText('1 / 71')

    // Clearing the text leaves the label behind — six pages carry it in the
    // mirror, and the count is the receipt for that.
    await filter.fill('')
    await expect(view.getByTestId('docs-count')).toHaveText('6 / 71')
    // The rows themselves, walked — a page carrying three labels shows two of
    // them, so what is on screen is not the test of what was kept.
    const walked = await walkRows(view.getByTestId('docs-scroll'))
    expect(new Set(walked.map((r) => r.key))).toEqual(new Set(labelled.map((p) => p.key)))

    // The chip in the header is the way out of it, in the place it is stated.
    await view.getByTestId('docs-label-chip').click()
    await expect(view.getByTestId('docs-label-chip')).toHaveCount(0)
    await expect(view.getByTestId('docs-count')).toHaveText('71')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('labels are on the lists that answer "what else", and off the rest', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocFromTree(page, 'Billing Settings Spec')
    await openDocuments(page)

    const view = page.getByTestId('docs-view')
    // Viewed is a return path: the row is a bookmark, not a query to follow.
    await expect(view.getByTestId('docs-tab').filter({ hasText: 'Viewed' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    await expect(view.getByTestId('doc-row')).toHaveCount(1)
    await expect(view.getByTestId('doc-label')).toHaveCount(0)

    // The activity lists carry them, capped at two so the row stays a sentence.
    await view.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    await expect(view.getByTestId('doc-label').first()).toBeVisible()
    for (const row of await view.getByTestId('doc-row').all()) {
      expect(await row.getByTestId('doc-label').count()).toBeLessThanOrEqual(2)
    }

    // The tree is read to see where something sits, not to browse by subject.
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }).click()
    const spaceView = page.getByTestId('space-docs-view')
    await expect(spaceView.getByTestId('doc-label').first()).toBeVisible()
    await spaceView.getByTestId('space-tree-toggle').click()
    await expect(spaceView.getByTestId('doc-tree-node').first()).toBeVisible()
    await expect(spaceView.getByTestId('doc-label')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the filter marks what it matched, in the clause that matched', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    const view = page.getByTestId('docs-view')
    await view.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    await filter.fill('runbook')

    // Every row kept says why it was kept — a list that filters silently reads
    // as an arbitrary list (the same gap the issue rows closed).
    const rows = view.getByTestId('doc-row')
    await expect(rows.first().locator('mark').first()).toHaveText(/runbook/i)
    for (const row of await rows.all()) {
      expect(await row.locator('mark').count()).toBeGreaterThan(0)
    }

    // Every clause the row draws, not only the ones the filter reads. The
    // excerpt and the labels are outside the haystack (doc-search.ts), and they
    // used to be the two places where the query was sitting in plain sight with
    // nothing on it — which reads as the highlighting having missed, not as a
    // statement about what is searched.
    const marked = (testid: string) =>
      view.getByTestId(testid).filter({ has: page.locator('mark') }).count()
    expect(await marked('doc-excerpt'), 'marks in the excerpt line').toBeGreaterThan(0)
    expect(await marked('doc-label'), 'marks in a label chip').toBeGreaterThan(0)

    // A row matched by its space is marked there instead: the mark follows the
    // match, so it never claims the title matched when the space did.
    await filter.fill('ENG')
    const spaceMatched = rows
      .filter({ hasNot: page.locator('span').first().locator('mark') })
      .first()
    await expect(spaceMatched).toBeVisible()
    await expect(rows.first().locator('mark').filter({ hasText: 'ENG' }).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the tree tells an answer from the path to it', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('docs-spaces').click()
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }).click()
    const view = page.getByTestId('space-docs-view')
    await view.getByTestId('space-tree-toggle').click()

    // A parent says how much is under it, so a collapsed branch is not a
    // guess — the root of ENG holds eight sections in the mirror.
    const root = view.getByTestId('doc-tree-node').first()
    await expect(root.getByTestId('doc-tree-count')).toHaveText('8')

    // A child's chevron begins exactly where its parent's title does. The step
    // used to be narrower than the control it indents past, which left every
    // level slightly out of true.
    const parentTitle = await root.getByRole('button').nth(1).boundingBox()
    const child = view.getByTestId('doc-tree-node').nth(1)
    const childToggle = await child.getByTestId('doc-tree-toggle').boundingBox()
    expect(Math.abs((childToggle?.x ?? 0) - (parentTitle?.x ?? 0))).toBeLessThanOrEqual(1)

    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    await filter.fill('Rate Limit Storm')

    // The hit and the two rows that say where it lives are all on screen, and
    // they are not the same weight: context recedes, the answer does not.
    const hit = view.getByTestId('doc-tree-node').filter({ hasText: 'Rate Limit Storm' })
    await expect(hit).toHaveAttribute('data-hit', 'true')
    const context = view.getByTestId('doc-tree-node').filter({ hasText: 'Runbooks' }).first()
    await expect(context).toHaveAttribute('data-hit', 'false')
    // One mark for the phrase. The filter still answers word by word — marking
    // the query as one literal string leaves a matched row showing no reason —
    // but three marks with the spaces between them left outside drew a phrase
    // as a staircase, so runs that only whitespace separates are rejoined.
    expect(await hit.locator('mark').allInnerTexts()).toEqual(['Rate Limit Storm'])
    await expect(context.locator('mark')).toHaveCount(0)

    const tone = await view.evaluate(() => {
      const nodes = [...document.querySelectorAll('[data-testid="doc-tree-node"]')]
      const titleColor = (row: Element) =>
        getComputedStyle(row.querySelectorAll('button')[row.querySelectorAll('button').length - 1])
          .color
      const answer = nodes.find((n) => n.getAttribute('data-hit') === 'true')!
      const path = nodes.find((n) => n.getAttribute('data-hit') === 'false')!
      return { answer: titleColor(answer), path: titleColor(path) }
    })
    expect(tone.answer).not.toBe(tone.path)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

})

/*
 * An empty DOCS section, told apart by cause.
 *
 * The section used to have one sentence for four situations, and it was the
 * wrong one for the person who had just chosen a space and saved: the sidebar
 * went on asking them to connect a source they had already connected. These
 * tests pin each cause to its own sentence — the CTA belongs to "off" alone.
 */
test.describe('docs empty states', () => {
  const RUNS_CONFLUENCE = '**/sync/runs/**'

  /** Serve a mirror with no pages, and say whether Confluence is configured. */
  async function emptyDocs(page: Page, confluenceEnabled: boolean): Promise<void> {
    await page.route('**/config.json', async (route) => {
      const res = await route.fetch()
      const doc = JSON.parse(await res.text())
      doc.confluenceEnabled = confluenceEnabled
      await route.fulfill({ response: res, body: JSON.stringify(doc) })
    })
    await page.route(PAGES_URL, (route) => route.fulfill({ json: { pages: [] } }))
  }

  /** Confluence's own run history. Anything else (Jira's) passes through. */
  async function confluenceRuns(page: Page, runs: unknown[]): Promise<void> {
    await page.route(RUNS_CONFLUENCE, (route) => {
      if (!route.request().url().includes('source=confluence')) return route.continue()
      return route.fulfill({ json: { runs, source: 'confluence' } })
    })
  }

  test('off: the section asks for the one thing missing', async ({ page }) => {
    await emptyDocs(page, false)
    await gotoApp(page)

    const cta = page.getByTestId('docs-empty-cta')
    await expect(cta).toHaveAttribute('data-state', 'off')
    await expect(cta).toContainText('No documents mirrored')
    await expect(cta).toContainText('Turn on Confluence')

    // It is a way in, not a label: the errand it names is one click away.
    await cta.click()
    await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible()
  })

  test('on but never fetched: it offers the sync, not the setup', async ({ page }) => {
    await emptyDocs(page, true)
    await confluenceRuns(page, [])
    await gotoApp(page)

    const cta = page.getByTestId('docs-empty-cta')
    await expect(cta).toHaveAttribute('data-state', 'never')
    await expect(cta).toContainText('Documents not fetched yet')
    // The old copy would have sent someone who is already set up back to Settings.
    await expect(cta).not.toContainText('Turn on Confluence')
  })

  test('on and the last pass failed: it says so, and carries the reason', async ({ page }) => {
    await emptyDocs(page, true)
    await confluenceRuns(page, [
      {
        kind: 'full',
        started_at: '2026-08-07T08:00:00Z',
        finished_at: '2026-08-07T08:00:04Z',
        fetched: 0,
        changed: 0,
        deleted: 0,
        error: 'confluence: 403 forbidden',
      },
    ])
    await gotoApp(page)

    const cta = page.getByTestId('docs-empty-cta')
    await expect(cta).toHaveAttribute('data-state', 'failed')
    await expect(cta).toContainText('Could not fetch documents')
    // The reason is on the row, not in a log the user will never open.
    await expect(cta).toHaveAttribute('title', /403/)
  })

  test('on, fetched, and the spaces are empty: it blames the selection', async ({ page }) => {
    await emptyDocs(page, true)
    await confluenceRuns(page, [
      {
        kind: 'full',
        started_at: '2026-08-07T08:00:00Z',
        finished_at: '2026-08-07T08:00:04Z',
        fetched: 0,
        changed: 0,
        deleted: 0,
      },
    ])
    await gotoApp(page)

    const cta = page.getByTestId('docs-empty-cta')
    await expect(cta).toHaveAttribute('data-state', 'empty')
    await expect(cta).toContainText('No documents in these spaces')
    await expect(cta).toContainText('Change the selection')
  })
})

/*
 * One mirror, one sentence.
 *
 * The app reported sync in three places that could disagree: the sidebar's sync
 * row ("Synced 3m ago"), the freshness chip ("Syncing"), and the DOCS section
 * ("Fetching documents") — at the same moment, about the same pass. Read
 * together, issue sync and document sync looked like two systems. These pin
 * that every surface renders the same string, and that the count is in it.
 */
test.describe('sync status is one sentence', () => {
  const API = 'http://127.0.0.1:7877/api/v1/issues/'

  /**
   * Stub the process-wide activity slot. `agoMs` ages the pass.
   *
   * started_at is stamped once, not per request: a timestamp regenerated on
   * every poll describes a pass that restarts forever, which is not a state the
   * server can be in and which quietly defeats any test of elapsed time.
   */
  async function activity(
    page: Page,
    a: { running: boolean; source: string; fetched: number },
    agoMs = 60_000,
  ): Promise<void> {
    const startedAt = new Date(Date.now() - agoMs).toISOString()
    await page.route(`${API}sync/progress/`, (route) =>
      route.fulfill({
        json: {
          running: false,
          phase: 'idle',
          fetched: 0,
          changed: 0,
          deleted: 0,
          done: false,
          error: '',
          started_at: '',
          finished_at: '',
          activity: { ...a, changed: a.fetched, started_at: startedAt },
        },
      }),
    )
  }

  test('a document pass names itself, with the count, in every surface', async ({ page }) => {
    await activity(page, { running: true, source: 'documents', fetched: 350 })
    await page.route(`${API}pages/`, (route) => route.fulfill({ json: { pages: [] } }))
    await page.route('**/config.json', async (route) => {
      const res = await route.fetch()
      const doc = JSON.parse(await res.text())
      doc.confluenceEnabled = true
      await route.fulfill({ response: res, body: JSON.stringify(doc) })
    })
    await gotoApp(page)

    const expected = 'Fetching documents · 350'
    // The sidebar's sync row: this is the one that used to say "Synced 3m ago"
    // while the section below it claimed to be fetching.
    await expect(page.getByTestId('sidebar-sync-now')).toContainText(expected, { timeout: 20_000 })
    await expect(page.getByTestId('freshness-chip')).toContainText(expected)
    await expect(page.getByTestId('docs-empty-cta')).toContainText(expected)
  })

  test('an issue pass says issues, not documents', async ({ page }) => {
    await activity(page, { running: true, source: 'issues', fetched: 6932 })
    await gotoApp(page)

    await expect(page.getByTestId('sidebar-sync-now')).toContainText('Syncing issues · 6,932', {
      timeout: 20_000,
    })
  })

  test('a pass is announced only once it has run long enough to wonder about', async ({ page }) => {
    // The watch loop finishes an incremental in a second or two, every minute.
    // Narrating those would put a blinking status in front of someone all day;
    // the six-minute backfill is what needed saying. So the rule is elapsed
    // time, and this asserts both halves of it rather than the quiet first
    // instant, which would pass with no rule at all.
    await activity(page, { running: true, source: 'issues', fetched: 2 }, 500)
    await gotoApp(page)

    const chip = page.getByTestId('freshness-chip')
    await page.waitForTimeout(1_500) // two polls in, still young
    await expect(chip).not.toContainText('Syncing')
    // Same stubbed pass, now old enough: the wording appears without anything
    // else changing, which is the threshold and not a coincidence of timing.
    await expect(chip).toContainText('Syncing issues', { timeout: 15_000 })
  })
})

test.describe('sync status at rest', () => {
  test('the row and the chip report the mirror identically, and it fits', async ({ page }) => {
    // At rest the two used to describe different things with the same verb: the
    // row read the browser↔server delta cursor ("Synced 18:12"), the chip read
    // the mirror's age ("Synced yesterday"). Only the second is what anyone
    // means by synced — a delta poll keeps the screen current with a mirror
    // that stopped yesterday.
    await gotoApp(page)

    const row = page.getByTestId('sidebar-sync-now')
    const chip = page.getByTestId('freshness-chip')
    const rowText = ((await row.textContent()) ?? '').trim()
    const chipText = ((await chip.textContent()) ?? '').trim()
    expect(rowText).toBe(chipText)
    // The verdict travels with the age: "delayed" alone never says how far
    // behind, and an age alone never says that being behind is a problem.
    expect(rowText).toMatch(/·/)

    // The sidebar row shares its line with the issue count, and the settled
    // string is now the longest it has ever been. Measure rather than trust it.
    const fits = await row.evaluate((el) => {
      const line = el.closest('div')?.parentElement
      if (!line) return true
      return el.getBoundingClientRect().right <= line.getBoundingClientRect().right + 1
    })
    expect(fits).toBe(true)
  })
})
