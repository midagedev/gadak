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

test('the palette is dormant on boot, and the heading opens it without the keyboard', async ({
  page,
}) => {
  // Protects DESIGN.md §2: "It never focuses on boot — the first paint is
  // the owner's rows, so 'what's on my plate' stays a glance with no taps."
  // An autofocused field puts the keyboard over the first screen.
  //
  // GDK-1974: the heading is the scope door, and a person who tapped it to
  // change scope was getting the keyboard in their face. Opening the owner
  // list must not focus the field — the keyboard is what the search door
  // (button.search, the test below) is for.
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

test('the magnifier is the search door: it opens the palette with the field focused', async ({
  page,
}) => {
  // GDK-1974: nothing on the list screen said "search" — the only door was
  // the heading, and the header's right side held only the create control
  // and the gear. The magnifier (button.search, left of button.new) is the
  // door that means a query: the same palette, the field focused on mount.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await page.locator('button.search').click()
  const field = page.locator('.palette-field input')
  await expect(field).toBeVisible()
  await expect(field).toBeFocused()
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
