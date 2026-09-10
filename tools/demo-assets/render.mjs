/**
 * Renders the demo attachment images from inline HTML with Playwright.
 *
 * Why generated, not captured: a committed screenshot of somebody's real screen
 * is a licensing and privacy problem, and a generated one is reproducible. These
 * are the artifacts a bug report actually carries — an error state, a latency
 * chart, a sketch — for the fictional product the demo snapshot describes.
 *
 * Usage: node tools/demo-assets/render.mjs [outDir]
 */
import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

const outDir = process.argv[2] ?? 'tools/demo-assets/out'
mkdirSync(outDir, { recursive: true })

const shell = (body, w, h) => `<!doctype html><meta charset="utf-8">
<style>
  * { box-sizing: border-box; margin: 0; }
  body { width:${w}px; height:${h}px; background:#0f1116; color:#e6e8ec;
         font:14px/1.5 -apple-system,'Segoe UI',Roboto,sans-serif; }
  .chrome { height:34px; background:#191d24; display:flex; align-items:center; gap:6px; padding:0 12px;
            border-bottom:1px solid #262c36; }
  .dot { width:10px; height:10px; border-radius:50%; }
  .url { flex:1; margin-left:10px; background:#0f1116; border:1px solid #262c36; border-radius:6px;
         padding:4px 10px; font-size:11px; color:#8b93a1; }
  .pad { padding:22px 26px; }
  h1 { font-size:17px; font-weight:600; }
  .muted { color:#8b93a1; }
  .card { background:#151922; border:1px solid #262c36; border-radius:10px; padding:16px; }
  .toast { position:absolute; right:26px; bottom:24px; background:#2a1416; border:1px solid #7f1d1d;
           border-left:3px solid #ef4444; border-radius:8px; padding:12px 14px; max-width:330px; }
  .bar { height:22px; border-radius:4px; background:#6366f1; }
  .row { display:flex; align-items:center; gap:10px; margin-top:9px; font-size:12px; }
  .row span:first-child { width:74px; color:#8b93a1; text-align:right; }
  code { font:12px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace; color:#a5b4fc; }
</style>${body}`

