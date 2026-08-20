/*
 * GDK-434 / GDK-435: sidebar section collapse + order persist in localStorage.
 *
 * Contracts:
 *  1. Collapse a section, reload → still collapsed
 *  2. Reorder sections, reload → order kept
 *  3. A collapsed section's header stays visible with aria-expanded=false
 */
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const SHOT_COLLAPSED = path.join(here, '../scratch/gdk-434-sidebar-collapsed.png')
const SHOT_EXPANDED = path.join(here, '../scratch/gdk-434-sidebar-expanded.png')

function sectionHeader(page: Page, id: string) {
  return page.getByTestId(`sidebar-section-header-${id}`)
}

function sectionBody(page: Page, id: string) {
  return page.getByTestId(`sidebar-section-body-${id}`)
}

/** Visible section ids in DOM order (data-section on each listitem). */
async function sectionOrder(page: Page): Promise<string[]> {
  return page
    .locator('[data-testid="sidebar-sections"] [data-section]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-section') ?? ''))
}

async function reloadSidebar(page: Page): Promise<void> {
  await page.reload()
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('sidebar-sections')).toBeVisible()
}

test.describe('sidebar section collapse and order', () => {
  test('collapsing a section survives reload; header stays visible', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const header = sectionHeader(page, 'builtin')
    await expect(header).toBeVisible()
    await expect(header).toHaveAttribute('aria-expanded', 'true')
    await expect(sectionBody(page, 'builtin')).toBeVisible()

    await page.locator('aside.issue-sidebar').screenshot({ path: SHOT_EXPANDED })

    await header.click()
    await expect(header).toHaveAttribute('aria-expanded', 'false')
    await expect(header).toBeVisible()
    await expect(sectionBody(page, 'builtin')).toBeHidden()

    await page.locator('aside.issue-sidebar').screenshot({ path: SHOT_COLLAPSED })

    await reloadSidebar(page)

    const after = sectionHeader(page, 'builtin')
    await expect(after).toBeVisible()
    await expect(after).toHaveAttribute('aria-expanded', 'false')
    await expect(sectionBody(page, 'builtin')).toBeHidden()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('reordering sections with Alt+Arrow survives reload', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const before = await sectionOrder(page)
    expect(before.length, 'demo fixture should show at least two sections').toBeGreaterThanOrEqual(2)
    expect(before[0]).toBe('builtin')

    const header = sectionHeader(page, 'builtin')
    await header.focus()
    await header.press('Alt+ArrowDown')

    const moved = await sectionOrder(page)
    expect(moved[0]).not.toBe('builtin')
    expect(moved).toContain('builtin')
    expect(moved.length).toBe(before.length)

    await reloadSidebar(page)

    expect(await sectionOrder(page)).toEqual(moved)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
