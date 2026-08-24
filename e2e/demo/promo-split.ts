/**
 * Shared paper-split chrome for the tokens / dashboards promo recordings.
 *
 * Sibling of e2e/demo/agent-demo.spec.ts: a paper terminal types a command,
 * spawnSync runs that same string against the serve fixture, and the iframe
 * is the real app. Default layout is left terminal / right web (the promo
 * contract). GADAK_PROMO_LAYOUT=vertical stacks bar + terminal + web at
 * 1080×1350 without changing the landscape signature or markup.
 *
 * Commands are real. Stdout/stderr shown in the terminal is gadak's own.
 * Loopback `web\t` / `deeplink\t` lines are dropped so a 127.0.0.1 URL does
 * not land in a public frame (same filter as agent-demo).
 */
import { spawnSync } from 'node:child_process'
import { writeFileSync, mkdirSync, appendFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { type Page, type Frame } from '@playwright/test'

const here = path.dirname(fileURLToPath(import.meta.url))
export const E2E_DIR = path.join(here, '..')
export const REPO_ROOT = path.join(E2E_DIR, '..')

/** Dedicated promo port so a leftover :7877 e2e suite is not this take. */
export const PROMO_PORT = process.env.GADAK_E2E_PORT || '7888'

export const TERM_W = 720
export const WEB_W = 1024
export const BAR_H = 32
export const WEB_H = 640
export const FRAME_W = TERM_W + WEB_W
export const FRAME_H = BAR_H + WEB_H

/**
 * Flagship landscape (claude-drive) only. tokens/dashboards keep WEB_W/WEB_H
 * so they still record 1744×672. 1160 and 688 are even (h264 yuv420p) and
 * sit at the top of the crop-fix range 1100–1160 / 670–690: last x-tick,
 * card corner, page margin, and legend padding were clipped at 1024×640.
 * Frame = (TERM_W + FLAGSHIP_L_WEB_W) × (BAR_H + FLAGSHIP_L_WEB_H) = 1880×720.
 */
export const FLAGSHIP_L_WEB_W = 1160
export const FLAGSHIP_L_WEB_H = 688
export const FLAGSHIP_L_FRAME_W = TERM_W + FLAGSHIP_L_WEB_W
export const FLAGSHIP_L_FRAME_H = BAR_H + FLAGSHIP_L_WEB_H

/**
 * Vertical (4:5) social cut. Same paper chrome, stacked: mac bar, terminal
 * band, web panel. 48+340+962 = 1350. GADAK_PROMO_LAYOUT=vertical selects
 * this; unset keeps the landscape path byte-identical.
 *
 * V_TERM_H started at 260 (spec table). Dashboards `save` wraps to 9 lines
 * at 20px/1080 and the last datasource line plus `saved\t` were clipped
 * (self-verif 2026-08-24). Grew the band; web height fell by the same 80
 * so the frame stays 1350.
 */
export const V_FRAME_W = 1080
export const V_FRAME_H = 1350
export const V_BAR_H = 48
export const V_TERM_H = 340
export const V_WEB_W = V_FRAME_W
export const V_WEB_H = V_FRAME_H - V_BAR_H - V_TERM_H
export const V_FONT_PX = 20

export function isPromoVertical(): boolean {
  return process.env.GADAK_PROMO_LAYOUT === 'vertical'
}

export function promoBin(): string {
  return path.join(E2E_DIR, '.tmp', 'gadak')
}

export function promoHome(): string {
  return path.join(E2E_DIR, '.tmp', `home-${PROMO_PORT}`)
}

export function promoAppOrigin(): string {
  return `http://127.0.0.1:${PROMO_PORT}`
}

export function promoFrameHtml(
  appSrc: string,
  landscape?: { webW: number; webH: number },
): string {
  if (isPromoVertical()) return promoFrameHtmlVertical(appSrc)
  const webW = landscape?.webW ?? WEB_W
  const webH = landscape?.webH ?? WEB_H
  const frameW = TERM_W + webW
  const frameH = BAR_H + webH
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <style>
    html, body { margin: 0; padding: 0; background: #f4efe4; }
    body {
      width: ${frameW}px; height: ${frameH}px; overflow: hidden;
      font-family: "D2Coding", Menlo, Monaco, ui-monospace, monospace;
    }
    #bar {
      height: ${BAR_H}px; display: flex; align-items: center; gap: 7px;
      padding: 0 14px; background: #e8e0d0; box-sizing: border-box;
      border-bottom: 1px solid #d5cbb8;
    }
    #bar i { width: 11px; height: 11px; border-radius: 50%; display: block; }
    #bar i:nth-child(1) { background: #e25d4a; }
    #bar i:nth-child(2) { background: #e0b040; }
    #bar i:nth-child(3) { background: #5aa05a; }
    #bar span { margin-left: 8px; font-size: 12px; color: #635a4f; letter-spacing: 0.01em; }
    #split { display: flex; height: ${webH}px; }
    #term {
      width: ${TERM_W}px; height: ${webH}px; box-sizing: border-box;
      padding: 14px 16px 12px;
      color: #1c1812; font-size: 15px; line-height: 1.4;
      background: #f4efe4; border-right: 1px solid #d5cbb8;
      overflow: hidden;
    }
    #line { white-space: pre-wrap; word-break: break-word; }
    #prompt { color: #2e4560; }
    #cursor {
      display: inline-block; width: 8px; height: 16px;
      margin-left: 1px; background: #2e4560; vertical-align: -3px;
      animation: blink 1s steps(1) infinite;
    }
    #cursor.off { visibility: hidden; animation: none; }
    #out {
      margin-top: 10px; color: #1c1812; min-height: 1.4em;
      white-space: pre-wrap; word-break: break-word; font-size: 14px;
    }
    #out.err { color: #8f3530; }
    @keyframes blink { 50% { opacity: 0; } }
    iframe#app {
      width: ${webW}px; height: ${webH}px; border: 0; display: block;
      background: #f4efe4;
    }
  </style>
