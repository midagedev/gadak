/*
 * Document screens in the URL.
 *
 * Every document surface used to live only in memory: a reload landed back on
 * the issue list, and a page could not be linked to at all — while the issue
 * panel beside it had survived both since the first release. These pin the four
 * params that closed that gap (`doc`, `space`, `dview`, `docs`), and the line
 * they must not cross: they are selection, never view, so the sidebar's active
 * view and the saved-view string read exactly what they read before.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp } from './helpers'

const PAGES_URL = 'http://127.0.0.1:7877/api/v1/issues/pages/'

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
 * Read by `aria-current="true"`, which rides the same condition as the row's
 * paint — a palette-token rename cannot turn this red (GDK-613). The DOCS
 * section is excluded by testid rather than by position: those rows mark the
 * document screen someone is standing on (aria-pressed, no aria-current),
 * which is exactly what a restored `space=` is now supposed to light up
 * (2026-08-07) — a legitimate highlight that has nothing to do with the
 * view-config match this measures. Every row that does come from a view
 * config is a plain button with no testid.
 */
async function activeSidebarView(page: Page): Promise<string[]> {
  return page
    .locator('aside nav button[aria-current="true"]:not([data-testid^="docs-"])')
    .allInnerTexts()
}

test.describe('document screens survive a reload and a link', () => {
  test('an opened document is in the URL, and comes back from it', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as { pages: { key: string; title: string }[] }
    const first = [...body.pages].sort((a, b) => a.key.localeCompare(b.key))[0]

    await gotoApp(page)
    await page.getByTestId('docs-documents').click()
    await page.getByTestId('docs-view').getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    const row = page.getByTestId('doc-row').first()
    const key = await row.getAttribute('data-doc-key')
    const title = (await page.getByTestId('doc-row').first().locator('span').first().innerText()).trim()
    await row.click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()

    // Opening it wrote the link; nothing else had to be done to get one.
    expect(page.url()).toContain(`doc=${key}`)

    // The reload is the whole point: this used to come back to the issue list.
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(title)

    // A link built by hand — a colleague pasting a key — opens the same panel,
    // from a cold start rather than from the state this test happens to be in.
    await gotoParams(page, `doc=${encodeURIComponent(first.key)}`)
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(first.title)

    // Closing it takes the param with it, so the link never outlives the panel.
    await page.keyboard.press('x')
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)
    expect(page.url()).not.toContain('doc=')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a space comes back, and only the tree is written down', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('docs-spaces').click()
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }).click()
    const view = page.getByTestId('space-docs-view')
    await expect(view).toBeVisible()
    expect(page.url()).toContain('space=ENG')
    // The flat list is the default, so it is not a value worth carrying: a URL
    // that spells out the default is a URL that has to be kept in step with it.
    expect(page.url()).not.toContain('dview')

    await view.getByTestId('space-tree-toggle').click()
    await expect(view.getByTestId('doc-tree-node').first()).toBeVisible()
    expect(page.url()).toContain('dview=tree')

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await expect(view).toBeVisible()
    await expect(view).toHaveAttribute('data-space', 'ENG')
    await expect(view.getByTestId('space-tree-toggle')).toHaveAttribute('aria-pressed', 'true')
    await expect(view.getByTestId('doc-tree-node').first()).toBeVisible()

    // Back to the list and the value disappears again rather than reading
    // `dview=list`, which nothing would ever need.
    await view.getByTestId('space-list-toggle').click()
    await expect(view.getByTestId('doc-row').first()).toBeVisible()
    expect(page.url()).not.toContain('dview')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the tabbed view is one param, and the tab is not', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // The tab is this browser's return path, remembered locally. A link that
    // carried it would impose one person's place on everyone who opens it.
    await page.getByTestId('docs-documents').click()
    const view = page.getByTestId('docs-view')
    await view.getByTestId('docs-tab').filter({ hasText: 'By author' }).click()
    expect(page.url()).toContain('docs=1')
    expect(page.url()).not.toContain('tab')

    await gotoParams(page, 'docs=1')
    await expect(view).toBeVisible()
    await expect(view.getByTestId('docs-tab').filter({ hasText: 'By author' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )

    // Leaving takes the param with it.
    await view.getByTestId('docs-close').click()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(page.url()).not.toContain('docs=1')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('back and forward move between document screens', async ({ page }) => {
    const errors = attachConsoleErrors(page)

    // Each address is a screen, so the browser's own buttons move between them
    // — the URL is what the app reads, not a label it writes afterwards.
    await gotoParams(page, 'docs=1')
    await expect(page.getByTestId('docs-view')).toBeVisible()

    await page.goto('/#/?space=ENG&dview=tree')
    await expect(page.getByTestId('space-docs-view')).toBeVisible()
    await expect(page.getByTestId('doc-tree-node').first()).toBeVisible()

    await page.goBack()
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page.getByTestId('space-docs-view')).toHaveCount(0)

    await page.goForward()
    await expect(page.getByTestId('space-docs-view')).toBeVisible()
    await expect(page.getByTestId('doc-tree-node').first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a link to a page restores the screen the page lives on', async ({ page, request }) => {
    /*
     * A restored `doc` used to bring back the panel and nothing else, so a
     * shared link opened a document floating over the issue list — a screen the
     * person who sent it had never been looking at, whose Close button leads
     * somewhere they were never taken. What arrived was one address, and
     * everything it restores has to belong to the same place.
     */
    const errors = attachConsoleErrors(page)
    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as { pages: { key: string; space_key: string }[] }
    const first = [...body.pages].sort((a, b) => a.key.localeCompare(b.key))[0]
    const inEng = body.pages.find((p) => p.space_key === 'ENG')!

    await gotoParams(page, `doc=${encodeURIComponent(first.key)}`)
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    // Behind it: the document screen, not the issue list.
    await expect(page.getByTestId('docs-view')).toBeVisible()
    await expect(page.getByTestId('issue-list-scroller')).toHaveCount(0)
    // And the sidebar says so, the same way it does for someone who clicked
    // their way here — a restored screen that the nav does not mark is a screen
    // rendering two different ways depending on how it was reached.
    await expect(page.getByTestId('docs-documents')).toHaveAttribute('aria-pressed', 'true')
    // The address is now the screen it restored, so a reload from here is a
    // second arrival at the same place rather than a slower one.
    expect(page.url()).toContain('docs=1')

    // Closing the panel leaves the screen it was opened on, not the app's
    // default one. This is the half a floating panel could never get right.
    await page.keyboard.press('x')
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)
    await expect(page.getByTestId('docs-view')).toBeVisible()
    expect(page.url()).not.toContain('doc=')

    // A page linked with its space behind it restores that space instead, and
    // the disclosure holding it opens — a highlight inside a collapsed section
    // is a highlight nobody can see.
    await gotoParams(page, `doc=${encodeURIComponent(inEng.key)}&space=ENG`)
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    const spaceView = page.getByTestId('space-docs-view')
    await expect(spaceView).toBeVisible()
    await expect(spaceView).toHaveAttribute('data-space', 'ENG')
    await expect(page.getByTestId('docs-spaces')).toHaveAttribute('aria-expanded', 'true')
    await expect(
      page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }),
    ).toHaveAttribute('aria-pressed', 'true')

    await page.keyboard.press('x')
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)
    await expect(spaceView).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a page opened from the issue list stays over the issue list', async ({ page }) => {
    /*
     * The other direction of the same rule, and the one the promotion above
     * must not swallow: search results are worked through several hits at a
     * time, so opening one is a request to read it *beside* the list, never a
     * request to leave. Only an address arriving with nothing behind it means
     * "this is the screen" — a click inside a screen means what the screen says.
     */
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const search = page.getByTestId('search-input')
    await search.click()
    await search.fill('runbook')
    await search.press('Enter')
    await expect(page.getByTestId('search-docs')).toBeVisible()

    await page.getByTestId('search-doc-row').first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    // The list is still the list, and the URL carries the panel alone.
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page.getByTestId('docs-view')).toHaveCount(0)
    expect(page.url()).toContain('doc=')
    expect(page.url()).not.toContain('docs=1')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the document params are selection, not view', async ({ page, request }) => {
    /*
     * The failure this rules out is quiet: if a document param reached the view
     * config, the sidebar would stop marking the view someone is on (nothing
     * would match its canon), and the saved "last view" would carry a document
     * key into the next session. Both are read here rather than reasoned about.
     *
     * The document params are *added to the URL the app is already on* rather
     * than written fresh: view state is the URL, so a hand-typed address
     * without `sc=` is a different view by definition, and comparing against it
     * would be measuring that instead of these four params.
     */
    const errors = attachConsoleErrors(page)
    const res = await request.get(PAGES_URL)
    const body = (await res.json()) as { pages: { key: string }[] }

    await gotoApp(page)
    const before = await activeSidebarView(page)
    expect(before.length).toBeGreaterThan(0)
    const viewUrl = page.url()
    expect(viewUrl, 'the boot view must be in the URL for this to control anything').toContain('sc=')

    const added = `space=ENG&dview=tree&doc=${encodeURIComponent(body.pages[0].key)}`
    await page.goto(`${viewUrl}${viewUrl.includes('?') ? '&' : '?'}${added}`)
    await expect(page.getByTestId('space-docs-view')).toBeVisible()
    expect(await activeSidebarView(page)).toEqual(before)

    const saved = await page.evaluate(() => localStorage.getItem('gadak:last-view') ?? '')
    for (const key of ['doc=', 'space=', 'dview=', 'docs=']) {
      expect(saved, `saved view must not carry ${key}`).not.toContain(key)
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
