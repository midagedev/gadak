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

    // Read-only: write UI (comment composer) must stay gated — no credential.
    await expect(panel.locator('textarea[placeholder*="Write a comment"]')).toHaveCount(0)

    // Filter out noisy SW / favicon misses; fail on real app errors.
    const serious = errors.filter(
      (e) =>
        !/favicon|Download the React DevTools|service.?worker/i.test(e) &&
        !/Failed to load resource/i.test(e),
    )
    expect(serious, `console errors:\n${serious.join('\n')}`).toEqual([])
  })
})
