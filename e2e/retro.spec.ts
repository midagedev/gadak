import { test, expect } from '@playwright/test'
import type { Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * Unfold the complete table.
 *
 * GDK-1724 moved it to the foot of the report and folded it: the sections
 * above are the first read and the grid is where a number gets checked. The
 * specs below were written when it was the whole screen, so they open it
 * first — the assertions about what is *in* it are unchanged.
 */
async function openTable(page: Page): Promise<void> {
  await page.getByTestId('retro-table-toggle').click()
  await expect(page.getByTestId('retro-table')).toBeVisible()
}

/*
 * Weekly retro as a column view (GDK-1660). What a fresh e2e home yields is
 * measured first (GET /api/v1/issues/retro/ on the e2e serve) and the
 * assertions read that: the table renders, the closed cell is a door when it
 * has keys, the URL round-trips, Esc returns the column to the list.
 *
 * The serve now seeds local.db with a month of reading (GDK-1720), so the
 * session rows have real values instead of the zeros this file used to
 * measure — the last case below is the gate on that, and the two cases that
 * need an empty report stub it rather than relying on a blank fixture.
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
    // The column headers live inside the fold now, so the table is opened
    // before they are counted (GDK-1724).
    await openTable(page)
    // "4w" is four whole ISO weeks plus the partial current one (measured on
    // the e2e serve: five buckets, the last with partial=true).
    await expect(page.getByTestId('retro-week')).toHaveCount(5)
    await expect(page.getByTestId('retro-week').last()).toContainText('this week')
    expect(page.url()).toContain('retro=1')

    // GDK-1712: the summary strip names the bucket still filling and reads
    // the four numbers out of it, so the question "how is this week going"
    // is answered above the grid rather than in its rightmost column.
    const summary = page.getByTestId('retro-summary')
    await expect(summary).toBeVisible()
    await expect(summary.getByTestId('retro-summary-cell')).toHaveCount(4)
    await expect(page.getByTestId('retro-summary-title')).toContainText('this week')

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
    await openTable(page)
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
    // The reason stands outside the fold: it is the answer to "why is the
    // table empty", and a reader who has to open the table to find it has
    // already concluded the feature is broken (GDK-1679).
  })

  /*
   * And the empty state must not swallow it: a cold mirror has no sessions,
   * no closures and nothing in progress, which is exactly when the reason
   * matters most. "No sessions in this range" there blames sessions for a
   * missing table (GDK-1679).
   */
  /*
   * GDK-1720: sessions, resume and seen-vs-touched all read local.visits, and
   * nothing in the fixture pipeline ever wrote a visit — so the demo, every
   * recording and every run of this file saw zeros on the half of the retro
   * built on reading. e2e/serve.sh seeds that history now, and this is the
   * gate: a run whose local.db went back to empty puts the zeros back.
   */
  test('the session rows read the seeded browsing history, not zeros', async ({ page }) => {
    await gotoApp(page)
    const res = await page.request.get('/api/v1/issues/retro/?since=4w')
    expect(res.ok()).toBe(true)
    const body = (await res.json()) as { buckets: { sessions?: number }[] }
    const total = body.buckets.reduce((sum, b) => sum + (b.sessions ?? 0), 0)
    expect(total, `buckets: ${JSON.stringify(body.buckets.map((b) => b.sessions))}`).toBeGreaterThan(0)
  })

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
    await openTable(page)
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
    await openTable(page)
    await expect(page.getByTestId('retro-week')).toHaveCount(5)
    // GDK-1712: the definitions are folded away by default — eight
    // paragraphs standing between the reader and the numbers is what forced
    // the 260px label column. The toggle is where they live now.
    await expect(page.getByTestId('retro-def')).toHaveCount(0)
    await page.getByTestId('retro-defs-toggle').click()
    // The default cut is weeks, and the definitions say so.
    await expect(page.getByTestId('retro-table')).toContainText('at week end')

    await page.getByTestId('retro-range').filter({ hasText: 'By sprint' }).click()
    // Sprint 41 and the running Sprint 42; Sprint 43 starts in the future.
    await expect(page.getByTestId('retro-week')).toHaveCount(2)
    await expect(page.getByTestId('retro-week').first()).toContainText('Sprint 41')
    await expect(page.getByTestId('retro-week').last()).toContainText('Sprint 42')
    // The partial column says which unit is still filling.
    await expect(page.getByTestId('retro-week').last()).toContainText('running')
    // …and the sentence under every row names a sprint, not a week. The
    // fold survives the cut: a person who opened the definitions keeps them.
    await expect(page.getByTestId('retro-def')).toHaveCount(8)
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

/*
 * GDK-1713: several boards with sprints is a question, not a failure. The
 * demo fixture has one board — the server picks it and the cut just works —
 * so the refusal is stubbed. That is the half that was actually broken: the
 * view read every non-2xx as `retro.loadFailed`, one line with no boards in
 * it and nothing for the reader to do, while the server had been sending the
 * board list all along.
 *
 * FAIL-first: before this round the assertions below found "Could not load
 * the retro." and no picker.
 */
test.describe('retro board picker', () => {
  const BOARDS = [
    { id: 1, name: 'Team board' },
    { id: 7, name: 'Platform board' },
  ]

  test('the ambiguous-board refusal names the boards and offers the choice', async ({ page }) => {
    const asked: string[] = []
    await page.route('**/api/v1/issues/retro/**', async (route) => {
      const url = route.request().url()
      asked.push(url)
      // Only the board-less sprint cut is ambiguous; naming one answers it.
      if (url.includes('by=sprint') && !url.includes('board=')) {
        await route.fulfill({
          status: 409,
          json: {
            error: 'ambiguous_board',
            message: 'retro: several boards have sprints — name one with --board:\n  1  Team board\n  7  Platform board',
            boards: BOARDS,
          },
        })
        return
      }
      // A named board is answered by the fixture's own sprint cut: this
      // mirror has one board, so `?by=sprint` alone is the report board 7
      // would have produced. The contract under test is the round trip —
      // refusal → choice → a table — not the numbers in it.
      const url2 = new URL(url)
      url2.searchParams.delete('board')
      await route.fulfill({ response: await route.fetch({ url: url2.toString() }) })
    })
    await page.route('**/api/v1/issues/boards/**', async (route) => {
      await route.fulfill({
        json: { boards: BOARDS.map((b) => ({ ...b, type: 'scrum', has_sprints: true })) },
      })
    })

    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()

    await page.getByTestId('retro-range').filter({ hasText: 'By sprint' }).click()

    // The server's own sentence, not the one-line failure copy.
    await expect(page.getByText('Several boards have sprints — pick one')).toBeVisible()
    // …and the copy points at the picker rather than reprinting the CLI's
    // own sentence, which names a --board flag no web reader has.
    await expect(page.getByText('Choose a board above.')).toBeVisible()
    await expect(page.getByText('name one with --board')).toBeHidden()
    await expect(page.getByText('Could not load the retro.')).toBeHidden()

    // …and the picker is on the header, built from the same rows.
    const picker = page.getByTestId('retro-board')
    await expect(picker).toBeVisible()
    await expect(picker.locator('option[value="7"]')).toHaveText('Platform board')
    await picker.selectOption('7')

    // Choosing one asks again with the board named, and the report returns —
    // the summary strip, since the table is folded (GDK-1724).
    await expect(page.getByTestId('retro-summary')).toBeVisible()
    await openTable(page)
    expect(asked.some((u) => u.includes('board=7'))).toBe(true)
  })
})

/*
 * GDK-1712: the row's own shape and its step. Both are stubbed onto a known
 * series rather than measured off the fixture — the point is the mapping
 * from numbers to marks, and a fixture that drifts would make this test say
 * something else next month.
 */
test.describe('retro trend marks', () => {
  test('rows carry a sparkline, and cells carry a coloured step only where direction is agreed', async ({
    page,
  }) => {
    await page.route('**/api/v1/issues/retro/**', async (route) => {
      const res = await route.fetch()
      const body = await res.json()
      const b = body.buckets
      // Closed climbs, cycle p85 climbs (worse), sessions climb (neutral).
      b.forEach((x: Record<string, unknown>, i: number) => {
        x.closed = 2 + i
        x['cycle p85'] = 1 + i
        x.sessions = 3 + i
        x.partial = i === b.length - 1
      })
      await route.fulfill({ response: res, json: body })
    })
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()
    await openTable(page)

    // The row's own line, one per metric that has two or more values. Named
    // rather than counted: how many of the eight the fixture fills is the
    // fixture's business, not this contract's.
    for (const m of ['closed', 'cycle p85', 'sessions']) {
      await expect(page.locator(`[data-testid="retro-sparkline"][data-metric="${m}"]`)).toHaveCount(1)
    }

    const closed = page.locator('[data-testid="retro-delta"][data-metric="closed"]')
    // Four steps across five buckets.
    await expect(closed).toHaveCount(4)
    await expect(closed.first()).toHaveText('↑+1')
    await expect(closed.first()).toHaveAttribute('data-tone', 'good')
    // Rising cycle time is the amber side of the same rule.
    await expect(
      page.locator('[data-testid="retro-delta"][data-metric="cycle p85"]').first(),
    ).toHaveAttribute('data-tone', 'bad')
    // Sessions move without being scored.
    await expect(
      page.locator('[data-testid="retro-delta"][data-metric="sessions"]').first(),
    ).toHaveAttribute('data-tone', 'none')
    // …and the running bucket's step is never scored, whatever the metric.
    await expect(closed.last()).toHaveAttribute('data-tone', 'none')

    // The summary strip reads the running bucket, uncoloured for the same reason.
    await expect(page.getByTestId('retro-summary-cell').first()).toContainText('6')
    await expect(page.getByTestId('retro-summary-delta').first()).toHaveAttribute('data-tone', 'none')
  })
})
