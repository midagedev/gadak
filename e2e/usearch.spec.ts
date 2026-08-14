import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import {
  createUnifiedSearch,
  excludeLocalKeys,
  isStale,
  projectUnifiedHits,
} from '../web/src/lib/unified-search'

/*
 * Unified search in the command palette (⌘K) plus the visible way in.
 *
 * Fixture tokens (examples/demo.db — do not invent, do not edit the seed):
 *   workaround   — 31 comments / 2 issue bodies; 0 titles, labels, page titles
 *   consequences — 5 page bodies; 0 issue or page titles
 * Local palette matching is key/title/assignee/labels (issues) and
 * title/space (docs), so both tokens are invisible there today.
 */

test.describe('unified search — cancel / project (no browser)', () => {
  test('isStale is true only when the request id is not current', () => {
    expect(isStale(1, 2)).toBe(true)
    expect(isStale(4, 4)).toBe(false)
    expect(isStale(0, 1)).toBe(true)
  })

  test('excludeLocalKeys drops keys the local section already shows', () => {
    expect(excludeLocalKeys(['A', 'B', 'C'], new Set(['B']))).toEqual(['A', 'C'])
    expect(excludeLocalKeys(['A'], new Set())).toEqual(['A'])
  })

  test('projectUnifiedHits dedupes local keys and caps the preview', () => {
    const keys = Array.from({ length: 12 }, (_, i) => `NMB-${i}`)
    const pages = Array.from({ length: 9 }, (_, i) => ({
      key: `p-${i}`,
      title: `Page ${i}`,
      space_key: 'ENG',
      parent_id: null,
    }))
    const res = {
      keys,
      total: 21,
      pages,
      matches: { 'NMB-3': { field: 'body' as const, snippet: 'body hit' } },
    }
    const out = projectUnifiedHits(res, new Set(['NMB-0', 'NMB-1']), new Set(['p-0']))
    expect(out.issues.map((h) => h.key)).toEqual(keys.slice(2, 10))
    expect(out.pages.map((h) => h.key)).toEqual(pages.slice(1, 7).map((p) => p.key))
    expect(out.issues[1]?.match?.field).toBe('body')
    expect(out.truncated).toBe(true)
  })

  test('createUnifiedSearch drops a stale response after a newer query', async () => {
    let finishSlow: (value: { keys: string[]; total: number; pages: []; matches: {} }) => void
    const slow = new Promise<{ keys: string[]; total: number; pages: []; matches: {} }>((resolve) => {
      finishSlow = resolve
    })
    const views: { status: string; query: string }[] = []
    const handle = createUnifiedSearch({
      debounceMs: 1,
      fetch: (q) => {
        if (q === 'old') return slow
        return Promise.resolve({ keys: ['NEW-1'], total: 1, pages: [], matches: {} })
      },
      onView: (v) => views.push({ status: v.status, query: v.query }),
    })
    handle.request('old')
    await new Promise((r) => setTimeout(r, 15))
    handle.request('new')
    await new Promise((r) => setTimeout(r, 15))
    finishSlow!({ keys: ['OLD-1'], total: 1, pages: [], matches: {} })
    await new Promise((r) => setTimeout(r, 15))
    const ready = views.filter((v) => v.status === 'ready')
    expect(ready).toEqual([{ status: 'ready', query: 'new' }])
    handle.cancel()
  })

  test('createUnifiedSearch cancel drops an in-flight response (closed palette)', async () => {
    let finish: (value: { keys: string[]; total: number; pages: []; matches: {} }) => void
    const pending = new Promise<{ keys: string[]; total: number; pages: []; matches: {} }>((resolve) => {
      finish = resolve
    })
    const views: string[] = []
    const handle = createUnifiedSearch({
      debounceMs: 1,
      fetch: () => pending,
      onView: (v) => views.push(v.status),
    })
    handle.request('workaround')
    await new Promise((r) => setTimeout(r, 15))
    handle.cancel()
    finish!({ keys: ['NMB-42'], total: 1, pages: [], matches: {} })
    await new Promise((r) => setTimeout(r, 15))
    expect(views.filter((s) => s === 'ready')).toEqual([])
  })
})

test.describe('unified search — palette + entry', () => {
  test('⌘K finds a comment-only issue under All search, with a snippet', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('workaround', { delay: 15 })

    // Local jump sections cannot see a comment-only token. Rank order is the
    // server's; the claim is a unified row with a comment snippet, not a key.
    const row = palette.getByTestId('palette-unified-issue').first()
    await expect(row).toBeVisible()
    const snippet = row.getByTestId('palette-unified-snippet')
    await expect(snippet).toBeVisible()
    await expect(snippet).toHaveAttribute('data-match-field', 'comment')
    await expect(snippet).toContainText(/workaround/i)

    const sections = await palette
      .getByTestId('palette-section')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-section')))
    expect(sections).toContain('unified')
    // Server section sits below the local jump groups.
    const last = sections.lastIndexOf('unified')
    const issue = sections.indexOf('issue')
    if (issue >= 0) expect(issue).toBeLessThan(last)

    const key = (await row.locator('.font-mono').first().textContent())?.trim() ?? ''
    expect(key).toMatch(/^[A-Z]+-\d+$/)
    await row.click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(page.getByTestId('issue-detail-panel').getByText(key).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('⌘K finds a page that matches only in its body', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('consequences', { delay: 15 })

    // Local doc rows are title/space only — this token is body-only.
    await expect(palette.getByTestId('palette-doc-row')).toHaveCount(0)
    const doc = palette.getByTestId('palette-unified-doc').first()
    await expect(doc).toBeVisible()
    await expect(doc).toContainText('ADR')
    await expect(doc.getByTestId('palette-unified-snippet')).toBeVisible()
    await expect(doc.getByTestId('palette-unified-snippet')).toContainText(/consequences/i)

    await doc.click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('doc-panel')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the top-bar entry opens the palette; SearchBox says it narrows this list', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const box = searchInput(page)
    await expect(box).toHaveAttribute('placeholder', /narrow this list/i)

    await page.getByTestId('palette-open').click()
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await expect(palette.getByTestId('palette-empty-hint')).toBeVisible()
    await expect(palette.getByTestId('palette-empty-hint')).toContainText(/every issue and document/i)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a failed server search is an error, not an empty result', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.route('**/api/v1/issues/search/**', (route) =>
      route.fulfill({ status: 500, body: 'nope' }),
    )

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('workaround', { delay: 15 })

    await expect(palette.getByTestId('palette-unified-error')).toBeVisible()
    await expect(palette.getByTestId('palette-unified-empty')).toHaveCount(0)
    await expect(palette.getByTestId('palette-unified-issue')).toHaveCount(0)

    // The 500 is the case under test; Chromium logs it as a console error.
    expect(
      errors.filter((e) => !e.includes('500')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
