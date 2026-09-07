// GDK-1495 / GDK-1497 A4 captures. The phone now draws the 0.21 awareness
// concepts the desk already had: the session strip above the list, the
// work-item age on the row, the resume card above the comments, and the five
// built-in views in the scope sheet. This spec photographs them on the shared
// fixture (`gadak demo` on 7899, vite on 5182 — mobile/playwright.config.ts)
// for the lead's vision round. Nothing here is judged; the captures are.
//
// Measured fixture truth (2026-09-07, GET /api/v1/issues/bootstrap/ and
// /api/v1/auth/me/ on a scratch demo serve), and the three route mocks it
// forces. Each one is a SINGLE field, each is labeled in the log, and none of
// them exercises a write:
//
//  1. `last_session_ended_at` is ABSENT. The demo home has no local.db person
//     reads at all, so the serve has no previous session to report and the
//     strip is correctly silent. The route adds the field — a boundary three
//     days back — so the strip has something true to say about real rows.
//     Everything the strip then counts is the fixture's own `updated_at`
//     (18 rows moved in those three days, measured).
//
//  2. `auth/me` answers `{"email": null}` — the demo serve carries no origin
//     credential, so it knows no identity. Two of the five built-ins are
//     identity views and are therefore ABSENT, not disabled (an anonymous
//     reader has no "mine" — the desk hides them for the same reason), which
//     would photograph three rows under a heading the round is about. The
//     route answers with `demo-alex`, the fixture's own busiest account
//     (72 issues assigned, 198 reported), so both identity views select real
//     rows rather than an empty plate.
//
//  3. `flow` is PRESENT but degenerate: `cycle_p85_hours` is 0.00031 — about
//     one second — because the fixture's 47 finished issues carry a
//     `resolved_at` equal to their `started_at`. The learned threshold beats
//     the default by contract, so on the raw fixture EVERY open row is stale
//     at maximum weight and the band carries no information at all. The route
//     drops the block, which lands the phone on the shared 72h default (the
//     desk's own precedence step 3) and makes the bands legible: 16 open rows
//     then carry no age, 15 quiet, 24 mid, 313 loud. The web has the same
//     reading on the same fixture — this is a fixture defect, not a phone one.
//
//  4. One detail's `last_visited_at`. No app has ever opened an issue on this
//     home, so both visit fields are absent everywhere and no resume card can
//     exist. The route gives the ONE photographed issue a previous read; the
//     history and comments it is diffed against are the fixture's own.
import { expect, test } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { SERVE_ORIGIN } from '../playwright.config'

const SHOT_DIR = join(dirname(fileURLToPath(import.meta.url)), '.shots')

/** The fixture's own busiest account — see note 2. */
const IDENTITY = { email: 'demo@example.com', account_id: 'demo-alex', name: 'Alex Kim' }

/**
 * The issue to photograph for the resume card: the one whose changelog
 * carries the most *kinds* of change, so the card's counting is visible
 * rather than a single part. Chosen dynamically — the pick must survive a
 * fixture regeneration — with the boundary placed inside its changelog so
 * some entries fall after it and some before.
 */
type HistoryEntry = { at: string | null; field: string }
type CommentDoc = { created_at: string | null }
type DetailDoc = { issue_key: string; history?: HistoryEntry[]; comments?: CommentDoc[] }

/** The parts resumeLabel would render for a boundary — the card's own rule,
 *  re-derived here only to *choose* what to photograph. */
function partsAfter(doc: DetailDoc, sinceMs: number): number {
  const h = doc.history ?? []
  const status = h.filter((e) => e.field === 'status' && Date.parse(e.at ?? '') > sinceMs).length
  const assignee = h.some((e) => e.field === 'assignee' && Date.parse(e.at ?? '') > sinceMs)
  const other = h.filter(
    (e) => e.field !== 'status' && e.field !== 'assignee' && Date.parse(e.at ?? '') > sinceMs,
  ).length
  const comments = (doc.comments ?? []).filter((c) => Date.parse(c.created_at ?? '') > sinceMs).length
  return (status > 0 ? 1 : 0) + (assignee ? 1 : 0) + (other > 0 ? 1 : 0) + (comments > 0 ? 1 : 0)
}

