import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { test, expect, type Page, type Route } from '@playwright/test'
import { KEYS_CAP } from '../web/src/lib/view-config'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * The capture for the lead's vision review is env-gated (v0.21 release
 * audit, capture-hygiene finding): it used to write scratch/session-strip.png
 * on every run, so CI paid for a capture nobody looked at and the scratch
 * tree filled on behavior-only runs. Set SESSION_STRIP_SHOT_DIR to take it.
 */
async function shootSessionStrip(page: Page): Promise<void> {
  const dir = process.env.SESSION_STRIP_SHOT_DIR
  if (!dir) return
  mkdirSync(dir, { recursive: true })
  await page.screenshot({ path: join(dir, 'session-strip.png') })
}

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
 *   C9 (f) GDK-1537: the boundary rides X-Gadak-Session-Boundary, so a
 *      returning tab — warm IndexedDB, delta path, no bootstrap body to read
 *      — still hears it. See the case's own comment for the measurement.
 *   RL (e) re-latch (research F #24, 2026-09-07): a tab hidden longer than
 *      the session gap comes back to a new session — the strip speaks once
 *      more, about what changed while it was hidden. FAIL-first: the
 *      pre-change component had no visibilitychange listener; (e) timed out
 *      waiting for a strip that never appeared.
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
const DELTA_ROUTE = '**/api/v1/issues/delta/**'
const AUTH_ME_ROUTE = '**/api/v1/auth/me/**'
/** Lower-case: Playwright's Route.headers() keys are normalized. */
const SESSION_BOUNDARY_HEADER = 'x-gadak-session-boundary'

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
    await shootSessionStrip(page)

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

  test('(e) hidden longer than the session gap, the strip re-latches on return', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // No boundary at load — the strip is absent, as in (b).
    const served = await mockLastSession(page, (boot) => {
      const { last_session_ended_at: _l, ...rest } = boot
      return rest
    })
    // Delta passthrough with one injected change once armed: an issue whose
    // updated_at moved to "now" — what arrives while a tab is hidden. The
    // real server still answers; only the upserted list and server_time are
    // touched, and only for the one poll that follows the return.
    let inject: IssueRow | null = null
    await page.route(
      (url) => url.pathname.includes('/delta/'),
      async (route) => {
        const response = await route.fetch()
        const body = (await response.json()) as { server_time: string; upserted?: IssueRow[] } & Record<
          string,
          unknown
        >
        if (inject) {
          body.upserted = [...(body.upserted ?? []), inject]
          body.server_time = new Date().toISOString()
          inject = null
        }
        await route.fulfill({ response, json: body })
      },
    )
    await gotoApp(page)
    await expect(page.getByTestId('list-count')).toBeVisible()
    await expect(page.getByTestId('session-strip')).toHaveCount(0)

    // Hide the tab. visibilityState is a getter on Document.prototype; an own
    // property on the instance shadows it, and the component reads it on the
    // event itself.
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' })
      document.dispatchEvent(new Event('visibilitychange'))
    })
    // Overnight: the page's clock jumps past the session gap (30m).
    await page.evaluate((ms) => {
      const real = Date.now
      Date.now = () => real() + ms
    }, 31 * 60 * 1000)
    // Something changed while hidden — stamped after the hidden moment.
    const victim = (served().issues ?? [])[0]
    expect(victim, 'fixture must have an issue to change').toBeTruthy()
    inject = { ...victim, updated_at: new Date(Date.now() + 60_000).toISOString() }

    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    const strip = page.getByTestId('session-strip')
    await expect(strip).toBeVisible()
    await expect(strip).toContainText('1 issue changed')
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

  /*
   * (f) GDK-1537 — the returning visitor.
   *
   * Every case above opens a fresh context, so the app bootstraps and the
   * boundary arrives in the body. That is the *first* visit. On the second
   * one, IndexedDB already holds the pool and the cursor, so the store takes
   * the delta path (stores/issues.svelte.ts #sync: pool.size > 0 && lastSync)
   * — and if the mirror has not moved, a bootstrap would answer 304 with no
   * body anyway. Either way the body was the wrong seat, and the strip went
   * missing on the one morning nothing had changed.
   *
   * The boundary now rides X-Gadak-Session-Boundary on both endpoints and
   * both status codes. This case is the end-to-end proof: load once with no
   * boundary anywhere (so the cache is warm and the strip silent), then load
   * again with only the header, and the strip must speak.
   *
   * FAIL-first: on the pre-change tree the second load ends with 0 strips —
   * neither api.getBootstrap nor api.getDelta read a header.
   */
  test('(f) a returning tab with a warm cache still learns the boundary — header, not body', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const since = thirtyDaysAgo()

    // ① First visit: no boundary in the body, none in the header. The pool
    //    lands in IndexedDB; the strip stays silent (case (b)'s contract).
    const strip1 = async (route: Route): Promise<void> => {
      const response = await route.fetch()
      const headers = { ...response.headers() }
      delete headers[SESSION_BOUNDARY_HEADER]
      if (response.status() !== 200) {
        await route.fulfill({ response, headers })
        return
      }
      const { last_session_ended_at: _l, ...rest } = (await response.json()) as BootBody
      await route.fulfill({ response, headers, json: rest })
    }
    const stripHeaderOnly = async (route: Route): Promise<void> => {
      const response = await route.fetch()
      const headers = { ...response.headers() }
      delete headers[SESSION_BOUNDARY_HEADER]
      await route.fulfill({ response, headers })
    }
    await page.route(BOOTSTRAP_ROUTE, strip1)
    await page.route(DELTA_ROUTE, stripHeaderOnly)
    await gotoApp(page)
    await expect(page.getByTestId('list-count')).toBeVisible()
    await expect(page.getByTestId('session-strip')).toHaveCount(0)

    // ② Second visit, same context: the cache is warm. The boundary exists
    //    only as a header now — the body is stripped on both endpoints, so
    //    nothing but the header can put the strip on screen.
    await page.unroute(BOOTSTRAP_ROUTE, strip1)
    await page.unroute(DELTA_ROUTE, stripHeaderOnly)
    let bootstrapHits = 0
    let deltaHits = 0
    const inject =
      (count: () => void) =>
      async (route: Route): Promise<void> => {
        count()
        const response = await route.fetch()
        const headers = { ...response.headers(), [SESSION_BOUNDARY_HEADER]: since }
        if (response.status() !== 200) {
          await route.fulfill({ response, headers })
          return
        }
        const { last_session_ended_at: _l, ...rest } = (await response.json()) as BootBody
        await route.fulfill({ response, headers, json: rest })
      }
    await page.route(BOOTSTRAP_ROUTE, inject(() => bootstrapHits++))
    await page.route(DELTA_ROUTE, inject(() => deltaHits++))

    await gotoApp(page)

    const strip = page.getByTestId('session-strip')
    await expect(strip, 'the returning tab must still hear the boundary').toBeVisible()
    await expect(strip).toContainText('Since last session')
    // The measurement this case exists for: a warm start reaches the server
    // through delta, which is why the header rides that response too.
    expect(deltaHits, `bootstrap hits ${bootstrapHits}, delta hits ${deltaHits}`).toBeGreaterThan(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
