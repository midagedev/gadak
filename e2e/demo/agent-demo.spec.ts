/**
 * Agent-focus promo for docs/media/agent.{gif,mp4}.
 *
 * One recording, two panes, two beats: a paper terminal types a command while
 * the real app sits underneath, and on Enter the test runs it against the
 * serve fixture — the iframe polls ui-focus and the list follows.
 *
 *   1. SQL   `gadak sql --no-header … | gadak views open --keys -`
 *            → an arbitrary answer becomes the view (a `5 keys` chip).
 *   2. JQL   `gadak views open --jql '…'`
 *            → the query a Jira user already knows becomes chips
 *              (project, priority, and unresolved, from one clause each).
 *
 * Two beats because the two are different promises. The first says any answer
 * you can compute is a view; the second says you do not have to leave JQL to
 * get one. A reader who only saw the pipe would think gadak needs SQL.
 *
 * No live model, no credential, no DOM caption burned into the app.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1024×808
 * (see agent.config.ts) or Playwright letterboxes the capture.
 */
import { spawnSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type Page } from '@playwright/test'

import { forceLocale, DEMO_ISSUE_COUNT_EN_RE } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA
const here = path.dirname(fileURLToPath(import.meta.url))
const gadakBin = path.join(here, '../.tmp/gadak')
const gadakHome = path.join(here, '../.tmp/home')

// "Stuck the longest in progress" — a universal question (every team has
// in-progress work; not every team reopens issues), and one JQL cannot rank
// this way. --no-header keeps the header row from becoming a fake key.
const SQL =
  "select key from issues where status_category='inprogress' order by status_changed_at asc limit 5"
const COMMAND = `gadak sql --no-header "${SQL}" \\\n| gadak views open --keys -`

// The same JQL a Jira user would paste into the navigator. `resolution is
// EMPTY` is the interesting clause: it has no chip of its own and lands as
// the two open status categories (decision 0007).
const JQL = 'project = NMA AND priority = High AND resolution is EMPTY'
const JQL_COMMAND = `gadak views open --jql \\\n  '${JQL}'`

