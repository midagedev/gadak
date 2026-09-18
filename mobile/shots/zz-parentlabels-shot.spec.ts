/*
 * Captures for the GDK-1871 review round: the epic's New child control, the
 * create sheet that knows its parent, the labels sheet and the due sheet.
 *
 * Two tests, two serves, and the split is the point.
 *
 *  - `captures` runs against the bundled demo, which is credential-less: the
 *    store's writability probe says off there and every write control would
 *    ship disabled, so this arms GET credential/ the way the e2e specs do.
 *    Nothing else is faked — no write is sent, and none would land.
 *  - `writes` is skipped unless SHOTS_WRITES=1, because it needs a serve that
 *    can actually write. Point it at a THROWAWAY built-in-tracker workspace
 *    (never `reviewdemo`, which is the App Store review origin):
 *
 *      GADAK_HOME=$HOME/.gadak <gate binary> --workspace wt1871 \
 *        migrate --from demo --skip-attachments
 *      GADAK_HOME=$HOME/.gadak <gate binary> --workspace wt1871 \
 *        serve --addr 127.0.0.1:7927 --no-sync --no-open
 *      SHOTS_WRITES=1 GADAK_MOBILE_E2E_PORT=5193 GADAK_MOBILE_API_PORT=7927 \
 *        npm run shots -- --grep parentlabels
 *
 *    The epic is discovered from the serve rather than spelled here: a
 *    migrated mirror is not required to keep the demo's keys, and
 *    hierarchy_level is the axis the control keys on — never the type's
 *    display name.
 *
 * Same fixture and same 402×874 viewport as the walk, its own output dir so
 * the walk's rmSync cannot take it.
 */
import { test, expect, type Page } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const outDir = join(here, '..', '..', 'scratch', 'mobile-shots', 'parentlabels')

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
}

/** Light then dark, one scene, so the pair can be read side by side. */
async function shootPair(page: Page, name: string): Promise<void> {
  await settle(page)
  await page.screenshot({ path: join(outDir, `${name}.png`) })
  await page.emulateMedia({ colorScheme: 'dark' })
  await settle(page)
  await page.screenshot({ path: join(outDir, `${name}-dark.png`) })
  await page.emulateMedia({ colorScheme: 'light' })
  await settle(page)
}

/** Search → row → detail, the one road that does not depend on a scope. */
async function openIssue(page: Page, key: string): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('button.search').click()
  await page.locator('.pane:not(.off) input').first().fill(key)
  const hit = page.locator('.pane:not(.off) button.row', { hasText: key }).first()
  await hit.waitFor()
  await hit.click()
  await page.locator('button.back').waitFor()
}

/** The serve's own answer for which row is an epic and which is under it. */
async function pickKeys(page: Page): Promise<{ epic: string; child: string }> {
  const res = await page.request.get('/api/v1/issues/bootstrap/')
  expect(res.ok(), 'bootstrap').toBeTruthy()
  const body = (await res.json()) as {
    issues: { issue_key: string; hierarchy_level?: number; parent_key?: string | null }[]
  }
  const epic = body.issues.find((i) => i.hierarchy_level === 1)
  expect(epic, 'an epic in the mirror').toBeTruthy()
  const child = body.issues.find(
    (i) => i.hierarchy_level === 0 && i.parent_key === epic!.issue_key,
  )
  expect(child, 'a standard row under that epic').toBeTruthy()
  console.log(`[parentlabels] epic ${epic!.issue_key} · standard ${child!.issue_key}`)
  return { epic: epic!.issue_key, child: child!.issue_key }
}

test('captures — the epic control and the three sheets, light and dark', async ({ page }) => {
  rmSync(outDir, { recursive: true, force: true })
  mkdirSync(outDir, { recursive: true })

  // The demo serve has no origin credential, so the probe would disable every
  // write control before a single one could be photographed.
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  const { epic, child } = await pickKeys(page)

  await openIssue(page, epic)
  const newChild = page.getByRole('button', { name: 'New child' })
  await expect(newChild).toBeVisible()
  await shootPair(page, '01-epic-header')

  await newChild.click()
  await page.locator('.sheet').waitFor()
  await expect(page.locator('[data-testid="create-parent"]')).toContainText(epic)
  await shootPair(page, '02-create-sheet-under-epic')
  await page.locator('.sheet button.cancel').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })

  await openIssue(page, child)
  const labelsRow = page.locator('[data-testid="field-labels"]')
  await labelsRow.scrollIntoViewIfNeeded()
  await labelsRow.click()
  await page.locator('.sheet .label-add input').waitFor()
  await shootPair(page, '03-labels-sheet')
  await page.locator('.sheet button.cancel').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })

  await page.locator('.meta button.m-btn.due').click()
  await page.locator('.sheet input[type="date"]').waitFor()
  await shootPair(page, '04-due-sheet')
})

test('writes — a child lands, a label set saves, a due date is set and cleared', async ({
  page,
}) => {
  test.skip(
    !process.env.SHOTS_WRITES,
    'needs a write-capable serve: SHOTS_WRITES=1 GADAK_MOBILE_API_PORT=<serve port>',
  )
  mkdirSync(outDir, { recursive: true })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  const { epic, child } = await pickKeys(page)

  /* ── The child ── */
  await openIssue(page, epic)
  await page.getByRole('button', { name: 'New child' }).click()
  await page.locator('.sheet input#create-summary').waitFor()
  const title = `Filed from the phone under ${epic}`
  await page.locator('.sheet input#create-summary').fill(title)
  await page.locator('.sheet .create button.go').click()
  // The sheet closes and the new child is what the screen is showing.
  await page.locator('.sheet').waitFor({ state: 'hidden' })
  const subject = page.locator('.detail-layer h1.type-subject').last()
  await expect(subject).toHaveText(title)
  // Filed under the epic, which the Fields section is the proof of.
  await expect(page.locator('[data-testid="detail-fields"] .f-key').first()).toHaveText(epic)
  await shootPair(page, '05-child-created')

  /* ── The labels ── */
  await openIssue(page, child)
  const labelsRow = page.locator('[data-testid="field-labels"]')
  await labelsRow.scrollIntoViewIfNeeded()
  const before = (await labelsRow.locator('.f-value').innerText()).trim()
  await labelsRow.click()
  await page.locator('.sheet .label-add input').waitFor()
  const fresh = `phone-${Date.now().toString(36)}`
  await page.locator('.sheet .label-add input').fill(fresh)
  await page.locator('.sheet .label-add button.ghost').click()
  await expect(page.locator('.sheet button.t-row', { hasText: fresh })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await page.locator('.sheet button.save').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })
  // The row carries what the origin answered with, not what was typed.
  await expect(labelsRow.locator('.f-value')).toContainText(fresh)
  expect((await labelsRow.locator('.f-value').innerText()).trim()).not.toBe(before)
  await labelsRow.scrollIntoViewIfNeeded()
  await shootPair(page, '06-labels-saved')

  /* ── The due date ── */
  const due = page.locator('.meta button.m-btn.due')
  await due.click()
  await page.locator('.sheet input[type="date"]').fill('2026-12-24')
  await page.locator('.sheet button.save').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })
  await expect(due).toContainText('2026')
  await shootPair(page, '07-due-set')

  await due.click()
  await page.locator('.sheet input[type="date"]').waitFor()
  await page.locator('.sheet button.ghost').click()
  await page.locator('.sheet').waitFor({ state: 'hidden' })
  // Cleared: the item is back to the bare label, which is the affordance a
  // row without a due date wears.
  await expect(due).not.toContainText('2026')
  await shootPair(page, '08-due-cleared')
})
