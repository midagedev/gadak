import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings } from './helpers'

const PAGES_URL = 'http://127.0.0.1:7877/api/v1/issues/pages/'

/** Open PROD → Feature Specs → the named page, the way docs.spec walks the tree. */
async function openDocFromTree(page: Page, title: string): Promise<void> {
  const section = page.getByTestId('docs-section')
  await section.getByTestId('docs-space').filter({ hasText: 'PROD' }).click()
  await section
    .getByTestId('doc-tree-node')
    .filter({ hasText: 'Feature Specs' })
    .getByTestId('doc-tree-toggle')
    .click()
  await section
    .getByTestId('doc-tree-node')
    .filter({ hasText: title })
    .getByRole('button', { name: title, exact: true })
    .click()
  await expect(page.getByTestId('doc-title')).toHaveText(title)
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

  test('Recently updated lists every space by last edit, newest first', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('docs-recent').click()
    const view = page.getByTestId('recent-docs-view')
    await expect(view).toBeVisible()
    // It replaces the issue list in the main column.
    await expect(page.getByTestId('issue-list-scroller')).toHaveCount(0)

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

    const rows = view.getByTestId('recent-doc-row')
    await expect(rows).toHaveCount(expected.length)
    // Rows carry the space and a timestamp too, so compare the leading title only.
    const titles = await rows.evaluateAll((els) =>
      els.map((el) => el.querySelector('span')?.textContent?.trim() ?? ''),
    )
    expect(titles).toEqual(expected)

    // A row opens the document beside the list, which stays put.
    await rows.first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(expected[0])
    await expect(view).toBeVisible()

    // And the sidebar entry toggles back to the issue list.
    await page.getByTestId('docs-recent').click()
    await expect(view).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()

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
