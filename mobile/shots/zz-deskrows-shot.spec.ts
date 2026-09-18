/*
 * GDK-1874 captures: the five placements, light and dark.
 *
 * The walk next door photographs the screens but never the rows added here,
 * and one of the five is unreachable on the fixture at all — `gadak demo`
 * configures no custom field, so the Fields desk row can only be seen with a
 * spec'd field in the payload. That case (and only that case) is intercepted
 * the way a7-captures.spec.ts does it: one `field_specs` entry and one
 * top-level value on NMB-105, shaped exactly as internal/server/read.go
 * emits them. The gate spec next door stays on real data and asserts the
 * absence; this adds the picture the gate cannot take.
 *
 * Output is its own directory so the walk's rmSync cannot take it.
 */
import { test, type Page } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const outDir = join(here, '..', '..', 'scratch', 'mobile-shots', 'deskrows')

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
}

async function shoot(page: Page, name: string): Promise<void> {
  await settle(page)
  await page.screenshot({ path: join(outDir, `${name}.png`) })
  console.log(`[deskrows] shot ${join(outDir, `${name}.png`)}`)
}

/** One configured field on NMB-105, so the Fields desk row has a reason. */
async function withCustomField(page: Page): Promise<void> {
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const res = await route.fetch()
    if (res.status() !== 200) {
      await route.fulfill({ response: res })
      return
    }
    const boot = (await res.json()) as {
      field_specs?: unknown[]
      issues?: Array<Record<string, unknown>>
    }
    boot.field_specs = [
      ...(boot.field_specs ?? []),
      { alias: 'story_points', label: 'Story points', role: 'plain', kind: 'number' },
    ]
    for (const issue of boot.issues ?? []) {
      if (issue.issue_key === 'NMB-105') issue.story_points = 5
    }
    await route.fulfill({ response: res, json: boot })
  })
}

/**
 * The issue queue under an issue scope, whatever the previous pass left
 * behind. The walk runs twice in one context, so the second pass inherits
 * the first one's tab, scope and open detail from storage — and under a
 * documents scope the sprint line (and the board row with it) is correctly
 * absent, which reads as a hang rather than as state.
 */
async function bootIssues(page: Page): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  const back = page.locator('button.back').first()
  if (await back.isVisible().catch(() => false)) {
    await back.click()
  }
  // GDK-902 2026-09-15: there is no tab to return to — the list is the
  // only owner unless the shell was entered, and this walk never enters it.
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'All open' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
}

async function openIssue(page: Page, key: string): Promise<void> {
  await page.locator('button.search').click()
  await page.locator('.pane:not(.off) input').first().fill(key)
  const hit = page.locator('.pane:not(.off) button.row', { hasText: key }).first()
  await hit.waitFor()
  await hit.click()
  await page.locator('button.back').waitFor()
}

async function openFirstPage(page: Page): Promise<void> {
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  const row = page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first()
  await row.waitFor()
  await row.click()
  await page.locator('.page-detail button.back').waitFor()
}

/** The four reachable placements, in one pass at the given scheme. */
async function walk(page: Page, suffix: string): Promise<void> {
  await bootIssues(page)
  // 1. The board row, under the sprint line it belongs to.
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()
  await shoot(page, `01-sprint-board-${suffix}`)

  // 2+3. View settings and Dashboards, in the scope sheet. Two frames: the
  // top of the list, where the saved-views heading stands over its one row,
  // and the end of it, where the dashboards row closes the sheet.
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, `02-scope-sheet-top-${suffix}`)
  await page.locator('[data-testid="desk-row-dashboards"]').scrollIntoViewIfNeeded()
  await shoot(page, `03-scope-sheet-end-${suffix}`)
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })

  // 4. The page content row, above the body it describes.
  await openFirstPage(page)
  await shoot(page, `04-page-content-${suffix}`)
  await page.locator('.page-detail button.back').click()
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()

  // 5. The Fields row, after the last field on an issue that has a spec'd
  // one. Scrolled so the section and the row under it are both in frame.
  await openIssue(page, 'NMB-105')
  await page.locator('[data-testid="desk-row-fields"]').scrollIntoViewIfNeeded()
  await shoot(page, `05-detail-fields-${suffix}`)
}

test('the five desk rows, light and dark', async ({ page }) => {
  rmSync(outDir, { recursive: true, force: true })
  mkdirSync(outDir, { recursive: true })

  await withCustomField(page)
  await page.emulateMedia({ colorScheme: 'light' })
  await walk(page, 'light')

  await page.emulateMedia({ colorScheme: 'dark' })
  await walk(page, 'dark')
  await page.emulateMedia({ colorScheme: 'light' })
})
