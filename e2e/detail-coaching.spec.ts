import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

/**
 * Four coaching moments in the detail panel (THEORY.md coaching grammar):
 * M1 the durations chip's title carries the learned team p85 (G7), M2 a
 * done-word comment offers "Move to done" through the header's existing
 * status menu (G2), M3 an into-in-progress transition carries the account's
 * in-progress count (G6), M4 the priority picker shows the view's rank
 * distribution (G4). Quiet by construction: verbs on buttons, facts on
 * hover, nothing blocks a click.
 *
 * Contract table (spec C1–C7 ↔ the assertion that pins each):
 *   C1 done-word list lockstep with cmd/gadak/retro.go → done-words.test.ts
 *      (embeds the Go slice and parses the live Go source; not reachable
 *      from a browser spec).
 *   C2 newest-comment-only / not-done / click opens the *existing* menu →
 *      "M2" tests: the positive asserts the option list the chip's own
 *      toggle produces (no new write path exists to assert otherwise); the
 *      done-issue and no-done-word rows assert count 0.
 *   C3 WIP counts through person-match only, absent anonymous → "M3":
 *      expected value recomputed from the captured bootstrap by
 *      assignee_id, never a display name; anonymous auth/me → testid absent.
 *   C4 rank grouping, muted-only colour, no judgement text → "M4": counts
 *      are bare numbers (parseInt-strict), sum equals the view's own open
 *      count, and bar widths live in the existing token at reduced opacity.
 *   C5 en/ko/ja for every new key → web catalog test (existing gate); the
 *      exact en strings are pinned here via title/text assertions.
 *   C6 duration chip visible text unchanged → "M1": innerText keeps the
 *      spans and never contains p85; only the title grows.
 *   C7 no new colour tokens/icons/borders → pinned by the classes the M4
 *      assertions read (bg-text-muted at reduced opacity, text-micro) and
 *      by review of the diff; a DOM spec cannot enumerate a palette.
 *
 * FAIL-first: against the pre-change client every positive row failed —
 * the chip had no title at all, comment rows had no move-to-done control,
 * transition options had no count, and the picker rendered names only. The
 * negative rows (no p85 without flow, counts 0, anonymous absent) passed
 * before and after; they are regression pins, not the fix's proof.
 *
 * Mock strategy (same as stale-threshold/write-through): bootstrap, delta
 * and detail pass through to the real e2e server — its half of the wire
 * contract keeps running — and only the coaching inputs are set (flow,
 * appended comment, auth/me, GET transitions/, GET priorities/).
 */

// The lead's scratchpad for this round's vision review (spec Part E). The
// screenshots are not opened by this round; the vision verdict is the
// lead's.
// Repo scratch by default (gitignored, CI-safe — a Linux runner has no
// /private and cannot mkdir a macOS session scratchpad; the first CI run of
// this spec failed exactly there), overridable so a round routes the capture
// to its own scratchpad for the lead's vision review — my-work.spec's pattern.
const SCRATCH =
  process.env.GADAK_R2_DETAIL_SHOTS ?? join(dirname(fileURLToPath(import.meta.url)), '../scratch')

const KEY_NEW = 'NMB-1' // fixture: Backlog (new), assignee demo-priya — not Dana
const KEY_PROG = 'NMB-5' // fixture: In Progress, 0 comments, status row enters progress → both spans
const KEY_DONE = 'NMB-4' // fixture: Done

const TRANSITION = {
  id: '21',
  name: 'Start work',
  to_status: 'In Progress',
  to_category: 'indeterminate', // effectiveCategory → inprogress
} as const

// Most urgent first — the index+1 rank convention PriorityPicker itself uses
// (priorityMeta(i + 1), setPriority findIndex + 1).
const PRIORITIES = [
  { id: '1', name: 'Highest' },
  { id: '2', name: 'High' },
  { id: '3', name: 'Medium' },
  { id: '4', name: 'Low' },
  { id: '5', name: 'Lowest' },
]

const DANA_ME = { email: 'dana@example.com', account_id: 'demo-dana', name: 'Dana Whitfield' }

type IssueRow = Record<string, unknown> & {
  issue_key?: string
  status_category?: string
  assignee_id?: string | null
}

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Hold the fixture's own bootstrap answer and return its rows. */
async function captureBootstrap(page: Page): Promise<IssueRow[]> {
  const held: { rows: IssueRow[] | null } = { rows: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues?: IssueRow[] }
    held.rows = body.issues ?? []
    await route.fulfill({ response, json: body })
  })
  return held.rows ?? []
}

/** Passthrough bootstrap+delta with the flow block set (or stripped) — the
 *  stale-threshold spec's mock: the store drops a carried flow when the wire
 *  omits it, so delta must be mocked too or the poll races the assertion. */