</head>
<body>
  <div id="bar"><i></i><i></i><i></i><span>gadak</span></div>
  <div id="split">
    <div id="term">
      <div id="line"><span id="prompt">$ </span><span id="typed"></span><span id="cursor"></span></div>
      <div id="out"></div>
    </div>
    <iframe id="app" src="${appSrc}" title="gadak"></iframe>
  </div>
</body>
</html>`
}

/**
 * Stacked paper chrome for the 4:5 social cut. Same ids as the landscape
 * frame (#bar/#term/#typed/#cursor/#out/#app) so tokens-demo.spec.ts and
 * dashboards-demo.spec.ts reuse without edits. Chrome tokens (paper fill,
 * traffic-light colours, prompt ink) are the landscape values; sizes scale
 * with the 32→48 bar and 15→20px terminal font.
 */
export function promoFrameHtmlVertical(appSrc: string): string {
  const outFontPx = 19
  const cursorW = 11
  const cursorH = 21
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <style>
    html, body { margin: 0; padding: 0; background: #f4efe4; }
    body {
      width: ${V_FRAME_W}px; height: ${V_FRAME_H}px; overflow: hidden;
      font-family: "D2Coding", Menlo, Monaco, ui-monospace, monospace;
    }
    #bar {
      height: ${V_BAR_H}px; display: flex; align-items: center; gap: 10.5px;
      padding: 0 21px; background: #e8e0d0; box-sizing: border-box;
      border-bottom: 1px solid #d5cbb8;
    }
    #bar i { width: 16.5px; height: 16.5px; border-radius: 50%; display: block; }
    #bar i:nth-child(1) { background: #e25d4a; }
    #bar i:nth-child(2) { background: #e0b040; }
    #bar i:nth-child(3) { background: #5aa05a; }
    #bar span { margin-left: 12px; font-size: 18px; color: #635a4f; letter-spacing: 0.01em; }
    #split { display: flex; flex-direction: column; height: ${V_TERM_H + V_WEB_H}px; }
    #term {
      width: ${V_FRAME_W}px; height: ${V_TERM_H}px; box-sizing: border-box;
      padding: 21px 24px 18px;
      color: #1c1812; font-size: ${V_FONT_PX}px; line-height: 1.4;
      background: #f4efe4; border-bottom: 1px solid #d5cbb8;
      overflow: hidden;
    }
    #line { white-space: pre-wrap; word-break: break-word; }
    #prompt { color: #2e4560; }
    #cursor {
      display: inline-block; width: ${cursorW}px; height: ${cursorH}px;
      margin-left: 1px; background: #2e4560; vertical-align: -4px;
      animation: blink 1s steps(1) infinite;
    }
    #cursor.off { visibility: hidden; animation: none; }
    #out {
      margin-top: 15px; color: #1c1812; min-height: 1.4em;
      white-space: pre-wrap; word-break: break-word; font-size: ${outFontPx}px;
    }
    #out.err { color: #8f3530; }
    @keyframes blink { 50% { opacity: 0; } }
    iframe#app {
      width: ${V_WEB_W}px; height: ${V_WEB_H}px; border: 0; display: block;
      background: #f4efe4;
    }
  </style>
</head>
<body>
  <div id="bar"><i></i><i></i><i></i><span>gadak</span></div>
  <div id="split">
    <div id="term">
      <div id="line"><span id="prompt">$ </span><span id="typed"></span><span id="cursor"></span></div>
      <div id="out"></div>
    </div>
    <iframe id="app" src="${appSrc}" title="gadak"></iframe>
  </div>
</body>
</html>`
}

