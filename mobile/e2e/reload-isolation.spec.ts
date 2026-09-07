// GDK-1526 — the dev server must not reload the app because the test
// harness wrote something.
//
// Why this exists. mobile/ is both the vite root and the whole Playwright
// harness: the specs, the captures they save and Playwright's output dir
// all sit inside the watch root. A full reload throws the app back to its
// first screen, so a reload that lands mid-run kills whatever spec is in
// flight on a click timeout — one genuine failure then drags the rest of
// the gate red, which is the shape of the "8 red in CI" Mobile runs.
//
// Measured 2026-09-07 on vite 6.4.3 with a chromium client attached: a
// byte-identical re-save of e2e/viewport.spec.ts reloaded the page, and so
// did src/lib/types.ts (that one is wanted — it is app source). Writes
// under test-results/ and e2e/.shots/ did not, because vite already
// ignores test-results by default and a .png nothing imports matches no
// module. The fix in mobile/vite.config.ts closes the whole class rather
// than the one path the ticket named; this spec is what proves it.
//
// The last case is a positive control, and it is what gives the four
// before it teeth: without it, a page that simply never receives HMR would
// pass every "did not reload" assertion. index.html is re-saved with
// identical bytes, which vite always answers with a full reload, so the
// spec fails loudly if the reload channel it is asserting about is dead.
//
// If this goes red while someone is editing mobile/src or web/src in the
// same tree, read the `[WebServer] … page reload <file>` line in the run
// log before believing the boundary broke: app source is *supposed* to
// reload, and an editor working alongside the gate reloads the app under
// it. That is the one reload this file cannot fence off.
import { expect, test } from '@playwright/test'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const HERE = fileURLToPath(import.meta.url)
const E2E_DIR = dirname(HERE)
const MOBILE_DIR = dirname(E2E_DIR)
const SHOT_PROBE_DIR = join(E2E_DIR, '.shots', '.reload-probe')
const INDEX_HTML = join(MOBILE_DIR, 'index.html')
const UNIT_TEST = join(MOBILE_DIR, 'src', 'lib', 'watch-boundary.test.ts')

const MARKER = '__gdk1526_reload_probe'

/**
 * How long to wait for a reload that should not come. Reload latency was
 * measured at well under 500 ms from write to navigation on this tree, so
 * 900 ms is generous; the whole spec costs about 5 s in the gate. Erring
 * short can only make this test silently lenient, never falsely red — the
 * positive control below is what stops that leniency from being total.
 */
const SETTLE_MS = 900

test('harness writes do not reload the app; app sources still do', async ({ page }, testInfo) => {
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
    // — reporting the throw as an error instead would turn the positive
    // control below into a red gate whenever it worked (measured: it did,
    // first run).
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
    //    actually measured to reload before the fix. Re-saved with its own
    //    bytes, so the file on disk is unchanged.
    await plant()
    writeFileSync(HERE, readFileSync(HERE))
    await page.waitForTimeout(SETTLE_MS)
    expect(await survived(), 'saving a spec file reloaded the app under test').toBe(true)

    // 3b. A vitest unit saved while the gate runs. This is the case that
    //     was caught happening for real: on 2026-09-07 an edit to this very
    //     file's sibling contract test reloaded the app in the middle of a
    //     viewport-gate run. The file re-saved here is that contract test,
    //     with its own bytes.
    await plant()
    writeFileSync(UNIT_TEST, readFileSync(UNIT_TEST))
    await page.waitForTimeout(SETTLE_MS)
    expect(await survived(), 'saving a vitest unit reloaded the app under test').toBe(true)

    // 4. Positive control: app source must still reach the page. Anything
    //    other than a reload here means the assertions above proved nothing.
    await plant()
    writeFileSync(INDEX_HTML, readFileSync(INDEX_HTML))
    await expect
      .poll(survived, {
        timeout: 10_000,
        message: 'index.html was re-saved but the page never reloaded — the dev server never had a live HMR channel, so every assertion above was vacuous',
      })
      .toBe(false)
  } finally {
    for (const clean of cleanups) clean()
  }
})