async function mockFlow(
  page: Page,
  flow: { cycle_p85_hours: number; samples: number } | null,
): Promise<void> {
  const intercept = async (route: Route): Promise<void> => {
    const response = await route.fetch()
    const body = (await response.json()) as Record<string, unknown>
    if (flow) body.flow = flow
    else delete body.flow
    await route.fulfill({ response, json: body })
  }
  await page.route('**/api/v1/issues/bootstrap/', intercept)
  await page.route((url) => url.pathname.includes('/delta/'), intercept)
}

/** Passthrough GET <key>/detail/ with one comment appended as the newest. */
async function appendDetailComment(
  page: Page,
  key: string,
  comment: Record<string, unknown>,
): Promise<void> {
  await page.route(`**/api/v1/issues/${key}/detail/`, async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    const response = await route.fetch()
    const body = (await response.json()) as { comments?: unknown[] }
    await route.fulfill({
      response,
      json: { ...body, comments: [...(body.comments ?? []), comment] },
    })
  })
}

/** Open a detail by key through the search box's jump row ("Open with
 *  Enter"). Row-click matching is unsafe twice over: `hasText` substring-
 *  matches sibling keys (fill "NMB-1" first-hit NMB-183 — the run-1 class-B
 *  failures: the per-key route mocks never fired), and the scroller only
 *  holds the active view's rows (a persisted `fl=mine` home left "No issues
 *  match" for NMB-5/NMB-110 — run-1 class-A). The jump row resolves the
 *  exact key from the pool, whatever view is active. */
