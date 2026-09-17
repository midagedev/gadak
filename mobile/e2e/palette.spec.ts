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

test('the palette is dormant on boot, and the heading opens it with the keyboard down', async ({
  page,
}) => {
  // Protects DESIGN.md §2: "It never focuses on boot — the first paint is
  // the owner's rows, so 'what's on my plate' stays a glance with no taps."
  // An autofocused field puts the keyboard over the first screen.
  //
  // GDK-1985: the heading is the only door, and opening it never raises the
  // keyboard — a person tapping their view's name wants a scope, and the
  // owner list must not sit under a keyboard they did not ask for. The
  // field rides the head of the body; a person who wants to type taps it.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await expect(page.locator('.palette-field input')).toHaveCount(0)
  const activeOnBoot = await page.evaluate(() => document.activeElement?.tagName ?? null)
  expect(activeOnBoot, 'nothing is focused on boot').toBe('BODY')

  await page.locator('h1 button.scope').click()
  const field = page.locator('.palette-field input')
  await expect(field).toBeVisible()
  await expect(field).not.toBeFocused()
  const activeOnScope = await page.evaluate(
    () => document.activeElement?.tagName ?? null,
  )
  expect(activeOnScope, 'the heading tap does not focus the field').not.toBe('INPUT')
})

test('the heading is one toggling door: a second tap closes what the first opened', async ({
  page,
}) => {
  // Protects DESIGN.md §2: the heading is the palette's only door — one
  // 44pt control wearing the magnifier, so the screen says search without a
  // second control (GDK-1985, superseding GDK-1974's two doors). With one
  // door the second tap has exactly one meaning, so the door toggles
  // (GDK-1984): both of GDK-1974's doors carried aria-expanded and neither
  // could close what it opened.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const door = page.locator('h1 button.scope')
  await door.click()
  const field = page.locator('.palette-field input')
  await expect(field).toBeVisible()
  await expect(field).not.toBeFocused()
  await expect(door).toHaveAttribute('aria-expanded', 'true')

  await door.click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
  await expect(door).toHaveAttribute('aria-expanded', 'false')
})

test('closing by the heading returns the list to the scroll position it had', async ({ page }) => {
  // Protects DESIGN.md §2: "The list keeps its scroll position across a
  // palette open-and-cancel." Cancel, system back and the heading's second
  // tap are one road out — app.palette going false — and the single $effect
  // in Issues.svelte restores the position for all of them; the toggle
  // (GDK-1984) must not author a second close path, which is why this reads
  // the scroller after leaving by the heading itself.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const scroller = page.locator('.pane:not(.off) main')
  await scroller.evaluate((el) => {
    el.scrollTop = 420
  })
  const before = await scroller.evaluate((el) => el.scrollTop)
  expect(before, 'the list actually scrolled').toBeGreaterThan(0)

  await page.locator('h1 button.scope').click()
  await expect(page.locator('.palette-field input')).toBeVisible()
  await page.locator('h1 button.scope').click()
  await expect(page.locator('.palette-field input')).toHaveCount(0)
  await expect
    .poll(async () => scroller.evaluate((el) => el.scrollTop), {
      message: 'scroll restored after closing by the heading',
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
