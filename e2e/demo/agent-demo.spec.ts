/**
 * Agent-focus promo for docs/media/agent.{gif,mp4}.
 *
 * One recording, two panes: a paper terminal types `gadak views open --jql …`
 * while the real app sits underneath. On Enter the test runs that command
 * against the serve fixture; the iframe polls ui-focus and the chips land.
 * No live model, no credential, no DOM caption burned into the app.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1024×808
 * (see agent.config.ts) or Playwright letterboxes the capture.
 */
import { spawnSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type Page } from '@playwright/test'

import { forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA
const here = path.dirname(fileURLToPath(import.meta.url))
const gadakBin = path.join(here, '../.tmp/gadak')
const gadakHome = path.join(here, '../.tmp/home')

const COMMAND = `gadak views open --jql 'project = NMA AND statusCategory = "In Progress"'`
const OUTPUT = 'hash\tpj=NMA&sc=inprogress'

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

  test('views open snaps the paper list', async ({ page }) => {
    await forceLocale(page, 'en')
    await page.setContent(FRAME, { waitUntil: 'domcontentloaded' })

    const app = page.frameLocator('#app')
    await expect(app.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    await expect(app.getByTestId('issue-list-scroller')).toBeVisible()
    await page.waitForTimeout(700)

    await typeCommand(page, COMMAND, 48)
    await page.locator('#cursor').evaluate((el) => el.classList.add('off'))
    await page.waitForTimeout(180)

    const ran = spawnSync(
      gadakBin,
      [
        'views',
        'open',
        '--jql',
        `project = NMA AND statusCategory = "In Progress"`,
        '--no-open',
      ],
      { env: { ...process.env, GADAK_HOME: gadakHome }, encoding: 'utf8' },
    )
    if (ran.status !== 0) {
      throw new Error(`views open failed: ${ran.stderr || ran.stdout}`)
    }
    await page.locator('#out').evaluate((el, text) => {
      el.textContent = text
    }, OUTPUT)

    await expect(app.getByTestId('filter-chip').filter({ hasText: 'NMA' })).toBeVisible({
      timeout: 5_000,
    })
    await expect(app.getByTestId('filter-chip').filter({ hasText: 'In Progress' })).toBeVisible()
    await expect(app.getByTestId('list-count')).not.toHaveText(/534/)
    await expect(app.getByTestId('sidebar-jira-filter')).toBeVisible()
    await page.waitForTimeout(3600)
  })
})