export async function typeCommand(page: Page, text: string, delayMs: number): Promise<void> {
  const typed = page.locator('#typed')
  for (const ch of text) {
    await typed.evaluate((el, c) => {
      el.textContent = (el.textContent ?? '') + c
    }, ch)
    await page.waitForTimeout(delayMs)
  }
}

export async function clearTerminal(page: Page): Promise<void> {
  await page.locator('#typed').evaluate((el) => {
    el.textContent = ''
  })
  await page.locator('#out').evaluate((el) => {
    el.textContent = ''
    el.classList.remove('err')
  })
  await page.locator('#cursor').evaluate((el) => el.classList.remove('off'))
}

/** Env for a visible gadak child: same home as serve, no unknown GADAK_* (those print on stderr). */
function gadakChildEnv(): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = { ...process.env }
  delete env.GADAK_E2E_PORT
  delete env.GADAK_SEED_DB
  delete env.GADAK_PROMO_LAYOUT
  env.GADAK_HOME = promoHome()
  env.GADAK_NO_OPEN = '1'
  env.PATH = `${path.dirname(promoBin())}:${process.env.PATH ?? ''}`
  return env
}

/** Drop loopback URL lines so they never appear in a public frame. */
export function visibleCliText(stdout: string, stderr: string): { text: string; err: boolean } {
  const keep = (s: string) =>
    s
      .split('\n')
      .filter(
        (line) =>
          line.length > 0 &&
          !line.startsWith('web\t') &&
          !line.startsWith('deeplink\t') &&
          !/ignoring unrecognised GADAK_/.test(line),
      )
      .join('\n')
  const out = keep(stdout)
  const err = keep(stderr)
  if (err && !out) return { text: err, err: true }
  if (err && out) return { text: out + '\n' + err, err: true }
  return { text: out, err: false }
}

export function runTyped(command: string): { status: number; stdout: string; stderr: string } {
  const ran = spawnSync('bash', ['-c', command], {
    cwd: REPO_ROOT,
    env: gadakChildEnv(),
    encoding: 'utf8',
  })
  return {
    status: ran.status ?? 1,
    stdout: ran.stdout || '',
    stderr: ran.stderr || '',
  }
}

