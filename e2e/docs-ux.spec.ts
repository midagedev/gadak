import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings } from './helpers'

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

/** The title line of each document row (the meta line is the second span). */
async function rowTitles(rows: import('@playwright/test').Locator): Promise<string[]> {
  return rows.evaluateAll((els) => els.map((el) => el.querySelector('span')?.textContent?.trim() ?? ''))
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

    const rows = view.getByTestId('doc-row')
    await expect(rows).toHaveCount(expected.length)
    expect(await rowTitles(rows)).toEqual(expected)

    // A row opens the document beside the list, which stays put.
    await rows.first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(expected[0])
    await expect(view).toBeVisible()

    // The sidebar entry toggles back to the issue list.
    await page.getByTestId('docs-documents').click()
    await expect(view).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()

    // Reopening after a reload lands on the tab that was left open.
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await openDocuments(page)
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

    const groups = view.getByTestId('docs-author-group')
    await expect(groups).toHaveCount(authors.length)
    await expect(groups.first()).toContainText(authors[0])
    // Every page is still listed — grouping regroups, it does not filter.
    await expect(view.getByTestId('doc-row')).toHaveCount(body.pages.length)

    // First group's pages appear first (newest edit inside the group); later
    // groups follow. Assert the leading slice, not the whole list (multi-author).
    const first = authorGroups[0].pages.map((p) => p.title)
    const titles = await rowTitles(view.getByTestId('doc-row'))
    expect(titles.slice(0, first.length)).toEqual(first)

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
    await expect(view.getByTestId('doc-row')).toHaveCount(43)
    // The space is the screen, so the row's "in ENG" clause would be noise.
    await expect(view.getByTestId('doc-row').first()).not.toContainText('in ENG')
    await expect(view.getByTestId('doc-row').first()).toContainText('Alex Kim')

    // Tree is available on the same screen and gives the hierarchy back.
    await view.getByTestId('space-tree-toggle').click()
    await expect(view.getByTestId('doc-tree-node').first()).toBeVisible()
    await expect(view.getByTestId('doc-row')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Sources tab picks Jira projects and hides spaces while Confluence is off', async ({
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

    // The fixture profile has no Confluence block, so there is no space scope to
    // edit — the section is absent rather than empty.
    await expect(sources.getByTestId('scope-spaces')).toHaveCount(0)

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
})
