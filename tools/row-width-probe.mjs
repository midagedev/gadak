#!/usr/bin/env node
/*
 * Row width probe (GDK-1046 follow-up, 2026-08-27). One command that
 * answers "where did this row's width go": dumps the width allocation of
 * the issue-list rows — leading parts, title, every trailing slot with its
 * paint vs the list scroller's right edge — plus which container-query
 * thresholds fired for the measured row width. That split (geometry +
 * fired-thresholds) is what tracking a "column painted outside the row"
 * report actually needs; before this it was a hand-written Playwright spec
 * per incident (the GDK-1046 probe-specs round).
 *
 * Diagnostic, not a gate: nothing in CI runs this. It always starts its own
 * `gadak serve` on its own port (default 7898, ROW_PROBE_PORT) via
 * e2e/serve.sh — cold and fresh on purpose, because a reused server can be
 * serving a stale bundle and width rules live in that bundle. First start
 * builds the binary + UI and can take a couple of minutes.
 *
 * Usage:
 *   node tools/row-width-probe.mjs                          # 1280, panel closed
 *   node tools/row-width-probe.mjs --viewport 1440 --issue NMB-110
 *   node tools/row-width-probe.mjs --cl updated,labels --json
 *   node tools/row-width-probe.mjs --help
 *
 * Exit 0 ran (geometry printed) · 1 harness failure · 2 usage.
 */
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { chromium } from '@playwright/test'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const PORT = process.env.ROW_PROBE_PORT || '7898'
const BASE = `http://127.0.0.1:${PORT}`
const DEFAULT_CL = 'assignee,updated,labels,reopen,stale,qa_impact,deploy'

// Mirrors web/src/app.css @container issuerow thresholds — keep in step.
const THRESHOLDS = [
  ['stale-glyph hidden', 1100],
  ['qa_impact column dropped', 1000],
  ['epic column dropped', 750],
  ['trail-fold-1 (assignee, updated)', 620],
  ['trail-fold-2 (labels)', 480],
  ['trail-fold-3 (reopen, deploy)', 400],
]

const usage = () => {
  console.log(`usage: node tools/row-width-probe.mjs [--viewport N] [--issue KEY] [--cl a,b,c] [--rows N] [--json]

Dumps the issue-list row width allocation — leading parts, title, trailing
slots with px past the scroller edge, and which @container issuerow
thresholds fired — against a cold \`gadak serve\` on :${PORT}
(ROW_PROBE_PORT). --issue opens the detail panel via the URL place param
(the shared-link path); --cl pins the column set; --rows limits the dump.

exit 0 ran · 1 harness failure · 2 usage`)
}

function die(code, msg) {
  console.error(`[row-width] ${msg}`)
  process.exit(code)
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

/** Poll until fn() returns truthy; state-based, never a bare sleep. */
async function waitFor(what, fn, timeoutMs = 30_000, intervalMs = 200) {
  const t0 = Date.now()
  for (;;) {
    const v = await fn()
    if (v) return v
    if (Date.now() - t0 > timeoutMs) throw new Error(`timeout waiting for ${what}`)
    await sleep(intervalMs)
  }
}

async function startServer() {
  const child = spawn('bash', [join(ROOT, 'e2e', 'serve.sh')], {
    cwd: ROOT,
    env: { ...process.env, GADAK_E2E_PORT: PORT },
    stdio: ['ignore', 'inherit', 'inherit'],
  })
  const stop = () => {
    try {
      child.kill('SIGTERM')
    } catch {
      /* already gone */
    }
  }
  process.on('SIGINT', () => {
    stop()
    process.exit(130)
  })
  await waitFor(
    `serve on :${PORT} (building binary + UI first — this can take a minute)`,
    async () => {
      try {
        const r = await fetch(`${BASE}/healthz`)
        return r.ok
      } catch {
        return false
      }
    },
    240_000,
    500,
  )
  return stop
}

function parseArgs(argv) {
  const out = { viewport: 1280, issue: null, cl: DEFAULT_CL, rows: 6, json: false }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    if (a === '-h' || a === '--help') {
      usage()
      process.exit(0)
    }
    if (a === '--json') {
      out.json = true
      continue
    }
    if (a === '--viewport' || a === '--issue' || a === '--cl' || a === '--rows') {
      const v = argv[++i]
      if (v === undefined) die(2, `${a} needs a value`)
      if (a === '--viewport') out.viewport = Number(v)
      else if (a === '--issue') out.issue = v
      else if (a === '--cl') out.cl = v
      else out.rows = Number(v)
      continue
    }
    die(2, `unknown argument ${a} (--help for usage)`)
  }
  if (!Number.isInteger(out.viewport) || out.viewport < 320 || out.viewport > 10000) {
    die(2, `--viewport must be an integer 320-10000, got ${out.viewport}`)
  }
  return out
}