export function freezePromoHome(): void {
  const env = gadakChildEnv()
  delete env.GADAK_NO_OPEN
  const ran = spawnSync(promoBin(), ['config', 'set', 'frozen', 'true'], {
    env,
    encoding: 'utf8',
  })
  if (ran.status !== 0) {
    throw new Error(`gadak config set frozen true failed: ${ran.stderr || ran.stdout}`)
  }
}

export function assertFrozen(): void {
  const ran = spawnSync(promoBin(), ['status'], {
    env: { ...process.env, GADAK_HOME: promoHome() },
    encoding: 'utf8',
  })
  const text = `${ran.stdout || ''}\n${ran.stderr || ''}`
  if (!/\bfrozen\b[^\n]*\btrue\b/i.test(text) && !/^frozen\s+true\b/m.test(text)) {
    throw new Error(`capture home is not frozen; status said:\n${text}`)
  }
}

export function appFrame(page: Page): Frame {
  const needle = `127.0.0.1:${PROMO_PORT}`
  const frame = page.frames().find((f) => {
    const u = f.url()
    return u.includes(needle) && !u.includes('/dashboards/')
  })
  if (!frame) throw new Error(`app iframe not found (wanted ${needle})`)
  return frame
}

/**
 * Vertical-only: the 1080 app iframe still has the sidebar, so the nested
 * dashboard document's viewport is ~840px and triage.html's max-width:960px
 * stack fires — 2×2 cards and the uPlot chart drop below the fold (the
 * reason the vertical cut exists). Force the desktop grid the 1080
 * contract assumed. No-op on landscape. Capture chrome only; triage.html
 * is not edited.
 */
export async function applyVerticalDashboardLayout(page: Page): Promise<void> {
  if (!isPromoVertical()) return
  const wall = page.frameLocator('#app').frameLocator('iframe[data-testid="dashboard-frame"]')
  await wall.locator('html').evaluate((html) => {
    const doc = html.ownerDocument
    const id = 'gadak-vertical-dash-desktop'
    let s = doc.getElementById(id)
    if (!s) {
      s = doc.createElement('style')
      s.id = id
      s.textContent =
        '@media (max-width: 960px){' +
        '.cards{grid-template-columns:repeat(4,minmax(120px,1fr))!important;}' +
        'main{grid-template-columns:1.5fr 1fr!important;}' +
        '}' +
        'html,body{height:100%;overflow:hidden;}' +
        'body{display:flex;flex-direction:column;}' +
        'header,.cards,.foot{flex:none;}' +
        'main{flex:1;min-height:0;grid-template-rows:minmax(0,1fr) 190px;align-items:stretch;}' +
        '.chart-panel{min-height:0;overflow:hidden;}' +
        '.panel{overflow:auto;}'
      doc.head.appendChild(s)
    }
    doc.defaultView?.dispatchEvent(new Event('resize'))
  })
}

export async function showCli(page: Page, stdout: string, stderr: string): Promise<void> {
  const vis = visibleCliText(stdout, stderr)
  await page.locator('#out').evaluate(
    (el, payload: { text: string; err: boolean }) => {
      el.textContent = payload.text
      el.classList.toggle('err', payload.err)
    },
    vis,
  )
}

export function writeTrimSeconds(name: 'tokens' | 'dashboards', seconds: number): void {
  mkdirSync(path.join(E2E_DIR, '.tmp'), { recursive: true })
  writeFileSync(path.join(E2E_DIR, '.tmp', `promo-trim-${name}`), seconds.toFixed(2) + '\n')
}

export function timelinePath(name: 'tokens' | 'dashboards'): string {
  return path.join(E2E_DIR, '.tmp', `promo-timeline-${name}.jsonl`)
}

export function beat(name: 'tokens' | 'dashboards', label: string, extra: Record<string, unknown> = {}): void {
  const file = timelinePath(name)
  mkdirSync(path.dirname(file), { recursive: true })
  appendFileSync(file, JSON.stringify({ t: Date.now(), label, ...extra }) + '\n')
}
