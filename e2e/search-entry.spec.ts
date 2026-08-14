import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * Ways in to the one search.
 *
 * The mirror has held issues and pages together for a while, and the server has
 * searched both since the unified index landed — but the client only ever
 * offered that on the issue list. The palette matched issues alone, `/` was
 * bound inside the list's search box (so it died on a document screen), and the
 * document screens had no narrowing field at all. These pin the three entry
 * points, and that none of them costs a request on a keystroke.
 */

/** Open the tabbed Documents view from the sidebar. */
async function openDocuments(page: Page): Promise<void> {
  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible()
}

/** Let the boot chatter (bootstrap / write-meta / pages) finish before a spec
 *  starts counting requests. Deliberately not networkidle: the app polls for a
 *  delta every 15s, so "quiet for 500ms" only becomes true after that fires. */
async function settled(page: Page): Promise<void> {
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
  await page.waitForTimeout(300)
}

test.describe('search entry points', () => {
  test('the palette matches documents, above the issues, and says how many it hid', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await settled(page)

    const apiDuringType: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (url.includes('/api/') && !url.includes('/ui-focus/')) apiDuringType.push(url)
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // Seven pages in the mirror are runbooks; four of them fit the section.
    await page.keyboard.type('runbook', { delay: 20 })

    const docRows = palette.getByTestId('palette-doc-row')
    await expect(docRows).toHaveCount(4)
    await expect(docRows.first()).toContainText('Runbook')
    // The space rides along, because two pages can share a title and the space
    // is what tells them apart. (This mirror has no space names yet, so the row
    // shows the key — the same fallback the document rows use.)
    await expect(docRows.first()).toContainText('ENG')

    // Capped, and it says so — four rows with no total read as the whole answer.
    // Spelled out, not "4 / 7": the slash fraction belongs to the document
    // screens' filter (shown / total), and the palette's truncation must not
    // wear the same glyph with a different meaning.
    await expect(palette.getByTestId('palette-doc-count')).toHaveText('4 of 7')

    // A row shows the part of the title the query found, so it never asks to be
    // taken on trust.
    await expect(docRows.first().locator('mark', { hasText: /runbook/i })).toBeVisible()

    // Documents lead: when someone types words rather than a key, the page is
    // often what they came for, and the issues section would bury it.
    const sections = await palette
      .getByTestId('palette-section')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-section')))
    expect(sections).toContain('doc')
    expect(sections).toContain('issue')
    expect(sections.indexOf('doc')).toBeLessThan(sections.indexOf('issue'))

    expect(
      apiDuringType,
      `expected no /api/ requests while typing, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    // Choosing one opens the page, the same as it always did from the recent list.
    await docRows.first().click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('doc-panel')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('/ reaches the narrowing field on a document screen, not only on the list', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // The list is where it always worked; assert it still does after the binding
    // moved out of the search box and into the shell.
    await page.keyboard.press('/')
    await expect(page.getByTestId('search-input')).toBeFocused()
    await page.keyboard.press('Escape')

    await openDocuments(page)
    await page.keyboard.press('/')
    await expect(page.getByTestId('docs-filter-input')).toBeFocused()

    // Esc clears what is typed before it gives the keyboard back — SearchBox's
    // contract, and the reason the field is safe to leave text in.
    await page.keyboard.type('runbook')
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('docs-filter-input')).toHaveValue('')
    await expect(page.getByTestId('docs-filter-input')).toBeFocused()

    // A space screen has the same field, reached the same way.
    await page.getByTestId('docs-spaces').click()
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }).click()
    await expect(page.getByTestId('space-docs-view')).toBeVisible()
    await page.keyboard.press('/')
    await expect(page.getByTestId('docs-filter-input')).toBeFocused()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('typing in the document filter narrows locally, with zero /api/ traffic', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)
    await page.getByTestId('docs-view').getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    await settled(page)

    const view = page.getByTestId('docs-view')
    await expect(view.getByTestId('docs-count')).toHaveText('71')
    const before = await view.getByTestId('doc-row').count()
    expect(before).toBeGreaterThan(0)

    const apiDuringType: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (url.includes('/api/') && !url.includes('/ui-focus/')) apiDuringType.push(url)
    })

    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    await filter.pressSequentially('runbook', { delay: 20 })

    // The count becomes a fraction: what is left, out of what the tab holds.
    await expect(view.getByTestId('docs-count')).toHaveText('7 / 71')
    await expect(view.getByTestId('doc-row')).toHaveCount(7)
    expect(await view.getByTestId('doc-row').count()).toBeLessThan(before)

    expect(
      apiDuringType,
      `expected no /api/ requests while filtering, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    // Nothing matched is a state with a way out of it, not a dead end.
    await filter.fill('zzzznotathing')
    await expect(view.getByTestId('doc-row')).toHaveCount(0)
    await expect(view).toContainText('No documents match')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Enter in the document filter leaves for the whole mirror', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    await filter.fill('runbook')
    await filter.press('Enter')

    // Unified results have one home — the issue list's search section, where the
    // page hits sit above the issues. The document screen hands over to it.
    await expect(page.getByTestId('docs-view')).toHaveCount(0)
    await expect(page.getByTestId('search-docs')).toBeVisible()
    // The server ranks these (a page whose body says "runbooks" can outrank a
    // title), so the claim is that the runbooks are in the group, not first.
    await expect(
      page.getByTestId('search-doc-row').filter({ hasText: 'Runbook —' }).first(),
    ).toBeVisible()
    // The query it left with is the query the list is now showing.
    await expect(page.getByTestId('search-input')).toHaveValue('runbook')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
