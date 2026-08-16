import { expect, test } from '@playwright/test'
import { gotoApp } from './helpers'

/**
 * B-tail: I8 author_id grouping, C5 profile in cache scope, priority id-first.
 * Unit cases: web/src/stores/pages.test.ts, config-scope.test.ts, view-config.test.ts.
 * The env-dependent `views open` gate is a Go test (cmd/gadak/views_test.go).
 */

test.describe('tail-audit e2e', () => {
  test('C5: config.json names the profile (default)', async ({ request, page }) => {
    const res = await request.get('/config.json')
    expect(res.ok()).toBeTruthy()
    const doc = (await res.json()) as { profile?: string; jiraBaseUrl?: string }
    expect(doc.profile, 'config.json must carry a stable profile name').toBe('default')
    await gotoApp(page)
    await expect(page.locator('html')).toHaveAttribute(
      'data-cache-scope',
      'site:nimbus.example.com',
    )
  })

  test('priority: id filter matches when the display name is localized', async ({ page }) => {
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        issues: Array<{ priority: string | null; priority_id?: string | null }>
      }
      const sample = body.issues.find((it) => it.priority)
      expect(sample, 'fixture must have a named priority').toBeTruthy()
      sample!.priority_id = 'pri-1'
      sample!.priority = '로캘-우선순위'
      for (const it of body.issues) {
        if (it !== sample) it.priority_id = it.priority_id || 'other'
      }
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)
    await page.goto('/#/?pr=pri-1&g=none')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/1 issue/).first()).toBeVisible()
  })
})
