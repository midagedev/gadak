import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('detail', () => {
  test('row click opens detail panel with summary/history/comments sections', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Open an issue known to exist with changelog entries (NMB-110 has history).
    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    // Section titles from en.ts detail.*
    await expect(panel.getByRole('heading', { name: 'Details' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Description' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'History' })).toBeVisible()
    // Issue key visible in the sticky header
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    // Write gate: the configured credential alone must unlock the write UI
    // (me/ → email → identified). Regression guard for the boot-time identity probe.
    await expect(panel.locator('textarea[placeholder*="Add a comment"]')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the list beside an open panel gives its width up from the chips, not the title', async ({
    page,
  }) => {
    /*
     * With a panel open, the rows behind it used to hand over the whole column:
     * the title fell to "Su…" and then to a single character, while the label
     * chips beside it kept every pixel they had asked for — the widest thing on
     * the row surrendering to the narrowest, and the document rows in the same
     * product ordering it the other way round (vision verdict 2026-08-07).
     *
     * Both halves are read, because a floor on the title alone would also be
     * satisfied by a row that simply overflows the column it sits in.
     */
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)

    const scroller = page.getByTestId('issue-list-scroller')
    // The row carrying the widest strip of chips: it is the one whose title
    // reaches its floor first, so it is where the two sides actually compete.
    // A row with one short label never runs out of room and would pass this
    // whether or not the rule exists.
    const widest = await scroller.evaluate((el) => {
      let best = { key: '', width: 0 }
      for (const row of el.querySelectorAll<HTMLElement>('[data-issue-key]')) {
        const strip = [...row.querySelectorAll<HTMLElement>('span')].find((s) =>
          s.querySelector('[title^="Labels:"]'),
        )
        const width = strip?.getBoundingClientRect().width ?? 0
        if (width > best.width) best = { key: row.dataset.issueKey ?? '', width }
      }
      return best
    })
    expect(widest.width, 'the fixture must show label chips somewhere').toBeGreaterThan(0)
    const key = widest.key
    const chips = scroller.locator(`[data-issue-key="${key}"] button[title^="Labels:"]`).first()
    const strip = scroller.locator(`[data-issue-key="${key}"]`).locator('span').filter({
      has: page.locator('button[title^="Labels:"]'),
    })
    const chipped = scroller.locator(`[data-issue-key="${key}"]`)

    // Open some other row, so the row being measured is not also the selected
    // one — selection paints a row, and a painted row is a second variable.
    await scroller.locator(`[data-issue-key]:not([data-issue-key="${key}"])`).first().click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(chipped).toBeVisible()

    const rows = scroller.locator('[data-issue-key]')
    const n = Math.min(8, await rows.count())
    expect(n).toBeGreaterThan(0)
    for (let i = 0; i < n; i++) {
      const title = await rows.nth(i).locator('span.flex-1').first().boundingBox()
      expect(title?.width ?? 0, `row ${i} title width`).toBeGreaterThanOrEqual(96)
    }

    // And the space came from the chips: that same row's strip is narrower than
    // it was with the whole column to itself.
    const narrow = await strip.boundingBox()
    expect(narrow?.width ?? 0).toBeLessThan(widest.width)

    /*
     * It came from dropping chips, not from grinding them down. The strip used
     * to hand its width over by truncation, which at this width left "cust…"
     * "d…" "r…" — labels narrower than the words in them, which read as a
     * rendering fault. Every chip still on screen has to be wide enough to be
     * a word; 40px sits under the 48px floor they carry, so this fails on
     * fraying rather than on a rounding difference.
     */
    const visibleChips = () =>
      scroller.evaluate((el) =>
        // Buttons only: the +N counters carry the same title (they name the
        // labels they stand for), and a count is not a chip.
        [...el.querySelectorAll<HTMLElement>('button[title^="Labels:"]')]
          .filter((chip) => chip.offsetParent !== null)
          .map((chip) => ({
            text: chip.textContent?.trim() ?? '',
            width: chip.getBoundingClientRect().width,
          })),
      )

    for (const chip of await visibleChips()) {
      expect(chip.width, `label chip "${chip.text}" width`).toBeGreaterThanOrEqual(40)
    }
    // One chip always stays — a detail-open list that folds to "+3" reads as
    // having no labels. Extra labels may still collapse to +N beside it.
    expect((await chips.innerText()).trim().length).toBeGreaterThan(1)
    expect((await chips.innerText()).trim()).not.toMatch(/^\+\d+$/)

    // The fold reverses. Widen until the list is back above the last step and
    // the chips return — otherwise "no frayed chips" would also be satisfied by
    // a row that had quietly stopped drawing labels at every width.
    await page.setViewportSize({ width: 1920, height: 900 })
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect
      .poll(async () => (await visibleChips()).length, {
        message: 'chips must come back once the list is wide again',
      })
      .toBeGreaterThan(0)
    for (const chip of await visibleChips()) {
      expect(chip.width, `wide-list label chip "${chip.text}" width`).toBeGreaterThanOrEqual(40)
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail header offers a label editor when a credential is stored', async ({ page }) => {
    /*
     * The fixture credential is fake, so a real PUT would fail at Jira. This
     * stops where triage.spec.ts stops: the add surface has to appear. The
     * write itself is TestLabelsSetAndClear.
     */
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const editor = panel.getByTestId('label-editor')
    await expect(editor).toBeVisible()

    await editor.getByTestId('label-editor-add').click()
    const field = editor.getByTestId('label-editor-input')
    await expect(field).toBeVisible()
    await field.fill('tech-debt')
    await expect(field).toHaveValue('tech-debt')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail header offers a priority picker when a credential is stored', async ({ page }) => {
    /*
     * GET priorities/ would hit Jira; the fixture token is fake. The catalog
     * is mocked so the menu can render. The write itself is TestPrioritySetAndClear.
     */
    const errors = attachConsoleErrors(page)
    await page.route('**/priorities/', (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      return route.fulfill({
        json: {
          priorities: [
            { id: '1', name: 'Highest' },
            { id: '2', name: 'High' },
            { id: '3', name: 'Medium' },
            { id: '4', name: 'Low' },
            { id: '5', name: 'Lowest' },
          ],
        },
      })
    })
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const chip = panel.getByTestId('priority-picker')
    await expect(chip).toBeVisible()
    await chip.click()
    const menu = page.getByRole('listbox', { name: 'Priority' })
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('option', { name: 'None' })).toBeVisible()
    await expect(menu.getByRole('option', { name: 'High', exact: true })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail title becomes a field when clicked', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const title = panel.getByTestId('title-editor')
    await expect(title).toBeVisible()
    await title.click()
    const field = panel.getByTestId('title-editor-input')
    await expect(field).toBeVisible()
    await expect(field).toBeFocused()
    await field.press('Escape')
    await expect(panel.getByTestId('title-editor')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('copy-link writes the gadak:// and http issue forms to the clipboard', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const copy = panel.getByTestId('issue-copy-link')
    await expect(copy).toBeVisible()
    await copy.click()

    const origin = new URL(page.url()).origin
    const want = `gadak://view?issue=NMB-110\n${origin}/#/?issue=NMB-110`
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(want)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
