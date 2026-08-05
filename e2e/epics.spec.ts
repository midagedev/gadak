import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * Epic hierarchy across the three surfaces that show it: the grouped list, the
 * row chip, and the detail panel (breadcrumb + rollup).
 *
 * Fixture facts (examples/demo.db): 15 epics own 163 issues. NMB-195 "Billing
 * reliability" holds 12, of which 4 are done; NMB-106 is one of them, and its
 * parent is the epic itself — the shape the breadcrumb collapses to one segment.
 */

/** Switch the list's sectioning axis through the Breakdown menu, as a user would. */
async function groupBy(page: Page, label: string): Promise<void> {
  await page.getByRole('button', { name: /Breakdown/ }).click()
  await page.getByRole('button', { name: label, exact: true }).click()
}

/** Narrow the list to one issue and open it. */
async function openIssue(page: Page, key: string): Promise<void> {
  const input = searchInput(page)
  await input.fill(key)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: key })
    .first()
    .click()
  await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
}

test.describe('epic hierarchy', () => {
  test('grouping by epic names each section with the epic key and summary', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await groupBy(page, 'Epic')

    // Sections sort by key, so the first one is NMA-174 — and it carries the
    // epic's summary, not just the key the rows store.
    const headers = page.getByTestId('group-header')
    await expect(headers.first()).toContainText('NMA-174')
    await expect(headers.first()).toContainText('REST API correctness')

    // Issues outside any epic keep their own bucket rather than a fake one.
    await page.getByTestId('issue-list-scroller').evaluate((el) => {
      el.scrollTop = el.scrollHeight
    })
    await expect(headers.filter({ hasText: 'No epic' }).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('row epic chip opens the epic it belongs to', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-106')

    const row = page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-106' })
      .first()
    const chip = row.getByTestId('epic-chip')
    await expect(chip).toHaveText('NMB-195')
    // The tooltip carries what the chip has no room for.
    await expect(chip).toHaveAttribute('title', 'Epic: Billing reliability')

    await chip.click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel.getByRole('heading', { name: 'Billing reliability' })).toBeVisible()

    // Sectioning by epic already says this on every header, so the chip stands down.
    await page.keyboard.press('Escape')
    await groupBy(page, 'Epic')
    await expect(row.getByTestId('epic-chip')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an epic detail rolls up its children and walks into one', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await openIssue(page, 'NMB-195')

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel.getByTestId('epic-progress')).toContainText('4 of 12 done')
    await expect(panel.getByTestId('epic-progress')).toContainText('33%')

    const childRows = panel.getByTestId('epic-child-row')
    await expect(childRows).toHaveCount(12)
    // 12 fits under the collapse threshold, so nothing is hidden behind a toggle.
    await expect(panel.getByTestId('epic-children-toggle')).toHaveCount(0)

    await childRows.filter({ hasText: 'NMB-126' }).click()
    await expect(panel.getByRole('heading', { name: /tax twice/ })).toBeVisible()
    // The child is not an epic, so the rollup is gone with it.
    await expect(panel.getByTestId('epic-progress')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('issue breadcrumb shows the epic and navigates up to it', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await openIssue(page, 'NMB-106')

    const panel = page.getByTestId('issue-detail-panel')
    const crumb = panel.getByTestId('issue-breadcrumb')
    await expect(crumb).toBeVisible()
    await expect(crumb).toContainText('NMB-195')
    await expect(crumb).toContainText('Billing reliability')
    await expect(crumb).toContainText('NMB-106')
    // Parent is the epic here, so it is not repeated as a second segment.
    await expect(crumb.getByTestId('issue-breadcrumb-ancestor')).toHaveCount(1)

    await crumb.getByTestId('issue-breadcrumb-ancestor').click()
    await expect(panel.getByRole('heading', { name: 'Billing reliability' })).toBeVisible()
    // The epic sits at the top of its own tree — no crumb of its own.
    await expect(panel.getByTestId('issue-breadcrumb')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
