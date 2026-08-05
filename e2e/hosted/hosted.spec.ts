import { test, expect, type ConsoleMessage, type Page } from '@playwright/test'

/**
 * Zero-install hosted demo smoke: boot, client-side search, detail, attachment
 * image. Runs against the static snapshot (service worker + JSON), no scry
 * binary and no Jira account.
 */

const DEMO = '/scry/'

async function forceLocale(page: Page, locale: 'en' | 'ko' = 'en'): Promise<void> {
  await page.addInitScript((loc) => {
    try {
      if (!localStorage.getItem('scry_locale')) {
        localStorage.setItem('scry_locale', loc)
      }
    } catch {
      /* ignore */
    }
  }, locale)
}

function attachConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg: ConsoleMessage) => {
    if (msg.type() === 'error') errors.push(msg.text())
  })
  page.on('pageerror', (err) => errors.push(String(err)))
  return errors
}

test.describe('hosted demo', () => {
  test('boots 519 issues, searches, opens detail with attachment image', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')

    // Service worker registration needs a secure context / localhost — fine on 127.0.0.1.
    await page.goto(DEMO)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText(/519 issues/).first()).toBeVisible({ timeout: 60_000 })
    await expect(page.getByPlaceholder(/Search issues/)).toBeVisible()

    // Client-side (in-memory) search — no server FTS on the hosted snapshot.
    const input = page.getByPlaceholder(/Search issues/)
    await input.fill('NMB-110')
    await expect(page.getByText(/1 issues?|1 issue/)).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Details' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Description' })).toBeVisible()

    // NMB-110 ships three demo screenshots (examples/attachments). The SW must
    // rewrite content_url onto attachments/{id} so the <img> gets a real PNG.
    const galleryImg = panel.locator('img[src*="/attachments/"]').first()
    await expect(galleryImg).toBeVisible({ timeout: 15_000 })
    await expect
      .poll(async () => galleryImg.evaluate((el: HTMLImageElement) => el.naturalWidth))
      .toBeGreaterThan(0)

    // The composer is offered on the demo (writes apply locally, see the write
    // test below), so its placeholder is the normal one, not the credential nag.
    await expect(panel.locator('textarea[placeholder*="Write a comment"]')).toHaveCount(1)

    // Filter out noisy SW / favicon misses; fail on real app errors.
    const serious = errors.filter(
      (e) =>
        !/favicon|Download the React DevTools|service.?worker/i.test(e) &&
        !/Failed to load resource/i.test(e),
    )
    expect(serious, `console errors:\n${serious.join('\n')}`).toEqual([])
  })

  test('says it is a demo and never offers to take a Jira token', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.goto(DEMO)

    // Without this the page reads as a real Jira client someone left signed in,
    // and the next thing a visitor looks for is where to put their token.
    const banner = page.getByTestId('demo-banner')
    await expect(banner).toBeVisible({ timeout: 30_000 })
    await expect(banner).toContainText('Demo')
    await expect(banner).toContainText('read-only')
    await expect(page).toHaveTitle(/demo/i)

    // The credential dialog asks for a real Atlassian API token. On a static
    // snapshot served from someone else's domain there is nothing it could ever
    // be used for, so no path may lead there.
    await expect(page.getByText(/set credentials/i)).toHaveCount(0)
    await expect(page.locator('input[type="password"]')).toHaveCount(0)

    // Writes answer 501 here, so the entry point is disabled up front rather
    // than failing after the visitor has typed something.
    await expect(page.getByRole('button', { name: /new issue/i }).first()).toBeDisabled()
  })

  test('a comment applies locally, is labelled unsaved, and reaches no server', async ({
    page,
  }) => {
    await forceLocale(page, 'en')
    const writes: string[] = []
    page.on('request', (r) => {
      if (!['GET', 'HEAD'].includes(r.method())) writes.push(`${r.method()} ${r.url()}`)
    })
    await page.goto(DEMO)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await page.getByPlaceholder(/Search issues/).fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    const composer = panel.getByTestId('comment-composer')
    await expect(composer).toBeVisible()
    await composer.fill('Does the write path feel real here?')
    await panel.getByRole('button', { name: /^Comment$/ }).click()

    // The optimistic comment stays put — a demo where writes bounce is not worth
    // trying — but everything around it has to say it went nowhere.
    await expect(panel.getByText('Does the write path feel real here?')).toBeVisible()
    await expect(page.getByTestId('demo-edited-notice')).toBeVisible()
    await expect(page.getByTestId('demo-banner')).toContainText(/local edit/i)

    // The claim on the banner is only true if nothing was actually sent.
    expect(writes, `unexpected writes:\n${writes.join('\n')}`).toEqual([])

    // "Not saved" has to survive the obvious test a visitor would run.
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText('Does the write path feel real here?')).toHaveCount(0)
  })
})
