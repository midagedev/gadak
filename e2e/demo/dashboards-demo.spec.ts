/**
 * Dashboards promo for docs/media/dashboards.{gif,mp4} (GDK-794).
 *
 * Left: paper terminal types real `gadak dashboards` commands (the same
 * string is spawnSync'd). Right: the open tab follows.
 *
 *   1. save triage (example HTML + two SQL datasources)
 *   2. open triage → wall renders (count cards + uPlot)
 *   3. save again with an HTML marker change → frame swap
 *   4. Refresh click, then a Refresh burst (2s throttle)
 *
 * SQL keys status_category, never a localized status name.
 *
 * Gated by GADAK_MEDIA=1. Viewport must stay FRAME_W×FRAME_H (promo-split.ts)
 * or Playwright letterboxes the capture.
 */
import { readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'

import { test, expect } from '@playwright/test'

import { forceLocale, DEMO_ISSUE_COUNT_EN_RE } from '../helpers'
import {
  E2E_DIR,
  REPO_ROOT,
  applyVerticalDashboardLayout,
  assertFrozen,
  beat,
  clearTerminal,
  freezePromoHome,
  isPromoVertical,
  promoAppOrigin,
  promoFrameHtml,
  runTyped,
  showCli,
  timelinePath,
  typeCommand,
  writeTrimSeconds,
} from './promo-split'

const isMedia = !!process.env.GADAK_MEDIA

const BY_STATUS =
  'by_status=sql:select status_category, count(*) as n from issues_full group by 1 order by 1'
const MONTHLY =
  "monthly_opened=sql:select strftime('%s', substr(created_at,1,7)||'-01') as t, count(*) as n from issues_full where created_at >= date('now','-11 months') group by 1 order by 1"

const TOP_OPEN =
  "top_open=sql:select key, priority_rank, summary from issues_full where status_category != 'done' order by priority_rank, updated_at desc limit 8"
const MINE = 'mine=jql:assignee = currentUser() AND resolution is EMPTY'

const SAVE_V1 = `gadak dashboards save triage --html examples/dashboards/triage.html \\
  --datasource "${BY_STATUS}" \\
  --datasource "${TOP_OPEN}" \\
  --datasource "${MONTHLY}" \\
  --datasource "${MINE}"`

const V2_REL = 'e2e/.tmp/triage-v2.html'
const SAVE_V2 = `gadak dashboards save triage --html ${V2_REL} \\
  --datasource "${BY_STATUS}" \\
  --datasource "${TOP_OPEN}" \\
  --datasource "${MONTHLY}" \\
  --datasource "${MINE}"`

const OPEN_CMD = 'gadak dashboards open triage'

const FRAME_SEL = 'iframe[data-testid="dashboard-frame"]'

test.describe('dashboards promo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('save, open, HTML swap, refresh — terminal drives the wall', async ({ page }) => {
    writeFileSync(timelinePath('dashboards'), '')
    const t0 = Date.now()
    freezePromoHome()
    assertFrozen()
    beat('dashboards', 'frozen')

    const src = readFileSync(path.join(REPO_ROOT, 'examples/dashboards/triage.html'), 'utf8')
    if (!src.includes('<h1>Triage</h1>')) {
      throw new Error('triage.html h1 marker drifted — update SAVE_V2 companion')
    }
    writeFileSync(path.join(E2E_DIR, '.tmp', 'triage-v2.html'), src.replace('<h1>Triage</h1>', '<h1>Triage · v2</h1>'))

    await forceLocale(page, 'en')
    await page.emulateMedia({ colorScheme: 'light' })
    await page.setContent(promoFrameHtml(`${promoAppOrigin()}/`), { waitUntil: 'domcontentloaded' })

    const app = page.frameLocator('#app')
    await expect(app.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect(app.getByTestId('issue-list-scroller')).toBeVisible()
    writeTrimSeconds('dashboards', Math.max(0.16, (Date.now() - t0) / 1000 - 0.45))
    beat('dashboards', 'web_ready')
    await page.waitForTimeout(700)

    await typeCommand(page, SAVE_V1, 16)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(160)
    beat('dashboards', 'enter_save_v1')
    const saved = runTyped(SAVE_V1)
    if (saved.status !== 0) {
      throw new Error(`dashboards save v1 failed: ${saved.stderr || saved.stdout}`)
    }
    await showCli(page, saved.stdout, saved.stderr)
    expect(saved.stdout).toMatch(/^saved\t/m)
    await page.waitForTimeout(900)

    await clearTerminal(page)
    await page.waitForTimeout(240)
    await typeCommand(page, OPEN_CMD, 38)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(160)
    beat('dashboards', 'enter_open')
    const opened = runTyped(OPEN_CMD)
    if (opened.status !== 0) {
      throw new Error(`dashboards open failed: ${opened.stderr || opened.stdout}`)
    }
    await showCli(page, opened.stdout, opened.stderr)

    await expect(app.getByTestId('dashboard-view')).toBeVisible({ timeout: 5_000 })
    const wall = app.frameLocator(FRAME_SEL)
    if (isPromoVertical()) {
      await expect(wall.locator('h1')).toBeVisible({ timeout: 8_000 })
      await applyVerticalDashboardLayout(page)
    }
    await expect(wall.locator('#n-open')).toHaveText(/^[1-9][0-9]*$/, { timeout: 8_000 })
    await expect(wall.locator('#monthly canvas').first()).toBeVisible({ timeout: 8_000 })
    const sideRow = app.getByTestId('sidebar-dashboard-row').filter({ hasText: 'triage' })
    await expect(sideRow).toBeVisible({ timeout: 5_000 })
    await sideRow.scrollIntoViewIfNeeded()
    beat('dashboards', 'wall_rendered')
    await page.waitForTimeout(1800)

    await clearTerminal(page)
    await page.waitForTimeout(240)
    await typeCommand(page, SAVE_V2, 16)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(160)
    beat('dashboards', 'enter_save_v2')
    const saved2 = runTyped(SAVE_V2)
    if (saved2.status !== 0) {
      throw new Error(`dashboards save v2 failed: ${saved2.stderr || saved2.stdout}`)
    }
    await showCli(page, saved2.stdout, saved2.stderr)

    await expect(app.locator(FRAME_SEL)).toHaveAttribute('data-render-gen', /[1-9]/, { timeout: 5_000 })
    if (isPromoVertical()) await applyVerticalDashboardLayout(page)
    await expect(wall.locator('h1')).toHaveText('Triage · v2', { timeout: 5_000 })
    await expect(wall.locator('#n-open')).toHaveText(/^[1-9][0-9]*$/)
    beat('dashboards', 'frame_swapped')
    await page.waitForTimeout(1200)

    beat('dashboards', 'click_refresh')
    await wall.locator('#refresh').click()
    await page.waitForTimeout(900)

    beat('dashboards', 'click_refresh_burst')
    await wall.locator('#refresh').click()
    await wall.locator('#refresh').click()
    await wall.locator('#refresh').click()
    await wall.locator('#refresh').click()
    await page.waitForTimeout(2400)
    beat('dashboards', 'end')
  })
})
