import { test, expect, type Page } from '@playwright/test'
import {
  DEMO_ISSUE_COUNT,
  appConsoleErrors,
  attachConsoleErrors,
  gotoApp,
} from './helpers'

/*
 * GDK-1170 — a write made somewhere else reaches an open board on the 500ms
 * ui-focus poll, not on the issue store's 15s backstop.
 *
 * The product's claim is that the CLI moves the board. Before this, `gadak
 * claim` was up to fifteen seconds from true on screen: the mirror had already
 * changed and only the window did not know. The server now carries the
 * mirror's disk identity on the poll that already ran twice a second, and the
 * board pulls a delta when it moves.
 *
 * What this spec drives is that signal, not the mirror: the ui-focus response
 * is rewritten so mirrorVersion moves on command, and the delta is rewritten
 * so the pull has something visible to carry. The mirror-side half — that the
 * value moves when and only when the mirror does, WAL included — is pinned in
 * Go (internal/store/version_test.go, internal/server/focus_test.go).
 *
 * FAIL-first: with the client ignoring mirrorVersion, the injected issue
 * appears only on the backstop tick, and this stopped on
 * 'the board must pull within 3s of the mirror moving'.
 */

type IssueRow = Record<string, unknown> & { issue_key: string }

/** Injected key: not in demo.db, so its arrival is unambiguous. */
const INJECTED_KEY = 'NMB-991170'

/** POLL_MS in web/src/stores/issues.svelte.ts. The bar this must clear. */
const BACKSTOP_MS = 15_000
/** The window a pull has to land in to be this signal and not the backstop. */
const INSTANT_MS = 3_000

interface Rig {
  /** Move the mirror identity the poll reports. */
  bump(): void
  /** Start injecting a new issue into every delta response. */
  arm(): void
  /** How many delta requests the page has made. */
  deltas(): number
}

async function installRig(page: Page): Promise<Rig> {
  const state = { version: 'v0', armed: false, deltas: 0, template: null as IssueRow | null }

  // Keep one real issue to clone: the store's row shape is the API's, and a
  // hand-written literal would drift from it silently.
  await page.route('**/api/v1/issues/bootstrap/**', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    state.template = body.issues[0] ?? null
    await route.fulfill({ response, json: body })
  })

  await page.route(
    (url) => url.pathname.includes('/ui-focus/'),
    async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as Record<string, unknown>
      body.mirrorVersion = state.version
      await route.fulfill({ response, json: body })
    },
  )

  await page.route(
    (url) => url.pathname.includes('/delta/'),
    async (route) => {
      state.deltas++
      const response = await route.fetch()
      const body = (await response.json()) as { upserted?: IssueRow[] }
      if (state.armed && state.template) {
        const injected: IssueRow = {
          ...state.template,
          issue_key: INJECTED_KEY,
          key: INJECTED_KEY,
          summary: 'written while the board was open',
        }
        body.upserted = [...(body.upserted ?? []), injected]
      }
      await route.fulfill({ response, json: body })
    },
  )

  return {
    bump: () => {
      state.version = `v${Number(state.version.slice(1)) + 1}`
    },
    arm: () => {
      state.armed = true
    },
    deltas: () => state.deltas,
  }
}

test.describe('GDK-1170 a write elsewhere reaches an open board', () => {
  test('the board pulls when mirrorVersion moves, not on the 15s backstop', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const rig = await installRig(page)
    await gotoApp(page)

    const poolCount = page.getByText(new RegExp(`${DEMO_ISSUE_COUNT} issues`)).first()
    await expect(poolCount).toBeVisible({ timeout: 30_000 })

    // Anchor on a backstop tick. The next one is BACKSTOP_MS away, so a pool
    // that grows within INSTANT_MS of the bump cannot be its doing — and
    // waiting for it also proves the poll this must beat is actually running.
    await page.waitForResponse((r) => r.url().includes('/delta/'), { timeout: 30_000 })
    const anchored = rig.deltas()

    // Control: 500ms ticks with an unmoved mirror must not pull. The duration
    // IS the contract here (absence over ~6 ticks), and it ends well before
    // the next backstop.
    await page.waitForTimeout(INSTANT_MS) // duration is the contract: no pull while the mirror sits still
    expect(rig.deltas(), 'a still mirror must not pull a delta on the 500ms tick').toBe(anchored)

    rig.arm()
    rig.bump()

    // The assertion waits on the state itself: the injected issue is in the
    // pool, which is only true after a delta landed and was applied.
    await expect(
      page.getByText(new RegExp(`${DEMO_ISSUE_COUNT + 1} issues`)).first(),
      'the board must pull within 3s of the mirror moving',
    ).toBeVisible({ timeout: INSTANT_MS })

    expect(rig.deltas(), 'exactly one delta for one move').toBe(anchored + 1)
    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('a mirror that keeps moving pulls once per move, never stacked', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const rig = await installRig(page)
    await gotoApp(page)
    await expect(page.getByText(new RegExp(`${DEMO_ISSUE_COUNT} issues`)).first()).toBeVisible({
      timeout: 30_000,
    })
    await page.waitForResponse((r) => r.url().includes('/delta/'), { timeout: 30_000 })
    const anchored = rig.deltas()

    // Three moves inside one 500ms tick. The tab must not fire three deltas —
    // a poll that stacks requests is a new defect, not a fixed one.
    rig.bump()
    rig.bump()
    rig.bump()

    await expect
      .poll(() => rig.deltas(), { timeout: INSTANT_MS })
      .toBeGreaterThan(anchored)
    // Settle, then count. Still short of the backstop at anchored + 15s.
    await page.waitForTimeout(INSTANT_MS) // duration is the contract: no follow-up pull after the burst settles
    expect(rig.deltas(), 'three moves inside one tick are one pull').toBe(anchored + 1)
    expect(BACKSTOP_MS).toBeGreaterThan(2 * INSTANT_MS)
    expect(appConsoleErrors(errors)).toEqual([])
  })
})
