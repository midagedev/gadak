/*
 * GDK-1873 end to end: a wiki page takes a comment from the phone.
 *
 * src/lib/drafts.test.ts measures the storage module — that the new
 * 'page-comment' kind round-trips and shares the cap with the issue kinds.
 * It cannot measure the halves that reach a person: that PageDetail draws a
 * composer at all, that it restores into the composer it saved from, and
 * that a refused send keeps the words instead of eating them.
 *
 * One route handler, the same one drafts.spec.ts registers and for the same
 * reason: `gadak demo` is a credential-less serve, so the store's
 * writability probe (GET credential/) says off and the composer ships
 * disabled on this fixture — there would be nothing to type into. Answering
 * that one GET configured:true is the smallest change that puts the real
 * composer on screen. The POST itself is NOT faked: the serve still has no
 * origin credential, so `wikiWriter` answers 409 credential_required, which
 * is exactly the refusal the third test measures.
 */
import { expect, test } from './helpers'

const TYPED = 'one line on the doc, kept'

/** Arm writes, boot, and open the first page detail through the scope
 *  sheet's Documents → Updated row (the path viewport.spec.ts walks). */
async function openFirstPage(page: import('@playwright/test').Page): Promise<void> {
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.pane:not(.off) h1 button.scope').click()
  // The scrim carries aria-label="Cancel" too, so the labelled button inside
  // the panel is what every call site here clicks.
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await reopenFirstPage(page)
}

/** Open (or reopen) the first document row. The pane stays scoped to
 *  Updated after a back-tap, so this is the whole return trip. */
async function reopenFirstPage(page: import('@playwright/test').Page): Promise<void> {
  const row = page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first()
  await row.waitFor()
  await row.click()
  await page.locator('.page-detail button.back').waitFor()
}

test('a page detail carries a comment composer at the touch floor', async ({ page }) => {
  await openFirstPage(page)

  const input = page.locator('.page-detail .composer input')
  const send = page.locator('.page-detail button.send')
  await expect(input).toBeEnabled()
  await expect(send).toBeVisible()

  // DESIGN.md §4.1: 44pt on every control. The gate next door floors every
  // visible button; the input is not a button, so it is floored here.
  const box = await input.boundingBox()
  expect(box, 'composer input box').not.toBeNull()
  expect(box!.height, 'composer input height').toBeGreaterThanOrEqual(44)
  const sendBox = await send.boundingBox()
  expect(sendBox!.height, 'send button height').toBeGreaterThanOrEqual(44)

  // The thread says what the composer is for even when it is empty. The
  // count span is what picks this heading out of a body that may render
  // h3s of its own.
  await expect(page.locator('.page-detail h3:has(.h-n)')).toContainText('Comments')

  // Empty is not sendable; a word arms the control.
  await expect(send).toBeDisabled()
  await input.fill(TYPED)
  await expect(send).toBeEnabled()
})

