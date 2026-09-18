/*
 * The palette is the heading, and it is dormant on boot (GDK-902,
 * 2026-09-15). DESIGN.md §2: "No tab bar, no drawer, no nested stacks" —
 * the list body becomes the palette in place, and the first paint is the
 * owner's rows with nothing focused.
 *
 * FAIL-first (this file against the pre-GDK-902 tree, 2026-09-15):
 *   dormant on boot …                  → `h1 button.scope` opened the
 *                                        ScopeSheet, no `.palette-field`
 *   no tab bar …                       → nav.safe-bottom count 1, not 0
 *   Terminal row iff a terminal pairing → the row lived in the tab bar
 *   Cancel restores the scroll         → no Cancel control existed
 *   back from Detail returns …         → the palette did not exist
 * The recorded failures are in scratch/gdk-902r1/failfirst-palette.log.
 */
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { SERVE_ORIGIN } from './serve'
import { closeSettings, openPalette, openSettings, waitPaired } from './nav'
import { sheetBottomInset } from '../src/lib/inset'

function makeTerminalOffer(label: string): string {
  const doc = JSON.stringify({
    v: 1,
    endpoint: `${SERVE_ORIGIN}`,
    token: crypto.randomUUID(),
    expires_at: '',
    label,
  })
  return Buffer.from(doc).toString('base64url')
}

/** Pairs the shell through the gear, the only road to it now. */
async function pairShell(page: Page, label = 'This Mac (dev)'): Promise<void> {
  await openSettings(page)
  await page.locator('#term-offer').fill(makeTerminalOffer(label))
  await page.getByRole('button', { name: 'Pair', exact: true }).click()
  await expect(page.locator('#term-offer')).toHaveCount(0)
  await closeSettings(page)
}

test('the phone has no tab bar', async ({ page }) => {
  // Protects DESIGN.md §2: an unbounded set of owners does not fit fixed
  // slots, so there are no slots. The owner changes at the heading.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await expect(page.locator('nav.safe-bottom')).toHaveCount(0)
  await expect(page.locator('button.tab')).toHaveCount(0)
})

test('the palette is dormant on boot, and each road decides the keyboard', async ({
  page,
}) => {
  // Protects DESIGN.md §2: "It never focuses on boot — the first paint is
  // the owner's rows, so 'what's on my plate' stays a glance with no taps."
  // An autofocused field puts the keyboard over the first screen.
  //
  // Re-pinned 2026-09-18 (GDK-1994). The heading no longer opens the palette
  // at all, so the focus rule this used to pin at the heading now lives on
  // the two roads in: the magnifier is the door of someone who said they
  // want to type (GDK-1990), and the "this list" sheet's scope row is
  // someone asking for the owner list — which must not arrive under a
  // keyboard they did not ask for. FAIL-first on the mechanical rename of
  // this test's door: `expect(field).not.toBeFocused()` after the magnifier.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await expect(page.locator('.palette-field input')).toHaveCount(0)
  const activeOnBoot = await page.evaluate(() => document.activeElement?.tagName ?? null)
  expect(activeOnBoot, 'nothing is focused on boot').toBe('BODY')

  await page.locator('button.search').click()
  const field = page.locator('.palette-field input')
  await expect(field).toBeVisible()
  await expect(field, 'the magnifier is the door of someone typing').toBeFocused()
  await page.locator('button.palette-cancel').click()
  await expect(field).toHaveCount(0)

  await page.locator('h1 button.scope').click()
  await page.getByRole('dialog', { name: 'This list' }).locator('button.scope-row').click()
  await expect(field).toBeVisible()
  await expect(field, 'the owner list does not raise the keyboard').not.toBeFocused()
  const activeOnScope = await page.evaluate(() => document.activeElement?.tagName ?? null)
  expect(activeOnScope, 'the scope road does not focus the field').not.toBe('INPUT')
})

test('each door toggles: a second tap closes what the first opened', async ({
  page,
}) => {
  // Protects DESIGN.md §2 and GDK-1984: a door that carries aria-expanded
  // must be able to close what it opened — both of GDK-1974's doors carried
  // it and neither could.
  //
  // Re-pinned 2026-09-18 (GDK-1994): the two doors now open two bodies, so
  // each is checked against its own. The magnifier owns the palette (find
  // one issue in the snapshot) and toggles, because the palette replaces the
  // body and has no scrim. The heading owns the "this list" sheet, which
  // does have one — so its way out is its own Cancel (and the scrim, and
  // system back), and the heading's second tap is not a road the finger can
  // even take. What is checked on the heading is that `aria-expanded` follows
  // whichever of the two bodies is up, which is the one place it could lie.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const door = page.locator('button.search')
  await door.click()
  const field = page.locator('.palette-field input')
  await expect(field).toBeVisible()
  await expect(door).toHaveAttribute('aria-expanded', 'true')

  await door.click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
  await expect(door).toHaveAttribute('aria-expanded', 'false')

  // The heading's own door, and its own body.
  const heading = page.locator('h1 button.scope')
  await heading.click()
  const sheet = page.getByRole('dialog', { name: 'This list' })
  await expect(sheet).toBeVisible()
  await expect(heading).toHaveAttribute('aria-expanded', 'true')
  await sheet.locator('button.cancel').click()
  await expect(sheet).toHaveCount(0)
  await expect(heading).toHaveAttribute('aria-expanded', 'false')
})

