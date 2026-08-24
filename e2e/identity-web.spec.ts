import { expect, test } from '@playwright/test'
import { gotoApp, searchInput } from './helpers'

/**
 * B-identity-web: C7 / I5 wiring (the app actually calls the functions).
 * I8 is asserted in web/src/stores/pages.test.ts — PageLite carries author_id since 1988d2b.
 * I9 / L1 live in web/src/lib/view-config.test.ts (moved from this file).
 * C5 unit: web/src/lib/config-scope.test.ts.
 * C6 lives in web/src/lib/storage.test.ts (composeCommentDraftKey) — the e2e
 * copy re-asserted that unit's exact key string, so it moved down (GDK-826).
 * C7/I5 stay: a delta upsert actually dropping the open detail and the
 * palette opening an email-less member are wiring, not pure helpers.
 */

test.describe('identity-web e2e', () => {
  test('C7: a delta upsert drops the open issue from the detail cache', async ({ page }) => {
    let nmb110: Record<string, unknown> | null = null
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as { issues: Array<Record<string, unknown>> }
      nmb110 = body.issues.find((it) => it.issue_key === 'NMB-110') ?? null
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)
    const input = searchInput(page)
    await input.fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect
      .poll(() => page.locator('html').getAttribute('data-detail-cache'))
      .toContain('NMB-110')

    expect(nmb110).toBeTruthy()
    let deltaSeen = 0
    await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
      deltaSeen++
      const response = await route.fetch()
      const body = (await response.json()) as {
        upserted: Array<Record<string, unknown>>
        server_time?: string
      }
      body.upserted = [{ ...nmb110, updated_at: new Date().toISOString() }]
      await route.fulfill({ response, json: body })
    })

    const detailGets: string[] = []
    page.on('request', (req) => {
      if (req.url().includes('/NMB-110/detail/')) detailGets.push(req.url())
    })

    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', {
        configurable: true,
        get: () => 'hidden',
      })
      document.dispatchEvent(new Event('visibilitychange'))
      Object.defineProperty(document, 'visibilityState', {
        configurable: true,
        get: () => 'visible',
      })
      document.dispatchEvent(new Event('visibilitychange'))
    })
    await expect.poll(() => deltaSeen).toBeGreaterThan(0)
    // Invalidate + open panel refetch: a new detail GET is the proof. The
    // cache key is allowed to return after that fetch (fresh, not stale).
    await expect.poll(() => detailGets.length).toBeGreaterThan(0)
  })

  test('I5: palette still opens an email-less member by account id', async ({ page }) => {
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        members: Array<{ email: string; name: string; display_name?: string | null; jira_account_id?: string | null }>
        issues: Array<{ assignee_id?: string | null; assignee_email?: string | null; reporter_id?: string | null; reporter_email?: string | null }>
      }
      for (const m of body.members) {
        if (m.jira_account_id === 'demo-alex' || /alex/i.test(m.name) || /alex/i.test(m.display_name ?? '')) {
          m.email = ''
        }
      }
      for (const it of body.issues) {
        if (it.assignee_id === 'demo-alex') it.assignee_email = null
        if (it.reporter_id === 'demo-alex') it.reporter_email = null
      }
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('alex', { delay: 20 })
    await expect(palette.getByTestId('palette-person-row')).toBeVisible()
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('person-panel')).toBeVisible()
    await expect(page.getByTestId('person-name')).toContainText(/alex/i)
  })
})
