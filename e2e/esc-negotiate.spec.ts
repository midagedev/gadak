import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, forceLocale, gotoApp } from './helpers'

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
})
