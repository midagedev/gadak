import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, forceLocale, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-604: one Esc closes only the topmost surface.
 *
 * Pins the same negotiation triage.spec already owns for bulk-then-detail
 * (first Esc clears the selection, second Esc closes the panel) and
 * list-menus-esc owns for a list menu over an open panel. The viewer and
 * the person panel were missing from that contract.
 */

function lastKeyCmd(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-last-key-cmd')
}

async function openIssueWithAttachment(page: Page) {
  await forceLocale(page, 'en')
  await page.goto('/#/?issue=NMB-110')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  await expect(panel).toHaveClass(/is-open/)

  // Gallery tile, not the same file inlined in a comment (adf-media-image).
  const thumb = panel.locator('button.group[aria-label^="Enlarge "]').first()
  await expect(thumb).toBeVisible({ timeout: 15_000 })
  const label = await thumb.getAttribute('aria-label')
  expect(label, 'attachment enlarge control should name the file').toBeTruthy()
  const filename = label!.slice('Enlarge '.length)
  expect(filename.length).toBeGreaterThan(0)
  return { panel, filename, thumb }
}

async function openViewer(page: Page, thumb: ReturnType<Page['locator']>, filename: string) {
  await thumb.click()
  const viewer = page.getByRole('dialog', { name: filename })
  await expect(viewer).toBeVisible()
  return viewer
}

test.describe('Esc negotiation (GDK-604)', () => {
  test('one Esc closes the media viewer and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { panel, filename, thumb } = await openIssueWithAttachment(page)
    const viewer = await openViewer(page, thumb, filename)

    await page.keyboard.press('Escape')

    await expect(viewer).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Tab stays inside the media viewer', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { filename, thumb } = await openIssueWithAttachment(page)
    const viewer = await openViewer(page, thumb, filename)

    await expect
      .poll(async () => viewer.evaluate((el) => el.contains(document.activeElement)))
      .toBe(true)

    for (let i = 0; i < 6; i++) {
      await page.keyboard.press('Tab')
      expect(
        await viewer.evaluate((el) => el.contains(document.activeElement)),
        `Tab ${i + 1} left the media viewer`,
      ).toBe(true)
    }

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc over bulk + person panel clears the selection only', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)
    await page.keyboard.press('x')
    const bar = page.getByTestId('bulk-bar')
    await expect(bar).toBeVisible()
    await expect(bar.getByText('1 selected')).toBeVisible()

    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')
    const person = page.getByTestId('person-panel')
    await expect(person).toBeVisible()
    await expect(bar).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(bar).toBeHidden()
    await expect(person).toBeVisible()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc dismisses the top toast and leaves the column view open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // GDK-829: a toast is a surface too. The cheapest deterministic toast in
    // the demo app is docs.spec's page-comment 409 — the refused POST toasts
    // the catalog sentence instead of the wire error.
    await page.route('**/api/v1/issues/pages/*/comment/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        json: { error: 'credential_required' },
      })
    })
    await gotoApp(page)
    await page.getByTestId('docs-documents').click()
    await expect(page.getByTestId('docs-view')).toBeVisible()
    // The default tab is Viewed, which a fresh profile has none of — Updated
    // lists the mirrored pages, so the row to open is always there.
    await page.locator('[data-testid="docs-tab"][data-tab="updated"]').click()
    await page.getByTestId('doc-row').first().click()
    const panel = page.getByTestId('doc-panel')
    await expect(panel.getByTestId('doc-comment-composer')).toBeVisible()
    await panel.getByTestId('doc-comment-composer').fill('wt-e2e esc toast')
    await panel.getByTestId('doc-comment-submit').click()

    const toast = page.getByTestId('toast')
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(en['write.needToken'])
    // Focus may rest on the submit button; either way the next Esc must be
    // the toast's, not the field's or the panel's.
    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
    })

    await page.keyboard.press('Escape')
    await expect(toast).toHaveCount(0)
    await expect(panel).toBeVisible()
    await expect(page.getByTestId('docs-view')).toBeVisible()

    // The refused POST leaves the draft in the composer, and an unfocused
    // non-empty draft spends an Esc of its own (GDK-462) — clear it (and
    // drop the focus fill() left there) so the next Escs walk the real
    // chain this test pins: panel first, then the column view.
    await panel.getByTestId('doc-comment-composer').fill('')
    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
    })
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)
    await expect(page.getByTestId('docs-view')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByTestId('docs-view')).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('close-docs')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
