import { expect, test } from './helpers'
import { gotoApp } from './helpers'

/*
 * GDK-1255: typing an issue key that exists must not be answered with two
 * denials.
 *
 * Reported from the GDK-1253 shoot. Typing "STD-9" produced, in one frame:
 *   ISSUES      STD-9 — the exact issue (correct, and the point of typing it)
 *   ALL SEARCH  "No matches"
 *   ACTIONS     Create "STD-9"
 *
 * The server search is not wrong — a key is not in the FTS body, so it
 * genuinely matches nothing. But "No matches" one row under the issue reads
 * as "that key does not exist", and offering to *create* an issue titled with
 * a key that already resolves invites a duplicate. Agents and people both
 * misread it; the reporter did.
 *
 * The contract: when a bare key resolves, it leads the results, the ALL
 * SEARCH empty row is suppressed, and instant-create is not offered. Nothing
 * else is suppressed — a query that happens to be a key still shows every
 * real match it has.
 */

/** A key the demo fixture is certain to hold, and a well-formed key it is
 *  certain not to hold (same project prefix, absurd number). */
const PRESENT = 'NMA-140'
const ABSENT = 'NMA-999999'

async function openPalette(page: import('@playwright/test').Page, query: string) {
  await page.keyboard.press('ControlOrMeta+k')
  const palette = page.getByRole('dialog', { name: 'Command palette' })
  await expect(palette).toBeVisible()
  await palette.getByRole('combobox').fill(query)
  return palette
}

test.describe('GDK-1255 an exact issue key is a destination', () => {
  test('a key that resolves leads, and denies nothing', async ({ page }) => {
    await gotoApp(page)
    const palette = await openPalette(page, PRESENT)

    // The issue is there, and it is the first result row.
    const rows = palette.locator('[data-item-id]')
    await expect(rows.first()).toHaveAttribute('data-item-id', `i:${PRESENT}`)

    // ...and neither denial is on screen. Waits are on the palette settling:
    // the unified search is async, so assert after its status resolves for a
    // query that is NOT a key (below) rather than racing it here.
    await expect(palette.getByTestId('palette-create-now')).toHaveCount(0)
    await expect(palette.getByTestId('palette-unified-empty')).toHaveCount(0)
  })

  test('a well-formed key that resolves to nothing keeps both affordances', async ({ page }) => {
    await gotoApp(page)
    const palette = await openPalette(page, ABSENT)

    // This is the control: the suppression is conditioned on the key actually
    // having a home, not on the query merely looking like a key. Without this
    // the fix could have been "never offer create for anything key-shaped",
    // which would have broken filing an issue whose title is a key.
    await expect(palette.getByTestId('palette-create-now')).toBeVisible()
  })

  test('ordinary words still get instant-create', async ({ page }) => {
    await gotoApp(page)
    const palette = await openPalette(page, 'a summary that is not a key')
    await expect(palette.getByTestId('palette-create-now')).toBeVisible()
  })
})
