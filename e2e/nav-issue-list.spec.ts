import fs from 'node:fs'
import path from 'node:path'

import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, e2eHomeDir, gotoApp } from './helpers'

/*
 * Main-column occupancy: "show the issue list" must drop docs / space / feed.
 *
 * Contract → assertion
 *  1. DOCS → My issues (built-in) → docs-view gone, issue-list-scroller visible
 *  2. DOCS → All open → same (already worked via applyView)
 *  3. DOCS → ui-focus `pj=NMA&sc=inprogress` → list + chips
 *  4. space → My issues (built-in) → same as (1)
 *  5. DOCS → palette issue pick → panel opens, docs-view stays (selection contract)
 */

const focusFile = path.join(e2eHomeDir(), 'ui-focus.json')

function writeFocus(hash: string): void {
  fs.writeFileSync(
    focusFile,
    JSON.stringify({ hash, at: new Date().toISOString() }),
    'utf8',
  )
}

async function openDocuments(page: Page): Promise<void> {
  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible()
}

async function expectIssueList(page: Page): Promise<void> {
  await expect(page.getByTestId('docs-view')).toHaveCount(0)
  await expect(page.getByTestId('space-docs-view')).toHaveCount(0)
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
}

test.describe('issue list takes the main column', () => {
  test('DOCS → My issues shows the issue list', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    // The legacy "Assigned to me" row is gone (2026-09-07 sidebar
    // subtraction); the built-in My issues view is that question's one
    // owner, and its config serialises the mine flag, not an assignee.
    await page.locator('aside').getByRole('button', { name: 'My issues' }).click()
    await expectIssueList(page)
    expect(page.url()).not.toContain('docs=1')
    await expect(page).toHaveURL(/[#?&]fl=mine(&|$)/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('DOCS → All open still shows the issue list', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    await page.getByRole('button', { name: 'All open' }).click()
    await expectIssueList(page)
    expect(page.url()).not.toContain('docs=1')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('DOCS → ui-focus hash lands the list and chips', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    writeFocus('pj=NMA&sc=inprogress')

    await expect(page).toHaveURL(/pj=NMA/, { timeout: 10_000 })
    await expect(page).toHaveURL(/sc=inprogress/)
    await expectIssueList(page)
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'NMA' })).toBeVisible()
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'In Progress' })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('space → My issues shows the issue list', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('docs-spaces').click()
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }).click()
    await expect(page.getByTestId('space-docs-view')).toBeVisible()

    await page.locator('aside').getByRole('button', { name: 'My issues' }).click()
    await expectIssueList(page)
    expect(page.url()).not.toContain('space=')
    await expect(page).toHaveURL(/[#?&]fl=mine(&|$)/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('DOCS → palette issue pick opens the panel and leaves the document screen', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('NMB-110', { delay: 20 })
    await page.keyboard.press('Enter')

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page.getByTestId('issue-list-scroller')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
