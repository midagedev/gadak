import { expect, test } from '@playwright/test'
import { forceLocale, gotoApp, searchInput } from './helpers'
import { composeCacheScope } from '../web/src/lib/config'
import { composeCommentDraftKey } from '../web/src/lib/storage'
import { matchesIdFirst, prioritySortRank } from '../web/src/lib/view-config'

/**
 * B-identity-web: I9 / L1 / I5 / C5 / C6 / C7.
 * I8 is asserted in tail-audit.spec.ts — PageLite carries author_id since 1988d2b.
 */

test.describe('identity-web unit (no browser state)', () => {
  test('C5: distinct sites never share a cache scope', () => {
    const a = composeCacheScope('', 'https://a.example.com')
    const b = composeCacheScope('', 'https://b.example.com')
    expect(a).toBe('site:a.example.com')
    expect(b).toBe('site:b.example.com')
    expect(a).not.toBe(b)
  })

  test('C5: workspace + site both appear, empty site stays workspace-only', () => {
    expect(composeCacheScope('work', 'https://a.example.com')).toBe('ws:work|site:a.example.com')
    expect(composeCacheScope('work', '')).toBe('ws:work')
    expect(composeCacheScope('', '')).toBe('')
  })

  test('C6: draft keys split by workspace and by site', () => {
    const a = composeCommentDraftKey('', 'https://a.example.com', 'NMB-1')
    const b = composeCommentDraftKey('', 'https://b.example.com', 'NMB-1')
    const ws = composeCommentDraftKey('work', 'https://a.example.com', 'NMB-1')
    expect(a).toBe('gadak:comment-draft:site:a.example.com:NMB-1')
    expect(b).toBe('gadak:comment-draft:site:b.example.com:NMB-1')
    expect(ws).toBe('gadak:comment-draft:ws:work|site:a.example.com:NMB-1')
    expect(a).not.toBe(b)
    expect(a).not.toBe(ws)
  })

  test('I9: id wins; name is fallback; missing id still matches name', () => {
    expect(matchesIdFirst(['3'], '3', '진행 중')).toBe(true)
    expect(matchesIdFirst(['In Progress'], '3', '진행 중')).toBe(false)
    expect(matchesIdFirst(['진행 중'], '3', '진행 중')).toBe(true)
    expect(matchesIdFirst(['In Progress'], '', 'In Progress')).toBe(true)
    expect(matchesIdFirst(['In Progress'], undefined, 'In Progress')).toBe(true)
    expect(matchesIdFirst([], '3', '진행 중')).toBe(true)
  })

  test('L1: rank 0 (unset) sorts below Highest (1)', () => {
    expect(prioritySortRank(0)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(null)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(undefined)).toBe(Number.POSITIVE_INFINITY)
    expect(prioritySortRank(1)).toBe(1)
    expect(prioritySortRank(1)).toBeLessThan(prioritySortRank(0))
  })
})

test.describe('identity-web e2e', () => {
  test('I9: status_id filter matches when the display name is localized', async ({ page }) => {
    let statusId = ''
    let matchCount = 0
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        issues: Array<{ status: string; status_id?: string }>
      }
      const sample = body.issues.find((it) => it.status_id && it.status)
      expect(sample?.status_id, 'fixture must carry status_id on the wire').toBeTruthy()
      statusId = sample!.status_id!
      for (const it of body.issues) {
        if (it.status_id === statusId) {
          it.status = '로캘-상태명'
          matchCount++
        }
      }
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)
    await page.goto(`/#/?st=${encodeURIComponent(statusId)}&g=none`)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(new RegExp(`${matchCount} issues?`)).first()).toBeVisible()
  })

  test('I9: name fallback still matches a row with no status_id', async ({ page }) => {
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        issues: Array<{ issue_key: string; status: string; status_id?: string }>
      }
      const row = body.issues[0]
      delete row.status_id
      row.status = 'LegacyNameOnly'
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)
    await page.goto('/#/?st=LegacyNameOnly&g=none')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/1 issue/).first()).toBeVisible()
  })

  test('L1: unset rank 0 sorts after Highest when sorting by priority asc', async ({ page }) => {
    let unsetKey = ''
    let highKey = ''
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as {
        issues: Array<{ issue_key: string; priority_rank: number | null; priority: string | null }>
      }
      const unset = { ...body.issues[0], priority_rank: 0, priority: null }
      const high = { ...body.issues[1], priority_rank: 1, priority: 'Highest' }
      unsetKey = unset.issue_key
      highKey = high.issue_key
      body.issues = [unset, high]
      await route.fulfill({ response, json: body })
    })
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await page.goto('/#/?s=priority&d=asc&g=none')
    await expect(page.getByText(/2 issues/).first()).toBeVisible({ timeout: 30_000 })
    const order = await page.evaluate((keys: [string, string]) => {
      const rows = [...document.querySelectorAll<HTMLElement>('[data-issue-key]')]
      const pos = (k: string) => rows.findIndex((r) => r.dataset.issueKey === k)
      return { unset: pos(keys[0]), high: pos(keys[1]) }
    }, [unsetKey, highKey] as [string, string])
    expect(order.high, 'Highest row must be in the windowed list').toBeGreaterThanOrEqual(0)
    expect(order.unset, 'unset row must be in the windowed list').toBeGreaterThanOrEqual(0)
    expect(order.high).toBeLessThan(order.unset)
  })

  test('C5: documentElement exposes a site-partitioned cache scope', async ({ page }) => {
    await gotoApp(page)
    await expect(page.locator('html')).toHaveAttribute('data-cache-scope', 'site:nimbus.example.com')
  })

  test('C6: comment draft key includes the site partition', async ({ page }) => {
    await gotoApp(page)
    const input = searchInput(page)
    await input.fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()
    const composer = page.getByTestId('comment-composer')
    await expect(composer).toBeVisible()
    await composer.fill('draft-scope-probe')
    await expect
      .poll(async () =>
        page.evaluate(() => {
          const keys: string[] = []
          for (let i = 0; i < localStorage.length; i++) {
            const k = localStorage.key(i)
            if (k?.includes('comment-draft')) keys.push(k)
          }
          return keys
        }),
      )
      .toEqual(['gadak:comment-draft:site:nimbus.example.com:NMB-110'])
  })

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
