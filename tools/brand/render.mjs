/**
 * Rasterizes gadak brand assets from the strand mark.
 *
 * Usage: node tools/brand/render.mjs
 *
 * Writes:
 *   docs/media/logo.png
 *   docs/media/wordmark-light.png
 *   docs/media/wordmark-dark.png
 *   docs/media/og.png
 *   web/public/icon-16.png
 *   web/public/icon-32.png
 *   web/public/icon-512.png
 *   web/public/apple-touch-icon.png
 */
import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '../..')

const PAPER = '#f4efe4'
const TILE = '#ebe3d2' /* a shade deeper so a Dock tile is not cream-on-cream */
const INK = '#1c1812'
const MUTED = '#635a4f'
const STRAND = '#2e4560'
const STRAND_BACK = '#8a7a62'
const DESK = '#e6dcc8'

/** 가, drawn as strands: ㄱ is the one you follow, ㅏ the other thread. */
const ga = (stroke = STRAND, vowel = STRAND_BACK, sw = 2.35, vw = 1.9) => `
  <g transform="translate(-0.6 0.2)">
    <path d="M4.6 6.8c4.4-.6 8.6-.2 10.8 1.2c.25 3.6.2 7.4.1 11.4" fill="none" stroke="${stroke}" stroke-width="${sw}" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M18.7 5.8c.04 4.6.04 9 0 13.6" fill="none" stroke="${vowel}" stroke-width="${vw}" stroke-linecap="round"/>
    <path d="M18.7 12.2h3.7" fill="none" stroke="${vowel}" stroke-width="${vw}" stroke-linecap="round"/>
  </g>`

/** 16px favicon: 가 collapses, so just the ㄱ. */
const giyeok = (stroke = STRAND, sw = 2.5) => `
  <path d="M5.2 6.6c5-.7 9.6-.2 12.2 1.4c.3 4 .25 8.2.1 12.4" fill="none" stroke="${stroke}" stroke-width="${sw}" stroke-linecap="round"/>`

const tile = (size, svgPx, top, svg) => page(
  size,
  size,
  `<body style="background:${TILE}">
    <svg viewBox="0 0 24 24" width="${svgPx}" height="${svgPx}"
         style="position:absolute;left:${(size - svgPx) / 2}px;top:${top}px">
      ${svg}
    </svg>
  </body>`,
)

const page = (w, h, body, extra = '') => `<!doctype html><meta charset="utf-8">
<style>
  * { box-sizing: border-box; margin: 0; }
  html, body { width:${w}px; height:${h}px; overflow:hidden; background:transparent; }
  ${extra}
</style>${body}`

const jobs = [
  { path: 'docs/media/logo.png', w: 1024, h: 1024, html: tile(1024, 760, 132, ga(STRAND, STRAND_BACK, 2.2, 1.8)) },
  { path: 'web/public/icon-512.png', w: 512, h: 512, html: tile(512, 380, 66, ga(STRAND, STRAND_BACK, 2.25, 1.85)) },
  { path: 'web/public/apple-touch-icon.png', w: 180, h: 180, html: tile(180, 134, 23, ga(STRAND, STRAND_BACK, 2.35, 1.95)) },
  { path: 'web/public/icon-32.png', w: 32, h: 32, html: tile(32, 24, 4, ga(STRAND, STRAND_BACK, 2.6, 2.2)) },
  { path: 'web/public/icon-16.png', w: 16, h: 16, html: tile(16, 12, 2, giyeok(STRAND, 2.8)) },
  {
    path: 'docs/media/wordmark-light.png',
    w: 760,
    h: 144,
    html: page(
      760,
      144,
      `<body style="background:${PAPER};display:flex;align-items:center;padding:0 24px;gap:20px">
        <svg viewBox="0 0 24 24" width="72" height="72">${ga()}</svg>
        <span style="font:600 84px/1 ui-serif,'Iowan Old Style',Palatino,Georgia,serif;letter-spacing:-0.03em;color:${INK}">gadak</span>
      </body>`,
    ),
  },
  {
    path: 'docs/media/wordmark-dark.png',
    w: 760,
    h: 144,
    html: page(
      760,
      144,
      `<body style="background:${INK};display:flex;align-items:center;padding:0 24px;gap:20px">
        <svg viewBox="0 0 24 24" width="72" height="72">${ga('#c5d0dc', '#8a7d68')}</svg>
        <span style="font:600 84px/1 ui-serif,'Iowan Old Style',Palatino,Georgia,serif;letter-spacing:-0.03em;color:${PAPER}">gadak</span>
      </body>`,
    ),
  },
  {
    path: 'docs/media/og.png',
    w: 1280,
    h: 640,
    html: page(
      1280,
      640,
      `<body>
        <svg viewBox="0 0 24 24" width="280" height="280" style="position:absolute;right:72px;top:48px;opacity:.9">
          ${ga(STRAND, STRAND_BACK, 1.7, 1.4)}
        </svg>
        <div class="copy">
          <div class="name">gadak</div>
          <h1>Same Jira. No waiting.</h1>
          <p>Your team’s Jira — and its Confluence wiki —
          mirrored into one local SQLite file.</p>
          <div class="proof">
            <span>17 ms reads</span>
            <span>GROUP BY in one query</span>
            <span>agents speak SQL to it</span>
          </div>
        </div>
      </body>`,
      `
      body { background:${DESK}; color:${INK};
             font:16px/1.45 -apple-system,'Segoe UI',Roboto,sans-serif; }
      .copy { position:absolute; left:88px; top:178px; max-width:760px; }
      .name { font:600 28px/1 ui-serif,'Iowan Old Style',Palatino,Georgia,serif;
              letter-spacing:-0.03em; color:${STRAND}; }
      h1 { font:600 72px/1.05 ui-serif,'Iowan Old Style',Palatino,Georgia,serif;
           letter-spacing:-0.03em; margin:18px 0 22px; }
      p { font-size:26px; color:${MUTED}; max-width:700px; }
      .proof { display:flex; gap:40px; margin-top:36px; font-size:20px;
               color:${MUTED}; white-space:nowrap; }
      `,
    ),
  },
]

const browser = await chromium.launch()
for (const job of jobs) {
  const out = join(root, job.path)
  mkdirSync(dirname(out), { recursive: true })
  const tab = await browser.newPage({
    viewport: { width: job.w, height: job.h },
    deviceScaleFactor: 1,
  })
  await tab.setContent(job.html)
  await tab.screenshot({ path: out })
  await tab.close()
  console.log(job.path)
}
await browser.close()
