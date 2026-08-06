import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/** Click a visible tree row's title (each row also holds a toggle button). */
async function openDoc(page: Page, title: string): Promise<void> {
  await page
    .getByTestId('doc-tree-node')
    .filter({ hasText: title })
    .getByRole('button', { name: title, exact: true })
    .click()
}

/**
 * Open a space's screen and switch it to Tree. The tree moved out of the sidebar
 * in r9 (recency beats hierarchy) — it is now one toggle on the space screen,
 * which is the only place it can still be reached.
 */
async function openSpaceTree(page: Page, space: string): Promise<void> {
  await page.getByTestId('docs-spaces').click()
  await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: space }).click()
  await expect(page.getByTestId('space-docs-view')).toBeVisible()
  await page.getByTestId('space-tree-toggle').click()
}

test.describe('mirrored wiki documents', () => {
  test('a space opens as a flat list, and the tree is one toggle away', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const section = page.getByTestId('docs-section')
    await expect(section).toBeVisible()

    // Spaces are behind a collapsed disclosure — the sidebar shows the entry, not
    // the containers.
    await expect(section.getByTestId('docs-space')).toHaveCount(0)
    await section.getByTestId('docs-spaces').click()

    // demo.db mirrors 71 pages: ENG 43 + PROD 28.
    const spaces = section.getByTestId('docs-space')
    await expect(spaces).toHaveCount(2)
    await expect(spaces.filter({ hasText: 'ENG' })).toContainText('43')
    await expect(spaces.filter({ hasText: 'PROD' })).toContainText('28')

    // Space rows read as the space's name. The snapshot predates the name mirror
    // (space_name arrives empty), so this is the fallback: the key itself, and
    // the tooltip carries it either way.
    await expect(spaces.filter({ hasText: 'ENG' })).toHaveAttribute(
      'title',
      'ENG · 43 documents',
    )

    // A space opens flat, every page in it, newest edit first — no tree.
    await spaces.filter({ hasText: 'PROD' }).click()
    const view = page.getByTestId('space-docs-view')
    await expect(view).toBeVisible()
    await expect(view.getByTestId('doc-row')).toHaveCount(28)
    await expect(view.getByTestId('doc-tree-node')).toHaveCount(0)

    // Tree is the same screen, one toggle: PROD's root page and the level under
    // it (6 sections) — not the whole space.
    await view.getByTestId('space-tree-toggle').click()
    const nodes = view.getByTestId('doc-tree-node')
    await expect(nodes).toHaveCount(7)
    await expect(nodes.first()).toContainText('Nimbus Product Home')
    await expect(nodes.filter({ hasText: 'Billing Settings Spec' })).toHaveCount(0)

    // A node's toggle expands that node only: Feature Specs holds 5 pages.
    await nodes.filter({ hasText: 'Feature Specs' }).getByTestId('doc-tree-toggle').click()
    await expect(nodes).toHaveCount(12)
    await expect(nodes.filter({ hasText: 'Billing Settings Spec' })).toHaveCount(1)

    // Clicking a title opens the document panel with that title in the header.
    await openDoc(page, 'Billing Settings Spec')
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText('Billing Settings Spec')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('document breadcrumb shows the trail and walks back up it', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await openSpaceTree(page, 'PROD')
    await page
      .getByTestId('doc-tree-node')
      .filter({ hasText: 'Feature Specs' })
      .getByTestId('doc-tree-toggle')
      .click()
    await openDoc(page, 'Billing Settings Spec')

    // Space key, then every ancestor, then the open page.
    const panel = page.getByTestId('doc-panel')
    const crumb = panel.getByTestId('doc-breadcrumb')
    await expect(crumb).toBeVisible()
    await expect(crumb).toContainText('PROD')
    await expect(crumb.getByTestId('doc-breadcrumb-ancestor')).toHaveText([
      'Nimbus Product Home',
      'Feature Specs',
    ])

    // An ancestor is a link back to that page — the trail shortens with it.
    await crumb.getByTestId('doc-breadcrumb-ancestor').filter({ hasText: 'Feature Specs' }).click()
    await expect(panel.getByTestId('doc-title')).toHaveText('Feature Specs')
    await expect(crumb.getByTestId('doc-breadcrumb-ancestor')).toHaveText(['Nimbus Product Home'])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('server search surfaces documents alongside issues and opens one', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // '빌링' hits no issue field but two Korean pages — the case that only works
    // because the search response carries pages.
    const input = searchInput(page)
    await input.fill('빌링')
    await input.press('Enter')

    const rows = page.getByTestId('search-doc-row')
    await expect(rows).toHaveCount(2)
    await expect(rows.first()).toContainText('PROD')

    await rows.filter({ hasText: '제품 회의록 — 빌링 품질' }).click()

    const panel = page.getByTestId('doc-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByTestId('doc-title')).toHaveText('제품 회의록 — 빌링 품질')
    // The space reads from the breadcrumb, and a hit opened from search — never
    // expanded in the tree — still gets its full trail from the page index.
    await expect(panel.getByTestId('doc-breadcrumb')).toContainText('PROD')
    await expect(panel.getByTestId('doc-breadcrumb-ancestor')).toHaveText([
      'Nimbus Product Home',
      'Product Meetings',
    ])
    // Body ADF rendered (first heading of the mirrored page).
    await expect(panel.getByRole('heading', { name: '요약' })).toBeVisible()
    // Read-only surface: no comment composer, unlike the issue panel.
    await expect(panel.locator('textarea')).toHaveCount(0)

    // Clearing the query drops the docs group.
    await input.fill('')
    await input.press('Escape')
    await expect(page.getByTestId('search-doc-row')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
