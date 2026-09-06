/*
 * team-flow pack: the "Aging in progress" built-in view (THEORY.md T4/G4 —
 * the steward's age tail, arrangement as the coaching).
 *
 * Contract ↔ assertion table (spec C1–C6; the rows this file can see):
 *  B/C4 the sidebar lists the view by its localized name — getByRole button
 *      "Aging in progress" fails while builtin-views.ts has no such entry
 *      (unit FAIL-first sibling in web/src/lib/builtin-views.test.ts; the
 *      e2e shares that first failure mode, so its FAIL-first is the same
 *      absent-entry red, recorded there).
 *  C2/C4 clicking applies only the tenant-neutral sc=inprogress axis — the
 *      URL carries sc=inprogress and the full row set equals the fixture's
 *      in-progress keys, read from the serve's own bootstrap (the
 *      cross-links.spec.ts idiom: the serve is the single owner of fixture
 *      facts, so a fixture regen fails loudly instead of silently matching).
 *  C2  rows are read from the same DOM the other view specs read:
 *      [data-issue-key] under issue-list-scroller, scroll-accumulated
 *      (the list is virtualised; counting the DOM would count the window).
 *  —   Artifact: aging-view.png for the lead's vision review. This spec
 *      writes it and never judges it.
 */
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type Locator } from '@playwright/test'
import { apiURL, attachConsoleErrors, gotoApp } from './helpers'

const here = dirname(fileURLToPath(import.meta.url))

// Where aging-view.png lands: repo scratch by default (gitignored, CI-safe —
// a Linux runner has no /private), overridable so a round can route the
// capture to its own session scratchpad for the lead's vision review.
const SHOT_DIR = process.env.TEAM_FLOW_SHOT_DIR ?? join(here, '../scratch')
const SHOT = join(SHOT_DIR, 'aging-view.png')

type BootRow = { issue_key: string; status_category?: string }

/**
 * Every [data-issue-key] row of the virtualised issue list, in list order.
 *
 * helpers' walkRows selects by data-testid; issue rows deliberately carry no
 * testid — data-issue-key is their identity — so this is the same
 * scroll-accumulate loop keyed on that attribute instead. Bounded (500
 * steps, 20% overlap) so a list that never reports its end fails loudly.
 */
async function walkIssueKeys(scroller: Locator): Promise<string[]> {
  const seen = new Set<string>()
  await scroller.evaluate((el) => {
    el.scrollTop = 0
  })
  for (let step = 0; step < 500; step++) {
    const batch = await scroller.evaluate((el) => {
      const rows = [...el.querySelectorAll('[data-issue-key]')]
      return {
        keys: rows.map((r) => r.getAttribute('data-issue-key') ?? ''),
        atEnd: el.scrollTop + el.clientHeight >= el.scrollHeight - 1,
      }
    })
    for (const k of batch.keys) if (k) seen.add(k)
    if (batch.atEnd) break
    await scroller.evaluate((el) => {
      el.scrollTop += el.clientHeight * 0.8
    })
    // The virtual list mounts the next window on a later frame than the
    // scrollTop write; wait for the first mounted row to change (or the end
    // of the scroller) instead of a fixed delay that races the paint.
    const prevFirst = batch.keys[0] ?? ''
    await scroller.evaluate(
      (el, prev) =>
        new Promise<void>((resolve) => {
          const tick = () => {
            const first = el.querySelector('[data-issue-key]')?.getAttribute('data-issue-key') ?? ''
            const atEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 1
            if (first !== prev || atEnd) resolve()
            else requestAnimationFrame(tick)
          }
          requestAnimationFrame(tick)
        }),
      prevFirst,
    )
  }
  await scroller.evaluate((el) => {
    el.scrollTop = 0
  })
  return [...seen]
}

test.describe('team-flow: Aging in progress built-in view', () => {
  test('sidebar lists it; clicking shows exactly the in-progress set', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Expected key set from the fixture's own bootstrap — not hardcoded, so
    // a fixture regen renegotiates the set instead of silently drifting.
    const res = await page.request.get(apiURL('/api/v1/issues/bootstrap/'))
    expect(res.ok(), 'bootstrap must answer on the e2e serve').toBe(true)
    const body = (await res.json()) as { issues: BootRow[] }
    const expected = new Set(
      body.issues.filter((r) => r.status_category === 'inprogress').map((r) => r.issue_key),
    )
    expect(expected.size, 'demo fixture must carry in-progress issues').toBeGreaterThan(0)

    // The sidebar lists the view under Built-in views. No `exact`: the row's
    // accessible name also carries its issue count ("Aging in progress 144").
    const button = page.getByRole('button', { name: 'Aging in progress' })
    await expect(button).toBeVisible()
    await button.click()

    // Only the tenant-neutral axis moves: sc=inprogress in the URL.
    await expect(page).toHaveURL(/[#?&]sc=inprogress/, { timeout: 10_000 })

    // The list settles on the expected count before the walk reads it.
    await expect(page.getByTestId('list-count')).toHaveText(
      new RegExp(`^${expected.size} issues$`),
      { timeout: 10_000 },
    )

    const shown = await walkIssueKeys(page.getByTestId('issue-list-scroller'))
    expect(new Set(shown), 'the view shows exactly the in-progress issues').toEqual(expected)

    // Artifact for the lead's vision review — written, never judged here.
    mkdirSync(SHOT_DIR, { recursive: true })
    await page.screenshot({ path: SHOT })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
