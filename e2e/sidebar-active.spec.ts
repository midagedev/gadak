/*
 * Sidebar view-row tint vs place-row tint.
 *
 * A non-list main column (docs / history / feed) used to leave the matching
 * built-in view painted `bg-bg-active` as well, so two rows in different
 * sections both claimed "you are here". The filter config was never wrong —
 * only the tint. These pin the paint, not the match.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp } from './helpers'

/** Boot straight into a hash query, the way a shared link or a reload arrives. */
async function gotoParams(page: Page, query: string): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto(`/#/?${query}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
}

/**
 * The saved-view rows the sidebar is marking as active, by their label.
 *
 * The DOCS section is excluded by testid rather than by position: those rows
 * mark the document screen someone is standing on, which is exactly what a
 * restored `space=` is now supposed to light up (2026-08-07) — a legitimate
 * highlight that has nothing to do with the view-config match this measures.
 * Every row that does come from a view config is a plain button with no testid.
 */
async function activeSidebarView(page: Page): Promise<string[]> {
  return page.locator('aside nav button.bg-bg-active:not([data-testid^="docs-"])').allInnerTexts()
}

test.describe('sidebar view highlight follows the main column', () => {
  test('cold boot docs=1 tints Documents only, not the startup view', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoParams(page, 'docs=1')
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page.getByTestId('docs-documents')).toBeVisible()
    // applyStartupView writes the default view into the hash; without that
    // commit All open would not match, and a green here would not prove the
    // tint is suppressed rather than merely not-yet-applied.
    await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })

    await expect(page.getByTestId('docs-documents')).toHaveClass(/bg-bg-active/)
    expect(await activeSidebarView(page)).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('closing docs restores the view-row tint', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoParams(page, 'docs=1')
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
    expect(await activeSidebarView(page)).toEqual([])

    await page.getByTestId('docs-close').click()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(page.url()).not.toContain('docs=1')
    expect((await activeSidebarView(page)).length).toBeGreaterThan(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('cold boot list still tints the startup view', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect((await activeSidebarView(page)).length).toBeGreaterThan(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
