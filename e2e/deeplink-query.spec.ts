/*
 * Querystring-shaped deep links (GDK-164).
 *
 * External tools (Raycast, Slack, a pasted doc) produce `/?issue=KEY` and
 * `/?sc=done` — no `#/`. The app is a hash router, so those used to boot the
 * default view and leave the param sitting in location.search. These pin the
 * one-shot promotion: a param the app gives meaning to moves into the hash
 * and is stripped from the search; an unknown param is left alone.
 */
import { test, expect } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from './helpers'

/** Fixture issue used by keys-focus / detail for the hash form of the same link. */
const ISSUE = 'NMB-110'

test.describe('querystring deep links', () => {
  test('?issue=KEY opens the detail panel and leaves search empty of that key', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto(`/?issue=${ISSUE}`)

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible({ timeout: 30_000 })
    await expect(panel.getByText(ISSUE).first()).toBeVisible()

    const loc = await page.evaluate(() => ({ search: location.search, hash: location.hash }))
    expect(new URLSearchParams(loc.search).has('issue'), `search still has issue: ${loc.search}`).toBe(
      false,
    )
    expect(loc.hash).toMatch(new RegExp(`[?&]issue=${ISSUE}(?:&|$)`))

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('?sc=done applies the filter and promotes sc into the hash', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/?sc=done')

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })

    const loc = await page.evaluate(() => ({ search: location.search, hash: location.hash }))
    expect(new URLSearchParams(loc.search).has('sc'), `search still has sc: ${loc.search}`).toBe(false)
    expect(loc.hash).toMatch(/[?&]sc=done(?:&|$)/)
    expect(loc.hash).not.toMatch(/[?&]sc=new/)

    await expect(
      page.locator(
        '[data-testid="filter-chip"][data-filter-field="status_category"][data-filter-value="done"]',
      ),
    ).toBeVisible()
    await expect(page.getByTestId('list-count')).not.toHaveText('534 issues')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('unknown ?utm_source= stays in search; default view boots', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/?utm_source=x')

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
    // The aside is always mounted; closed is aria-hidden, not absent.
    await expect(page.getByTestId('issue-detail-panel')).toBeHidden()

    const loc = await page.evaluate(() => ({ search: location.search, hash: location.hash }))
    expect(new URLSearchParams(loc.search).get('utm_source')).toBe('x')
    expect(loc.hash).not.toMatch(/utm_source/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