const pages = {
  // A bug reporter's screenshot: the app in its broken state.
  'nimbus-error.png': {
    w: 900, h: 560,
    html: shell(`<div class="chrome"><i class="dot" style="background:#ef4444"></i>
      <i class="dot" style="background:#eab308"></i><i class="dot" style="background:#22c55e"></i>
      <div class="url">app.nimbus.example.com/workspace/billing/plan</div></div>
      <div class="pad">
        <h1>Plan &amp; billing</h1>
        <p class="muted" style="margin-top:4px">Workspace: Northwind (12 members)</p>
        <div class="card" style="margin-top:18px">
          <div style="display:flex;justify-content:space-between;align-items:baseline">
            <div><div style="font-weight:600">Growth</div>
              <div class="muted" style="font-size:12px;margin-top:2px">Renews 1 Sep 2026</div></div>
            <div style="font-size:22px;font-weight:600">$0<span class="muted" style="font-size:13px">/mo</span></div>
          </div>
        </div>
        <div class="card" style="margin-top:12px;opacity:.55">
          <div style="font-weight:600">Seats</div>
          <div class="muted" style="font-size:12px;margin-top:6px">Could not load seat count</div>
        </div>
      </div>
      <div class="toast"><div style="font-weight:600;font-size:13px">Stale plan tier</div>
        <div class="muted" style="font-size:12px;margin-top:4px">Showing a cached tier from a previous
        session. Reload the page to see the current plan.</div></div>`, 900, 560),
  },
  // The measurement that turns a complaint into a ticket.
  'latency-before-after.png': {
    w: 820, h: 420,
    html: shell(`<div class="pad">
      <h1>Filter latency — before / after the local mirror</h1>
      <p class="muted" style="margin-top:4px;font-size:12px">p50 over 200 filter changes, 10k issues</p>
      <div class="card" style="margin-top:20px">
        <div class="row"><span>Jira REST</span><div class="bar" style="width:560px;background:#ef4444"></div><span>412 ms</span></div>
        <div class="row"><span>+ cold cache</span><div class="bar" style="width:340px;background:#f59e0b"></div><span>251 ms</span></div>
        <div class="row"><span>local mirror</span><div class="bar" style="width:14px;background:#22c55e"></div><span>3 ms</span></div>
      </div>
      <p class="muted" style="margin-top:18px;font-size:12px">Measured with the bundled bench fixture:</p>
      <p style="margin-top:6px"><code>make bench   # BenchmarkSearch10k</code></p>
    </div>`, 820, 420),
  },
  // A sketch pasted into a comment while reasoning about the cause.
  'cache-key-sketch.png': {
    w: 820, h: 360,
    html: shell(`<div class="pad">
      <h1>Where the tier goes stale</h1>
      <svg width="760" height="210" style="margin-top:18px">
        <defs><marker id="a" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
          <path d="M0,0 L9,4.5 L0,9 z" fill="#8b93a1"/></marker></defs>
        <g font-family="-apple-system,Segoe UI,Roboto,sans-serif" font-size="12">
          <rect x="4" y="60" width="150" height="52" rx="8" fill="#151922" stroke="#262c36"/>
          <text x="79" y="90" fill="#e6e8ec" text-anchor="middle">workspace list</text>
          <rect x="230" y="60" width="180" height="52" rx="8" fill="#1a1730" stroke="#4338ca"/>
          <text x="320" y="83" fill="#a5b4fc" text-anchor="middle">cache</text>
          <text x="320" y="100" fill="#8b93a1" text-anchor="middle" font-size="11">key: plan_tier</text>
          <rect x="490" y="60" width="160" height="52" rx="8" fill="#151922" stroke="#262c36"/>
          <text x="570" y="90" fill="#e6e8ec" text-anchor="middle">billing service</text>
          <line x1="158" y1="86" x2="224" y2="86" stroke="#8b93a1" marker-end="url(#a)"/>
          <line x1="414" y1="86" x2="484" y2="86" stroke="#8b93a1" marker-end="url(#a)"/>
          <text x="320" y="150" fill="#ef4444" text-anchor="middle">missing workspace id →
            one tenant's tier answers for another</text>
          <text x="320" y="176" fill="#22c55e" text-anchor="middle">key: plan_tier:{workspace_id}</text>
        </g>
      </svg>
    </div>`, 820, 360),
  },
  // The onboarding guide's own diagram: what the first week is shaped like.
  // A page attachment (GDK-1541), not an issue's — same table, page owner.
  'onboarding-first-week.png': {
    w: 820, h: 400,
    html: shell(`<div class="pad">
      <h1>Your first week, at a glance</h1>
      <p class="muted" style="margin-top:4px;font-size:12px">Nimbus engineering onboarding — mentor check-ins in green</p>
      <svg width="760" height="250" style="margin-top:18px">
        <g font-family="-apple-system,Segoe UI,Roboto,sans-serif" font-size="12">
          <line x1="30" y1="200" x2="730" y2="200" stroke="#262c36"/>
          <g text-anchor="middle">
            <rect x="34" y="96" width="140" height="72" rx="8" fill="#151922" stroke="#262c36"/>
            <text x="104" y="122" fill="#e6e8ec">Day 1</text>
            <text x="104" y="142" fill="#8b93a1" font-size="11">laptop, repo,</text>
            <text x="104" y="158" fill="#8b93a1" font-size="11">first build</text>
            <rect x="210" y="96" width="140" height="72" rx="8" fill="#151922" stroke="#262c36"/>
            <text x="280" y="122" fill="#e6e8ec">Day 2–3</text>
            <text x="280" y="142" fill="#8b93a1" font-size="11">pick up a</text>
            <text x="280" y="158" fill="#8b93a1" font-size="11">good-first-issue</text>
            <rect x="386" y="96" width="140" height="72" rx="8" fill="#1a1730" stroke="#4338ca"/>
            <text x="456" y="122" fill="#a5b4fc">Day 4</text>
            <text x="456" y="142" fill="#8b93a1" font-size="11">ship it behind</text>
            <text x="456" y="158" fill="#8b93a1" font-size="11">a flag</text>
            <rect x="562" y="96" width="140" height="72" rx="8" fill="#151922" stroke="#262c36"/>
            <text x="632" y="122" fill="#e6e8ec">Day 5</text>
            <text x="632" y="142" fill="#8b93a1" font-size="11">walkthrough</text>
            <text x="632" y="158" fill="#8b93a1" font-size="11">with the team</text>
          </g>
          <circle cx="456" cy="70" r="10" fill="#22c55e"/>
          <text x="456" y="52" fill="#22c55e" text-anchor="middle" font-size="11">mentor check-in</text>
          <circle cx="632" cy="70" r="10" fill="#22c55e"/>
          <text x="632" y="52" fill="#22c55e" text-anchor="middle" font-size="11">demo Friday</text>
        </g>
      </svg>
    </div>`, 820, 400),
  },
}

const browser = await chromium.launch()
for (const [name, { w, h, html }] of Object.entries(pages)) {
  const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 2 })
  await page.setContent(html)
  await page.screenshot({ path: join(outDir, name) })
  await page.close()
  console.log('wrote', join(outDir, name))
}
await browser.close()
