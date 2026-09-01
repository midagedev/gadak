import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * GDK-1090 (2026-08-28 UI audit): opening a detail panel in a session with
 * no origin credential fired `GET <key>/linktypes/` on every open, and the
 * server answered 409 credential_required — ten console errors in the audit
 * session, on a screen that renders fine without the catalog. The boot-time
 * probe already knows the rule (internal/server/server.go handleMe: a local
 * tool with no credential "has no identity yet. 200 keeps the boot-time
 * probe out of the browser console"); the catalog fetch now asks the same
 * question the server asks before it 409s.
 *
 * That question is config.HasAtlassianCredential, carried to the web as
 * `originWritable` — not identity. auth/me answers from cfg.Email, which is
 * empty on a local-origin workspace and on a paired one, both of which accept
 * this request: a client gated on me.identified would silence the catalog
 * exactly there. Hence two tests, and the second is the one that fails if
 * someone reaches for identity again.
 *
 * The assertion is on the wire, not on a console filter: a handled 409 is
 * still a Chromium "Failed to load resource" line, so not sending the
 * request is the only state that clears it.
 */

function watchCatalogRequests(page: Page): string[] {
  const seen: string[] = []
  page.on('request', (req) => {
    if (req.url().includes('/linktypes/')) seen.push(`${req.method()} ${req.url()}`)
  })
  return seen
}

async function openDetails(page: Page, count: number): Promise<void> {
  for (let i = 0; i < count; i++) {
    await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').nth(i).click()
    await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)
  }
}

/** Serve config.json with originWritable forced to `writable`. */
async function serveOriginWritable(page: Page, writable: boolean): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = (await res.json()) as Record<string, unknown>
    await route.fulfill({
      response: res,
      json: { ...doc, originWritable: writable },
    })
  })
}

test.describe('linktypes catalog asks the origin, not the identity (GDK-1090)', () => {
  test('no origin credential: five detail opens send zero catalog requests', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const seen = watchCatalogRequests(page)
    await serveOriginWritable(page, false)
    await gotoApp(page)
    await openDetails(page, 5)
    expect(
      seen,
      `catalog requests sent without any credential (${seen.length}):\n${seen.join('\n')}`,
    ).toEqual([])
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a writable origin with no identity still loads the catalog', async ({ page }) => {
    const seen = watchCatalogRequests(page)
    // The local-origin/paired shape: the origin writes fine, auth/me has no
    // email to answer with. This is the row a me.identified gate gets wrong.
    await serveOriginWritable(page, true)
    await page.route('**/api/v1/auth/me/**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: '{"email":null}',
      })
    })
    await gotoApp(page)
    await openDetails(page, 1)
    await expect
      .poll(() => seen.length, {
        message: 'catalog request never sent on a writable origin',
      })
      .toBeGreaterThan(0)
  })
})
