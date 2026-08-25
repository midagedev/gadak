/**
 * Right-hand (landscape) / lower (vertical) half of the claude-drive clip.
 *
 * Loads the promo-split chrome (paper bar + empty terminal | real app iframe)
 * and records until e2e/.tmp/claude-drive[-vertical]/web-stop exists. The
 * terminal pane is discarded at composite time; VHS supplies it.
 *
 * After a dashboard opens, this file does camera work — not Claude
 * intervention: applyVerticalDashboardLayout (vertical) and a mouse-wheel
 * scroll until a chart canvas/heading is in the iframe viewport (both
 * layouts). Claude's HTML is not edited.
 *
 * Gated by GADAK_MEDIA=1. Does not spawn gadak children — Claude Code in
 * the VHS half is the only writer. The serve tab follows via ui-focus.
 */
import { existsSync, mkdirSync, writeFileSync, appendFileSync } from 'node:fs'
import path from 'node:path'

import { test, expect, type Page } from '@playwright/test'

import { forceLocale, DEMO_ISSUE_COUNT_EN_RE } from '../helpers'
import {
  E2E_DIR,
  FLAGSHIP_L_WEB_H,
  FLAGSHIP_L_WEB_W,
  V_BAR_H,
  applyVerticalDashboardLayout,
  appFrame,
  isPromoVertical,
  promoAppOrigin,
  promoFrameHtml,
} from './promo-split'

const isMedia = !!process.env.GADAK_MEDIA
const vertical = isPromoVertical()
// Which cut is recording: the flagship, or one of the split vertical clips.
// Only claude-dashboards ends by following a key off the wall (GDK-854).
const CLIP = process.env.GADAK_CLAUDE_DRIVE_CLIP || 'claude-drive'

const DIR = path.join(E2E_DIR, '.tmp', vertical ? 'claude-drive-vertical' : 'claude-drive')
const STOP = path.join(DIR, 'web-stop')
const READY = path.join(DIR, 'web-ready-epoch')
const TIMELINE = path.join(DIR, 'web-timeline.jsonl')
const START_EPOCH = path.join(DIR, 'web-start-epoch')

const APP_SRC = `${promoAppOrigin()}/#/?lb=quick-win&sc=new%2Cinprogress`
const FRAME_SEL = 'iframe[data-testid="dashboard-frame"]'

/**
 * Flagship vertical split. tokens/dashboards use V_TERM_H=340 (CLI lines).
 * Claude TUI fills 601/640 of the landscape terminal, so the band grows
 * and the web shrinks: 48+520+782 = 1350. Applied on top of promo-split
 * chrome — that file is not edited.
 */
const FLAGSHIP_V_TERM_H = 520
const FLAGSHIP_V_WEB_H = 782

function beat(label: string, extra: Record<string, unknown> = {}): void {
  mkdirSync(DIR, { recursive: true })
  appendFileSync(TIMELINE, JSON.stringify({ t: Date.now(), label, ...extra }) + '\n')
}

async function applyFlagshipVerticalSplit(page: Page): Promise<void> {
  if (!vertical) return
  await page.evaluate(
    ({ termH, webH }) => {
      const term = document.getElementById('term')
      const app = document.getElementById('app') as HTMLIFrameElement | null
      const split = document.getElementById('split')
      if (!term || !app || !split) return
      term.style.height = `${termH}px`
      app.style.height = `${webH}px`
      split.style.height = `${termH + webH}px`
    },
    { termH: FLAGSHIP_V_TERM_H, webH: FLAGSHIP_V_WEB_H, barH: V_BAR_H },
  )
}

