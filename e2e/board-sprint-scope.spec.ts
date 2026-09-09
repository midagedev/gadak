import { test, expect } from './helpers'
import { DEMO_ISSUE_COUNT_EN_RE, attachConsoleErrors, forceLocale, gotoApp } from './helpers'

/*
 * GDK-1656 — the board's sprint scope. Active sprint · Backlog · All is a
 * filter on `sprint_state`, so the URL carries it (`sst=active`, `sst=none`)
 * and the back button undoes it; the control exists only because the demo
 * fixture has sprints (derived at `make demo-fixture` from the "Sprint N"
 * fix versions: 41 closed, 42 active, 43 future — 20 issues in the active
 * one). A kanban workspace with no sprints never shows it.
 */

test.describe('GDK-1656 board sprint scope', () => {
  test.beforeEach(async ({ page }) => {
    await forceLocale(page, 'en')
  })

  test('scopes the board to the active sprint, the backlog, and back to all through the URL', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.getByTestId('view-settings').click()
    await page.getByTestId('layout-board').click()
    await expect(page.getByTestId('board')).toBeVisible()

    const scope = page.getByTestId('sprint-scope')
    await expect(scope).toBeVisible()
    // The active segment names the one active sprint, not the generic word.
    await expect(page.getByTestId('sprint-scope-active')).toHaveText('Sprint 42')
    await expect(page.getByTestId('sprint-scope-all')).toHaveAttribute('aria-pressed', 'true')

    const allCount = await page.getByTestId('list-count').textContent()

    await page.getByTestId('sprint-scope-active').click()
    await expect(page).toHaveURL(/sst=active/)
    await expect(page.getByTestId('sprint-scope-active')).toHaveAttribute('aria-pressed', 'true')
    // The scope narrows the same view the startup filters already shaped
    // (open issues), so the number is "what is on the board", not the
    // sprint's whole membership: the count and the cards must agree.
    await expect(page.getByTestId('list-count')).not.toHaveText(allCount ?? '')
    const activeCount = await page.getByTestId('list-count').textContent()
    await expect(page.getByTestId('board-card')).toHaveCount(parseInt(activeCount ?? '0', 10))
    // The scope is a filter like any other: it shows as a chip and leaves
    // the list layout untouched.
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'Sprint state: Active sprint' })).toBeVisible()

    await page.getByTestId('sprint-scope-backlog').click()
    await expect(page).toHaveURL(/sst=none/)
    await expect(page.getByTestId('list-count')).not.toHaveText(activeCount ?? '')
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'Sprint state: No sprint' })).toBeVisible()

    await page.getByTestId('sprint-scope-all').click()
    await expect(page).not.toHaveURL(/sst=/)
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible()

    // Back walks the scope, because the scope lives in the URL.
    await page.goBack()
    await expect(page).toHaveURL(/sst=none/)
    await expect(page.getByTestId('sprint-scope-backlog')).toHaveAttribute('aria-pressed', 'true')

    expect(errors).toEqual([])
  })

  test('the Sprint axis lists the sprints by name and a shared sst= link opens scoped', async ({ page }) => {
    await gotoApp(page)
    await page.getByTestId('filter-add').click()
    await page.getByTestId('filter-axis-sprint_ids').click()
    // Inside the menu only: the board's scope control names the same sprint.
    const menu = page.getByTestId('filter-add').locator('..')
    await expect(menu.getByTestId('filter-value-row').filter({ hasText: 'Sprint 42' })).toBeVisible()
    await page.keyboard.press('Escape')

    // A link the CLI made (`views open --jql 'sprint in openSprints()'`)
    // arrives as the same hash key internal/jql/hash.go writes.
    await page.goto('/#/?ly=board&sst=active')
    await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('sprint-scope-active')).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'Active sprint' })).toBeVisible({
      timeout: 30_000,
    })
  })
})
