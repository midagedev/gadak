import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * Weekly retro as a column view (GDK-1660). The demo fixture carries no
 * visits of its own, so what a fresh e2e home yields is measured first
 * (GET /api/v1/issues/retro/ on the e2e serve: buckets with sessions 0 and
 * closed / in progress counts from the mirror) and the assertions read that:
 * the table renders, the closed cell is a door when it has keys, the URL
 * round-trips, Esc returns the column to the list.
 */
test.describe('weekly retro view', () => {
  test('opens from the palette, renders one column per week, and round-trips its URL', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await palette.getByTestId('palette-action-retro').click()

    const view = page.getByTestId('retro-view')
    await expect(view).toBeVisible()
    await expect(view.getByText('Weekly retro').first()).toBeVisible()
    // "4w" is four whole ISO weeks plus the partial current one (measured on
    // the e2e serve: five buckets, the last with partial=true).
    await expect(page.getByTestId('retro-week')).toHaveCount(5)
    await expect(page.getByTestId('retro-week').last()).toContainText('this week')
    await expect(page.getByTestId('retro-table')).toBeVisible()
    expect(page.url()).toContain('retro=1')

    // The address restores the view on its own.
    await page.reload()
    await expect(page.getByTestId('retro-view')).toBeVisible()

    // Esc hands the column back to the list.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(page.url()).not.toContain('retro=1')

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })

  test('a cell with issues behind it opens them on the list', async ({ page }) => {
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()
    const cells = page.getByTestId('retro-cell')
    // The demo mirror has issues in progress and closed inside four weeks
    // (measured on the fixture's spread), so at least one door exists.
    await expect(cells.first()).toBeVisible()
    const metric = await cells.first().getAttribute('data-metric')
    await cells.first().click()
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    // A keys view: the URL carries the key list, not a filter.
    await expect.poll(() => page.url()).toContain('ks=')
    expect(metric).toBeTruthy()
  })
  /*
   * GDK-1679: the report knows why a cell is empty and the CLI has always
   * printed it; this view showed the dashes and dropped the sentence, so a
   * mirror with no status_catalog read as a broken feature. Now that the demo
   * fixture derives a catalog the real server never sends a note, so the note
   * is stubbed — this is the FAIL-first for the half that was actually broken.
   */
  test('a report that names a missing table prints that reason under the table', async ({ page }) => {
    const NOTE =
      'empty — the buckets before the current one show no value for wip age p85, wip age max and in progress, and closed shows none everywhere; a sync fills the table'
    await page.route('**/api/v1/issues/retro/**', async (route) => {
      const res = await route.fetch()
      const body = await res.json()
      body.notes = [{ name: 'status_catalog', text: NOTE }]
      await route.fulfill({ response: res, json: body })
    })
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()

    const notes = page.getByTestId('retro-notes')
    await expect(notes).toBeVisible()
    await expect(notes).toContainText('status_catalog')
    await expect(notes).toContainText('a sync fills the table')
  })

  /*
   * And the empty state must not swallow it: a cold mirror has no sessions,
   * no closures and nothing in progress, which is exactly when the reason
   * matters most. "No sessions in this range" there blames sessions for a
   * missing table (GDK-1679).
   */
  test('an empty report with a reason shows the reason, not the empty copy', async ({ page }) => {
    await page.route('**/api/v1/issues/retro/**', async (route) => {
      const res = await route.fetch()
      const body = await res.json()
      body.buckets = body.buckets.map((b: Record<string, unknown>) => ({
        ...b,
        sessions: 0,
        closed: null,
        'in progress': null,
      }))
      body.notes = [{ name: 'status_catalog', text: 'empty — a sync fills the table' }]
      await route.fulfill({ response: res, json: body })
    })
    await gotoApp(page)
    await page.goto('/#/?retro=1')

    await expect(page.getByTestId('retro-notes')).toContainText('a sync fills the table')
    await expect(page.getByTestId('retro-table')).toBeVisible()
  })
})

/*
 * GDK-1693: the columns can be sprints instead of ISO weeks — the unit a
 * scrum team actually retrospects on. The fixture's board has three derived
 * sprints (41 closed, 42 active, 43 future), so the cut yields two columns:
 * the future one has no window to measure. Every definition names the unit,
 * which is why the sentence under a row must say sprint here and week in the
 * default view.
 */
test.describe('retro by sprint', () => {
  test('the range control cuts the report by sprint and the definitions follow', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()
    await expect(page.getByTestId('retro-week')).toHaveCount(5)
    // The default cut is weeks, and the definitions say so.
    await expect(page.getByTestId('retro-table')).toContainText('at week end')

    await page.getByTestId('retro-range').filter({ hasText: 'By sprint' }).click()
    // Sprint 41 and the running Sprint 42; Sprint 43 starts in the future.
    await expect(page.getByTestId('retro-week')).toHaveCount(2)
    await expect(page.getByTestId('retro-week').first()).toContainText('Sprint 41')
    await expect(page.getByTestId('retro-week').last()).toContainText('Sprint 42')
    // The partial column says which unit is still filling.
    await expect(page.getByTestId('retro-week').last()).toContainText('running')
    // …and the sentence under every row names a sprint, not a week.
    await expect(page.getByTestId('retro-table')).toContainText('at sprint end')
    await expect(page.getByTestId('retro-table')).not.toContainText('at week end')

    // A cell is still a door.
    const cell = page.locator('[data-testid="retro-cell"][data-metric="closed"]').last()
    await expect(cell).toBeVisible()
    await cell.click()
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect.poll(() => page.url()).toContain('ks=')

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })
})
