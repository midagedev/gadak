// GDK-1497 A2 captures. The phone can now write: assignee, priority,
// summary, description, and issue creation. This spec photographs the new
// control surfaces on the shared fixture (`gadak demo` on 7899, vite on
// 5182 — mobile/playwright.config.ts) for the lead's vision round.
//
// Fixture truth (measured 2026-09-07 on a scratch serve): the demo home
// carries no origin credential, so EVERY write PUT/POST and every origin
// catalog GET (priorities/, users/, create-meta/) answers 409
// credential_required. The captures therefore show sheets before any
// write fires — which is exactly what the spec asked for: no capture
// depends on a write succeeding. Two consequences are visible and honest:
//   · the create sheet wears its writes-off sentence (the sheet stays
//     readable; only its action recedes), and
//   · the priority sheet's catalog is route-mocked for the layout cut,
//     because the honest fixture closes that sheet before any row can
//     render. The mock is labeled in the log, fulfils only that one GET,
//     and the priority WRITE is never exercised here.
//
// One more measured fixture truth (2026-09-07, GET /api/v1/issues/bootstrap/
// on the demo home): every row answers `priority_id: ""` — the demo db is
// scrubbed of priority ids (examples/demo.db `issues_full.priority_id` is ''
// on all 534 rows). The sheet keys the current row on that id, as it must
// (CLAUDE.md bans keying on display names), so on the raw fixture NO row can
// carry the current-value mark and the cut would photograph a sheet that
// looks like it has no current value. The bootstrap route below therefore
// gives the ONE photographed issue the catalog id whose name it already
// displays, which makes the mocked world self-consistent instead of
// self-contradicting. It is labeled in the log, it touches one field of one
// issue, and no other capture reads it.
import { expect, test } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { SERVE_ORIGIN } from '../playwright.config'

const SHOT_DIR = join(dirname(fileURLToPath(import.meta.url)), '.shots')

/** The Jira default set, used by both mocks so they cannot disagree. */
const PRIORITY_CATALOG = [
  { id: '1', name: 'Highest' },
  { id: '2', name: 'High' },
  { id: '3', name: 'Medium' },
  { id: '4', name: 'Low' },
  { id: '5', name: 'Lowest' },
]

type IssueLite = { issue_key: string; assignee: string | null; assignee_id: string | null }
type DetailDoc = { issue_key: string; description_md?: string; description_text?: string }

/**
 * The issue to photograph: assigned (so the assignee sheet carries a
 * current row beyond Unassigned) with the longest description (so the
 * editor's prefill is real text, not a sliver). Every detail is fetched —
 * 534 loopback reads measured ~1s in a1-captures — because the lite rows
 * carry no description.
 */
