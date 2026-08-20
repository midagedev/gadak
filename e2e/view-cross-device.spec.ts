import { expect, test } from '@playwright/test'

import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * GDK-437 slice B — the default save must follow the user across devices.
 * Context A saves a view through the default path (type a name, press
 * Enter). Context B is a brand-new browser context: empty localStorage, so
 * the only way the view can appear there is the server. This spec is the
 * issue's completion condition itself — pre-fix, Enter saved to
 * localStorage (personal) and context B stayed empty.
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

  // The path a user actually takes: type a name and press Enter. Whatever
  // scope Enter submits is the product's default save.
  await pageA.getByRole('button', { name: 'Save as view' }).click()
  await pageA.getByPlaceholder('View name').fill(VIEW)
  await pageA.getByPlaceholder('View name').press('Enter')
  await expect(pageA.getByPlaceholder('View name')).toBeHidden()
  // Context A proves the save landed before B proves it traveled: team rows
  // render "name · owner" — the owner suffix marks a server-stored view.
  await expect(pageA.getByRole('button', { name: VIEW })).toBeVisible()

  await a.close()

  // Fresh context: empty localStorage. The sidebar team rows render
  // "name · owner" — the owner suffix is what marks a server-stored view
  // (personal rows carry none).
  const b = await browser.newContext({ locale: 'en-US' })
  const pageB = await b.newPage()
  errorsByContext.push(attachConsoleErrors(pageB))
  await gotoApp(pageB)
  await expect(pageB.getByRole('button', { name: VIEW })).toBeVisible()

  // Delete what this run saved: serve.sh reseeds gadak.db but not local.db,
  // so a leftover server view survives into the next run — where
  // palette.spec's "exactly one team row" assertion would count it.
  const views = (await pageB.evaluate(async () => {
    const res = await fetch('/api/v1/issues/views/')
    return (await res.json()) as { views?: { id: string; name: string }[] }
  })).views
  const mine = (views ?? []).filter((v) => v.name === VIEW)
  expect(mine, 'the saved view must be deletable by this run').toHaveLength(1)
  await pageB.evaluate(async (id) => {
    await fetch(`/api/v1/issues/views/${encodeURIComponent(id)}/`, { method: 'DELETE' })
  }, mine[0].id)
  await b.close()

  for (const errors of errorsByContext) {
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  }
})
