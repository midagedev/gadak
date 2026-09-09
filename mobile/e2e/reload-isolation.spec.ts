// GDK-1526 / GDK-1540 — nothing written to this tree while the gate runs may
// reach the app under test.
//
// Why this exists. mobile/ is both the vite root and the whole Playwright
// harness: the specs, the captures they save and Playwright's output dir all
// sit inside it, and mobile/src plus the web/src modules the phone imports
// are edited by whoever else is working in the tree. A full reload throws the
// app back to its first screen, so a reload that lands mid-run kills whatever
// spec is in flight on a click timeout — one genuine failure then drags the
// rest of the gate red, which is the shape of the "8 red in CI" Mobile runs.
//
// The fix came in two halves.
//
// GDK-1526 closed the harness half with `server.watch.ignored` in
// mobile/vite.config.ts: a re-saved spec or vitest unit no longer reloads the
// dev server's page. Measured 2026-09-07 on vite 6.4.3 with a chromium client
// attached — before that glob, a byte-identical re-save of
// e2e/viewport.spec.ts reloaded the page.
//
// GDK-1540 closed the app-source half, which no ignore glob could: reloading
// on an app-source edit is not a bug in a dev server, it is the contract of
// one. So the gate stopped serving a dev server. It builds the bundle and
// serves it through `vite preview` (mobile/e2e/gate-serve.sh), which has no
// watcher and no HMR channel, and the bytes under a spec are then fixed for
// the length of the run.
//
// That flips the last case in this file. It used to be a positive control —
// re-save index.html, watch the page reload, and thereby prove the four "did
// not reload" assertions above it were not vacuous. Under a built bundle an
// app-source edit *must not* reach the page either, so it is now an assertion
// like the others, and the vacuity guard is an explicit page.reload(): if the
// marker survives that, the detector is broken and nothing here means
// anything.
import { expect, test } from './helpers'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { BUNDLE_STAMP_FILE } from './serve'

const HERE = fileURLToPath(import.meta.url)
const E2E_DIR = dirname(HERE)
const MOBILE_DIR = dirname(E2E_DIR)
const SHOT_PROBE_DIR = join(E2E_DIR, '.shots', '.reload-probe')
const INDEX_HTML = join(MOBILE_DIR, 'index.html')
const APP_ENTRY = join(MOBILE_DIR, 'src', 'main.ts')
const UNIT_TEST = join(MOBILE_DIR, 'src', 'lib', 'watch-boundary.test.ts')

const MARKER = '__gdk1526_reload_probe'

/**
 * How long to wait for a reload that should not come. Reload latency was
 * measured at well under 500 ms from write to navigation on this tree, so
 * 900 ms is generous; the whole spec costs about 6 s in the gate. Erring
 * short can only make this test silently lenient, never falsely red — the
 * page.reload() control at the end is what stops that leniency from being
 * total.
 */
const SETTLE_MS = 900

test('the gate serves a built bundle, not a dev server', async ({ page }) => {
  // The stamp is emitted by `vite build` into the served directory
  // (mobile/e2e/gate-serve.sh). A dev server has no such file and answers
  // the path with the SPA fallback — index.html, text/html — so this is the
  // one assertion that fails loudly if the gate ever slides back onto
  // `vite`, taking every "did not reload" assertion below with it.
  const res = await page.request.get(`/${BUNDLE_STAMP_FILE}`)
  expect(res.status(), `GET /${BUNDLE_STAMP_FILE}`).toBe(200)
  expect(
    res.headers()['content-type'] ?? '',
    'the gate origin answered the build stamp with HTML — that is a dev server SPA fallback, not a built bundle',
  ).toContain('application/json')
  expect((await res.json()) as { role?: string }).toMatchObject({ role: 'ui' })
})

