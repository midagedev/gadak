import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { mockBrowseRoutes, pretendDesktop, type BrowseMock } from './browse-mock'

/*
 * The in-app browser pane.
 *
 * Atlassian forbids iframes, so the original page is a native WKWebView the
 * desktop app draws over this document — and everything else about it is the
 * SPA's: the tab strip, which tab is showing, the rectangle the native side
 * must land in, and the resync that follows a tab closing. That is the part a
 * browser can hold to account, with desktop/browse.go's four routes standing in
 * as a mock (see browse-mock.ts). The rectangle's contents are the only thing
 * out of reach here; the app itself is the check for those.
 *
 * The last test is the other half: one bundle serves `gadak serve` and the app,
 * and the flag is the only thing between them.
 */

const JIRA = 'https://nimbus.example.com'

/** Open an issue in the right panel from the list. */
async function openIssue(page: Page, key: string): Promise<void> {
  await searchInput(page).fill(key)
  await expect(page.getByText(key).first()).toBeVisible()
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: key })
    .first()
    .click()
  await expect(page.getByTestId('issue-detail-panel').getByText(key).first()).toBeVisible()
}

/** Click the issue key in the detail header — the app's Atlassian deep link. */
async function openInApp(page: Page, key: string): Promise<void> {
  await page
    .getByTestId('issue-detail-panel')
    .locator(`a[href="${JIRA}/browse/${key}"]`)
    .first()
    .click()
}

/*
 * Wide enough that the pane takes the detail track and the list stays beside it
 * — the layout this was designed for. Under 1440 the pane floats over the list
 * the way the detail panel already does, which the narrow-window screenshot
 * covers and these flows would only be fighting.
 */
async function bootDesktop(page: Page): Promise<BrowseMock> {
  await page.setViewportSize({ width: 1680, height: 1000 })
  await pretendDesktop(page)
  const mock = await mockBrowseRoutes(page)
  await gotoApp(page)
  return mock
}