test('a page comment typed on the phone survives leaving the page', async ({ page }) => {
  await openFirstPage(page)

  const input = page.locator('.page-detail .composer input')
  await expect(input).toBeEnabled()
  await input.fill(TYPED)

  // Poll the document rather than sleeping past the 250 ms debounce: the
  // draft is on disk when the storage says so, not when a timer guesses.
  await expect
    .poll(async () =>
      page.evaluate(() =>
        Object.keys(localStorage).some((k) => k.startsWith('gadak.drafts.v1')),
      ),
    )
    .toBe(true)
  expect(
    await page.evaluate(() => {
      const k = Object.keys(localStorage).find((x) => x.startsWith('gadak.drafts.v1'))
      const doc = JSON.parse(localStorage.getItem(k as string) as string) as {
        drafts: { kind: string }[]
      }
      return doc.drafts.map((d) => d.kind)
    }),
  ).toEqual(['page-comment'])

  // Leaving the page and coming back: the text is there, and it says so.
  // Reopening inside the layer's 200 ms outro would reverse the transition
  // and keep the same component instance — a back-tap never completed,
  // not "coming back".
  await page.locator('.page-detail button.back').first().click()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await reopenFirstPage(page)
  await expect(page.locator('.page-detail .composer input')).toHaveValue(TYPED)
  await expect(page.locator('.page-detail .composer .draft-note')).toBeVisible()

  // The first keystroke dismisses the caption; the text stays. A restored
  // value leaves the caret at 0, so say where to type rather than assume.
  await page.locator('.page-detail .composer input').press('End')
  await page.locator('.page-detail .composer input').pressSequentially('!')
  await expect(page.locator('.page-detail .composer .draft-note')).toHaveCount(0)
  await expect(page.locator('.page-detail .composer input')).toHaveValue(`${TYPED}!`)

  // Closing the app, as this webview experiences it. GDK-1970: the reload
  // keeps the #/page/<key> hash, and the cold link restores the page detail —
  // words already back in the composer. Leave through the visible control and
  // re-enter the way this test has always walked; the trip is one leg longer,
  // so the draft now survives one more leave-and-return than it did before.
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.locator('.page-detail button.back').waitFor()
  await expect(page.locator('.page-detail .composer input')).toHaveValue(`${TYPED}!`)
  await expect(page.locator('.page-detail .composer .draft-note')).toBeVisible()
  await page.locator('.page-detail button.back').first().click()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await reopenFirstPage(page)
  await expect(page.locator('.page-detail .composer input')).toHaveValue(`${TYPED}!`)
  await expect(page.locator('.page-detail .composer .draft-note')).toBeVisible()
})

test('a send this serve cannot make is refused at the control, with the words kept', async ({
  page,
}) => {
  await openFirstPage(page)

  // What the phone actually dials, recorded on the way through. The Go mux
  // registers this comment route under apiBase (`/api/v1/issues/`, not
  // `/api/v1/`), and a screen that built the web client's shorter path
  // would 404 here with no other symptom — the pattern below would simply
  // never match and `posted` would stay empty.
  const posted: { url: string; body: unknown }[] = []
  await page.route('**/api/v1/issues/pages/*/comment/', async (route) => {
    posted.push({ url: route.request().url(), body: route.request().postDataJSON() })
    await route.continue()
  })

  const input = page.locator('.page-detail .composer input')
  const send = page.locator('.page-detail button.send')
  await expect(input).toBeEnabled()
  await input.fill(TYPED)
  await send.click()

  // `gadak demo` has no origin credential, so handlePageComment's wikiWriter
  // answers 409 credential_required. The screen latches that: every control
  // recedes and the slab says why — the same sentence api.ts maps the code
  // to, above .composer so the refusal's own dimming never dims it.
  await expect(page.locator('.page-detail .slab-err')).toBeVisible()
  await expect(input).toBeDisabled()
  await expect(send).toBeDisabled()

  // Read after the refusal is on screen, never straight after the click:
  // `click()` resolves when the click dispatches, and the interception is a
  // separate round-trip that need not have happened yet. The sentence above
  // proves the response landed, which proves the request went through here.
  expect(posted, 'the comment POST the phone made').toHaveLength(1)
  expect(posted[0].body).toEqual({ text: TYPED })
  expect(posted[0].url).toMatch(/\/api\/v1\/issues\/pages\/[^/]+\/comment\/$/)

  // The half-typed line is the thing a refusal must never eat.
  await expect(input).toHaveValue(TYPED)
  expect(
    await page.evaluate(() => {
      const k = Object.keys(localStorage).find((x) => x.startsWith('gadak.drafts.v1'))
      if (!k) return null
      const doc = JSON.parse(localStorage.getItem(k) as string) as {
        drafts: { kind: string; text: string }[]
      }
      return doc.drafts.find((d) => d.kind === 'page-comment')?.text ?? null
    }),
  ).toBe(TYPED)

  // And no optimistic comment is left standing on a send that never landed:
  // the overlay is dropped on both roads out of send(), so the words exist
  // in exactly one place — the box.
  await expect(page.locator('.page-detail .comment', { hasText: TYPED })).toHaveCount(0)
})
