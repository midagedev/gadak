import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('client-side search', () => {
  test('narrows the list immediately with zero /api/ traffic while typing', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Wait for the boot payload to land, plus a short settle for the trailing
    // boot requests. Deliberately not networkidle: the app polls for a delta
    // every 15s, so "no network for 500ms" only becomes true after that poll
    // fires, which added 15s to this test for nothing.
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    await page.waitForTimeout(300)

    const apiDuringType: string[] = []
    page.on('request', (req) => {
      const url = req.url()
      if (url.includes('/api/')) apiDuringType.push(url)
    })

    const input = searchInput(page)
    await input.click()
    await input.pressSequentially('NMB-110', { delay: 20 })

    // Count updates as the pool filters locally.
    await expect(page.getByText(/1 issues?|1 issue/)).toBeVisible()
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    // Matched substring is highlighted in the surviving row.
    await expect(page.locator('mark', { hasText: /NMB-110/i }).first()).toBeVisible()

    expect(
      apiDuringType,
      `expected no /api/ requests while typing, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('server search says why it matched', () => {
  test('a comment hit is labelled and shows the matched comment line', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // "reproduced" appears only in comments in the demo mirror, so every hit
    // here is the case that used to look unsupported.
    const input = searchInput(page)
    await input.click()
    await input.fill('reproduced')
    await input.press('Enter')

    await expect(page.getByText(/body matches · "reproduced"/)).toBeVisible()

    const snippet = page.locator('[data-testid="match-snippet"][data-match-field="comment"]').first()
    await expect(snippet).toBeVisible()
    await expect(snippet).toContainText('in a comment')
    // The client highlights the snippet against its own query — the server
    // sends plain text.
    await expect(snippet.locator('mark', { hasText: /reproduced/i }).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a page that matched on its body shows that line too', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.click()
    await input.fill('runbook')
    await input.press('Enter')

    await expect(page.getByTestId('search-docs')).toBeVisible()

    // Body hits carry a line; title hits do not (the highlighted title already
    // says why), so the group holds both kinds and only body rows have one.
    const bodyLine = page
      .getByTestId('search-doc-row')
      .locator('[data-testid="match-snippet"][data-match-field="body"]')
      .first()
    await expect(bodyLine).toBeVisible()
    await expect(bodyLine).not.toContainText('in a comment')
    await expect(bodyLine.locator('mark', { hasText: /runbook/i }).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  // The three faults a vision pass caught on 2026-08-06, each of which the
  // suite was blind to: a fixed height cap slicing a row through its text, the
  // snippet lines stepping right by whatever marker preceded them, and rows
  // sitting in a group with nothing on screen to support the claim.
  // "webhook replay" is deliberately two words — the search matches each word,
  // so it is the query that used to produce reasonless rows.
  for (const q of ['runbook', 'reproduced', 'webhook replay']) {
    test(`every row in a search group is whole, aligned and justified — "${q}"`, async ({
      page,
    }) => {
      const errors = attachConsoleErrors(page)
      await gotoApp(page)

      const input = searchInput(page)
      await input.click()
      await input.fill(q)
      await input.press('Enter')
      await expect(page.getByTestId('search-section-rows').first()).toBeVisible()

      const report = await page.evaluate(() => {
        const clipped: string[] = []
        const reasonless: string[] = []
        const lefts = new Set<number>()
        for (const sc of document.querySelectorAll('[data-testid="search-section-rows"]')) {
          const band = sc.getBoundingClientRect()
          const rows = sc.querySelectorAll('[data-issue-key], [data-testid="search-doc-row"]')
          for (const row of rows) {
            const r = row.getBoundingClientRect()
            // Scrolled out of the group entirely: nothing to judge.
            if (r.bottom <= band.top + 0.5 || r.top >= band.bottom - 0.5) continue
            const id = row.getAttribute('data-issue-key') || (row.textContent || '').trim().slice(0, 30)
            if (r.top < band.top - 0.5 || r.bottom > band.bottom + 0.5) {
              clipped.push(`${id} (${Math.round(r.height)}px row cut by the cap)`)
            }
            const snippet = row.querySelector('[data-testid="match-snippet"]')
            const text = row.querySelector('[data-testid="match-text"]')
            if (text) lefts.add(Math.round(text.getBoundingClientRect().x))
            const titleMarks = [...row.querySelectorAll('mark')].filter(
              (m) => !snippet || !snippet.contains(m),
            ).length
            const snippetMarks = snippet ? snippet.querySelectorAll('mark').length : 0
            if (titleMarks === 0 && snippetMarks === 0) reasonless.push(id)
          }
        }
        return { clipped, reasonless, lefts: [...lefts].sort((a, b) => a - b) }
      })

      expect(report.clipped, 'rows cut through the middle by the group cap').toEqual([])
      expect(report.reasonless, 'rows that show no highlight and no snippet').toEqual([])
      // One left edge for every snippet line, whether it carries a comment
      // marker, came from a body, or sits in the documents group.
      expect(report.lefts.length, `snippet left edges: ${report.lefts.join(', ')}`).toBeLessThan(2)

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  }
})
