import fs from 'node:fs'
import path from 'node:path'

import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { apiURL, appConsoleErrors, attachConsoleErrors, e2eHomeDir, forceLocale, gotoApp, DEMO_ISSUE_COUNT } from './helpers'

/*
 * keys axis + ui-focus handoff + the hidden-tab poll pause.
 *
 * W4 FAIL-first: App.svelte starts a 500 ms ui-focus interval that does not
 * look at document.hidden, so a backgrounded tab keeps GET-ing.
 */

const focusFile = path.join(e2eHomeDir(), 'ui-focus.json')

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
    // The clock is installed before boot so the app's timers are faked from
    // the moment they are armed. A mid-test install cannot help: the 500 ms
    // ui-focus interval created at boot holds a real native handle that a
    // later fake clearInterval cannot stop — the old test paid 2.2 s of real
    // waits (drain one period, then out-wait three more) to observe that
    // interval instead (GDK-723). Frozen time lets the same contract run in
    // milliseconds: fast-forward past whole poll periods and count.
    await page.clock.install()
    // Page-side counter: incremented synchronously where fetch is *called*,
    // so no CDP delivery lag can move the count after a snapshot — the race
    // the old 600 ms drain existed to cover.
    await page.addInitScript(() => {
      const hits: string[] = []
      ;(window as unknown as { __uiFocusHits: string[] }).__uiFocusHits = hits
      const orig = window.fetch.bind(window)
      window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof Request
              ? input.url
              : String(input)
        if (url.includes('/ui-focus/')) hits.push(url)
        return orig(input, init)
      }
    })
    await gotoApp(page)

    const hitCount = () =>
      page.evaluate(() => (window as unknown as { __uiFocusHits: string[] }).__uiFocusHits.length)

    // Two poll periods of fake time while visible: the interval is armed.
    await page.clock.fastForward(1200)
    await expect.poll(hitCount, 'visible tab never polled ui-focus').toBeGreaterThan(0)

    await hideDocument(page)
    // The app's own stop flag, not a duration: when this flips the interval
    // is cleared, and any fetch a visible tick started is already in the
    // page-side counter (it increments at call time). No drain wait needed.
    await page.waitForFunction(() => document.documentElement.dataset.uiFocusPoll === 'off')
    const before = await hitCount()
    // 500 ms poll × 4 of fake time while hidden — the same "no NEW requests
    // across elapsed periods" contract, without the wall clock.
    await page.clock.fastForward(2000)
    const after = await hitCount()
    expect(after, `hidden tab kept polling ui-focus (${after - before} extra GETs)`).toBe(before)

    await showDocument(page)
    await page.clock.fastForward(600)
    await expect.poll(hitCount, 'visible tab did not resume polling ui-focus').toBeGreaterThan(
      before,
    )

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
    await expect(page.getByTestId('list-count')).toContainText(String(DEMO_ISSUE_COUNT))
    await expect(page).not.toHaveURL(/ks=/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('sidebar recents include a document visit in the mixed list', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    const list = (await (await request.get(apiURL('/api/v1/issues/pages/'))).json()) as {
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

/*
 * GDK-276: Enter on focused chrome must run that control's action, not
 * open-cursor. keys-focus owns this (focus + key) rather than keys-order
 * (ks= grouping). lastKeyCmd is the permanent keymap debug surface.
 *
 * Covered with existing hooks only: filter-add, view-settings (columns and
 * sort are one menu since GDK-1391) and docs-documents (testids). Breakdown
 * is the same class but not in the audit's four.
 */
function lastKeyCmd(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-last-key-cmd')
}

test.describe('Enter on focused chrome (GDK-276)', () => {
  test('filter, view settings, and documents activate on Enter', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Below 1440 the detail overlay covers the list toolbar (list-menus-esc).
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)
    // The steal only happens once a list cursor exists (the boot-normal state).
    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)

    const add = page.getByTestId('filter-add')
    await add.focus()
    await expect(add).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(page.getByText('Properties', { exact: true })).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')
    await expect(page.getByTestId('issue-detail-panel')).not.toHaveClass(/is-open/)
    await page.keyboard.press('Escape')
    await expect(page.getByText('Properties', { exact: true })).toBeHidden()

    const settings = page.getByTestId('view-settings')
    await settings.focus()
    await expect(settings).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(page.getByText('Visible columns', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '↓ Desc', exact: true })).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')
    await expect(page.getByTestId('issue-detail-panel')).not.toHaveClass(/is-open/)
    await page.keyboard.press('Escape')
    await expect(page.getByText('Visible columns', { exact: true })).toBeHidden()
    await expect(page.getByRole('button', { name: '↓ Desc', exact: true })).toBeHidden()

    const docs = page.getByTestId('docs-documents')
    await docs.focus()
    await expect(docs).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('docs-view')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Enter on body still opens the list cursor', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)

    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
    })

    await page.keyboard.press('Enter')
    await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)
    expect(await lastKeyCmd(page)).toBe('open-cursor')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('search box and palette keep Enter local', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)

    const search = page.getByTestId('search-input')
    await search.focus()
    await expect(search).toBeFocused()
    await page.keyboard.press('Enter')
    expect(await lastKeyCmd(page)).toBe('ignore')
    await expect(page.getByTestId('issue-detail-panel')).not.toHaveClass(/is-open/)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('zzz-no-such-action')
    expect(await lastKeyCmd(page)).toBe('ignore')
    await expect(palette).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(palette).toBeHidden()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * GDK-81: o is the header escape hatch (same openContainedUrl path as the
 * issue-key link). lastKeyCmd names the verb; window.open is what serve-mode
 * openContainedUrl calls (desktop POSTs /desktop/browse instead).
 */
test.describe('o opens the issue origin (GDK-81)', () => {
  test('list cursor + o calls the header open path', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)

    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
      const opened: string[] = []
      ;(window as unknown as { __gadakOpened: string[] }).__gadakOpened = opened
      window.open = (url?: string | URL) => {
        opened.push(String(url ?? ''))
        return null
      }
    })

    const key = await page.locator('[data-cursor="true"]').getAttribute('data-issue-key')
    expect(key).toBeTruthy()

    await page.keyboard.press('o')
    expect(await lastKeyCmd(page)).toBe('open-origin')

    const opened = await page.evaluate(
      () => (window as unknown as { __gadakOpened: string[] }).__gadakOpened,
    )
    expect(opened).toEqual([`https://nimbus.example.com/browse/${key}`])

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
