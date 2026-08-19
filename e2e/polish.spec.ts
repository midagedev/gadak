import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('keyboard cheat sheet', () => {
  test('? opens it, Esc closes it, and it only documents keys that exist', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const sheet = page.getByTestId('shortcuts-dialog')
    await expect(sheet).toBeHidden()

    await page.keyboard.press('?')
    await expect(sheet).toBeVisible()
    await expect(sheet.getByRole('heading', { name: 'Keyboard shortcuts' })).toBeVisible()

    // Every documented key has a live handler (App / IssueList / SearchBox /
    // CommandPalette / CommentComposer). 1-4 style view switches do not exist.
    for (const label of [
      'Open the command palette',
      'New issue (when no detail or cursor)',
      'Move cursor down',
      'Focus the search or filter box',
      'Submit the comment',
    ]) {
      await expect(sheet.getByText(label, { exact: true })).toBeVisible()
    }
    await expect(sheet.getByText(/switch view/i)).toHaveCount(0)

    await page.keyboard.press('Escape')
    await expect(sheet).toBeHidden()

    // `?` while typing must stay a literal character, not a shortcut.
    const input = searchInput(page)
    await input.click()
    await input.pressSequentially('?')
    await expect(sheet).toBeHidden()
    await expect(input).toHaveValue('?')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('empty states', () => {
  test('a zero-result filter offers a reset that brings the list back', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.click()
    await input.pressSequentially('zzzz-no-such-issue', { delay: 10 })

    await expect(page.getByText('No issues match')).toBeVisible()
    const reset = page.getByRole('button', { name: 'Clear filters' })
    await expect(reset).toBeVisible()

    await reset.click()
    await expect(page.getByText('No issues match')).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller').locator('[role="button"]').first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