async function pickIssue(): Promise<string> {
  const boot = (await (await fetch(`${SERVE_ORIGIN}/api/v1/issues/bootstrap/`)).json()) as {
    issues: IssueLite[]
  }
  const details = await Promise.all(
    boot.issues.map(async (lite) => {
      const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/${lite.issue_key}/detail/`)
      return (await res.json()) as DetailDoc
    }),
  )
  let best = { key: '', assigned: false, descLen: -1 }
  for (const doc of details) {
    const lite = boot.issues.find((i) => i.issue_key === doc.issue_key)
    const assigned = Boolean(lite?.assignee_id)
    const descLen = (doc.description_md ?? doc.description_text ?? '').length
    const better =
      (assigned && !best.assigned) ||
      (assigned === best.assigned && descLen > best.descLen)
    if (better) best = { key: doc.issue_key, assigned, descLen }
  }
  if (!best.key) throw new Error('no issues on the fixture to capture')
  console.log(
    `[a2] issue ${best.key} — assigned: ${best.assigned}, description ${best.descLen} chars`,
  )
  return best.key
}

test('captures the A2 write surfaces for the vision round', async ({ page }) => {
  mkdirSync(SHOT_DIR, { recursive: true })
  const issueKey = await pickIssue()

  // See the header note: the demo db carries no priority ids, so give the
  // photographed issue the catalog id it already displays. One field, one
  // issue; everything else passes through untouched.
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const res = await route.fetch()
    const body = (await res.json()) as { issues: Array<Record<string, unknown>> }
    const row = body.issues.find((i) => i.issue_key === issueKey)
    const match = PRIORITY_CATALOG.find((p) => p.name === row?.priority)
    if (row && match && !row.priority_id) {
      row.priority_id = match.id
      console.log(
        `[a2] bootstrap route: ${issueKey}.priority_id "" → ${match.id} (${match.name}) — fixture carries no priority ids`,
      )
    }
    await route.fulfill({ response: res, json: body })
  })

  await page.goto('/')
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // Issue via search — the pane's own road to any key (a1's pattern).
  const tabs = page.locator('nav.safe-bottom button.tab')
  await tabs.nth(1).click()
  await page.locator('.pane:not(.off) input').first().fill(issueKey)
  await page.locator('.pane:not(.off) button.row', { hasText: issueKey }).first().click()
  await page.locator('button.back').waitFor()
  await page.waitForLoadState('networkidle').catch(() => {})

  // (a) Detail header at rest: the summary glyph, and the priority and
  // assignee fragments as meta-line controls. The header must have settled
  // before the photograph (summary text present, not the ghost).
  await expect(page.locator('.subject h1')).toContainText(/\S/)
  await expect(page.locator('.meta .m-btn')).toHaveCount(2)
  await page.screenshot({
    path: join(SHOT_DIR, 'a2-detail-header.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a2] shot ${join(SHOT_DIR, 'a2-detail-header.png')}`)

  // (b) Assignee sheet: opens with no request on this build — the fixture's
  // 409s only arrive when a row or the search fires, and neither does here.
  await page.locator('.meta .m-btn').nth(1).click()
  await page.locator('.sheet button.t-row').first().waitFor()
  await expect(page.locator('.sheet')).toContainText('Unassigned')
  await page.screenshot({
    path: join(SHOT_DIR, 'a2-assignee-sheet.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a2] shot ${join(SHOT_DIR, 'a2-assignee-sheet.png')}`)
  await page.locator('.sheet button.cancel').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })

  // (d) Description editor: prefilled from the server's markdown — the
  // textarea must actually carry the body, or the capture would lie.
  await page.locator('.h-row .edit').click()
  const draft = page.locator('.desc-edit textarea')
  await draft.waitFor()
  await expect(draft).not.toHaveValue('')
  await page.screenshot({
    path: join(SHOT_DIR, 'a2-description-editor.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a2] shot ${join(SHOT_DIR, 'a2-description-editor.png')}`)
  await page.locator('.sheet button.cancel').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })

  // (c) Priority sheet — the one mocked cut (see the header comment): the
  // honest fixture 409s the catalog and the sheet recedes before any row
  // renders, so this route fulfils the GET with the Jira default set and
  // labels that in the log. No PUT is ever sent from this spec.
  await page.route('**/api/v1/issues/*/priorities/', async (route) => {
    console.log('[a2] priority catalog route-mocked (fixture 409s it — layout cut only)')
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ priorities: PRIORITY_CATALOG }),
    })
  })
  await page.locator('.meta .m-btn').nth(0).click()
  await page.locator('.sheet button.t-row').first().waitFor()
  await expect(page.locator('.sheet')).toContainText('Medium')
  await page.screenshot({
    path: join(SHOT_DIR, 'a2-priority-sheet.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a2] shot ${join(SHOT_DIR, 'a2-priority-sheet.png')}`)
  await page.locator('.sheet button.cancel').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })
  await page.unroute('**/api/v1/issues/*/priorities/')

  // (e) Issues header with the + action, then the create sheet. Here the
  // honest degradation is the point: create-meta 409s on the fixture, so
  // the sheet wears its one writes-off sentence and stays readable — wait
  // for that sentence before the photograph.
  await page.locator('button.back').first().click()
  await tabs.nth(0).click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await expect(page.locator('.head button.new')).toBeVisible()
  await page.screenshot({
    path: join(SHOT_DIR, 'a2-issues-header.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a2] shot ${join(SHOT_DIR, 'a2-issues-header.png')}`)
  await page.locator('.head button.new').click()
  await page.locator('.create input').waitFor()
  const sentence = page.locator('.create .off-note')
  await sentence.waitFor()
  await page.screenshot({
    path: join(SHOT_DIR, 'a2-create-sheet.png'),
    fullPage: true,
    animations: 'disabled',
  })
  console.log(`[a2] shot ${join(SHOT_DIR, 'a2-create-sheet.png')} (writes-off sentence present)`)
})
