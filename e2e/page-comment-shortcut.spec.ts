import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-650: the page comment composer shows the same submit-shortcut kbd
 * chip the issue composer does (UX_PRINCIPLES §3 — the shortcut lives
 * where the action is). The chord is write.commentShortcut interpolated
 * with the platform modifier, not a page-local spelling. Issue-composer
 * chip text is locked in ux-f12.spec.ts; this spec locks the page surface
 * to that same catalog key.
 */

async function openDocFromTree(page: Page, title: string): Promise<void> {
  await page.getByTestId('docs-spaces').click()
  await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'PROD' }).click()
  const view = page.getByTestId('space-docs-view')
  await view.getByTestId('space-tree-toggle').click()
  await view
    .getByTestId('doc-tree-node')
    .filter({ hasText: 'Feature Specs' })
    .getByTestId('doc-tree-toggle')
    .click()
  await view
    .getByTestId('doc-tree-node')
    .filter({ hasText: title })
    .getByRole('button', { name: title, exact: true })
    .click()
  await expect(page.getByTestId('doc-title')).toHaveText(title)
}

test.describe('GDK-650 page comment shortcut', () => {
  test('page composer shows the catalog-keyed shortcut chip next to submit', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocFromTree(page, 'Billing Settings Spec')

    const panel = page.getByTestId('doc-panel')
    await expect(panel.getByTestId('doc-comment-composer')).toBeVisible()
    await expect(panel.getByTestId('doc-comment-submit')).toBeVisible()

    const chip = panel.getByTestId('comment-shortcut')
    await expect(chip).toBeVisible()
    await expect(chip).toHaveCount(1)

    // Same platform test as modifierSymbol() / ux-f12: the catalog does not
    // hard-code ⌘; the chip interpolates {mod}.
    expect(en['write.commentShortcut']).toBe('{mod} ↵')
    expect(en['write.commentShortcut']).not.toMatch(/⌘/)
    const mod = await page.evaluate(() =>
      /Mac|iP(hone|ad)/.test(navigator.platform) ? '⌘' : 'Ctrl',
    )
    await expect(chip).toHaveText(en['write.commentShortcut'].replace('{mod}', mod))

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
