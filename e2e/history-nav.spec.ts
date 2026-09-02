/*
 * Back and forward, pressed for real (GDK-1296, GitHub discussion #80).
 *
 * The hash is the one source of where you are, and the rule for what a state
 * change does to history is three lines (lib/router.svelte.ts):
 *
 *   1. a place — the open issue, the filters, the layout, a committed query —
 *      pushes, so back is the previous place;
 *   2. a dialog opening pushes one entry and closing it is the same as back,
 *      so forward reopens it and back never lands on a dead press;
 *   3. continuous input inside a place — typing, a dialog's tabs — replaces,
 *      so one back closes the dialog however many tabs were visited.
 *
 * Every spec here drives the app from its own controls (not the address bar)
 * and then presses the browser's buttons — the URL alone is not the claim.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, openServerSettings, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

// An open fixture pair — both in the default "All open" view, so the first is
// reachable from the list: NMA-26 blocks NMS-7.
const BLOCKER = 'NMA-26'
const BLOCKED = 'NMS-7'

const historyLength = (page: Page) => page.evaluate(() => history.length)

/** The sidebar view rows lit as "the list you are on" (url-state.spec.ts). */
const activeViews = (page: Page) =>
  page.locator('aside nav button[aria-current="true"]:not([data-testid^="docs-"])').allInnerTexts()

const row = (page: Page, key: string) =>
  page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: new RegExp(`\\b${key}\\b`) })
    .first()

