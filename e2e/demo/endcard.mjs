/**
 * The round-trip cut's end card, as a PNG (GDK-1159).
 *
 * Why not ffmpeg drawtext: the ffmpeg on this machine is built without
 * libfreetype, so `drawtext` does not exist as a filter — measured, the cut
 * died on "No such filter: 'drawtext'". Playwright is already a dependency of
 * the rig, renders the same font stack the app itself uses, and gives real
 * typography (tracking, weights) instead of one bitmap string per line.
 *
 *   node e2e/demo/endcard.mjs scratch/roundtrip/endcard.png
 *
 * The colours are the take's own: --bg panel black, the paper ink the
 * terminal chrome uses, and the muted tan of the app's secondary text, so the
 * card reads as the last frame of the same film rather than as a slide.
 *
 * The line is the film's G1 sentence, and it has to survive the release-video
 * gate on its own: it makes no claim of absence, needs no product name to
 * parse, and describes what the previous four seconds just showed — three
 * cards crossing a board with nobody's cursor on screen.
 */
import { chromium } from '@playwright/test'
import { resolve } from 'node:path'

const out = resolve(process.argv[2] || 'scratch/roundtrip/endcard.png')

const html = `<!doctype html><meta charset="utf-8"><style>
  html,body{margin:0;height:100%;background:#0e0d0c}
  body{display:flex;flex-direction:column;align-items:center;justify-content:center;
       font-family:ui-sans-serif,-apple-system,"SF Pro Display","Helvetica Neue",sans-serif;
       -webkit-font-smoothing:antialiased}
  .name{font-size:132px;font-weight:700;color:#f4efe4;letter-spacing:-.035em;line-height:1}
  .name em{font-style:normal;color:#b9a98c;font-weight:600}
  .line{margin-top:38px;font-size:46px;color:#b9a98c;letter-spacing:-.012em}
  .url{margin-top:64px;font-size:38px;color:#cfc0a4;letter-spacing:.01em;
       font-family:ui-monospace,Menlo,monospace}
</style>
<div class="name">gadak <em>0.19</em></div>
<div class="line">every terminal knows its issue.</div>
<div class="url">github.com/midagedev/gadak</div>`

const b = await chromium.launch()
const page = await b.newPage({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 })
await page.setContent(html)
await page.waitForTimeout(300)
await page.screenshot({ path: out })
await b.close()
console.log(`endcard: ${out}`)