test.describe('in-app browser pane', () => {
  test('a Jira link opens a tab, and the pane reports its rectangle', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktop(page)

    await openIssue(page, 'NMB-110')
    await openInApp(page, 'NMB-110')

    const pane = page.getByTestId('browse-pane')
    await expect(pane).toBeVisible()
    // The layout knows it is browsing — that is what widens the pane's track.
    await expect(page.getByTestId('issue-layout')).toHaveAttribute('data-browse-open', 'true')

    // One tab, labelled by the page's own title.
    const tabs = page.getByTestId('browse-tab')
    await expect(tabs).toHaveCount(1)
    await expect(tabs.first()).toHaveText(/NMB-110/)
    await expect(tabs.first()).toHaveAttribute('data-active', 'true')
    await expect(page.getByTestId('browse-url')).toHaveText(`${JIRA}/browse/NMB-110`)

    // The native view is created already visible, so nothing has to be
    // activated for it to show — but the rectangle is ours to hand over.
    expect(mock.tabs().map((t) => t.url)).toEqual([`${JIRA}/browse/NMB-110`])
    await expect.poll(() => mock.frames.length).toBeGreaterThan(0)

    // …and it is the viewport box of the empty container, to the pixel.
    // ±1px, not toBeCloseTo(…, 0): the rect poll's last frame and
    // boundingBox() round subpixels on either side of an async boundary, so
    // a correct pane can legitimately report 944 against a measured 945
    // (GDK-1064, observed 2026-08-28; <0.5px was the flake).
    const box = await page.getByTestId('browse-viewport').boundingBox()
    expect(box).not.toBeNull()
    const last = mock.frames[mock.frames.length - 1]
    expect(Math.abs(last.x - box!.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(last.y - box!.y)).toBeLessThanOrEqual(1)
    expect(Math.abs(last.w - box!.width)).toBeLessThanOrEqual(1)
    expect(Math.abs(last.h - box!.height)).toBeLessThanOrEqual(1)
    // A pane worth showing a web page in.
    expect(last.w).toBeGreaterThan(500)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('tabs accumulate, switch, and close back to the detail underneath', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktop(page)

    await openIssue(page, 'NMB-110')
    await openInApp(page, 'NMB-110')
    await expect(page.getByTestId('browse-pane')).toBeVisible()

    // Opening something else in the panel is a request to see Gadak's copy: the
    // pane steps aside, and the tab it was showing is still open behind it.
    await openIssue(page, 'NMB-111')
    await expect(page.getByTestId('browse-pane')).toHaveCount(0)
    await expect(page.getByTestId('browse-reentry')).toBeVisible()
    expect(mock.activates[mock.activates.length - 1]).toBe('')

    await openInApp(page, 'NMB-111')
    const tabs = page.getByTestId('browse-tab')
    await expect(tabs).toHaveCount(2)
    await expect(tabs.nth(1)).toHaveAttribute('data-active', 'true')
    await expect(page.getByTestId('browse-url')).toHaveText(`${JIRA}/browse/NMB-111`)

    // Switching tabs is one activate and one URL.
    await tabs.nth(0).getByRole('button').first().click()
    await expect(tabs.nth(0)).toHaveAttribute('data-active', 'true')
    await expect(page.getByTestId('browse-url')).toHaveText(`${JIRA}/browse/NMB-110`)
    await expect.poll(() => mock.active()).toBe('1')

    // Closing the visible tab hands the pane to its neighbour, not to Gadak.
    await tabs.nth(0).getByTestId('browse-tab-close').click()
    await expect(page.getByTestId('browse-tab')).toHaveCount(1)
    await expect(page.getByTestId('browse-pane')).toBeVisible()
    await expect(page.getByTestId('browse-url')).toHaveText(`${JIRA}/browse/NMB-111`)

    // Closing the last one ends the pane and gives the detail panel back.
    await page.getByTestId('browse-tab-close').first().click()
    await expect(page.getByTestId('browse-pane')).toHaveCount(0)
    await expect(page.getByTestId('browse-reentry')).toHaveCount(0)
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(
      page.getByTestId('issue-detail-panel').getByText('NMB-111').first(),
    ).toBeVisible()
    expect(mock.tabs()).toEqual([])

    // Every tab asked for a resync of the item it was opened on — which is the
    // whole point of remembering what that was. (Leaving the pane resyncs too,
    // so NMB-110 appears more than once; the set is what this is about.)
    await expect
      .poll(() => [...new Set(mock.resynced)].sort())
      .toEqual(['issue:NMB-110', 'issue:NMB-111'])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('leaving hides the native view; the indicator brings the same tab back', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktop(page)

    await openIssue(page, 'NMB-110')
    await openInApp(page, 'NMB-110')
    await expect(page.getByTestId('browse-pane')).toBeVisible()

    await page.getByTestId('browse-back').click()
    await expect(page.getByTestId('browse-pane')).toHaveCount(0)
    // Hidden, not closed: the tab survives, and nothing is drawing over the app.
    expect(mock.tabs()).toHaveLength(1)
    await expect.poll(() => mock.active()).toBe('')

    const reentry = page.getByTestId('browse-reentry')
    await expect(reentry).toBeVisible()
    await expect(reentry).toContainText('1')

    await reentry.click()
    await expect(page.getByTestId('browse-pane')).toBeVisible()
    await expect.poll(() => mock.active()).toBe('1')

    // The system-browser handoff carries the tab's current URL.
    await page.getByTestId('browse-open-external').click()
    await expect.poll(() => mock.opened).toEqual([`${JIRA}/browse/NMB-110`])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a tab closed natively (⌘W) is noticed by the poll', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktop(page)

    await openIssue(page, 'NMB-110')
    await openInApp(page, 'NMB-110')
    await expect(page.getByTestId('browse-pane')).toBeVisible()

    // ⌘W is a native menu item; this document never hears the keystroke.
    mock.closeNatively('1')

    await expect(page.getByTestId('browse-pane')).toHaveCount(0)
    await expect(page.getByTestId('browse-reentry')).toHaveCount(0)
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an SPA overlay hides the native view for as long as it is up', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const mock = await bootDesktop(page)

    await openIssue(page, 'NMB-110')
    await openInApp(page, 'NMB-110')
    await expect.poll(() => mock.active()).toBe('1')

    // The command palette would open *underneath* the page otherwise — a native
    // view draws over every pixel of this document inside its rectangle.
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await page.keyboard.press('ControlOrMeta+k')
    await expect(palette).toBeVisible()
    await expect.poll(() => mock.active()).toBe('')
    // The pane's own chrome stays: it is what says the tab is still there.
    await expect(page.getByTestId('browse-pane')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(palette).toBeHidden()
    await expect.poll(() => mock.active()).toBe('1')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('at 1280×820 the palette is on top of the browse pane and clickable', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 820 })
    await pretendDesktop(page)
    const mock = await mockBrowseRoutes(page)
    await gotoApp(page)

    await openIssue(page, 'NMB-110')
    await openInApp(page, 'NMB-110')
    await expect(page.getByTestId('browse-pane')).toBeVisible()
    await expect.poll(() => mock.active()).toBe('1')

    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await page.keyboard.press('ControlOrMeta+k')
    await expect(palette).toBeVisible()
    await expect.poll(() => mock.active()).toBe('')
    await expect(page.locator('html')).toHaveAttribute('data-browse-yield', 'on')
    await expect(page.getByTestId('browse-pane')).toHaveAttribute('data-browse-yield', 'true')

    const box = await palette.boundingBox()
    expect(box).not.toBeNull()
    const hit = await page.evaluate(
      ({ x, y }) => {
        const el = document.elementFromPoint(x, y)
        return el?.closest('[role="dialog"]')?.getAttribute('aria-label') ?? null
      },
      { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 },
    )
    expect(hit, 'palette must be the frontmost surface at the shipped window size').toBe(
      'Command palette',
    )

    await palette.getByRole('combobox').click()
    await page.keyboard.type('NMB-110')
    await expect(palette.getByRole('option').first()).toContainText('NMB-110')

    await page.keyboard.press('Escape')
    await expect(palette).toBeHidden()
    await expect.poll(() => mock.active()).toBe('1')
    await expect(page.locator('html')).toHaveAttribute('data-browse-yield', 'off')
    await expect(page.getByTestId('browse-pane')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a browser tab has no pane and asks the desktop routes for nothing', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const desktopCalls: string[] = []
    page.on('request', (req) => {
      if (req.url().includes('/desktop/')) desktopCalls.push(req.url())
    })

    await gotoApp(page)
    await openIssue(page, 'NMB-110')

    // The link is an ordinary external anchor again — target=_blank, no
    // interception, no pane. Clicking it here would open a browser tab, which
    // is why this asserts the absence rather than the click.
    await expect(page.getByTestId('browse-pane')).toHaveCount(0)
    await expect(page.getByTestId('browse-reentry')).toHaveCount(0)
    await expect(page.getByTestId('issue-layout')).toHaveAttribute('data-browse-open', 'false')
    expect(desktopCalls).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