test.describe('history: places push (rule 1)', () => {
  test('list → issue → linked issue → back → back; a typed query is one entry', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const listUrl = page.url()

    // Typing a query is one entry however many characters land (empty → text).
    await searchInput(page).fill(BLOCKER)
    await expect(page).toHaveURL(/[?&]q=NMA-26(?![0-9])/)
    const afterQuery = await historyLength(page)
    await searchInput(page).fill(BLOCKER.slice(0, -1))
    await searchInput(page).fill(BLOCKER)
    await expect(page).toHaveURL(/[?&]q=NMA-26(?![0-9])/)
    expect(await historyLength(page)).toBe(afterQuery)

    await row(page, BLOCKER).click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toHaveClass(/is-open/)
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}(?![0-9])`))

    await panel.getByTestId('linked-issues').getByRole('button').filter({ hasText: BLOCKED }).click()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKED}(?![0-9])`))
    await expect(panel.getByRole('link', { name: BLOCKED, exact: true })).toBeVisible()

    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}(?![0-9])`))
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toBeVisible()

    await page.goBack()
    await expect(page).not.toHaveURL(/issue=/)
    await expect(panel).not.toHaveClass(/is-open/)
    await expect(page).toHaveURL(/[?&]q=NMA-26(?![0-9])/)

    // Back past the query: the URL drops it and so does the box.
    await page.goBack()
    await expect(page).not.toHaveURL(/[?&]q=/)
    await expect(searchInput(page)).toHaveValue('')
    expect(page.url()).toBe(listUrl)

    await page.goForward()
    await expect(page).toHaveURL(/[?&]q=NMA-26(?![0-9])/)
    await expect(searchInput(page)).toHaveValue(BLOCKER)
    await page.goForward()
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}(?![0-9])`))
    await expect(panel).toHaveClass(/is-open/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a view change and a layout change each leave one entry', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const allOpen = page.url()
    const bootViews = await activeViews(page)
    expect(bootViews.length).toBeGreaterThan(0)
    const len0 = await historyLength(page)

    await page.getByRole('button', { name: en['view.unassignedNew.name'] }).click()
    await expect(page).not.toHaveURL(allOpen)
    const unassigned = page.url()
    expect(await activeViews(page)).not.toEqual(bootViews)
    expect(await historyLength(page)).toBe(len0 + 1)

    await page.getByTestId('layout-board').click()
    await expect(page.getByTestId('board')).toBeVisible()
    await expect(page).not.toHaveURL(unassigned)
    expect(await historyLength(page)).toBe(len0 + 2)

    await page.goBack()
    await expect(page).toHaveURL(unassigned)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page.getByTestId('layout-list')).toHaveAttribute('aria-pressed', 'true')

    await page.goBack()
    await expect(page).toHaveURL(allOpen)
    await expect.poll(() => activeViews(page)).toEqual(bootViews)

    await page.goForward()
    await expect(page).toHaveURL(unassigned)
    await page.goForward()
    await expect(page.getByTestId('board')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the right panel is one place: person → issue → back is the person again', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('person-panel')).toBeVisible()
    await expect(page).toHaveURL(/person=/)
    const withPerson = page.url()
    const len0 = await historyLength(page)

    // Opening an issue over the person moves two params in one flush
    // (`person=` leaves, `issue=` arrives) — and is still one entry.
    await searchInput(page).fill(BLOCKER)
    // Let the query's own entry land first — otherwise its (debounced) push
    // and the click's fall into one flush and the count below reads one short.
    await expect(page).toHaveURL(/[?&]q=NMA-26(?![0-9])/)
    await row(page, BLOCKER).click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toHaveClass(/is-open/)
    await expect(page).toHaveURL(new RegExp(`issue=${BLOCKER}(?![0-9])`))
    await expect(page).not.toHaveURL(/person=/)
    await expect(page.getByTestId('person-panel')).toHaveCount(0)
    // query + issue open = two entries, no more
    expect(await historyLength(page)).toBe(len0 + 2)

    await page.goBack()
    await page.goBack()
    await expect(page).toHaveURL(withPerson)
    await expect(page.getByTestId('person-panel')).toBeVisible()
    await expect(panel.getByRole('link', { name: BLOCKER, exact: true })).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('history: dialogs (rules 2 and 3)', () => {
  test('opening settings is one entry; back closes it; closing is the same as back', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const listUrl = page.url()
    const len0 = await historyLength(page)
    const dialog = page.getByTestId('settings-dialog')

    await openServerSettings(page)
    await expect(page).toHaveURL(/settings=sync/)
    expect(await historyLength(page)).toBe(len0 + 1)

    // Back closes the dialog.
    await page.goBack()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(listUrl)

    // The close button is the same move as back: forward reopens the dialog,
    // and a back after closing never re-opens it.
    await openServerSettings(page)
    await expect(page).toHaveURL(/settings=sync/)
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(listUrl)

    await page.goForward()
    await expect(dialog).toBeVisible()
    await expect(page).toHaveURL(/settings=sync/)

    await dialog.getByRole('button', { name: en['common.cancel'], exact: true }).first().click()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(listUrl)
    await page.goBack()
    await expect(dialog).toHaveCount(0)
    await expect(page).not.toHaveURL(/settings=/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('three tab switches inside settings, one back: the dialog is closed', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const listUrl = page.url()
    const len0 = await historyLength(page)
    const dialog = page.getByTestId('settings-dialog')

    await openServerSettings(page)
    for (const key of ['settings.tabMembers', 'settings.tabFeatures', 'settings.tabAbout'] as const) {
      await dialog.getByRole('button', { name: en[key], exact: true }).click()
    }
    await expect(page).toHaveURL(/settings=about/)
    // Tabs are continuous input: the entry the dialog opened on is rewritten.
    expect(await historyLength(page)).toBe(len0 + 1)

    await page.goBack()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(listUrl)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the sidebar door into Settings → Workspaces closes like the gear does', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const listUrl = page.url()
    const dialog = page.getByTestId('settings-dialog')

    await page.getByTestId('workspace-new').click()
    await expect(dialog).toBeVisible()
    await expect(page).toHaveURL(/settings=workspaces/)

    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(listUrl)
    await page.goForward()
    await expect(dialog).toBeVisible()
    await expect(page).toHaveURL(/settings=workspaces/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('history: arriving by link', () => {
  test('a deep link adds no entry of its own, whatever the app promotes or normalizes', async ({
    page,
    request,
  }) => {
    const errors = attachConsoleErrors(page)
    const res = await request.get('/api/v1/issues/pages/')
    const body = (await res.json()) as { pages: { key: string }[] }
    const doc = [...body.pages].sort((a, b) => a.key.localeCompare(b.key))[0]

    // `doc=` alone lands on the documents screen (App promotes `docs=1`).
    await forceLocale(page, 'en')
    await page.goto(`/#/?doc=${encodeURIComponent(doc.key)}`)
    const len0 = await historyLength(page)
    await expect(page.getByTestId('doc-panel')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page).toHaveURL(/docs=1/)
    expect(await historyLength(page)).toBe(len0)

    // An unknown settings tab is normalized to sync — in place.
    await page.goto('/#/?settings=bogus')
    const len1 = await historyLength(page)
    await expect(page.getByTestId('settings-dialog')).toBeVisible({ timeout: 30_000 })
    await expect(page).toHaveURL(/settings=sync/)
    expect(await historyLength(page)).toBe(len1)

    // The default view at boot is written in place too: back from the first
    // screen does not land on an unfiltered pool.
    await gotoApp(page)
    const len2 = await historyLength(page)
    expect(len2).toBe(len1 + 1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