test('closing by the door returns the list to the scroll position it had', async ({ page }) => {
  // Protects DESIGN.md §2: "The list keeps its scroll position across a
  // palette open-and-cancel." Cancel, system back and the door's second
  // tap are one road out — app.palette going false — and the single $effect
  // in Issues.svelte restores the position for all of them; the toggle
  // (GDK-1984) must not author a second close path, which is why this reads
  // the scroller after leaving by the door itself. Re-pinned 2026-09-18
  // (GDK-1994): that door is the magnifier now.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const scroller = page.locator('.pane:not(.off) main')
  await scroller.evaluate((el) => {
    el.scrollTop = 420
  })
  const before = await scroller.evaluate((el) => el.scrollTop)
  expect(before, 'the list actually scrolled').toBeGreaterThan(0)

  await page.locator('button.search').click()
  await expect(page.locator('.palette-field input')).toBeVisible()
  await page.locator('button.search').click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
  await expect
    .poll(async () => scroller.evaluate((el) => el.scrollTop), {
      message: 'scroll restored after closing by the search door',
    })
    .toBe(before)
})

test('the palette has a Terminal row only once a terminal pairing is stored', async ({ page }) => {
  // Protects DESIGN.md §10: absence, not a greyed-out row — the same
  // stance PairGate takes. This was the tab-count assertion in
  // shell.spec.ts before the bar went away.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await openPalette(page)
  await expect(page.locator('button.palette-row', { hasText: 'Terminal' })).toHaveCount(0)
  await page.locator('button.palette-cancel').click()

  await pairShell(page)
  await openPalette(page)
  await expect(page.locator('button.palette-row', { hasText: 'Terminal' })).toBeVisible()
})

test('Cancel returns the list to the scroll position it had', async ({ page }) => {
  // Protects DESIGN.md §2: "The list keeps its scroll position across a
  // palette open-and-cancel." Opening the owner list must not cost the
  // reader their place in it.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const scroller = page.locator('.pane:not(.off) main')
  await scroller.evaluate((el) => {
    el.scrollTop = 420
  })
  const before = await scroller.evaluate((el) => el.scrollTop)
  expect(before, 'the list actually scrolled').toBeGreaterThan(0)

  await openPalette(page)
  await page.locator('button.palette-cancel').click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
  await expect
    .poll(async () => scroller.evaluate((el) => el.scrollTop), {
      message: 'scroll restored after Cancel',
    })
    .toBe(before)
})

test('a Detail opened from the palette goes back to the palette, query intact', async ({ page }) => {
  // Protects the back order (DESIGN.md §2 entry/exit table): the palette is
  // not a sheet, so the detail closes first. Registering it as one made
  // system back close the palette and leave the Detail standing.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await openPalette(page)
  await page.locator('.palette-field input').fill('tenant')
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('.detail-layer button.back').first().waitFor()

  await page.goBack()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await expect(page.locator('.palette-field input')).toHaveValue('tenant')
})

/*
 * GDK-1997. The detail layer keeps its DOM for the length of the fly-out
 * after `app.detail` goes null, and a prop in Svelte 5 is a getter — so
 * `issueKey` was still `app.detail.key` while the store had already moved
 * on. `screens/Detail.svelte` re-reads that prop in its fetch continuation
 * (`if (key === issueKey)`, the guard that drops a response for an issue the
 * screen has left), so closing a detail whose request is still in flight
 * threw `Cannot read properties of null (reading 'key')` onto the page.
 *
 * The delay is the whole test. Without it the response has landed before the
 * back gesture and nothing re-reads the prop; the defect reached us as one
 * red CI run (35303892257, mobile/e2e/palette.spec.ts:195) in twenty-five
 * green ones on an unchanged tree, because the runner is slower than this
 * machine. FAIL-first with the delay, on the pre-fix source, every run:
 *
 *	Error: the page reported 1 uncaught error(s):
 *	  Cannot read properties of null (reading 'key')
 *
 * The assertion is the fixture's own `pageErrors` collector (helpers.ts:40),
 * which is why this test has no expectation of its own about the error: an
 * uncaught error anywhere in the run fails the test that was open.
 */
test('closing a detail whose request is still in flight throws nothing', async ({ page }) => {
  await page.route('**/api/v1/issues/*/', async (route) => {
    await new Promise((r) => setTimeout(r, 700))
    await route.fallback()
  })
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await openPalette(page)
  await page.locator('.palette-field input').fill('tenant')
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('.detail-layer').first().waitFor()

  // Back before the detail response lands.
  await page.goBack()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  // Long enough for the in-flight response to arrive and its continuation
  // to run, which is when the read happened.
  await page.waitForTimeout(1500)
  await expect(page.locator('.palette-field input')).toHaveValue('tenant')
})

test('a sheet in the column clears the home indicator on its own', async ({ page }) => {
  // GDK-902 2026-09-15. The tab bar used to stand between a sheet in the
  // column and the home indicator, so app.css exempted it from the bottom
  // inset ("a sheet inside .tabs already ends at the tab bar"). Removing
  // the bar turned that exemption into a gap: the create sheet and the
  // shell's session sheet became the bottom-most painted surface with
  // nothing paying their clearance. The rule now covers every sheet, and
  // this measures the one a person can reach from the list.
  //
  // FAIL-first, against `.detail-layer .sheet` in app.css: measured 0px,
  // expected 12.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await page.locator('button.new').click()
  const sheet = page.locator('.sheet')
  await sheet.waitFor()
  await sheet.evaluate(async (el) => {
    await Promise.all(el.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  const geo = await sheet.evaluate((el) => ({
    padBottom: parseFloat(getComputedStyle(el).paddingBottom),
    safeBottom:
      parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--safe-bottom')) || 0,
  }))
  expect(geo.padBottom, 'create sheet bottom inset').toBe(sheetBottomInset(geo.safeBottom))
})
