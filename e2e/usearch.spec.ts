import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/*
 * Unified search in the command palette (⌘K) plus the visible way in.
 *
 * Fixture tokens (examples/demo.db — do not invent, do not edit the seed):
 *   workaround   — 31 comments / 2 issue bodies; 0 titles, labels, page titles
 *   consequences — 5 page bodies; 0 issue or page titles
 * Local palette matching is key/title/assignee/labels (issues) and
 * title/space (docs), so both tokens are invisible there today.
 *
 * Cancel / project unit cases live in web/src/lib/unified-search.test.ts.
 */

test.describe('unified search — palette + entry', () => {
  test('⌘K finds a comment-only issue under All search, with a snippet', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('workaround', { delay: 15 })

    // Local jump sections cannot see a comment-only token. Rank order is the
    // server's; the claim is a unified row with a comment snippet, not a key.
    const row = palette.getByTestId('palette-unified-issue').first()
    await expect(row).toBeVisible()
    const snippet = row.getByTestId('palette-unified-snippet')
    await expect(snippet).toBeVisible()
    await expect(snippet).toHaveAttribute('data-match-field', 'comment')
    await expect(snippet).toContainText(/workaround/i)

    const sections = await palette
      .getByTestId('palette-section')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-section')))
    expect(sections).toContain('unified')
    // Server section sits below the local jump groups.
    const last = sections.lastIndexOf('unified')
    const issue = sections.indexOf('issue')
    if (issue >= 0) expect(issue).toBeLessThan(last)

    const key = (await row.locator('.font-mono').first().textContent())?.trim() ?? ''
    expect(key).toMatch(/^[A-Z]+-\d+$/)
    await row.click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(page.getByTestId('issue-detail-panel').getByText(key).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('⌘K finds a page that matches only in its body', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('consequences', { delay: 15 })

    // Local doc rows are title/space only — this token is body-only.
    await expect(palette.getByTestId('palette-doc-row')).toHaveCount(0)
    const doc = palette.getByTestId('palette-unified-doc').first()
    await expect(doc).toBeVisible()
    await expect(doc).toContainText('ADR')
    await expect(doc.getByTestId('palette-unified-snippet')).toBeVisible()
    await expect(doc.getByTestId('palette-unified-snippet')).toContainText(/consequences/i)

    await doc.click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('doc-panel')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the top-bar entry opens the palette; SearchBox says it narrows this list', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const box = searchInput(page)
    await expect(box).toHaveAttribute('placeholder', /narrow this list/i)

    await page.getByTestId('palette-open').click()
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await expect(palette.getByTestId('palette-empty-hint')).toBeVisible()
    await expect(palette.getByTestId('palette-empty-hint')).toContainText(/every issue and document/i)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a failed server search is an error, not an empty result', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.route('**/api/v1/issues/search/**', (route) =>
      route.fulfill({ status: 500, body: 'nope' }),
    )

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('workaround', { delay: 15 })

    await expect(palette.getByTestId('palette-unified-error')).toBeVisible()
    await expect(palette.getByTestId('palette-unified-empty')).toHaveCount(0)
    await expect(palette.getByTestId('palette-unified-issue')).toHaveCount(0)

    // The 500 is the case under test; Chromium logs it as a console error.
    expect(
      errors.filter((e) => !e.includes('500')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
