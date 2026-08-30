import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect } from '@playwright/test'
import { drainTerminalSessions, forceLocale, readTerm } from '../helpers'
import {
  BOUND_ISSUE,
  COMMAND,
  LONE_ISSUE,
  injectCommandBodies,
  openIssue,
  openPaneBoundTo,
} from '../issue-command-fixture'

/*
 * The command block's captures (GDK-1162 / GDK-1164-A), for the design round
 * that reads them. Not a gate: it asserts nothing a reviewer would act on, it
 * writes PNGs.
 *
 * It lives under e2e/demo/ because that is a directory the CI set already
 * excludes (`testIgnore: ['**\/demo/**', '**\/hosted/**', '**\/perf/**']` in
 * e2e/playwright.config.ts) — it was in e2e/issue-command.spec.ts until
 * 2026-08-30, where it ran on every merge and took the whole Playwright job
 * red with it. Every recording config in this directory names its own
 * `testMatch`, so nothing here picks this file up by accident; run it with
 * e2e/demo/issue-command-shots.config.ts:
 *
 *   npx playwright test --config e2e/demo/issue-command-shots.config.ts
 */

const SHOT_DIR = join(
  dirname(fileURLToPath(import.meta.url)),
  '..',
  '..',
  'scratch',
  'issue-command-shots',
)

test.describe('command block shots', () => {
  test.use({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 })

  test.beforeEach(async ({ page }) => {
    await forceLocale(page, 'en')
    await injectCommandBodies(page)
  })

  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  test('capture attached, detached, unattended mark', async ({ page }) => {
    test.setTimeout(120_000)
    mkdirSync(SHOT_DIR, { recursive: true })

    await openIssue(page, BOUND_ISSUE)
    await openPaneBoundTo(page, BOUND_ISSUE)
    await expect(page.getByTestId('issue-body')).toHaveAttribute('data-shell', 'attached', {
      timeout: 20_000,
    })
    await page.getByTestId('issue-body').locator('[data-run-command]').first().click()
    await expect.poll(async () => readTerm(page)).toContain(COMMAND)
    await page.screenshot({ path: join(SHOT_DIR, '01-attached-placed.png') })

    await openIssue(page, LONE_ISSUE)
    await expect(page.getByTestId('issue-body')).toHaveAttribute('data-shell', 'none')
    await expect(page.getByTestId('unattended-chip')).toBeVisible({ timeout: 20_000 })
    await page.screenshot({ path: join(SHOT_DIR, '02-no-shell.png') })

    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'dark'))
    await page.screenshot({ path: join(SHOT_DIR, '03-no-shell-dark.png') })
  })
})
