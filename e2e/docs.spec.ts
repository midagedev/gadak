import { test, expect, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput, walkRows } from './helpers'

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
    // Walked, not counted: the list is windowed and holds a screenful at a time.
    expect((await walkRows(view.getByTestId('space-list-scroll'))).length).toBe(28)
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

    // '빌링' hits no issue field but two page bodies (CJK in body_text) — the case
    // that only works because the search response carries pages.
    const input = searchInput(page)
    await input.fill('빌링')
    await input.press('Enter')

    const rows = page.getByTestId('search-doc-row')
    await expect(rows).toHaveCount(2)
    await expect(rows.first()).toContainText('PROD')

    await rows.filter({ hasText: 'Product Meeting Notes — Billing Quality' }).click()

    const panel = page.getByTestId('doc-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByTestId('doc-title')).toHaveText(
      'Product Meeting Notes — Billing Quality',
    )
    // The space reads from the breadcrumb, and a hit opened from search — never
    // expanded in the tree — still gets its full trail from the page index.
    await expect(panel.getByTestId('doc-breadcrumb')).toContainText('PROD')
    await expect(panel.getByTestId('doc-breadcrumb-ancestor')).toHaveText([
      'Nimbus Product Home',
      'Product Meetings',
    ])
    // Body ADF rendered (first heading of the mirrored page).
    await expect(panel.getByRole('heading', { name: '요약' })).toBeVisible()
    // The only write surface is the page comment composer (GDK-381) — the
    // body itself stays read-only in the panel.
    await expect(panel.getByTestId('doc-comment-composer')).toHaveCount(1)

    // Clearing the query drops the docs group.
    await input.fill('')
    await input.press('Escape')
    await expect(page.getByTestId('search-doc-row')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('page comment: thread shows the body the origin returned', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const SERVER_COMMENT = 'wt-e2e page comment from origin'
    const TYPED_COMMENT = 'wt-e2e typed page comment'

    type PageJSON = Record<string, unknown> & { comments?: unknown[] }
    let pageJSON: PageJSON | null = null

    async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
      await route.fulfill({ status, contentType: 'application/json', json })
    }

    await page.route('**/api/v1/issues/pages/*/', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      const response = await route.fetch()
      pageJSON = (await response.json()) as PageJSON
      await route.fulfill({ response, json: pageJSON })
    })
    await page.route('**/api/v1/issues/pages/*/comment/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      const comments = Array.isArray(pageJSON?.comments) ? [...pageJSON.comments] : []
      comments.push({
        author: 'Dana Whitfield',
        created_at: '2026-08-15T12:00:00.000Z',
        body_adf: {
          type: 'doc',
          version: 1,
          content: [{ type: 'paragraph', content: [{ type: 'text', text: SERVER_COMMENT }] }],
        },
        body_text: SERVER_COMMENT,
      })
      await fulfillJSON(route, { page: { ...pageJSON, comments } })
    })

    await gotoApp(page)
    await openSpaceTree(page, 'PROD')
    await page
      .getByTestId('doc-tree-node')
      .filter({ hasText: 'Feature Specs' })
      .getByTestId('doc-tree-toggle')
      .click()
    await openDoc(page, 'Billing Settings Spec')

    const panel = page.getByTestId('doc-panel')
    await expect(panel.getByTestId('doc-comment-composer')).toBeVisible()
    expect(pageJSON, 'GET pages/{id}/ must have run before the comment POST').toBeTruthy()

    await panel.getByTestId('doc-comment-composer').fill(TYPED_COMMENT)
    await panel.getByTestId('doc-comment-submit').click()

    await expect(panel.getByTestId('doc-comment-composer')).toHaveValue('')
    await expect(panel.getByText(SERVER_COMMENT)).toBeVisible()
    await expect(panel.getByText(TYPED_COMMENT)).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