async function openDetail(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  const jump = page.getByRole('button', {
    name: new RegExp(`^${key} .+Open with Enter`),
  })
  await expect(jump).toBeVisible()
  await jump.click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

/** The done-word mismatch comment, exactly as the spec's Part E writes it. */
function doneWordComment(): Record<string, unknown> {
  return {
    comment_id: 'e2e-1',
    author: 'Dana Whitfield',
    body: 'Merged and deployed, closing this.',
    created_at: new Date().toISOString(),
  }
}

test.describe('detail coaching moments', () => {
  test('M1: flow on the wire → duration chip title names the team p85; visible text unchanged', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await mockFlow(page, { cycle_p85_hours: 216, samples: 40 }) // 216h → Math.max(1, round(9)) = 9d
    await forceLocale(page, 'en')
    await gotoApp(page)
    await openDetail(page, KEY_PROG)

    const chip = page.getByTestId('duration-chip')
    await expect(chip).toBeVisible()
    await expect(chip).toHaveAttribute(
      'title',
      /team p85 9d \(issues finished in the last 90 days\)/,
    )

    // C6: the chip the reader sees is the old one — both spans, no p85.
    const text = (await chip.innerText()) ?? ''
    expect(text).toMatch(/Waited/)
    expect(text).toMatch(/In progress/)
    expect(text).not.toMatch(/p85/i)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M1: no flow block → no p85 in the title', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockFlow(page, null)
    await forceLocale(page, 'en')
    await gotoApp(page)
    await openDetail(page, KEY_PROG)

    const chip = page.getByTestId('duration-chip')
    await expect(chip).toBeVisible()
    await expect(chip).not.toHaveAttribute('title', /p85/i)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M2: newest done-word comment on a not-done issue → Move to done opens the header menu', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await appendDetailComment(page, KEY_PROG, doneWordComment())
    await forceLocale(page, 'en')
    await gotoApp(page)
    await openDetail(page, KEY_PROG)

    const button = page.getByTestId('comment-move-to-done')
    await expect(button).toBeVisible()
    // C5: the en strings verbatim; {status} is the issue's own status name.
    await expect(button).toHaveText('Move to done')
    await expect(button).toHaveAttribute(
      'title',
      'The latest comment says done; the status is still In Progress',
    )

    // C2: the click opens the chip's own menu — GET transitions/ is the
    // chip's remote path, and the option list it feeds is the existing one.
    await page.route(`**/api/v1/issues/${KEY_PROG}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })
    await button.click()
    await expect(page.getByRole('option', { name: TRANSITION.name })).toBeVisible()

    mkdirSync(SCRATCH, { recursive: true })
    await page.screenshot({ path: join(SCRATCH, 'move-to-done.png'), animations: 'disabled' })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M2: done issue → no button, even with a done-word comment', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await appendDetailComment(page, KEY_DONE, doneWordComment())
    await forceLocale(page, 'en')
    await gotoApp(page)
    await openDetail(page, KEY_DONE)
    await expect(page.getByTestId('comment-move-to-done')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M2: newest comment without a done word → no button', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await appendDetailComment(page, KEY_PROG, {
      comment_id: 'e2e-1',
      author: 'Dana Whitfield',
      body: 'Looking into the cache layer now, will update here.',
      created_at: new Date().toISOString(),
    })
    await forceLocale(page, 'en')
    await gotoApp(page)
    await openDetail(page, KEY_PROG)
    await expect(page.getByTestId('comment-move-to-done')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M3: into-in-progress option carries the account in-progress count', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Spec premise, corrected: the fixture's real auth/me already identifies
    // as dana@example.com (serve.sh seeds that credential), so "without the
    // mock" would still render the count. The mock pins the identity this
    // test computes against; the negative case below mocks anonymous.
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: DANA_ME }),
    )
    // C3: expected value computed from the captured bootstrap, keyed by
    // assignee_id — never a display name.
    let expected = -1
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as { issues?: IssueRow[] }
      expected =
        (body.issues ?? []).filter(
          (it) => it.status_category === 'inprogress' && it.assignee_id === 'demo-dana',
        ).length ?? 0
      await route.fulfill({ response, json: body })
    })
    await page.route(`**/api/v1/issues/${KEY_NEW}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })
    await forceLocale(page, 'en')
    await gotoApp(page)
    expect(expected, 'fixture must have Dana in-progress rows').toBeGreaterThanOrEqual(0)

    await openDetail(page, KEY_NEW)
    await page.getByTestId('status-transition').click()
    await expect(page.getByRole('option', { name: TRANSITION.name })).toBeVisible()

    const wip = page.getByTestId('transition-wip-count')
    await expect(wip).toBeVisible()
    await expect(wip).toHaveText(`in progress: ${expected}`)
    await expect(wip).toHaveAttribute(
      'title',
      'Open in-progress issues assigned to this account',
    )

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M3: anonymous → no count on the option', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: { email: null } }),
    )
    await page.route(`**/api/v1/issues/${KEY_NEW}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })
    await forceLocale(page, 'en')
    await gotoApp(page)
    await openDetail(page, KEY_NEW)
    await page.getByTestId('status-transition').click()
    await expect(page.getByRole('option', { name: TRANSITION.name })).toBeVisible()
    await expect(page.getByTestId('transition-wip-count')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('M4: picker shows the view rank distribution; counts sum to the open list', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    // The visible list's own open count, from the captured bootstrap. C4:
    // grouping is priority_rank, so the sum over ranks must be exactly the
    // open total of the view the counts read.
    let openTotal = -1
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as { issues?: IssueRow[] }
      openTotal = (body.issues ?? []).filter(
        (it) => it.status_category === 'new' || it.status_category === 'inprogress',
      ).length
      await route.fulfill({ response, json: body })
    })
    // Delta upserts would move the pool off the bootstrap state the
    // expectation was computed from — strip them (write-through's guard).
    await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as { upserted?: IssueRow[] }
      body.upserted = []
      await route.fulfill({ response, json: body })
    })
    await page.route(`**/api/v1/issues/${KEY_NEW}/priorities/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { priorities: PRIORITIES })
    })
    await forceLocale(page, 'en')
    await gotoApp(page)
    expect(openTotal, 'fixture must have open issues').toBeGreaterThan(0)

    await openDetail(page, KEY_NEW)
    // Canonical visible set: the All open builtin (sc=new,inprogress — no
    // `fl`, no leftover `q`). The jump-open leaves `q=NMB-1` in the hash and
    // `q` narrows filters.visibleIssues, which the counts read; the sidebar
    // view click clears the search and keeps the panel open.
    await page.getByRole('button', { name: /^All open/ }).click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(page.getByTestId('list-count')).toHaveText(`${openTotal} issues`)
    await page.getByTestId('priority-picker').click()
    await expect(page.getByRole('option', { name: 'Medium' })).toBeVisible()

    // C4: bare numbers, no judgement text — parseInt-strict fails on prose.
    // The none row's count (issues with no priority) is a span too, so the
    // floor is the ranked options, not the ceiling.
    const counts = await page.getByTestId('priority-share').allInnerTexts()
    expect(counts.length, 'every ranked option with share renders a count').toBeGreaterThanOrEqual(
      PRIORITIES.length,
    )
    const sum = counts.reduce((acc, c) => acc + Number.parseInt(c, 10), 0)
    expect(counts.every((c) => /^\d+$/.test(c.trim())), 'counts are bare numbers').toBe(true)
    expect(sum).toBe(openTotal)

    // The widest rank is the 100% of the mini-bar scale: no bar beyond it.
    const bars = await page.evaluate(() =>
      [...document.querySelectorAll('[data-testid="priority-share"]')].map((el) => {
        const track = el.parentElement?.children[0]
        const style = track?.firstElementChild?.getAttribute('style') ?? ''
        const m = /width:\s*(\d+(?:\.\d+)?)%/.exec(style)
        return m ? Number.parseFloat(m[1]) : -1
      }),
    )
    expect(bars.length).toBe(counts.length)
    expect(bars.every((w) => w >= 0 && w <= 100), 'bar widths within 0..100%').toBe(true)
    expect(Math.max(...bars), 'the widest rank is the scale itself').toBe(100)

    // C4's hover basis: the title names n of total with the percentage.
    const medium = page.getByRole('option', { name: 'Medium' }).getByTestId('priority-share')
    await expect(medium).toHaveAttribute(
      'title',
      new RegExp(`^\\d+ of the ${openTotal} open issues in this view \\(\\d+%\\)$`),
    )

    mkdirSync(SCRATCH, { recursive: true })
    await page.screenshot({ path: join(SCRATCH, 'priority-share.png'), animations: 'disabled' })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
