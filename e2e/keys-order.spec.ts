import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, forceLocale } from './helpers'

/*
 * GDK-2: a keys view (ks=, no g) must paint FLAT in the given order.
 * status_category grouping shreds that order across buckets; the default
 * grouping for an unset g on a keys view is therefore none. An explicit
 * g= or Breakdown pick still wins.
 *
 * Fixture: NMA-11 and NMA-118 are status_category=new; NMA-1 is inprogress.
 * Group sort puts inprogress first, so NMA-11,NMA-1,NMA-118 becomes
 * NMA-1,NMA-11,NMA-118 when sectioned — a sort-miss cannot hide a grouping miss.
 */

const ORDERED = ['NMA-11', 'NMA-1', 'NMA-118'] as const
const GROUPED = ['NMA-1', 'NMA-11', 'NMA-118'] as const
const KS = ORDERED.join(',')

async function openKeys(page: Page, extra = ''): Promise<string[]> {
  await forceLocale(page, 'en')
  const errors = attachConsoleErrors(page)
  await page.goto(`/#/?ks=${KS}${extra}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('list-count')).toHaveText('3 issues')
  return errors
}

async function rowKeys(page: Page): Promise<string[]> {
  return page
    .locator('[data-testid="issue-list-scroller"] [data-issue-key]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-issue-key') ?? ''))
}

test.describe('keys-order e2e', () => {
  test('ks= without g is flat in the given order; Breakdown Progress restores headers', async ({
    page,
  }) => {
    const errors = await openKeys(page)

    await expect(page.getByTestId('group-header')).toHaveCount(0)
    expect(await rowKeys(page)).toEqual([...ORDERED])
    await expect(page.getByRole('button', { name: /Given order/ })).toBeVisible()

    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Progress', exact: true }).click()

    await expect(page.getByTestId('group-header').first()).toBeVisible()
    await expect(page).toHaveURL(/[?&]g=status_category(?:&|$)/)
    expect(await rowKeys(page)).toEqual([...GROUPED])

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('explicit g=status_category on a keys view still sections the list', async ({ page }) => {
    const errors = await openKeys(page, '&g=status_category')

    await expect(page.getByTestId('group-header').first()).toBeVisible()
    expect(await rowKeys(page)).toEqual([...GROUPED])

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
