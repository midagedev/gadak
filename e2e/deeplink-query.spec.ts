/*
 * Querystring-shaped deep links (GDK-164).
 *
 * External tools (Raycast, Slack, a pasted doc) produce `/?issue=KEY` and
 * `/?sc=done` — no `#/`. The app is a hash router, so those used to boot the
 * default view and leave the param sitting in location.search. Hash promotion
 * itself is web/src/lib/promote-search.test.ts; this file keeps the one wiring
 * case: the promoted `issue` actually opens the detail panel.
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
})