test('nothing written to the tree reloads the app under test', async ({ page }, testInfo) => {
  const cleanups: Array<() => void> = []
  try {
    await page.goto('/')
    await page.waitForLoadState('load')

    const plant = () =>
      page.evaluate((key) => {
        ;(window as unknown as Record<string, string>)[key] = 'alive'
      }, MARKER)
    // A reload can be caught in two states: finished, so the marker is gone
    // from a fresh context, or still in flight, so evaluating at all throws
    // "Execution context was destroyed". Both mean the page did not survive
    // — reporting the throw as an error instead would turn the control at
    // the end into a red gate whenever it worked (measured: it did, first
    // run, back when the control was an index.html re-save).
    const survived = async () => {
      try {
        return await page.evaluate(
          (key) => (window as unknown as Record<string, string>)[key] === 'alive',
          MARKER,
        )
      } catch {
        return false
      }
    }

    // 1. Playwright's own output dir — a trace and a screenshot of a
    //    failed test, written exactly where Playwright writes them.
    await plant()
    const traceProbe = testInfo.outputPath('gdk1526-trace-probe.zip')
    const shotProbe = testInfo.outputPath('gdk1526-shot-probe.png')
    writeFileSync(traceProbe, Buffer.alloc(4096, 7))
    writeFileSync(shotProbe, Buffer.alloc(2048, 9))
    cleanups.push(() => rmSync(traceProbe, { force: true }))
    cleanups.push(() => rmSync(shotProbe, { force: true }))
    await page.waitForTimeout(SETTLE_MS)
    expect(await survived(), 'a Playwright trace/screenshot write reloaded the app').toBe(true)

    // 2. A capture written by a spec into e2e/.shots/ mid-run.
    await plant()
    mkdirSync(SHOT_PROBE_DIR, { recursive: true })
    writeFileSync(join(SHOT_PROBE_DIR, 'probe.png'), Buffer.alloc(2048, 9))
    cleanups.push(() => rmSync(SHOT_PROBE_DIR, { recursive: true, force: true }))
    await page.waitForTimeout(SETTLE_MS)
    expect(await survived(), 'a capture written into e2e/.shots/ reloaded the app').toBe(true)

    // 3. A spec source saved while the gate runs — the case that was
    //    actually measured to reload before GDK-1526. Re-saved with its own
    //    bytes, so the file on disk is unchanged.
    await plant()
    writeFileSync(HERE, readFileSync(HERE))
    await page.waitForTimeout(SETTLE_MS)
    expect(await survived(), 'saving a spec file reloaded the app under test').toBe(true)

    // 3b. A vitest unit saved while the gate runs. This is the case that
    //     was caught happening for real: on 2026-09-07 an edit to a sibling
    //     contract test reloaded the app in the middle of a viewport-gate
    //     run. The file re-saved here is the watch-boundary unit, with its
    //     own bytes.
    await plant()
    writeFileSync(UNIT_TEST, readFileSync(UNIT_TEST))
    await page.waitForTimeout(SETTLE_MS)
    expect(await survived(), 'saving a vitest unit reloaded the app under test').toBe(true)

    // 4. GDK-1540, the case an ignore glob could never close: *app source*.
    //    A dev server is supposed to reload for these two, which is exactly
    //    why the gate no longer serves one. Both are re-saved with their own
    //    bytes; both are in the module graph, and both were measured to
    //    produce a full reload on the dev server (index.html always does,
    //    and a plain .ts module has nothing to accept the HMR update).
    await plant()
    writeFileSync(INDEX_HTML, readFileSync(INDEX_HTML))
    await page.waitForTimeout(SETTLE_MS)
    expect(
      await survived(),
      'saving mobile/index.html reloaded the app — the gate is serving a dev server again (GDK-1540)',
    ).toBe(true)

    await plant()
    writeFileSync(APP_ENTRY, readFileSync(APP_ENTRY))
    await page.waitForTimeout(SETTLE_MS)
    expect(
      await survived(),
      'saving mobile/src/main.ts reloaded the app — the gate is serving a dev server again (GDK-1540)',
    ).toBe(true)

    // 5. The vacuity guard. Every assertion above is "the marker is still
    //    there"; a page that could never lose it would pass them all. An
    //    explicit reload must lose it.
    await plant()
    await page.reload()
    await page.waitForLoadState('load')
    expect(
      await survived(),
      'an explicit page.reload() left the marker in place — the detector is broken and every assertion above is vacuous',
    ).toBe(false)
  } finally {
    for (const clean of cleanups) clean()
  }
})
