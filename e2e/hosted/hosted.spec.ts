import { test, expect, type ConsoleMessage, type Page } from '@playwright/test'
import { DEMO_ISSUE_COUNT_EN_RE } from '../helpers'
import { dismissHostedFirstFrame } from './helpers'

/**
 * Zero-install hosted demo smoke: boot, client-side search, detail, attachment
 * image. Runs against the static snapshot (service worker + JSON), no gadak
 * binary and no Jira account.
 */

const DEMO = '/gadak/'

/** Placeholder copy is not the contract (see e2e/helpers.ts searchInput). */
function searchInput(page: Page) {
  return page.getByTestId('search-input')
}

/**
 * Built-in Epic breakdown: open work grouped by epic — the README's
 * "which epic is stuck?" question. Grouping renders section headers, so the
 * first paint shows at least two epic groups.
 */
async function expectEpicBreakdownLanding(page: Page): Promise<void> {
  // Every view groups by status_category by default, so header presence is
  // not the discriminator — the URL `g` param is: it is only written when
  // group_by differs from that default (view-config GROUP_KEY).
  await expect(page).toHaveURL(/[?&]g=epic(?:&|$)/)
  await expect(page.locator('[data-testid="group-header"]').first()).toBeVisible()
}

/**
 * The demo lands on Epic breakdown. Switch to the All open built-in (same
 * applyConfig path as the sidebar) before a key search — it restores the
 * default status_category grouping, dropping `g=epic` from the URL.
 */
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

function attachConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg: ConsoleMessage) => {
    if (msg.type() === 'error') errors.push(msg.text())
  })
  page.on('pageerror', (err) => errors.push(String(err)))
  return errors
}

test.describe('hosted demo', () => {
  test('first paint opens the Epic breakdown view', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expectEpicBreakdownLanding(page)
  })

  test('boots 534 issues, searches, opens detail with attachment image', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')

    // Service worker registration needs a secure context / localhost — fine on 127.0.0.1.
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 60_000 })
    await expectEpicBreakdownLanding(page)
    await applyAllOpen(page)
    await expect(searchInput(page)).toBeVisible()

    // Client-side (in-memory) search — no server FTS on the hosted snapshot.
    const input = searchInput(page)
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
    await expect(panel.locator('textarea[placeholder*="Add a comment"]')).toHaveCount(1)

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
    await dismissHostedFirstFrame(page)

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
      if (['GET', 'HEAD'].includes(r.method())) return
      // Opening an issue POSTs local.db visit/search rows (api.ts postVisit).
      // The SW answers 501; the request still fires. Not a Jira write.
      if (/\/history\/(visits|searches)\//.test(r.url())) return
      writes.push(`${r.method()} ${r.url()}`)
    })
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await applyAllOpen(page)
    await searchInput(page).fill('NMB-110')
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

  /*
   * GDK-52 — the snapshot must not advertise verbs it cannot answer.
   *
   * Three surfaces, three tests, on purpose. Playwright stops a test at its
   * first failed assertion, so one test covering all three would report the
   * search regression and say nothing about docs or settings — the next round
   * would then be debugging blind. Split, a red run names the surface.
   */

  test('body search says what it searches, and never asks (GDK-52)', async ({ page }) => {
    await forceLocale(page, 'en')
    const searchRequests: string[] = []
    page.on('request', (r) => {
      if (/\/api\/v1\/issues\/search/.test(r.url())) searchRequests.push(`${r.method()} ${r.url()}`)
    })
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await applyAllOpen(page)

    // The snapshot has no server FTS (ADR 0004 addendum) and the network is
    // fine — so "check the connection" would be a lie. The page must say what
    // is true (titles/keys only) and must not even send the request.
    await searchInput(page).fill('performance')
    await searchInput(page).press('Enter')
    await expect(page.getByText('This snapshot searches titles and keys only.')).toBeVisible()
    await expect(page.getByText('Check the connection and try again')).toHaveCount(0)
    expect(searchRequests, `search requests:\n${searchRequests.join('\n')}`).toEqual([])
  })

  test('the docs section offers no errand it cannot serve (GDK-52)', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })

    // Unavailable is not unconfigured: the snapshot carries issues only, and
    // sending the visitor to Settings for it is an errand with no destination.
    const cta = page.locator('[data-testid="docs-empty-cta"][data-state="unavailable"]')
    await expect(cta).toBeVisible()
    await expect(cta).toContainText('This snapshot carries issues only.')
    await expect(page.getByText('Turn on Confluence in Settings')).toHaveCount(0)
  })

  test('no path opens the server settings dialog (GDK-52)', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.goto(DEMO)
    await dismissHostedFirstFrame(page)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })

    // The dialog edits a live server's config, and on the snapshot its own
    // load (settings/ → 404) is an error screen. The docs CTA is the path that
    // used to go there, so it is the one worth clicking.
    await page.locator('[data-testid="docs-empty-cta"]').click()
    await expect(page.getByTestId('settings-dialog')).toHaveCount(0)

    // The footer entry point is absent altogether — not disabled, absent: the
    // errand this dialog serves does not exist on a static snapshot.
    await expect(page.locator('button[title^="Server settings"]')).toHaveCount(0)
  })
})
