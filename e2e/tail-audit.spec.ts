import { expect, test } from '@playwright/test'
import { composeCacheScope, profileName } from '../web/src/lib/config'
import { matchesIdFirst } from '../web/src/lib/view-config'
import { gotoApp } from './helpers'

/** Same rule as pageAuthorGroupKey in web/src/stores/pages.svelte.ts. */
function pageAuthorGroupKey(p: { author?: string | null; author_id?: string | null }): string {
  const id = (p.author_id ?? '').trim()
  return id || (p.author ?? '')
}

/**
 * B-tail: I8 author_id grouping, C5 profile in cache scope, priority id-first.
 * The env-dependent `views open` gate is a Go test (cmd/gadak/views_test.go).
 */

test.describe('tail-audit unit (no browser state)', () => {
  test('I8: group key is author_id, then display name', () => {
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: 'acc-1' })).toBe('acc-1')
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: 'acc-2' })).toBe('acc-2')
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: '' })).toBe('Kim')
    expect(pageAuthorGroupKey({ author: 'Kim' })).toBe('Kim')
    expect(pageAuthorGroupKey({ author: '', author_id: '' })).toBe('')
  })

  test('C5: named profile on / is a distinct scope; default is omitted', () => {
    const def = composeCacheScope('', 'https://a.example.com', 'default')
    const named = composeCacheScope('', 'https://a.example.com', 'work')
    expect(def).toBe('site:a.example.com')
    expect(named).toBe('site:a.example.com|profile:work')
    expect(def).not.toBe(named)
    // Workspace already partitions; profile is not double-counted.
    expect(composeCacheScope('work', 'https://a.example.com', 'work')).toBe(
      'ws:work|site:a.example.com',
    )
    expect(profileName('')).toBe('default')
    expect(profileName('default')).toBe('default')
    expect(profileName('work')).toBe('work')
  })

  test('priority: id wins; name is fallback for saved views', () => {
    expect(matchesIdFirst(['1'], '1', 'Highest')).toBe(true)
    expect(matchesIdFirst(['Highest'], '1', 'Highest')).toBe(true)
    expect(matchesIdFirst(['Highest'], '1', '최고')).toBe(false)
    expect(matchesIdFirst(['Highest'], '', 'Highest')).toBe(true)
    expect(matchesIdFirst(['Highest'], undefined, 'Highest')).toBe(true)
  })
})

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
