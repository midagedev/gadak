import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-651: docs / history / feed share Esc-to-close and palette entry with
 * the issue list. j/k stays issue-list-only (no shared cursor); Tab is the
 * documented walk on these three views.
 */

function lastKeyCmd(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-last-key-cmd')
}

const feedClose = (page: Page) => page.getByRole('button', { name: en['feed.backToList'] })

async function openDocuments(page: Page): Promise<void> {
  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible()
}

async function openHistory(page: Page): Promise<void> {
  await page.getByTestId('history-open').click()
  await expect(page.getByTestId('history-view')).toBeVisible()
}

async function openPalette(page: Page) {
  await page.keyboard.press('ControlOrMeta+k')
  const palette = page.getByRole('dialog', { name: 'Command palette' })
  await expect(palette).toBeVisible()
  return palette
}

async function blurActive(page: Page): Promise<void> {
  await page.evaluate(() => {
    const active = document.activeElement
    if (active instanceof HTMLElement) active.blur()
  })
}

test.describe('column-view interaction parity (GDK-651)', () => {
  test('Esc closes the documents view', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)
    await blurActive(page)

    await page.keyboard.press('Escape')
    await expect(page.getByTestId('docs-view')).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('close-docs')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Esc closes the history view', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openHistory(page)
    await blurActive(page)

    await page.keyboard.press('Escape')
    await expect(page.getByTestId('history-view')).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('close-history')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Esc closes the feed', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Same cold-hash boot as e2e/url-state.spec.ts gotoParams: feed= is a
    // place param, and the sidebar count is the bootstrap landmark.
    await forceLocale(page, 'en')
    await page.goto('/#/?feed=all')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect(feedClose(page)).toBeVisible()
    await blurActive(page)

    await page.keyboard.press('Escape')
    await expect(feedClose(page)).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('close-feed')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Esc over a detail opened on documents closes the panel first', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    const palette = await openPalette(page)
    await page.keyboard.type('NMB-110', { delay: 20 })
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await blurActive(page)

    await page.keyboard.press('Escape')
    // RightPanel stays mounted; open state is the is-open class / layout flag.
    await expect(panel).not.toHaveClass(/is-open/)
    await expect(page.getByTestId('issue-layout')).toHaveAttribute('data-detail-open', 'false')
    await expect(page.getByTestId('docs-view')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('clear-selection')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('palette opens documents, history, and feed', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    let palette = await openPalette(page)
    await palette.getByRole('combobox').fill('documents')
    await palette.getByTestId('palette-action-docs').click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('docs-view')).toBeVisible()

    palette = await openPalette(page)
    await palette.getByRole('combobox').fill('history')
    await palette.getByRole('option', { name: en['palette.actionHistory'] }).click()
    await expect(page.getByTestId('history-view')).toBeVisible()
    await expect(page.getByTestId('docs-view')).toHaveCount(0)

    palette = await openPalette(page)
    await palette.getByRole('combobox').fill('feed')
    await palette.getByTestId('palette-action-feed').click()
    await expect(feedClose(page)).toBeVisible()
    await expect(page.getByTestId('history-view')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('palette lists favorite (and watch when identified) for the cursor row', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)

    const palette = await openPalette(page)
    await expect(palette.getByTestId('palette-action-favorite')).toBeVisible()
    await expect(palette.getByTestId('palette-action-watch')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  // GDK-826: the cheat-sheet content test that lived here (Tab / Esc labels
  // re-asserted through an open sheet) moved down — surface-consistency.test.ts
  // owns the registry keys and commands.test.ts owns "the sheet renders
  // helpSections()". polish.spec.ts remains the one browser path: ? opens,
  // Esc closes, and every documented key has a live handler.
})
