import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, forceLocale, gotoApp } from './helpers'

/*
 * keys axis + ui-focus handoff + the hidden-tab poll pause.
 *
 * W4 FAIL-first: App.svelte starts a 500 ms ui-focus interval that does not
 * look at document.hidden, so a backgrounded tab keeps GET-ing.
 */

const here = path.dirname(fileURLToPath(import.meta.url))
const focusFile = path.join(here, '.tmp/home/ui-focus.json')

function writeFocus(hash: string): void {
  fs.writeFileSync(
    focusFile,
    JSON.stringify({ hash, at: new Date().toISOString() }),
    'utf8',
  )
}

async function hideDocument(page: Page): Promise<void> {
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'hidden',
    })
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

async function showDocument(page: Page): Promise<void> {
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'visible',
    })
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

test.describe('keys view and ui-focus', () => {
  test('ui-focus poll sends no requests while the tab is hidden', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const hits: number[] = []
    page.on('request', (req) => {
      if (req.url().includes('/ui-focus/')) hits.push(Date.now())
    })

    await expect.poll(() => hits.length).toBeGreaterThan(0)
    await hideDocument(page)
    // Snapshot AFTER the page has processed visibilitychange, past one full
    // poll period so a tick dispatched before the transition has drained
    // (GDK-175, 2026-08-17: snapshotting before hideDocument raced the
    // 500 ms interval — a request sent while still visible landed after the
    // snapshot and read as "polled while hidden"). The contract measured is
    // unchanged: no NEW requests while hidden.
    await page.waitForTimeout(600)
    const before = hits.length
    // 500ms poll × 3 plus slack: the contract is that the count stays put
    // across an elapsed interval; there is no other "poll did not fire" state.
    await page.waitForTimeout(1600)
    expect(
      hits.length,
      `hidden tab kept polling ui-focus (${hits.length - before} extra GETs)`,
    ).toBe(before)

    await showDocument(page)
    await expect.poll(() => hits.length).toBeGreaterThan(before)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('#/?issue= focuses the detail panel', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/#/?issue=NMB-110')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    // 409 is the fixture credential refusing writes during boot; not this path.
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('keys view via ui-focus lands the exact list in given order', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Opposite of default updated-desc so a sort miss cannot pass by accident.
    const ordered = ['NMA-1', 'NMB-110']
    writeFocus(`ks=${ordered.join(',')}&g=none`)

    await expect(page).toHaveURL(/ks=NMA-1,NMB-110/, { timeout: 5_000 })
    await expect(page.getByTestId('filter-chip').filter({ hasText: /2 keys/ })).toBeVisible()
    await expect(page.getByTestId('list-count')).toHaveText('2 issues')

    const keys = await page
      .locator('[data-testid="issue-list-scroller"] [data-issue-key]')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-issue-key')))
    expect(keys).toEqual(ordered)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a ks= URL is an OR of exact keys and the chip clears them', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/#/?ks=nmb-110,nma-1&g=none')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

    // In-memory keys are uppercased; the hash keeps the typed case until a
    // later mutation re-serializes.
    await expect(page).toHaveURL(/ks=nmb-110,nma-1/i)
    await expect(page.getByTestId('filter-chip').filter({ hasText: /2 keys/ })).toBeVisible()
    await expect(page.getByTestId('list-count')).toHaveText('2 issues')

    const keys = await page
      .locator('[data-testid="issue-list-scroller"] [data-issue-key]')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-issue-key')))
    expect(keys).toEqual(['NMB-110', 'NMA-1'])

    await page.getByTestId('filter-chip').filter({ hasText: /2 keys/ }).click()
    await expect(page.getByTestId('list-count')).toContainText('534')
    await expect(page).not.toHaveURL(/ks=/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('sidebar recents include a document visit in the mixed list', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    const list = (await (await request.get('http://127.0.0.1:7877/api/v1/issues/pages/')).json()) as {
      pages: { key: string; title: string }[]
    }
    const doc = list.pages[0]
    expect(doc?.key).toBeTruthy()

    await forceLocale(page, 'en')
    await page.addInitScript((visit) => {
      localStorage.setItem(
        'gadak:recent',
        JSON.stringify([
          { key: visit.key, viewed_at: new Date().toISOString(), kind: 'doc' },
          { key: 'NMB-110', viewed_at: new Date().toISOString(), kind: 'issue' },
        ]),
      )
    }, doc)
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

    await expect(page.getByTestId(`recent-doc-${doc.key}`)).toBeVisible()
    await expect(page.getByTestId(`recent-doc-${doc.key}`)).toContainText(doc.title)
    await expect(page.getByTestId('recent-issue-NMB-110')).toBeVisible()

    await page.getByTestId(`recent-doc-${doc.key}`).click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    await expect(page.getByTestId('doc-title')).toHaveText(doc.title)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
