import { test, expect, type Locator, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * Desktop window drag is a CSS contract the Wails v3 runtime reads off the
 * event target (`--wails-draggable`). One bundle serves `gadak serve` and the
 * app: only `config.desktop` may turn the regions on. This spec pins the
 * computed property — whether the native window actually moves is
 * tools/check-desktop-drag.sh against Gadak.app.
 *
 * Geometry of the traffic-light inset lives in desktop-chrome.spec.ts.
 */

async function pretendDesktop(page: Page): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = JSON.parse(await res.text())
    doc.desktop = true
    await route.fulfill({ response: res, body: JSON.stringify(doc) })
  })
}

async function cssProp(el: Locator, name: string): Promise<string> {
  return el.evaluate((node, prop) => getComputedStyle(node).getPropertyValue(prop).trim(), name)
}

/** First band of the main column — SearchBox / filters / menus. */
function listToolbar(page: Page): Locator {
  return page.getByTestId('list-toolbar')
}

test.describe('desktop drag regions', () => {
  test('a browser tab does not mark a drag region or suppress selection', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const toolbar = listToolbar(page)
    await expect(toolbar).toBeVisible()
    expect(await cssProp(toolbar, '--wails-draggable')).not.toBe('drag')
    expect(await cssProp(toolbar, 'user-select')).not.toBe('none')

    const logo = page.getByTestId('sidebar-logo-row')
    expect(await cssProp(logo, '--wails-draggable')).not.toBe('drag')
    expect(await cssProp(logo, 'user-select')).not.toBe('none')

    const input = searchInput(page)
    await input.click()
    await expect(input).toBeFocused()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the app marks the sidebar row and the list toolbar as drag regions', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await pretendDesktop(page)
    await gotoApp(page)

    const logo = page.getByTestId('sidebar-logo-row')
    expect(await cssProp(logo, '--wails-draggable')).toBe('drag')
    expect(await cssProp(logo, 'user-select')).toBe('none')

    const toolbar = listToolbar(page)
    await expect(toolbar).toBeVisible()
    expect(await cssProp(toolbar, '--wails-draggable')).toBe('drag')
    expect(await cssProp(toolbar, 'user-select')).toBe('none')

    // Interactive clusters inside the toolbar must opt out: Wails reads the
    // property off the event target, and it inherits.
    const noDrag = [
      searchInput(page),
      page.getByTestId('search-help'),
      page.getByTestId('view-settings'),
      page.getByRole('button', { name: '+ Filter', exact: true }),
      // Copy JQL is not on the bare view any more (GDK-1336: view actions
      // appear once the reader has narrowed the view); it inherits no-drag
      // from the same FilterBar wrapper as `+ Filter` when it does.
      page.getByTestId('freshness-chip'),
    ]
    for (const el of noDrag) {
      await expect(el).toBeVisible()
      expect(await cssProp(el, '--wails-draggable'), await el.evaluate((n) => n.outerHTML.slice(0, 80))).toBe(
        'no-drag',
      )
    }

    // The search field stays a text field: user-select:none on the region
    // must not inherit onto the input.
    expect(await cssProp(searchInput(page), 'user-select')).toBe('text')
    await searchInput(page).click()
    await expect(searchInput(page)).toBeFocused()

    // The issue-count label is not a control — it stays a grab surface.
    const count = page.getByTestId('list-count')
    expect(await cssProp(count, '--wails-draggable')).toBe('drag')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
