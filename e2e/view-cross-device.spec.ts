import { expect, test, type Page } from '@playwright/test'

import { attachConsoleErrors, gotoApp } from './helpers'

async function deleteServerViewByName(page: Page, name: string): Promise<void> {
  const views = (
    await page.evaluate(async () => {
      const res = await fetch('/api/v1/issues/views/')
      return (await res.json()) as { views?: { id: string; name: string }[] }
    })
  ).views
  const mine = (views ?? []).filter((v) => v.name === name)
  expect(mine, 'the saved view must be deletable by this run').toHaveLength(1)
  await page.evaluate(async (id) => {
    await fetch(`/api/v1/issues/views/${encodeURIComponent(id)}/`, { method: 'DELETE' })
  }, mine[0].id)
}

function viewRow(page: Page, name: string) {
  return page.locator('[data-testid="sidebar-view-row"]').filter({ hasText: name })
}

/*
 * GDK-437 — the save is one kind. Context A types a name and presses Enter.
 * Context B is a brand-new browser context: empty localStorage, so the only
 * way the view can appear there is the server. data-view-storage=server is
 * what marks that landing; the old owner-email suffix is gone (it was the
 * user's own email).
 */
test('Enter-saved view appears in a fresh browser on the same server', async ({ browser }) => {
  // serve.sh reseeds gadak.db but not local.db, so server-saved views from
  // earlier runs linger. A per-run name keeps this spec's assertion about
  // *this* run's save, not the fixture's residue.
  const VIEW = `Cross-device triage ${Date.now()}`
  const errorsByContext: string[][] = []

  const a = await browser.newContext({ locale: 'en-US' })
  const pageA = await a.newPage()
  errorsByContext.push(attachConsoleErrors(pageA))
  await gotoApp(pageA)

  // A filter makes "Save as view" appear (dates.spec's testid path).
  await pageA.getByTestId('filter-add').click()
  await pageA.getByTestId('filter-date-axis-created').click()
  await pageA.getByTestId('filter-date-from').fill('2026-01-01')
  await expect(pageA).toHaveURL(/cf=2026-01-01/, { timeout: 10_000 })

  await pageA.getByRole('button', { name: 'Save as view' }).click()
  const popover = pageA.getByTestId('filter-save-popover')
  await expect(popover).toBeVisible()
  await expect(popover.getByRole('button')).toHaveCount(1)
  await expect(popover.getByTestId('filter-save-view')).toBeVisible()

  await pageA.getByPlaceholder('View name').fill(VIEW)
  await pageA.getByPlaceholder('View name').press('Enter')
  await expect(pageA.getByPlaceholder('View name')).toBeHidden()

  const rowA = viewRow(pageA, VIEW)
  await expect(rowA).toBeVisible()
  await expect(rowA).toHaveAttribute('data-view-storage', 'server')
  await expect(rowA).not.toContainText('@')
  await expect(pageA.getByText('Shared team views')).toHaveCount(0)

  await a.close()

  const b = await browser.newContext({ locale: 'en-US' })
  const pageB = await b.newPage()
  errorsByContext.push(attachConsoleErrors(pageB))
  await gotoApp(pageB)
  const rowB = viewRow(pageB, VIEW)
  await expect(rowB).toBeVisible()
  await expect(rowB).toHaveAttribute('data-view-storage', 'server')
  await expect(rowB).not.toContainText('@')

  await deleteServerViewByName(pageB, VIEW)
  await b.close()

  for (const errors of errorsByContext) {
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  }
})

/*
 * The absorb flag used to be one-shot: once written (even as `[]`), leftover
 * localStorage rows stayed in this browser. A later boot must still hand
 * unabsorbed rows to the server.
 */
test('leftover local view is absorbed after the flag is already written', async ({ browser }) => {
  const VIEW = `Leftover absorb ${Date.now()}`
  const errorsByContext: string[][] = []
  const scopedViews = 'gadak:site:nimbus.example.com:personal-views'

  const a = await browser.newContext({ locale: 'en-US' })
  const pageA = await a.newPage()
  errorsByContext.push(attachConsoleErrors(pageA))
  await pageA.addInitScript(
    ({ key, name }) => {
      localStorage.setItem(
        key,
        JSON.stringify([
          {
            id: 'p-leftover-437',
            name,
            created_at: new Date().toISOString(),
            config: {
              filters: {
                jira_project: ['NMB'],
                status_category: ['inprogress'],
              },
              display: { group_by: 'status_category', sort: 'updated', dir: 'desc' },
            },
          },
        ]),
      )
      localStorage.setItem(`${key}-absorbed`, '[]')
    },
    { key: scopedViews, name: VIEW },
  )
  await gotoApp(pageA)

  const rowA = viewRow(pageA, VIEW)
  await expect(rowA).toBeVisible()
  await expect(rowA).toHaveAttribute('data-view-storage', 'server')
  await a.close()

  const b = await browser.newContext({ locale: 'en-US' })
  const pageB = await b.newPage()
  errorsByContext.push(attachConsoleErrors(pageB))
  await gotoApp(pageB)
  const rowB = viewRow(pageB, VIEW)
  await expect(rowB).toBeVisible()
  await expect(rowB).toHaveAttribute('data-view-storage', 'server')

  await deleteServerViewByName(pageB, VIEW)
  await b.close()

  for (const errors of errorsByContext) {
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  }
})