async function measure(page, opts) {
  return page.evaluate(
    ({ maxRows }) => {
      const round = (n) => Math.round(n * 10) / 10
      const scroller = document.querySelector('[data-testid="issue-list-scroller"]')
      if (!scroller) return { error: 'no [data-testid="issue-list-scroller"]' }
      const sRight = scroller.getBoundingClientRect().right
      const rows = []
      let rowW = null
      for (const row of scroller.querySelectorAll('[data-issue-key]')) {
        const r = row.getBoundingClientRect()
        if (r.bottom < 0 || r.top > innerHeight) continue
        if (rowW === null) rowW = round(r.width)
        const title = row.querySelector('span.flex-1.truncate')
        const slots = []
        const trail = row.querySelector('[data-testid="issue-row-trail"]')
        if (trail) {
          for (const slot of [...trail.children]) {
            const s = getComputedStyle(slot)
            if (s.display === 'none' || s.visibility === 'hidden') continue
            const b = slot.getBoundingClientRect()
            if (b.width < 1) continue
            slots.push({
              col: slot.dataset.col ?? '(no data-col)',
              left: round(b.left),
              right: round(b.right),
              w: round(b.width),
              pastScroller: round(Math.max(0, b.right - sRight)),
            })
          }
        }
        rows.push({
          key: row.dataset.issueKey ?? '',
          titleW: title ? round(title.clientWidth) : null,
          titleTruncated: title ? title.scrollWidth > title.clientWidth + 1 : null,
          slots,
        })
        if (rows.length >= maxRows) break
      }
      const layout = document.querySelector('[data-testid="issue-layout"]')
      return {
        viewportW: innerWidth,
        regime: layout?.getAttribute('data-viewport-regime') ?? null,
        detailOpen: layout?.getAttribute('data-detail-open') ?? null,
        rowW,
        scrollerRight: round(sRight),
        rows,
      }
    },
    { maxRows: opts.rows },
  )
}

async function main() {
  const opts = parseArgs(process.argv.slice(2))

  const stopServer = await startServer()
  const browser = await chromium.launch()
  try {
    const page = await browser.newPage({ viewport: { width: opts.viewport, height: 900 } })
    await page.addInitScript(() => {
      try {
        if (!localStorage.getItem('gadak_locale')) localStorage.setItem('gadak_locale', 'en')
      } catch {
        /* ignore */
      }
    })
    const q = [`cl=${opts.cl}`]
    if (opts.issue) q.push(`issue=${opts.issue}`)
    await page.goto(`${BASE}/#/?${q.join('&')}`)
    await page.getByTestId('issue-layout').waitFor({ state: 'visible', timeout: 30_000 })
    await waitFor('first rendered issue row', () =>
      page
        .getByTestId('issue-list-scroller')
        .locator('[data-issue-key]')
        .first()
        .isVisible(),
    )
    if (opts.issue) {
      await page
        .locator('[data-testid="issue-layout"][data-detail-open="true"]')
        .waitFor({ state: 'visible' })
      // Panel width locks immediately; the slide is 160ms — wait on the
      // resting grid track, not a duration (narrow-clip.spec.ts idiom).
      await waitFor('resting panel width', async () => {
        const w = await page
          .getByTestId('issue-detail-panel')
          .evaluate((el) => Math.round(el.getBoundingClientRect().width))
        return w > 400
      })
    }

    const data = await measure(page, opts)
    if (data.error) throw new Error(data.error)
    const fired = THRESHOLDS.filter(([, px]) => data.rowW !== null && data.rowW <= px)
    const maxPast = Math.max(
      0,
      ...data.rows.flatMap((r) => r.slots.map((s) => s.pastScroller)),
    )

    if (opts.json) {
      console.log(JSON.stringify({ ...data, firedThresholds: fired.map((f) => f[0]), maxPast }, null, 1))
    } else {
      console.log(`[row-width] viewport ${data.viewportW} regime=${data.regime} detailOpen=${data.detailOpen}`)
      console.log(`[row-width] rowW=${data.rowW} scrollerRight=${data.scrollerRight} maxPastScroller=${maxPast}`)
      console.log(
        `[row-width] fired @container issuerow thresholds: ${fired.length ? fired.map((f) => `${f[0]} (≤${f[1]})`).join('; ') : 'none'}`,
      )
      for (const r of data.rows) {
        const slots = r.slots
          .map((s) => `${s.col} w=${s.w}${s.pastScroller > 0 ? ` PAST+${s.pastScroller}` : ''}`)
          .join(' · ')
        console.log(
          `[row-width] ${r.key} title=${r.titleW}${r.titleTruncated ? '!' : ''} | ${slots}`,
        )
      }
      if (maxPast > 0) {
        console.log(
          `[row-width] NOTE: ${maxPast}px painted past the scroller — e2e/list-row-overflow.spec.ts is the gate for this axis`,
        )
      }
    }
  } finally {
    await browser.close().catch(() => {})
    stopServer()
  }
  return 0
}

main().then(
  () => process.exit(0),
  (err) => die(1, err?.stack ?? String(err)),
)
