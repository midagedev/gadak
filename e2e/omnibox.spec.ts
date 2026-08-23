import { test, expect, type Page } from '@playwright/test'
import { apiURL, appConsoleErrors, attachConsoleErrors, forceLocale, gotoApp } from './helpers'

/*
 * Dedicated-browser address bar: a pasted Atlassian URL is never silently
 * swallowed. /browse/KEY opens the native panel; a miss names the key; a
 * synced ?filter= applies its chips; a mirrored wiki page opens natively.
 *
 * W2 FAIL-first: on HEAD, SearchBox preventDefault + applyJql(not_jql) eats
 * a /browse/KEY paste and returns false with no toast and no selection.
 */

const SITE = 'https://nimbus.example.com'
const PAGES_URL = apiURL('/api/v1/issues/pages/')

async function pasteIntoSearch(page: Page, text: string): Promise<void> {
  const box = page.getByTestId('search-input')
  await box.click()
  await box.evaluate((el, value) => {
    const dt = new DataTransfer()
    dt.setData('text/plain', value)
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }))
  }, text)
}

test.describe('omnibox paste routing', () => {
  test('a /browse/KEY paste selects the issue when it is in the pool', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await pasteIntoSearch(page, `${SITE}/browse/NMB-110`)

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    await expect(page.getByTestId('toast')).toHaveCount(0)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a /browse/KEY miss names the key instead of swallowing the paste', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await pasteIntoSearch(page, `${SITE}/browse/ZZZ-999`)

    await expect(page.getByTestId('issue-detail-panel')).toBeHidden()
    const toast = page.getByTestId('toast')
    await expect(toast).toBeVisible()
    await expect(toast).toContainText('ZZZ-999')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Enter on a /browse/KEY URL takes the same native path as paste', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const box = page.getByTestId('search-input')
    await box.fill(`${SITE}/browse/NMB-110`)
    await box.press('Enter')

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a synced ?filter= URL applies that source view', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page.getByTestId('sidebar-jira-filters')).toBeVisible()

    await pasteIntoSearch(page, `${SITE}/issues/?filter=e2e-open-nma`)

    await expect(page).toHaveURL(/pj=NMA/)
    await expect(page).toHaveURL(/sc=inprogress/)
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'NMA' })).toBeVisible()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a mirrored wiki page URL opens the native document panel', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    const list = (await (await request.get(PAGES_URL)).json()) as {
      pages: { key: string; title: string; url: string }[]
    }
    const doc = list.pages.find((p) => p.url && /\/wiki\/spaces\/[^/]+\/pages\/\d+/.test(p.url))
    expect(doc, 'fixture must expose a wiki page URL').toBeTruthy()

    await gotoApp(page)
    await expect(page.getByTestId('docs-documents')).toBeVisible()
    await pasteIntoSearch(page, doc!.url)

    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(doc!.title)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an ADF /browse/KEY body link opens the native panel, not a new tab', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.route('**/api/v1/issues/NMB-110/detail/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        description_adf?: unknown
      }
      body.description_adf = {
        type: 'doc',
        content: [
          {
            type: 'paragraph',
            content: [
              {
                type: 'text',
                text: 'See related',
                marks: [{ type: 'link', attrs: { href: `${SITE}/browse/NMA-1` } }],
              },
            ],
          },
        ],
      }
      await route.fulfill({ response, json: body })
    })
    await page.goto('/#/?issue=NMB-110')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()

    const popup: string[] = []
    page.on('popup', (p) => popup.push(p.url()))

    await page.getByTestId('issue-detail-panel').locator('a[href$="/browse/NMA-1"]').click()
    await expect(page.getByTestId('issue-detail-panel').getByText('NMA-1').first()).toBeVisible()
    expect(popup, 'body link must not open a browser tab').toEqual([])

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the palette lists imported Jira filters in the view section', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page.getByTestId('sidebar-jira-filters')).toBeVisible()
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('Open in NMA', { delay: 15 })
    // Skip the instant-create row (GDK-217): it echoes the query, so it also
    // contains 'Open in NMA'. The row under test is the Jira-filter view.
    const row = palette
      .locator('[role="option"]:not([data-testid="palette-create-now"])')
      .filter({ hasText: 'Open in NMA' })
    await expect(row).toBeVisible()
    await expect(row).toContainText('Jira filter')
    await row.click()
    await expect(page).toHaveURL(/pj=NMA/)
    await expect(page).toHaveURL(/sc=inprogress/)
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
