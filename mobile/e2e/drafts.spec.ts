/*
 * GDK-1863 end to end: a comment you were typing on the phone survives
 * closing the app.
 *
 * src/lib/drafts.test.ts measures the storage module — namespacing, the
 * cap, the demo's no-persistence contract. It cannot measure the half that
 * broke in the field: that Detail.svelte restores into the composer it
 * saved from, and that the draft outlives a full reload, which is what an
 * app switch does to this webview.
 *
 * One route handler: `gadak demo` is a credential-less serve, so the
 * store's writability probe (GET credential/) says off and the composer
 * ships disabled on this fixture — there would be nothing to type into.
 * Answering that one GET configured:true is the smallest change that puts
 * the real composer on screen; everything else is the serve's own data.
 */
import { expect, test } from './helpers'

const TYPED = 'half a thought, kept'

test('a comment typed on the phone survives leaving the issue and reloading the app', async ({
  page,
}) => {
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  const firstRow = page.locator('.pane:not(.off) button.row').first()
  await firstRow.waitFor()
  await firstRow.click()
  await page.locator('button.back').waitFor()
  const key = (await page.locator('.bar-key').first().innerText()).trim()
  expect(key).not.toBe('')

  const composer = page.locator('.composer input')
  await expect(composer).toBeEnabled()
  await composer.fill(TYPED)

  // Poll the document rather than sleeping past the 250 ms debounce: the
  // draft is on disk when the storage says so, not when a timer guesses.
  await expect
    .poll(async () =>
      page.evaluate(() =>
        Object.keys(localStorage).some((k) => k.startsWith('gadak.drafts.v1')),
      ),
    )
    .toBe(true)

  // Leaving the issue and coming back: the text is there, and it says so.
  await page.locator('button.back').first().click()
  // Wait for the detail layer to finish leaving: reopening inside its
  // 200 ms outro would reverse the transition and keep the same component
  // instance — a back-tap the user never completed, not "coming back".
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await firstRow.waitFor()
  await firstRow.click()
  await page.locator('button.back').waitFor()
  expect((await page.locator('.bar-key').first().innerText()).trim()).toBe(key)
  await expect(page.locator('.composer input')).toHaveValue(TYPED)
  await expect(page.locator('.composer .draft-note')).toBeVisible()

  // The first keystroke dismisses the caption; the text stays. A restored
  // value leaves the caret at 0, so say where to type rather than assume.
  await page.locator('.composer input').press('End')
  await page.locator('.composer input').pressSequentially('!')
  await expect(page.locator('.composer .draft-note')).toHaveCount(0)
  await expect(page.locator('.composer input')).toHaveValue(`${TYPED}!`)

  // Closing the app, as this webview experiences it. GDK-1970: the reload
  // keeps the #/KEY hash, and the cold link reopens the issue over the list —
  // the app switch returns the user to where they were, draft already in the
  // composer. Leave through the visible control and re-enter the way this
  // test has always walked; the trip is one leg longer, so the draft now
  // survives one more leave-and-return than it did before.
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.locator('.detail-layer button.back').waitFor()
  await expect(page.locator('.composer input')).toHaveValue(`${TYPED}!`)
  await expect(page.locator('.composer .draft-note')).toBeVisible()
  await page.locator('.detail-layer button.back').first().click()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await page.locator('h1 button.scope').waitFor()
  const rowAgain = page.locator('.pane:not(.off) button.row').first()
  await rowAgain.waitFor()
  await rowAgain.click()
  await page.locator('button.back').waitFor()
  await expect(page.locator('.composer input')).toHaveValue(`${TYPED}!`)
  await expect(page.locator('.composer .draft-note')).toBeVisible()
})
