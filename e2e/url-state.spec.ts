/*
 * Place params in the URL: the person panel, the personal feed, and the
 * server-settings dialog.
 *
 * The view hash already carried the list (view params) and, since the document
 * screens, the panels beside it (`issue`, `doc`, `space`…). These pin the three
 * surfaces that stayed memory-only — the third right-panel kind, the feed as a
 * main-column screen, the settings dialog and its tab — plus the line they must
 * not cross: a place param is selection, never view, so the sidebar's active
 * view and the saved-view string read exactly what they read before. And the
 * line the compose flows must not cross in the other direction: opening the
 * new-issue dialog leaves the URL untouched, because a link that prefills a
 * form someone is about to submit is a phishing surface.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, openServerSettings } from './helpers'
import { en } from '../web/src/lib/i18n/en'

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
 * Copied from docs-deeplink.spec.ts: the DOCS section is excluded by testid
 * because those rows mark the document screen someone is standing on — a
 * legitimate highlight that has nothing to do with the view-config match this
 * measures. The personal feed's two rows light up for the same reason when
 * `feed=` opens that screen, and MyIssuesNav carries no testids, so they are
 * excluded by their labels instead. Every row that does come from a view
 * config is a plain button with no testid.
 */
async function activeSidebarView(page: Page): Promise<string[]> {
  const rows = await page
    .locator('aside nav button.bg-bg-active:not([data-testid^="docs-"])')
    .allInnerTexts()
  // Anchor at the label so a view one day named "Feed…" is not swept out too.
  return rows.filter(
    (r) => !r.startsWith(en['personal.myReporter']) && !r.startsWith(en['common.feed']),
  )
}

/*
 * No route interception turns the feed on: serve.sh's config.json omits the
 * flag and the server defaults it on for exactly that case (features() in
 * internal/server/settings.go), so the fixture is a feed deployment as-is.
 */

/** The feed screen's one unambiguous landmark: its "Back to list" button. */
const feedClose = (page: Page) => page.getByRole('button', { name: en['feed.backToList'] })

test.describe('place params', () => {
  test('the person panel is in the URL, and comes back from it', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // The same palette path person.spec.ts drives: find Alex, open the panel.
    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('person-panel')).toBeVisible()

    // Opening it wrote the link; nothing else had to be done to get one.
    await expect(page).toHaveURL(/person=/)

    // The reload is the whole point: the third panel kind survives it now,
    // the same way the two beside it always have.
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('person-panel')).toBeVisible()
    await expect(page.getByTestId('person-name')).toHaveText('Alex Kim')

    // Closing takes the param with it, so the link never outlives the panel.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('person-panel')).toHaveCount(0)
    expect(page.url()).not.toContain('person=')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the feed is in the URL, comes back from it, and leaves with a close', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // "Reported by me" opens the feed on the reporter focus — presence says
    // open, the value says which slice.
    await page.getByRole('button', { name: en['personal.myReporter'] }).click()
    await expect(feedClose(page)).toBeVisible()
    await expect(page).toHaveURL(/feed=reporter/)

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(feedClose(page)).toBeVisible()
    await expect(page).toHaveURL(/feed=reporter/)

    // A link built by hand opens the other focus from a cold start.
    await gotoParams(page, 'feed=all')
    await expect(feedClose(page)).toBeVisible()
    await expect(page).toHaveURL(/feed=all/)

    // Back to the list takes the param with it.
    await feedClose(page).click()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(page.url()).not.toContain('feed=')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('settings=members boots onto the members tab; an unknown tab lands on sync', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoParams(page, 'settings=members')

    const dialog = page.getByTestId('settings-dialog')
    await expect(dialog).toBeVisible()
    const tab = (label: string) => dialog.getByRole('button', { name: label, exact: true })
    // Active tab styling is the only signal the dialog exposes (no aria-current).
    await expect(tab(en['settings.tabMembers'])).toHaveClass(/border-accent/)

    // A tab this build does not know (a link from before a rename, a typo)
    // must land on the default rather than a blank dialog — the tab list will
    // grow, and old links have to keep opening something real.
    await gotoParams(page, 'settings=nonsense')
    await expect(dialog).toBeVisible()
    await expect(tab(en['settings.tabSync'])).toHaveClass(/border-accent/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('opening settings writes the tab, and switching tabs moves the URL', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await openServerSettings(page)
    await expect(page).toHaveURL(/settings=sync/)

    const dialog = page.getByTestId('settings-dialog')
    await dialog.getByRole('button', { name: en['settings.tabMembers'], exact: true }).click()
    await expect(page).toHaveURL(/settings=members/)

    // Esc closes through the dialog's own handler; the param goes with it.
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    expect(page.url()).not.toContain('settings=')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the place params are selection, not view', async ({ page }) => {
    /*
     * The failure this rules out is quiet: if a place param reached the view
     * config, the sidebar would stop marking the view someone is on, and the
     * saved "last view" would carry a person or a tab into the next session.
     * Both are read here rather than reasoned about — same shape as
     * docs-deeplink's test, with the new params in the added string.
     *
     * This runs the strong case — every place param honored at once (the
     * fixture defaults the feed on): panels open, the feed's own sidebar row
     * lit as a place highlight. While the feed holds the main column no view
     * row is tinted (GDK-133: the highlight follows the main column), so the
     * view match is read where it is readable — after the feed closes, it
     * must be exactly what it was before any place param arrived.
     */
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const before = await activeSidebarView(page)
    expect(before.length).toBeGreaterThan(0)
    const viewUrl = page.url()
    expect(viewUrl, 'the boot view must be in the URL for this to control anything').toContain('sc=')

    const added = 'person=demo-alex&feed=reporter&settings=members'
    await page.goto(`${viewUrl}${viewUrl.includes('?') ? '&' : '?'}${added}`)
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await expect(page.getByTestId('person-panel')).toBeVisible()
    await expect(page.getByTestId('settings-dialog')).toBeVisible()
    // The feed screen took the main column — honored, not merely carried.
    await expect(feedClose(page)).toBeVisible()
    expect(page.url()).toContain('feed=reporter')
    // GDK-133: the feed owns the main column, so no view row is tinted now.
    expect(await activeSidebarView(page)).toEqual([])
    // Closing the overlays proves the view config was never touched: the
    // same view lights right back up once the list owns the column again.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('settings-dialog')).toHaveCount(0)
    await feedClose(page).click()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    expect(await activeSidebarView(page)).toEqual(before)

    const saved = await page.evaluate(() => localStorage.getItem('gadak:last-view') ?? '')
    for (const key of ['person=', 'feed=', 'settings=']) {
      expect(saved, `saved view must not carry ${key}`).not.toContain(key)
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('opening the new-issue dialog does not change the URL', async ({ page }) => {
    /*
     * The compose-flow exclusion, and the only one of the three exclusions a
     * test can hold down: a write dialog must never become addressable, or a
     * link could prefill a form someone is about to submit.
     */
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const before = page.url()

    await page.getByRole('button', { name: en['sidebar.newIssue'], exact: true }).click()
    await expect(page.getByRole('dialog', { name: en['write.newIssue'] })).toBeVisible()
    expect(page.url()).toBe(before)

    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: en['write.newIssue'] })).toHaveCount(0)
    expect(page.url()).toBe(before)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
