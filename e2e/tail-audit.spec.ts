import { expect, test } from '@playwright/test'
import { gotoApp } from './helpers'

/**
 * B-tail: C5 profile in cache scope (html attribute + config.json).
 * I8: web/src/stores/pages.test.ts.
 * Priority id-first: web/src/lib/view-config.test.ts (moved from this file).
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
})
