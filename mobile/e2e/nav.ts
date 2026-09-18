/*
 * Navigation helpers for the phone suite (GDK-902, 2026-09-15).
 *
 * Every spec used to reach a screen by clicking `nav.safe-bottom button.tab`
 * and to know the app was up by waiting for that nav. The tab bar is gone
 * (DESIGN.md §2), so the road to each surface is defined once here instead
 * of twenty-five times across e2e/ and shots/ — the next navigation change
 * is one edit, not a grep.
 *
 * `waitPaired` is the single "the app is showing its owner" signal: the
 * heading control exists (the owner's name, always present on the list) and
 * the first row is painted.
 *
 * GDK-1994 split the one door in two, and this file is why that was one
 * edit per road rather than a grep: the magnifier opens the palette, the
 * heading opens the "this list" sheet, and each has a helper below.
 */
import { expect, type Locator, type Page } from '@playwright/test'

/** The list is up and has rows. Replaces every `nav.safe-bottom` wait. */
export async function waitPaired(page: Page): Promise<void> {
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

/** Taps the header magnifier and waits for the field it focuses. */
export async function openPalette(page: Page): Promise<void> {
  await page.locator('button.search').click()
  await expect(page.locator('.palette-field input')).toBeVisible()
}

/** Taps the heading's chevron and waits for the sheet it opens (GDK-1994). */
export async function openListSheet(page: Page): Promise<Locator> {
  await page.locator('h1 button.scope').click()
  const sheet = page.getByRole('dialog', { name: 'This list' })
  await expect(sheet).toBeVisible()
  return sheet
}

/** Opens the Settings push layer from the gear. */
export async function openSettings(page: Page): Promise<void> {
  await page.locator('button.gear').click()
  await page.locator('.settings-layer button.back').waitFor()
}

/** Closes the Settings layer by its own back control. */
export async function closeSettings(page: Page): Promise<void> {
  await page.locator('.settings-layer button.back').click()
  await expect(page.locator('.settings-layer')).toHaveCount(0)
}
