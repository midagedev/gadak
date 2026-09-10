/*
 * GDK-1538 end to end: opening an issue on the phone leaves a visit on the
 * serve, and the serve then hands the resume boundary back.
 *
 * The unit test (src/lib/visit.test.ts) measures that the store posts. This
 * measures the half no mock can: that the route, the method and the body
 * the phone sends are the ones this serve actually accepts, and that the
 * detail response for the *second* open carries the read the first one
 * wrote. Before the fix a phone-only workspace could never populate
 * last_visited_at, so the resume card was silent forever.
 *
 * Nothing here is mocked and nothing is written to Jira: history rows live
 * in the serve's own local.db (`gadak demo`'s temp home), which this gate
 * creates and discards.
 */
import { expect, test } from './helpers'
import { SERVE_ORIGIN } from '../playwright.config'

test('the phone records the issues it opens, and the serve reads them back', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('nav.safe-bottom').waitFor()
  const firstRow = page.locator('.pane:not(.off) button.row').first()
  await firstRow.waitFor()

  // Open one issue and learn which key it was from the detail header.
  await firstRow.click()
  await page.locator('button.back').waitFor()
  const key = (await page.locator('.bar-key').first().innerText()).trim()
  expect(key).not.toBe('')

  // The visit is fire-and-forget, so poll the serve rather than racing it.
  await expect
    .poll(
      async () => {
        const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/history/`)
        const doc = (await res.json()) as { items?: Array<{ type?: string; kind?: string; key?: string }> }
        return (doc.items ?? []).some(
          (item) => item.type === 'visit' && item.kind === 'issue' && item.key === key,
        )
      },
      { message: `no issue visit for ${key} on the serve` },
    )
    .toBe(true)

  // And the boundary comes back on the next read of the same issue: the
  // detail response now knows when this phone last looked.
  const detail = (await (
    await fetch(`${SERVE_ORIGIN}/api/v1/issues/${encodeURIComponent(key)}/detail/`)
  ).json()) as { last_visited_at?: string }
  expect(detail.last_visited_at, 'detail carries the read the phone just wrote').toBeTruthy()
})
