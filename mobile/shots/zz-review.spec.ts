import { test, type Page } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/*
 * Review captures the walk cannot take (2026-09-14 screen review):
 *  - `gate`   PairGate — the first screen a reviewer sees. Dev adopts the
 *             proxy by construction; the explicit-unpaired flag keeps it off.
 *  - `ko`     the ledger in Korean: serif heading meets the CJK fallback.
 *  - `writes` every write surface, against a write-capable serve
 *             (GADAK_MOBILE_API_PORT pointed at a built-in-tracker workspace,
 *             e.g. `gadak --workspace reviewdemo serve`). On `gadak demo`
 *             these sheets refuse at the control and the pictures are wrong.
 * Run: SHOTS_CYCLE=<c> npm run shots -- --grep <label>
 *      `writes` is skipped unless SHOTS_WRITES=1 — on the demo serve the
 *      status control ends disabled and the sheet wait would time the whole
 *      bare `npm run shots` out (advisor, 2026-09-14).
 */
const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..')
const cycle = process.env.SHOTS_CYCLE || 'review'

function out(sub: string): string {
  const d = join(repoRoot, 'scratch', 'mobile-shots', `${cycle}-${sub}`)
  rmSync(d, { recursive: true, force: true })
  mkdirSync(d, { recursive: true })
  return d
}

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
}

async function shoot(page: Page, dir: string, name: string): Promise<void> {
  await settle(page)
  await page.screenshot({ path: join(dir, `${name}.png`) })
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

async function closeSheet(page: Page): Promise<void> {
  await page.locator('button.cancel').click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })
}

// GDK-902 2026-09-15: the scope picker is the palette in the list's body,
// and its dismiss is its own control — `button.cancel` belongs to Sheet.
async function closePalette(page: Page): Promise<void> {
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
}

test('gate', async ({ page }) => {
  const dir = out('gate')
  await page.addInitScript(() => localStorage.setItem('gadak.pairing.unpaired', '1'))
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('button, input').first().waitFor()
  await shoot(page, dir, '01-pairgate')
  await page.emulateMedia({ colorScheme: 'dark' })
  await shoot(page, dir, '02-pairgate-dark')
})

test('ko', async ({ page }) => {
  const dir = out('ko')
  await page.addInitScript(() => localStorage.setItem('gadak_locale', 'ko'))
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await shoot(page, dir, '01-issues-ko')
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, dir, '02-scope-ko')
  await closePalette(page)
  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  await shoot(page, dir, '03-detail-ko')
  await page.locator('button.back').first().click()
  await page.locator('.detail-layer').waitFor({ state: 'detached' }).catch(() => {})
  // GDK-902 2026-09-15: Pairing is the Settings push layer, from the gear.
  await page.locator('button.gear').click()
  await page.locator('.settings-layer h1').waitFor()
  await shoot(page, dir, '04-pairing-ko')
})

test('writes', async ({ page }) => {
  test.skip(!process.env.SHOTS_WRITES, 'needs a write-capable serve: SHOTS_WRITES=1 GADAK_MOBILE_API_PORT=<serve port>')
  const dir = out('writes')
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  await shoot(page, dir, '01-detail')

  await page.locator('button.status').first().click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, dir, '02-transition-sheet')
  await closeSheet(page)

  const meta = page.locator('.meta button.m-btn')
  await meta.nth(0).click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, dir, '03-priority-sheet')
  await closeSheet(page)
  await meta.nth(1).click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, dir, '04-assignee-sheet')
  await closeSheet(page)

  await page.locator('.composer input').fill('Reproduced on staging with the 2.5 client — see NMA-120.')
  await shoot(page, dir, '05-composer-armed')

  await page.locator('.subject button.edit').click()
  await page.locator('.summary-edit input').waitFor()
  await shoot(page, dir, '06-title-edit')
  await page.locator('.summary-edit button.ghost').click()

  await page.locator('section.body button.edit').first().click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, dir, '07-description-edit')
  await closeSheet(page)

  // Leave with the comment unsent, come back: the draft note (GDK-1863).
  await page.locator('button.back').first().click()
  await page.locator('.detail-layer').waitFor({ state: 'detached' }).catch(() => {})
  await page.waitForTimeout(300)
  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  await shoot(page, dir, '08-draft-restored')
  await page.locator('button.back').first().click()
  await page.locator('.detail-layer').waitFor({ state: 'detached' }).catch(() => {})

  await page.locator('button.new').click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, dir, '09-create-sheet')
  await page.locator('#create-summary').fill('Portal login loops when the IdP sends a lowercase domain')
  await shoot(page, dir, '10-create-filled')
  await closeSheet(page)
})