const FRAME = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <style>
    html, body { margin: 0; padding: 0; background: #f4efe4; }
    body {
      width: 1024px; height: 808px; overflow: hidden;
      font-family: "D2Coding", Menlo, Monaco, ui-monospace, monospace;
    }
    #chrome {
      height: 168px; box-sizing: border-box;
      border-bottom: 1px solid #d5cbb8;
    }
    #bar {
      height: 32px; display: flex; align-items: center; gap: 7px;
      padding: 0 14px; background: #e8e0d0;
    }
    #bar i { width: 11px; height: 11px; border-radius: 50%; display: block; }
    #bar i:nth-child(1) { background: #e25d4a; }
    #bar i:nth-child(2) { background: #e0b040; }
    #bar i:nth-child(3) { background: #5aa05a; }
    #bar span { margin-left: 8px; font-size: 12px; color: #635a4f; letter-spacing: 0.01em; }
    #term {
      height: 136px; box-sizing: border-box;
      padding: 16px 20px 12px;
      color: #1c1812; font-size: 17px; line-height: 1.45;
      background: #f4efe4;
    }
    #line { white-space: pre-wrap; }
    #prompt { color: #2e4560; }
    #cursor {
      display: inline-block; width: 9px; height: 18px;
      margin-left: 1px; background: #2e4560; vertical-align: -3px;
      animation: blink 1s steps(1) infinite;
    }
    #cursor.off { visibility: hidden; animation: none; }
    #out { margin-top: 8px; color: #1c1812; min-height: 1.45em; }
    @keyframes blink { 50% { opacity: 0; } }
    iframe { width: 1024px; height: 640px; border: 0; display: block; background: #f4efe4; }
  </style>
</head>
<body>
  <div id="chrome">
    <div id="bar"><i></i><i></i><i></i><span>gadak</span></div>
    <div id="term">
      <div id="line"><span id="prompt">$ </span><span id="typed"></span><span id="cursor"></span></div>
      <div id="out"></div>
    </div>
  </div>
  <iframe id="app" src="http://127.0.0.1:7877/" title="gadak"></iframe>
</body>
</html>`

async function typeCommand(page: Page, text: string, delayMs: number): Promise<void> {
  const typed = page.locator('#typed')
  for (const ch of text) {
    await typed.evaluate((el, c) => {
      el.textContent = (el.textContent ?? '') + c
    }, ch)
    await page.waitForTimeout(delayMs)
  }
}

test.describe('agent focus demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('sql keys, then jql chips, both land on the paper list', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.setContent(FRAME, { waitUntil: 'domcontentloaded' })

    const app = page.frameLocator('#app')
    await expect(app.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    await expect(app.getByTestId('issue-list-scroller')).toBeVisible()
    await page.waitForTimeout(700)

    await typeCommand(page, COMMAND, 48)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)

    const ran = spawnSync(
      'bash',
      [
        '-c',
        // Double quotes: the SQL contains single-quoted literals ('inprogress').
        `"${gadakBin}" sql --no-header "${SQL}" | "${gadakBin}" views open --keys - --no-open`,
      ],
      { env: { ...process.env, GADAK_HOME: gadakHome }, encoding: 'utf8' },
    )
    if (ran.status !== 0) {
      throw new Error(`sql | views open --keys failed: ${ran.stderr || ran.stdout}`)
    }
    // Print only the hash line. The command also emits web\thttp://… when a
    // serve tab is listening — that URL must not land in a public frame.
    const hashLine =
      (ran.stdout || '').split('\n').find((line) => line.startsWith('hash\t')) ?? ''
    if (!hashLine.startsWith('hash\tks=')) {
      throw new Error(`expected hash\\tks=… from views open, got: ${ran.stdout}`)
    }
    await page.locator('#out').evaluate((el, text) => {
      el.textContent = text
    }, hashLine)

    await expect(app.getByTestId('filter-chip').filter({ hasText: /5 keys/ })).toBeVisible({
      timeout: 5_000,
    })
    await expect(app.getByTestId('list-count')).toHaveText('5 issues')
    await page.waitForTimeout(2200)

    // ── Beat 2: the same handoff, starting from JQL instead of SQL ──────────
    await page.locator('#typed').evaluate((el) => {
      el.textContent = ''
    })
    await page.locator('#out').evaluate((el) => {
      el.textContent = ''
    })
    await page.locator('#cursor').evaluate((el) => el.classList.remove('off'))
    await page.waitForTimeout(400)

    await typeCommand(page, JQL_COMMAND, 34)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)

    const ranJql = spawnSync(
      'bash',
      ['-c', `"${gadakBin}" views open --jql '${JQL}' --no-open`],
      { env: { ...process.env, GADAK_HOME: gadakHome }, encoding: 'utf8' },
    )
    if (ranJql.status !== 0) {
      throw new Error(`views open --jql failed: ${ranJql.stderr || ranJql.stdout}`)
    }
    const jqlHash =
      (ranJql.stdout || '').split('\n').find((line) => line.startsWith('hash\t')) ?? ''
    if (!jqlHash.startsWith('hash\tpj=')) {
      throw new Error(`expected hash\\tpj=… from views open --jql, got: ${ranJql.stdout}`)
    }
    await page.locator('#out').evaluate((el, text) => {
      el.textContent = text
    }, jqlHash)

    // One clause each: the project, the priority, and the pair of open status
    // categories that `resolution is EMPTY` becomes.
    const chips = app.getByTestId('filter-chip')
    await expect(chips.filter({ hasText: /NMA/ })).toBeVisible({ timeout: 5_000 })
    await expect(chips.filter({ hasText: /High/ })).toBeVisible()
    await expect(chips.filter({ hasText: /5 keys/ })).toHaveCount(0)
    await page.waitForTimeout(3400)
  })
})
