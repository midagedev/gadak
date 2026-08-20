import { test, expect, type Page } from '@playwright/test'
import { dismissHostedFirstFrame } from './helpers'

/**
 * GDK-23: hosted demo must boot in in-app browsers that lack service workers.
 * Chromium always has navigator.serviceWorker; we shadow it to undefined the
 * way a WKWebView family WebView does, then assert the product (not the
 * full-screen "unsupported" notice) is what paints.
 */

const DEMO = '/gadak/'

const UNSUPPORTED_HEADING = '이 브라우저에서는 데모를 열 수 없어요'

function searchInput(page: Page) {
  return page.getByTestId('search-input')
}

async function applyAllOpen(page: Page): Promise<void> {
  await page.getByRole('button', { name: /All open/ }).click()
  await expect(page).not.toHaveURL(/[?&]g=epic(?:&|$)/)
}

async function forceLocale(page: Page, locale: 'en' | 'ko' = 'en'): Promise<void> {
  await page.addInitScript((loc) => {
    try {
      if (!localStorage.getItem('gadak_locale')) {
        localStorage.setItem('gadak_locale', loc)
      }
    } catch {
      /* ignore */
    }
  }, locale)
}

/** Shadow navigator.serviceWorker so `'serviceWorker' in navigator` stays true
 *  (property exists) but the value is undefined — matches the spec's
 *  defineProperty technique. Old main.ts then throws on `.register` and
 *  paints the unsupported notice. */
async function shadowServiceWorker(page: Page): Promise<void> {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      enumerable: false,
      get() {
        return undefined
      },
    })
  })
}

test.describe('hosted demo in an in-app browser', () => {
  test('defineProperty shadow makes navigator.serviceWorker undefined', async ({
    page,
  }) => {
    await shadowServiceWorker(page)
    await page.goto(DEMO)
    const proof = await page.evaluate(() => {
      let value: unknown
      try {
        value = (navigator as Navigator & { serviceWorker?: unknown }).serviceWorker
      } catch (e) {
        return { ok: false, err: String(e) }
      }
      return {
        ok: true,
        inNavigator: 'serviceWorker' in navigator,
        valueIsUndefined: value === undefined,
        hasRegister:
          !!value && typeof (value as { register?: unknown }).register === 'function',
      }
    })
    expect(proof, 'evaluate of serviceWorker shadow').toMatchObject({
      ok: true,
      valueIsUndefined: true,
      hasRegister: false,
    })
  })

  test('boots the issue list and an attachment without a service worker', async ({
    page,
  }) => {
    await forceLocale(page, 'en')
    await shadowServiceWorker(page)
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)

    const proof = await page.evaluate(
      () => (navigator as Navigator & { serviceWorker?: unknown }).serviceWorker,
    )
    expect(proof, 'serviceWorker must stay undefined after navigation').toBeUndefined()

    const layout = page.getByTestId('issue-layout')
    const notice = page.getByRole('heading', { name: UNSUPPORTED_HEADING })
    await Promise.race([
      layout.waitFor({ state: 'visible', timeout: 60_000 }),
      notice.waitFor({ state: 'visible', timeout: 60_000 }),
    ])
    await expect(notice).toHaveCount(0)
    await expect(page.getByText(/In-app browsers/)).toHaveCount(0)
    await expect(layout).toBeVisible()
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 60_000 })

    await applyAllOpen(page)
    await searchInput(page).fill('NMB-110')
    await expect(page.getByText(/1 issues?|1 issue/)).toBeVisible({ timeout: 15_000 })
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    const galleryImg = panel.locator('img[src*="/attachments/"]').first()
    await expect(galleryImg).toBeVisible({ timeout: 15_000 })
    await expect
      .poll(async () => galleryImg.evaluate((el: HTMLImageElement) => el.naturalWidth))
      .toBeGreaterThan(0)
  })

  test('adapter handles Request input, unknown delta as 404, and 501 writes', async ({ page }) => {
    await forceLocale(page, 'en')
    await shadowServiceWorker(page)
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })

    const delta = await page.evaluate(async () => {
      const req = new Request('/gadak/api/v1/issues/delta/?since=2026-01-01T00:00:00.000Z')
      const res = await fetch(req)
      return { status: res.status, body: (await res.json()) as Record<string, unknown> }
    })
    // GDK-440: hosted no longer fabricates an empty live delta.
    expect(delta.status).toBe(404)
    expect(delta.body).toEqual({ error: 'not_found' })

    const write = await page.evaluate(async () => {
      const res = await fetch('/gadak/api/v1/issues/NMB-110/comment/', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: 'should-not-reach-a-server' }),
      })
      return { status: res.status, body: (await res.json()) as { error?: string } }
    })
    expect(write.status).toBe(501)
    expect(write.body.error).toBe('demo_read_only')

    const detail = await page.evaluate(async () => {
      const res = await fetch(new Request('/gadak/api/v1/issues/NMB-110/detail/'))
      const body = (await res.json()) as {
        attachments?: { content_url?: string }[]
      }
      return {
        status: res.status,
        urls: (body.attachments ?? []).map((a) => a.content_url ?? ''),
      }
    })
    expect(detail.status).toBe(200)
    expect(detail.urls.length).toBeGreaterThan(0)
    for (const u of detail.urls) {
      expect(u, 'content_url must not stay live-shaped').not.toMatch(
        /\/api\/v1\/issues\/.+\/attachments\/.+\/content/,
      )
      expect(u).toMatch(/\/attachments\/\d+$/)
    }
  })
})