// Takes a FrameLocator: the wall lives in the dashboard iframe. (The
// annotation said Locator until 2026-08-25; Playwright transpiles specs
// without typechecking, so the mismatch never surfaced.)
async function chartInView(dash: ReturnType<Page['frameLocator']>): Promise<boolean> {
  const canvas = dash.locator('canvas').first()
  if (await canvas.count()) {
    try {
      return await canvas.evaluate((el) => {
        const r = el.getBoundingClientRect()
        const h = el.ownerDocument.documentElement.clientHeight
        return r.height > 40 && r.top >= 0 && r.bottom <= h + 4
      })
    } catch {
      return false
    }
  }
  const heading = dash.getByText(/per month/i).first()
  if (await heading.count()) {
    try {
      return await heading.evaluate((el) => {
        const r = el.getBoundingClientRect()
        const h = el.ownerDocument.documentElement.clientHeight
        return r.top >= 0 && r.bottom <= h + 4
      })
    } catch {
      return false
    }
  }
  return false
}

/**
 * Wait until the wall actually carries numbers, not just structure.
 *
 * The host pushes datasource results after the frame loads, so a take can
 * open a dashboard whose cards still read 0 — measured 2026-08-25: a
 * claude-dashboards take passed the old contract (chart heading present)
 * on a wall showing 0/0/0/0 and an empty plot, because "a canvas exists"
 * says nothing about data. `dashboard_data` is the beat that means the
 * push landed; the take contract requires it.
 */
async function waitForWallData(dash: ReturnType<Page['frameLocator']>): Promise<boolean> {
  for (let i = 0; i < 30; i++) {
    try {
      const text = await dash.locator('body').innerText({ timeout: 2_000 })
      // Two or more digits with a non-zero lead: a real count, never a 0
      // placeholder and never a stray single digit from a label.
      if (/(?:^|\D)[1-9]\d+(?:\D|$)/.test(text)) {
        beat('dashboard_data', { waitedMs: i * 400 })
        return true
      }
    } catch {
      /* frame still settling */
    }
    await new Promise((r) => setTimeout(r, 400))
  }
  beat('dashboard_data_missing')
  return false
}

/**
 * Click an issue key on the wall and let the app follow it (GDK-854).
 *
 * This is camera work — a viewer's click on a real link, not Claude
 * intervention and not an edit to Claude's document. The frame posts
 * `{type:'open', hash:'#/?issue=KEY'}`, the host validates it and
 * navigates, so the clip ends on the issue the wall pointed at.
 *
 * The key element is whatever Claude authored; it is found by text shape
 * (PROJ-123) among clickable nodes rather than by a class name, and the
 * take is rejected when the wall has no such link — the tape asks for one.
 */
async function clickThroughToIssue(
  page: Page,
  app: ReturnType<Page['frameLocator']>,
): Promise<boolean> {
  const dash = app.frameLocator(FRAME_SEL)
  const KEY_RE = /^[A-Z][A-Z0-9]{1,9}-\d+$/
  const clickable = dash.locator('button, a, [role="button"], [data-key]')
  const n = Math.min(await clickable.count(), 60)
  for (let i = 0; i < n; i++) {
    const el = clickable.nth(i)
    let label = ''
    try {
      label = ((await el.innerText({ timeout: 1_000 })) || '').trim()
    } catch {
      continue
    }
    if (!KEY_RE.test(label)) continue
    try {
      await el.scrollIntoViewIfNeeded({ timeout: 2_000 })
      await page.waitForTimeout(500)
      await el.click({ timeout: 3_000 })
    } catch {
      continue
    }
    const detail = app.getByTestId('issue-detail-panel')
    try {
      await detail.waitFor({ state: 'visible', timeout: 8_000 })
      beat('dashboard_link_nav', { key: label })
      await page.waitForTimeout(2_600) // hold on the issue the wall pointed at
      return true
    } catch {
      beat('dashboard_link_no_detail', { key: label })
      return false
    }
  }
  beat('dashboard_link_missing', { clickables: n })
  return false
}