async function pickResumeIssue(): Promise<{ key: string; visitedAt: string }> {
  const boot = (await (await fetch(`${SERVE_ORIGIN}/api/v1/issues/bootstrap/`)).json()) as {
    issues: Array<{ issue_key: string }>
  }
  const details = await Promise.all(
    boot.issues.map(async (lite) => {
      const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/${lite.issue_key}/detail/`)
      return (await res.json()) as DetailDoc
    }),
  )
  // Score by how many of the card's four parts a boundary inside the
  // changelog would produce: a card that renders one part photographs the
  // component but not the reading. Ties break on volume, then on key so the
  // pick is stable across runs.
  let best: { key: string; parts: number; volume: number; sinceMs: number } | null = null
  for (const doc of details) {
    const stamps = [
      ...(doc.history ?? []).map((e) => e.at),
      ...(doc.comments ?? []).map((c) => c.created_at),
    ]
      .filter((at): at is string => Boolean(at))
      .map((at) => Date.parse(at))
      .filter((ms) => Number.isFinite(ms))
      .sort((a, b) => a - b)
    if (stamps.length < 4) continue
    // Just before the first third, so most of the log falls after it.
    const sinceMs = stamps[Math.floor(stamps.length / 3)] - 1000
    const parts = partsAfter(doc, sinceMs)
    const volume = stamps.length
    const better =
      !best ||
      parts > best.parts ||
      (parts === best.parts && volume > best.volume) ||
      (parts === best.parts && volume === best.volume && doc.issue_key < best.key)
    if (better) best = { key: doc.issue_key, parts, volume, sinceMs }
  }
  if (!best) throw new Error('no issue on the fixture carries a changelog to diff')
  const visitedAt = new Date(best.sinceMs).toISOString()
  console.log(
    `[a4] resume issue ${best.key} — ${best.volume} timeline entries, ${best.parts} of the card's` +
      ` four parts fall after the mocked last_visited_at ${visitedAt}`,
  )
  return { key: best.key, visitedAt }
}

test('captures the A4 awareness surfaces for the vision round', async ({ page }) => {
  mkdirSync(SHOT_DIR, { recursive: true })
  const resume = await pickResumeIssue()
  const boundary = new Date(Date.now() - 3 * 24 * 3600 * 1000).toISOString()

  // (1) + (3): one field added, one field dropped. Everything else passes
  // through the serve untouched.
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const res = await route.fetch()
    const body = (await res.json()) as Record<string, unknown> & {
      issues: Array<Record<string, unknown>>
    }
    body.last_session_ended_at = boundary
    const flow = body.flow as { cycle_p85_hours: number; samples: number } | undefined
    delete body.flow
    const moved = body.issues.filter(
      (i) => typeof i.updated_at === 'string' && (i.updated_at as string) > boundary,
    ).length
    console.log(
      `[a4] bootstrap route: last_session_ended_at ← ${boundary} (${moved} rows moved since);` +
        ` flow dropped (fixture p85 was ${flow?.cycle_p85_hours}h over ${flow?.samples} issues — see header)`,
    )
    await route.fulfill({ response: res, json: body })
  })

  // (2): the identity the demo serve does not have.
  await page.route('**/api/v1/auth/me/', async (route) => {
    console.log(`[a4] auth/me route-mocked → ${IDENTITY.account_id} (fixture answers email:null)`)
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(IDENTITY) })
  })

  // (4): one previous read on one issue.
  await page.route(`**/api/v1/issues/${resume.key}/detail/`, async (route) => {
    const res = await route.fetch()
    const body = (await res.json()) as Record<string, unknown>
    body.last_visited_at = resume.visitedAt
    console.log(`[a4] detail route: ${resume.key}.last_visited_at ← ${resume.visitedAt}`)
    await route.fulfill({ response: res, json: body })
  })

  await page.goto('/')
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // (a) The Issues list: the session strip on its first line, and the work
  // age on the rows beneath it. Both must actually be there before the
  // photograph — a capture of an absent feature is the failure mode this
  // round exists to avoid.
  const strip = page.locator('[data-testid="session-strip"]')
  await strip.waitFor()
  console.log(`[a4] session strip reads: ${JSON.stringify(await strip.innerText())}`)
  const ages = page.locator('.pane:not(.off) button.row .age')
  await ages.first().waitFor()
  const bands = await page.locator('.pane:not(.off) button.row .age').evaluateAll((els) =>
    els.map((el) => el.getAttribute('data-age-band')),
  )
  console.log(`[a4] ${bands.length} rows wear an age; bands: ${JSON.stringify(bands)}`)
  await page.screenshot({
    path: join(SHOT_DIR, 'a4-issues-session-and-age.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a4] shot ${join(SHOT_DIR, 'a4-issues-session-and-age.png')}`)

  // (b) The scope sheet: the desk's five built-ins under the desk's heading.
  await page.locator('.head button.scope').click()
  await page.locator('.sheet button.row').first().waitFor()
  const names = await page.locator('.sheet button.row').allInnerTexts()
  console.log(`[a4] scope sheet rows: ${JSON.stringify(names)}`)
  await expect(page.locator('.sheet')).toContainText('Team flow')
  await page.screenshot({
    path: join(SHOT_DIR, 'a4-scope-sheet.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a4] shot ${join(SHOT_DIR, 'a4-scope-sheet.png')}`)
  await page.locator('.sheet button.cancel').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })

  // (c) The detail with the resume card. Reached by search — the pane's own
  // road to any key (a1/a2's pattern).
  const tabs = page.locator('nav.safe-bottom button.tab')
  await tabs.nth(1).click()
  await page.locator('.pane:not(.off) input').first().fill(resume.key)
  await page.locator('.pane:not(.off) button.row', { hasText: resume.key }).first().click()
  await page.locator('button.back').waitFor()
  const card = page.locator('[data-testid="resume-card"]')
  await card.waitFor()
  console.log(`[a4] resume card reads: ${JSON.stringify(await card.innerText())}`)
  await page.screenshot({
    path: join(SHOT_DIR, 'a4-detail-resume.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a4] shot ${join(SHOT_DIR, 'a4-detail-resume.png')}`)
})
