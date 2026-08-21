import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

function ignoreFixtureNoise(errors: string[]): string[] {
  return errors.filter((e) => !e.includes('409') && !e.includes('502'))
}

async function waitVisit(page: Page): Promise<void> {
  await page.waitForResponse(
    (r) => r.url().includes('/history/visits/') && r.request().method() === 'POST' && r.ok(),
  )
}

async function openIssue(page: Page, key: string): Promise<void> {
  // An open panel overlaps the list at CI's viewport width, so the next row
  // click lands on the panel instead. Closing first is also the real flow:
  // you leave one issue before you open the next.
  await closeDetail(page)
  const input = searchInput(page)
  await input.fill(key)
  await expect(
    page.locator('[data-testid="issue-list-scroller"]').getByText(key).first(),
  ).toBeVisible()
  const posted = waitVisit(page)
  // Exact row via data-issue-key: hasText('NMA-1') also matches NMA-10/-100,
  // and which one comes first depends on the boot view's ordering (GDK-100
  // made that epic-grouped).
  await page
    .locator(`[data-testid="issue-list-scroller"] [data-issue-key="${key}"]`)
    .first()
    .click()
  await posted
  await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
}

async function closeDetail(page: Page): Promise<void> {
  const layout = page.getByTestId('issue-layout')
  if ((await layout.getAttribute('data-detail-open')) !== 'true') return
  await page.getByTestId('issue-detail-close').click()
  await expect(layout).toHaveAttribute('data-detail-open', 'false')
}

async function openHistory(page: Page): Promise<void> {
  await page.getByTestId('history-open').click()
  await expect(page.getByTestId('history-view')).toBeVisible()
}

test.describe('history view', () => {
  test('two opens of one issue record two visits and show 2 times', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const posted: string[] = []
    page.on('request', (req) => {
      if (req.method() === 'POST' && req.url().includes('/history/visits/')) {
        posted.push(req.postData() ?? '')
      }
    })
    await openIssue(page, 'NMA-1')
    await openIssue(page, 'NMB-111')
    await openIssue(page, 'NMA-1')
    expect(posted.filter((b) => b.includes('NMA-1'))).toHaveLength(2)

    await openHistory(page)
    const row = page.locator('[data-testid="history-row"][data-key="NMA-1"]')
    await expect(row).toBeVisible()
    const badge = row.getByTestId('history-visit-count')
    await expect(badge).toBeVisible()
    const n = Number((await badge.innerText()).replace(/\D/g, ''))
    expect(n).toBeGreaterThanOrEqual(2)
    await expect(page.locator('[data-testid="history-row"][data-key="NMB-111"]')).toBeVisible()

    expect(ignoreFixtureNoise(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a document visit sits on the same timeline', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('docs-spaces').click()
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'PROD' }).click()
    const view = page.getByTestId('space-docs-view')
    await view.getByTestId('space-tree-toggle').click()
    await view
      .getByTestId('doc-tree-node')
      .filter({ hasText: 'Feature Specs' })
      .getByTestId('doc-tree-toggle')
      .click()
    const posted = waitVisit(page)
    await view
      .getByTestId('doc-tree-node')
      .filter({ hasText: 'Billing Settings Spec' })
      .getByRole('button', { name: 'Billing Settings Spec', exact: true })
      .click()
    await posted
    await expect(page.getByTestId('doc-title')).toHaveText('Billing Settings Spec')

    await openHistory(page)
    const docRow = page.getByTestId('history-row').filter({ hasText: 'Billing Settings Spec' })
    await expect(docRow).toBeVisible()
    await expect(docRow).toHaveAttribute('data-kind', 'page')

    expect(ignoreFixtureNoise(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('⌘K unified search is recorded and opening a hit links it', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('workaround', { delay: 15 })
    const row = palette.getByTestId('palette-unified-issue').first()
    await expect(row).toBeVisible()
    const key = (await row.locator('.font-mono').first().textContent())?.trim() ?? ''
    expect(key).toMatch(/^[A-Z]+-\d+$/)
    const recorded = page.waitForResponse(
      (r) => r.url().includes('/history/searches/') && r.ok(),
    )
    await row.click()
    await recorded
    await expect(palette).toBeHidden()

    await openHistory(page)
    await page.getByTestId('history-tab').filter({ hasText: 'Searches' }).click()
    const searchRow = page.getByTestId('history-row').filter({ hasText: 'workaround' }).first()
    await expect(searchRow).toBeVisible()
    await expect(searchRow.getByTestId('history-search-opened')).toContainText(key)

    expect(ignoreFixtureNoise(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('leaving history and coming back keeps the kind chip; another view turns it off', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openIssue(page, 'NMB-110')

    await openHistory(page)
    await page.getByTestId('history-tab').filter({ hasText: 'Issues' }).click()
    await expect(page.getByTestId('history-tab').filter({ hasText: 'Issues' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )

    await page.getByTestId('docs-documents').click()
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page.getByTestId('history-view')).toHaveCount(0)

    await page.getByTestId('history-open').click()
    await expect(page.getByTestId('history-view')).toBeVisible()
    await expect(page.getByTestId('history-tab').filter({ hasText: 'Issues' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )

    await closeDetail(page)
    await page.getByTestId('history-close').click()
    await expect(page.getByTestId('history-view')).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()

    expect(ignoreFixtureNoise(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Show issues in list writes the current keys onto ks=', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openIssue(page, 'NMB-110')
    await openIssue(page, 'NMB-111')

    await openHistory(page)
    await page.getByTestId('history-open-as-list').click()
    await expect(page.getByTestId('history-view')).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(page.url()).toMatch(/ks=NMB-/)

    expect(ignoreFixtureNoise(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