/** Mouse-wheel camera work. Does not edit Claude's document. */
async function revealChart(page: Page, app: ReturnType<Page['frameLocator']>): Promise<void> {
  const dashFrame = app.locator(FRAME_SEL)
  await dashFrame.waitFor({ state: 'visible', timeout: 15_000 })
  await waitForWallData(app.frameLocator(FRAME_SEL))
  if (vertical) {
    try {
      await applyVerticalDashboardLayout(page)
    } catch {
      /* Claude's HTML may not use the example's class names */
    }
  }
  await page.waitForTimeout(700)
  const box = await dashFrame.boundingBox()
  if (box) {
    await page.mouse.move(box.x + box.width * 0.55, box.y + box.height * 0.62)
    await page.waitForTimeout(120)
  }
  const dash = app.frameLocator(FRAME_SEL)
  for (let i = 0; i < 14; i++) {
    if (await chartInView(dash)) {
      beat('dashboard_chart_visible', { wheels: i })
      await afterChart(page, app)
      return
    }
    if (box) {
      await page.mouse.wheel(0, 220)
      await page.waitForTimeout(180)
    } else {
      break
    }
  }
  beat('dashboard_chart_visible', { wheels: 14, fallback: true })
  await afterChart(page, app)
}

/**
 * The closing beat of the dashboards cut: hold on the chart, then follow a
 * key off the wall into the issue. Only that clip asks Claude for links, so
 * the flagship take is left exactly as it was.
 */
async function afterChart(page: Page, app: ReturnType<Page['frameLocator']>): Promise<void> {
  if (CLIP !== 'claude-dashboards') return
  await page.waitForTimeout(1_400)
  await clickThroughToIssue(page, app)
}

test.describe('claude-drive web', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')
  test.describe.configure({ timeout: 600_000 })

  test('record serve tab until VHS stop file', async ({ page }) => {
    mkdirSync(DIR, { recursive: true })
    writeFileSync(TIMELINE, '')
    writeFileSync(START_EPOCH, `${Date.now() / 1000}\n`)

    await forceLocale(page, 'en')
    await page.emulateMedia({ colorScheme: 'light' })
    await page.setContent(
      promoFrameHtml(
        APP_SRC,
        vertical ? undefined : { webW: FLAGSHIP_L_WEB_W, webH: FLAGSHIP_L_WEB_H },
      ),
      { waitUntil: 'domcontentloaded' },
    )
    await applyFlagshipVerticalSplit(page)

    const app = page.frameLocator('#app')
    await expect(app.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect(app.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(app.getByTestId('filter-chip').filter({ hasText: /quick-win/ })).toBeVisible({
      timeout: 15_000,
    })

    writeFileSync(READY, `${Date.now() / 1000}\n`)
    beat('web_ready')
    await page.waitForTimeout(800)

    const t0 = Date.now()
    let lastAccent = ''
    let lastChip = ''
    let sawDash = false
    const deadline = t0 + 9 * 60 * 1000

    while (!existsSync(STOP) && Date.now() < deadline) {
      let frame
      try {
        frame = appFrame(page)
      } catch {
        frame = null
      }
      if (frame) {
        try {
          const accent = (
            await frame.evaluate(() =>
              getComputedStyle(document.documentElement).getPropertyValue('--color-accent').trim().toLowerCase(),
            )
          ).trim()
          if (accent && accent !== lastAccent) {
            if (lastAccent) beat('accent_changed', { from: lastAccent, to: accent })
            lastAccent = accent
          }
        } catch {
          /* frame navigated */
        }
        try {
          const chip = app.locator('[data-col="labels"] button').filter({ hasText: 'quick-win' }).first()
          if (await chip.count()) {
            const bg = await chip.evaluate((el) => getComputedStyle(el).backgroundColor)
            if (bg && bg !== lastChip) {
              if (lastChip) beat('label_changed', { from: lastChip, to: bg })
              lastChip = bg
            }
          }
        } catch {
          /* list not in view */
        }
        try {
          const dash = app.locator(FRAME_SEL)
          const n = await dash.count()
          if (n > 0 && !sawDash) {
            sawDash = true
            beat('dashboard_open')
            await revealChart(page, app)
          }
        } catch {
          /* still the list */
        }
      }
      await page.waitForTimeout(400)
    }

    beat('web_stop', { sawDash, lastAccent, lastChip, waitedMs: Date.now() - t0 })
    await page.waitForTimeout(1500)
  })
})
