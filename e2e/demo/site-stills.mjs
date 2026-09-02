// Landing stills (GDK-751): core-region crops at 2x for gadak.dev exhibits.
//
// The landing replaced full-screen clips with cropped stills for the claims
// that read from one frame (group-by counts, history-as-document, the live
// MCP answer). This script reproduces the two app stills against the running
// e2e serve fixture; the MCP still is an ffmpeg frame of docs/media/mcp.mp4
// (see docs/project/MEDIA.md → "Landing stills").
//
// Usage:
//   GADAK_FRESHEN=1 bash e2e/serve.sh &      # the standard :7877 fixture
//   node e2e/demo/site-stills.mjs            # writes docs/media/*-still.png
//
// Crops are CSS-pixel rects on a 1280×800 viewport at deviceScaleFactor 2,
// so the committed PNGs are 2x and the landing shows them at 1x width
// (MediaSlot displayWidth) — recorded glyph size, retina-sharp.
import { chromium } from 'playwright'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const out = join(root, 'docs', 'media')
const base = process.env.GADAK_STILLS_BASE || 'http://127.0.0.1:7877'

const browser = await chromium.launch()
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 2,
  colorScheme: 'light',
  locale: 'en-US',
})
const page = await ctx.newPage()

// --- groupby-still: Epics view, assignee filter submenu with live counts ---
await page.goto(base + '/#/')
await page.getByTestId('issue-list-scroller').waitFor({ timeout: 30000 })
await page.waitForTimeout(800)
await page.getByText('Epics', { exact: true }).first().click()
await page.waitForTimeout(1200)
await page.getByText('+ Filter', { exact: true }).first().click()
await page.waitForTimeout(400)
await page.getByText('Assignee', { exact: true }).first().click()
await page.waitForTimeout(600)
// 2026-09-02: the "+ Filter" popover moved from the chip row to the toolbar's
// right half (GDK-1343 toolbar). The frame starts at the title column (whole
// titles beat a whole toolbar) and just under the toolbar, so the assignee
// submenu with its counts floats over the epic-grouped rows.
await page.screenshot({
  path: join(out, 'groupby-still.png'),
  clip: { x: 452, y: 44, width: 828, height: 555 },
})
await page.keyboard.press('Escape')
await page.keyboard.press('Escape')

// --- history-still: NMB-139 thread — badges, bot comment, changelog -------
// NMB-139 needs the serve.sh enrichment (agent comment + linked PR).
await page.keyboard.press('Meta+k')
await page.keyboard.type('NMB-139', { delay: 40 })
await page.waitForTimeout(900)
await page.keyboard.press('Enter')
const panel = page.getByTestId('issue-detail-panel')
await panel.waitFor({ timeout: 15000 })
await page.waitForTimeout(1200)
const scroller = panel.getByTestId('detail-scroll')
// Land the frame on: agent comment (Bot badge) + HISTORY with the Reopened
// marker, under the sticky header's duration chip. The 0.20 detail is longer
// (breadcrumb, composer), so the viewport grows to 900 for this frame and
// the scroll is anchored on the COMMENTS heading instead of a fraction.
await page.setViewportSize({ width: 1280, height: 900 })
await page.waitForTimeout(400)
const heading = panel.getByText('Comments', { exact: true }).first()
// The header (key row, breadcrumb, title, badges, durations) sits above the
// scroller, not over it, so block:'start' lands the heading right under it.
await heading.evaluate((el) => el.scrollIntoView({ block: 'start' }))
await page.waitForTimeout(500)
await page.screenshot({
  path: join(out, 'history-still.png'),
  clip: { x: 842, y: 15, width: 438, height: 870 },
})

await browser.close()
console.log('site-stills: wrote groupby-still.png, history-still.png →', out)
