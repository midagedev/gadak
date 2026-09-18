/*
 * Narrowing the list you are looking at (GDK-1994).
 *
 * Before this round the phone had no control that narrowed the list on
 * screen. Search is not narrowing — it runs over the whole snapshot — so
 * "just the reopened ones" meant going back to the desk and saving a view
 * there. The chevron on the heading now opens a body of its own, which is
 * also what parts the two doors that used to open one palette.
 *
 * FAIL-first (this file against the pre-GDK-1994 tree, 2026-09-18): every
 * test here fails at the first step, because `h1 button.scope` opened the
 * palette and no dialog named "This list" existed.
 */
import { expect, test } from './helpers'
import { openListSheet, waitPaired } from './nav'

test('the chevron opens this list, and the picker is one row inside it', async ({ page }) => {
  // The structural answer to two doors into one room: the heading owns the
  // list on screen, the magnifier owns finding one issue. The picker did not
  // move out of reach — it is the first row, wearing the scope's own name.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const heading = page.locator('h1 button.scope .name')
  const name = (await heading.innerText()).trim()

  const sheet = await openListSheet(page)
  // The palette is NOT what this door opens.
  await expect(page.locator('.palette-field input')).toHaveCount(0)

  const scopeRow = sheet.locator('button.scope-row')
  await expect(scopeRow).toContainText(name)
  await scopeRow.click()
  await expect(sheet).toHaveCount(0)
  await expect(page.locator('.palette-field input')).toBeVisible()
  // The picker's own road out, unchanged.
  await page.locator('button.palette-cancel').click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
})

test('a toggle narrows the list and the heading count follows it', async ({ page }) => {
  // The whole point of the control, measured end to end: the number in the
  // heading is the narrowed number, the rows on screen are the narrowed
  // rows, and the dot says the smaller number is deliberate.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const count = page.locator('h1 button.scope .count')
  const before = Number((await count.innerText()).replace('·', ''))
  expect(before, 'the fixture list has rows').toBeGreaterThan(1)
  await expect(page.locator('h1 button.scope .narrowed')).toHaveCount(0)

  const sheet = await openListSheet(page)
  const toggle = sheet.locator('button.t-row[aria-pressed]').first()
  const want = Number((await toggle.locator('.count').innerText()).replace('·', ''))
  expect(want, 'the toggle is offered because it changes the list').toBeLessThan(before)

  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-pressed', 'true')
  // Live behind the scrim — no apply button, iOS Mail's shape.
  await expect(count).toHaveText(`·${want}`)
  await expect(page.locator('h1 button.scope .narrowed')).toHaveCount(1)

  // Clear is in the sheet that set it, and it puts the list back.
  await sheet.locator('button.clear').click()
  await expect(count).toHaveText(`·${before}`)
  await expect(page.locator('h1 button.scope .narrowed')).toHaveCount(0)
})

test('picking another view drops the narrow', async ({ page }) => {
  // A narrow carried into another view would make the heading disagree with
  // the count the picker row just showed for that same view.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const sheet = await openListSheet(page)
  await sheet.locator('button.t-row[aria-pressed]').first().click()
  await expect(page.locator('h1 button.scope .narrowed')).toHaveCount(1)

  await sheet.locator('button.scope-row').click()
  const rows = page.locator('button.palette-row:not([disabled])')
  const current = (await page.locator('h1 button.scope .name').innerText()).trim()
  const other = rows.filter({ hasNotText: current }).first()
  await other.click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
  await expect(page.locator('h1 button.scope .narrowed')).toHaveCount(0)
})

test('system back closes the sheet before anything else', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const sheet = await openListSheet(page)
  await page.goBack()
  await expect(sheet).toHaveCount(0)
  // The list is still there — back closed the sheet, not the screen.
  await expect(page.locator('h1 button.scope')).toHaveCount(1)
})
