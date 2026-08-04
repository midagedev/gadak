import { test, expect } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

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
      'New issue',
      'Move cursor down',
      'Focus the search box',
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

test.describe('first run', () => {
  test('an unconfigured, empty mirror shows the onboarding checklist', async ({ page }) => {
    await forceLocale(page, 'en')

    // Simulate a fresh install without touching the shared fixture server:
    // no projects in config, no identity, empty bootstrap, and no IndexedDB cache.
    await page.addInitScript(() => {
      indexedDB.deleteDatabase('issue-navigator')
    })
    await page.route('**/config.json', (route) =>
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ apiBase: '/api/v1/issues/', authBase: '/api/v1/auth/', projects: [] }),
      }),
    )
    await page.route('**/api/v1/issues/bootstrap/**', (route) =>
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          issues: [],
          members: [],
          server_time: new Date().toISOString(),
          sync_version: 1,
          sync_health: null,
        }),
      }),
    )
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ contentType: 'application/json', body: JSON.stringify({ email: null }) }),
    )

    await page.goto('/')

    const onboarding = page.getByTestId('onboarding')
    await expect(onboarding).toBeVisible({ timeout: 30_000 })
    await expect(onboarding.getByText('Jira credential')).toBeVisible()
    await expect(onboarding.getByText('Projects', { exact: true })).toBeVisible()
    await expect(onboarding.getByText('First sync')).toBeVisible()
    await expect(onboarding.getByRole('button', { name: 'Open settings' })).toBeVisible()
  })
})
