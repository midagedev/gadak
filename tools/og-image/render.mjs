/**
 * Renders the GitHub social-preview image (1280x640) with Playwright.
 * The name matters less than the one line — the card says what the tool is.
 *
 * Usage: node tools/og-image/render.mjs [outPath]
 */
import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname } from 'node:path'

const out = process.argv[2] ?? 'tools/og-image/og.png'
mkdirSync(dirname(out), { recursive: true })

const html = `<!doctype html><meta charset="utf-8">
<style>
  * { box-sizing: border-box; margin: 0; }
  body { width:1280px; height:640px; background:#0c0f13; color:#e6e8ec;
         font:16px/1.5 -apple-system,'Segoe UI',Roboto,sans-serif;
         display:flex; flex-direction:column; justify-content:center; padding:0 96px;
         background-image:radial-gradient(ellipse 900px 500px at 85% 15%, #1a2230 0%, transparent 60%); }
  .name { font-size:34px; font-weight:600; letter-spacing:-0.5px; color:#9ba1a9; }
  h1 { font-size:74px; font-weight:700; letter-spacing:-2px; margin:18px 0 26px;
       background:linear-gradient(90deg,#e6e8ec,#8fb8ff); -webkit-background-clip:text; color:transparent; }
  .sub { font-size:30px; color:#9ba1a9; max-width:980px; }
  .surfaces { display:flex; gap:14px; margin-top:44px; }
  .chip { border:1px solid #2a313d; border-radius:10px; padding:10px 22px; font-size:24px; color:#c7cbd2;
          background:#12161c; }
  .chip b { color:#8fb8ff; font-weight:600; }
</style>
<body>
  <div class="name">scry</div>
  <h1>A local-first Jira client</h1>
  <div class="sub">One binary mirrors your issues into SQLite — instant search,
  offline reads, and a database your coding agent can query.</div>
  <div class="surfaces">
    <div class="chip"><b>Web UI</b> keyboard triage</div>
    <div class="chip"><b>TUI</b> never leave tmux</div>
    <div class="chip"><b>CLI + SQL</b> agent-native</div>
  </div>
</body>`

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 640 } })
await page.setContent(html)
await page.screenshot({ path: out })
await browser.close()
console.log(out)
