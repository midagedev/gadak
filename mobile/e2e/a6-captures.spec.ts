// GDK-1497 A6 captures. The phone terminal grew a session roster — list,
// switch, name, issue binding, place-a-line — and the sticky Ctrl states were
// re-cut so locked differs from armed in FILL (GDK-951). This spec drives
// those surfaces on the shared fixture (`gadak demo` on 7899, vite on 5182 —
// mobile/playwright.config.ts) and photographs them for the lead's vision
// round.
//
// The setup goes through the UI on purpose rather than seeding over REST: a
// capture of a roster the app never built would be a picture of a fixture.
// The assertions here are not about looks — that judgement is the vision
// round's — they pin that the state actually arrived, so a silent no-op fails
// this spec instead of shipping a misleading photograph.
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { SERVE_ORIGIN } from '../playwright.config'

// Capture-only spec (GDK-1570): it runs when a vision round names the
// directory through A6_SHOT_DIR and is skipped otherwise, like a1/a2/a4.
const SHOT_DIR = process.env.A6_SHOT_DIR ?? join(dirname(fileURLToPath(import.meta.url)), '.shots')

function makeTerminalOffer(label: string): string {
  const doc = JSON.stringify({
    v: 1,
    endpoint: `${SERVE_ORIGIN}`,
    token: crypto.randomUUID(),
    expires_at: '',
    label,
  })
  return Buffer.from(doc).toString('base64url')
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

async function pairShell(page: Page, label = 'This Mac (dev)'): Promise<void> {
  await page.locator('nav.safe-bottom button.tab', { hasText: 'Pairing' }).click()
  await page.getByRole('heading', { name: 'Pairing' }).waitFor()
  await page.locator('#term-offer').fill(makeTerminalOffer(label))
  await page.getByRole('button', { name: 'Pair', exact: true }).click()
  await expect(page.locator('nav.safe-bottom button.tab', { hasText: 'Terminal' })).toBeVisible()
}

async function openShell(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom button.tab', { hasText: 'Terminal' }).click()
  await expect(page.getByTestId('terminal-pane')).toBeVisible()
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
}

/** Every shell this fixture is holding, gone — so a run starts from zero. */
async function drainSessions(page: Page): Promise<void> {
  const res = await page.request.get(`${SERVE_ORIGIN}/api/v1/terminal/sessions/`)
  if (!res.ok()) return
  const body = (await res.json()) as { sessions?: { id: string }[] }
  for (const s of body.sessions ?? []) {
    await page.request.delete(`${SERVE_ORIGIN}/api/v1/terminal/sessions/${s.id}/`)
  }
}

/** A real key from the fixture — a made-up one would photograph as a typo. */
async function anIssueKey(page: Page): Promise<string> {
  const res = await page.request.get(`${SERVE_ORIGIN}/api/v1/issues/bootstrap/`)
  const body = (await res.json()) as { issues?: { issue_key: string }[] }
  const key = body.issues?.[0]?.issue_key
  if (!key) throw new Error('no issues on the fixture to bind a shell to')
  return key
}

const sheet = (page: Page) => page.getByTestId('session-sheet')
const rows = (page: Page) => page.getByTestId('session-row')

async function openSheet(page: Page): Promise<void> {
  await page.getByTestId('shell-sessions').click()
  await expect(sheet(page)).toBeVisible()
}

/**
 * Open one row's drawer on one verb.
 *
 * By test id, not by accessible name: the verb buttons carry an aria-label
 * that includes the row's label ("Rename shell 1"), because a 44pt button
 * cannot show it and a screen reader must hear it. A name-based selector
 * therefore matches nothing exactly and everything loosely.
 */
async function rowDrawer(page: Page, index: number, verb: 'rename' | 'issue' | 'send' | 'kill'): Promise<void> {
  await rows(page).nth(index).getByTestId('session-more').click()
  await rows(page).nth(index).getByTestId(`verb-${verb}`).click()
}

test.describe('A6 captures', () => {
  test.beforeEach(async ({ page }) => {
    await drainSessions(page)
  })
  test.afterEach(async ({ page }) => {
    await drainSessions(page)
  })

  test('the session sheet, with one named shell and one bound to an issue', async ({ page }) => {
    test.skip(!process.env.A6_SHOT_DIR, 'capture-only; set A6_SHOT_DIR to run')
    mkdirSync(SHOT_DIR, { recursive: true })
    const key = await anIssueKey(page)
    console.log(`[a6] binding a shell to ${key}`)

    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    // One shell exists (the pane's own). Add a second through the sheet's +.
    await openSheet(page)
    await expect(rows(page)).toHaveCount(1)
    await page.getByTestId('session-new').click()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })

    // Name the first, bind the second.
    await openSheet(page)
    await expect(rows(page)).toHaveCount(2)
    await rowDrawer(page, 0, 'rename')
    await page.getByTestId('session-field').fill('build')
    await page.getByTestId('session-submit').click()
    await expect(rows(page).nth(0)).toContainText('build')

    await rowDrawer(page, 1, 'issue')
    await page.getByTestId('session-field').fill(key)
    await page.getByTestId('session-submit').click()
    await expect(rows(page).nth(1)).toContainText(key)

    // The state actually reached the serve, not just the roster in memory.
    const listed = (await (
      await page.request.get(`${SERVE_ORIGIN}/api/v1/terminal/sessions/`)
    ).json()) as { sessions: { name?: string; issue_key?: string }[] }
    expect(listed.sessions.map((s) => s.name ?? '')).toContain('build')
    expect(listed.sessions.map((s) => s.issue_key ?? '')).toContain(key)

    await page.screenshot({ path: `${SHOT_DIR}/a6-session-sheet.png`, animations: 'disabled' })

    // The place-a-line drawer, open on the shell the pane is not showing —
    // the one gesture that has no other road on a phone holding one socket.
    await rowDrawer(page, 0, 'send')
    await page.getByTestId('session-field').fill('git status')
    await page.getByTestId('session-submit').click()
    await expect(page.getByTestId('session-notice')).toBeVisible()
    await page.screenshot({ path: `${SHOT_DIR}/a6-send-line.png`, animations: 'disabled' })
  })

  test('sticky Ctrl armed and locked, as two fills', async ({ page }) => {
    test.skip(!process.env.A6_SHOT_DIR, 'capture-only; set A6_SHOT_DIR to run')
    mkdirSync(SHOT_DIR, { recursive: true })
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    const ctrl = page.getByRole('button', { name: 'Ctrl' })
    const alt = page.getByRole('button', { name: 'Alt' })
    const noMods = page.getByRole('button', { name: 'No Mods' })
    const bar = page.getByTestId('key-bar')

    // Two frames per state: the strip alone, which is where the fill is
    // judged, and the whole 402×874 phone, which is where "at arm's length"
    // means anything.
    const shoot = async (name: string): Promise<void> => {
      await bar.screenshot({ path: `${SHOT_DIR}/a6-keybar-${name}.png`, animations: 'disabled' })
      await page.screenshot({ path: `${SHOT_DIR}/a6-ctrl-${name}.png`, animations: 'disabled' })
    }

    // Armed: one tap. The next letter carries Control and then it lets go.
    await ctrl.click()
    await expect(ctrl).toHaveAttribute('data-slot', 'armed')
    await shoot('armed')

    // Back to idle first: glasskeys locks on a second tap inside 400ms and
    // re-arms outside it (LOCK_WINDOW_MS), and the screenshots above take
    // longer than that — a second plain click here would photograph armed
    // twice and nothing would be red about it.
    await noMods.click()
    await expect(ctrl).toHaveAttribute('data-slot', 'idle')

    // Locked: a real double-tap. Every letter carries Control until it is
    // released — the state that used to be a 2px underline where armed had a
    // 1px ring (GDK-951), which is what these frames are for.
    await ctrl.dblclick()
    await expect(ctrl).toHaveAttribute('data-slot', 'locked')
    await shoot('locked')

    // And both at once, which is the frame the question is actually asked
    // in: Ctrl still locked, Alt armed beside it. Two adjacent keys, and the
    // only honest test of whether a person can tell them apart is seeing
    // them together.
    await alt.click()
    await expect(alt).toHaveAttribute('data-slot', 'armed')
    await expect(ctrl).toHaveAttribute('data-slot', 'locked')
    await bar.screenshot({ path: `${SHOT_DIR}/a6-keybar-both.png`, animations: 'disabled' })

    // The panic exit still lands from locked (GDK-953), so the capture set is
    // not a state the strip cannot leave.
    await noMods.click()
    await expect(ctrl).toHaveAttribute('data-slot', 'idle')
    await expect(alt).toHaveAttribute('data-slot', 'idle')
  })
})
