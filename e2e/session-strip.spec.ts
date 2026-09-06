import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page, type Route } from '@playwright/test'
import { KEYS_CAP } from '../web/src/lib/view-config'
import { attachConsoleErrors, gotoApp } from './helpers'

// Where session-strip.png lands: repo scratch by default (gitignored,
// CI-safe), overridable so a round can route the capture to its own session
// scratchpad for the lead's vision review — the same pattern
// resume-card.spec.ts uses.
const SHOT_DIR = process.env.SESSION_STRIP_SHOT_DIR ?? join(dirname(fileURLToPath(import.meta.url)), '../scratch')
const SHOT = join(SHOT_DIR, 'session-strip.png')

/*
 * Session strip e2e (spec r2-session, Part C). Clause table — the clauses
 * this file owns:
 *
 *   C2 the strip exists exactly when a boundary and a change-set both exist —
 *      (a) shows it, (b) no field → 0 strips, (c) field=now → nothing after
 *      it → 0 strips. The absence is the design: no empty state element.
 *   C3 (a) asserts the en copy's parts — the subject is the issues, and the
 *      counts come from the bootstrap body this test itself mocked, computed
 *      here from that same body (never hand-counted off the fixture).
 *   C4 (a) asserts the chip classes on a native button — elevated bg, micro
 *      secondary text, no border.
 *   C5 (a) clicks and asserts list-count becomes exactly min(n, KEYS_CAP)
 *      issues — the click applies a keys view and nothing else — and the
 *      strip is gone for the tab's life.
 *   G7 (a) asserts the hover title is non-empty (the absolute boundary time).
 *
 * FAIL-first: against the pre-round tree (no SessionStrip mount in ListView)
 * case (a) fails at its first assertion — getByTestId('session-strip') never
 * appears; cases (b)/(c) would wrongly pass (0 strips either way), which is
 * why (a) is the gate-bearing case.
 *
 * The boundary is injected by route-mocking the bootstrap response, not by
 * relying on the server's local.db: other specs in this run open issues and
 * POST real visits, so server-side session state is whatever the run order
 * left behind. (b)/(c) therefore strip/set the field explicitly.
 */

const BOOTSTRAP_ROUTE = '**/api/v1/issues/bootstrap/'
const AUTH_ME_ROUTE = '**/api/v1/auth/me/**'

type IssueRow = Record<string, unknown> & {
  issue_key: string
  updated_at?: string | null
  assignee_id?: string | null
}
type BootBody = { issues?: IssueRow[] } & Record<string, unknown>

/** Intercept the bootstrap GET and rewrite only the session boundary. The
 *  rewritten body is kept (servedBoot) so the test counts what the page
 *  actually received — page.request would bypass this route, and
 *  hand-counting the fixture is exactly what the count assertions exist to
 *  avoid. */
async function mockLastSession(
  page: Page,
  rewrite: (boot: BootBody) => BootBody,
): Promise<() => BootBody> {
  let servedBoot: BootBody | null = null
  const intercept = async (route: Route): Promise<void> => {
    const response = await route.fetch()
    const boot = rewrite((await response.json()) as BootBody)
    servedBoot = boot
    await route.fulfill({ response, json: boot })
  }
  await page.route(BOOTSTRAP_ROUTE, intercept)
  return () => {
    if (!servedBoot) throw new Error('bootstrap route was never hit')
    return servedBoot
  }
}

/** 30 days before now — crosses enough of the fixture's updated_at spread
 *  that (a)'s n is the fixture's own recent-change count, not a constant. */
function thirtyDaysAgo(): string {
  return new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString()
}

/** Issues strictly after the boundary — changedSince's rule, recomputed in
 *  the test from the mocked body (parsed instants, equality is not after). */
function changedAfter(boot: BootBody, since: string): IssueRow[] {
  const sinceMs = Date.parse(since)
  return (boot.issues ?? []).filter(
    (it) => it.updated_at !== null && it.updated_at !== undefined && Date.parse(it.updated_at) > sinceMs,
  )
}

test.describe('session strip', () => {
  test('(a) a boundary with changes shows the strip; click becomes a keys view and the strip retires', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const since = thirtyDaysAgo()
    const served = await mockLastSession(page, (boot) => ({
      ...boot,
      last_session_ended_at: since,
    }))
    await gotoApp(page)

    const strip = page.getByTestId('session-strip')
    await expect(strip).toBeVisible()

    // Counts come from the response this test mocked — recompute, never
    // hard-code the fixture.
    const changed = changedAfter(served(), since)
    const n = changed.length
    expect(n, 'the mocked boundary must cross changes').toBeGreaterThan(0)

    const text = (await strip.textContent()) ?? ''
    expect(text).toContain('Since last session')
    expect(text).toContain(`${n} ${n === 1 ? 'issue' : 'issues'} changed`)

    // C4: the duration-chip's classes on a native button — no border, no
    // icon, no new colour token.
    const cls = (await strip.getAttribute('class')) ?? ''
    expect(cls).toContain('bg-bg-elevated')
    expect(cls).toContain('text-micro')
    expect(cls).toContain('text-text-secondary')
    expect(cls).not.toContain('border')
    // G7: hover states the basis — the absolute time the previous session ended.
    expect((await strip.getAttribute('title')) ?? '').not.toBe('')

    // Capture BEFORE the click: after it the strip is gone by design.
    mkdirSync(SHOT_DIR, { recursive: true })
    await page.screenshot({ path: SHOT })

    // C5: the click applies exactly the changed keys (KEYS_CAP-capped) — no
    // status filter, no query, nothing else.
    await strip.click()
    await expect(page.getByTestId('list-count')).toHaveText(`${Math.min(n, KEYS_CAP)} issues`)
    await expect(strip).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('(b) no boundary field → no strip, no empty state', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockLastSession(page, (boot) => {
      const { last_session_ended_at: _l, ...rest } = boot
      return rest
    })
    await gotoApp(page)

    await expect(page.getByTestId('list-count')).toBeVisible()
    await expect(page.getByTestId('session-strip')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('(c) boundary = now → nothing is after it → no strip', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockLastSession(page, (boot) => ({
      ...boot,
      last_session_ended_at: new Date().toISOString(),
    }))
    await gotoApp(page)

    await expect(page.getByTestId('list-count')).toBeVisible()
    await expect(page.getByTestId('session-strip')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('(d) an identified account sees its own share of the changes', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // person-match's rule is id-first: the mocked identity owns issues by
    // account id, so k comes from assignee_id alone.
    await page.route(AUTH_ME_ROUTE, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          email: 'dana@example.com',
          account_id: 'demo-dana',
          name: 'Dana Whitfield',
        }),
      })
    })
    const since = thirtyDaysAgo()
    const served = await mockLastSession(page, (boot) => ({
      ...boot,
      last_session_ended_at: since,
    }))
    await gotoApp(page)

    const strip = page.getByTestId('session-strip')
    await expect(strip).toBeVisible()

    const k = changedAfter(served(), since).filter((it) => it.assignee_id === 'demo-dana').length
    expect(k, 'the mocked identity must own changed issues').toBeGreaterThan(0)

    const text = (await strip.textContent()) ?? ''
    expect(text).toContain(`${k} of them assigned here`)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
