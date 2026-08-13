/**
 * Interaction performance budgets (10k-issue, 5k-page fixture).
 *
 * Budgets are pinned from local p95 with CI headroom — see README.md.
 * FAIL-first: pin only after a deliberately tight budget has failed once.
 *
 * Guard: the main e2e suite discovers *.spec.ts under e2e/. Without GADAK_PERF
 * these tests skip immediately so `npm run test:e2e` stays green and does not
 * need a main-config edit (ownership: e2e/perf only).
 */
import { test, expect, type Browser, type Page, type Request } from '@playwright/test'
import { performance } from 'node:perf_hooks'

const RUN = !!process.env.GADAK_PERF

/** Expected issue count text on the 10k fixture (en-US locale). */
const ISSUE_COUNT_RE = /10,000 issues|10000 issues/

/** Fixture issue count — IDB prime target and DOM count must both match. */
const FIXTURE_ISSUE_COUNT = 10_000

/** Unfiltered document count in the header (5,000-page fixture, en-US). */
const DOCS_COUNT_TEXT = /^5,000$|^5000$/

/** Timer-driven endpoints (sync status / freshness), never a keystroke's doing. */
const POLLED_ENDPOINTS = /\/sync\/(progress|runs)\/|\/meta\//

/**
 * Performance budgets (ms). Each pin: max(100, ceil(local_p95 * 2)).
 *
 * Original FAIL-first (2026-08-06): budgets at 1ms → all four red; see
 * e2e/perf/.tmp/fail-first-run.log. warmBoot was later found to measure
 * cold+contention (no IDB prime wait); re-pinned after priming fix.
 */
const BUDGETS = {
  // pinned 2026-08-06: local p95=971.3ms, budget=max(100, ceil(971.3*2))=1943
  coldBootMs: 1943,
  // re-pinned 2026-08-06: priming fixed (was measuring cold+contention),
  // quiet p95=132.1ms, budget=ceil(132.1*2)=265
  // FAIL-first: budget=66 (0.5×) failed with p95=138.0ms — e2e/perf/.tmp/fail-first-warmboot-0.5x.log
  warmBootMs: 265,
  // pinned 2026-08-06: local p95=26.8ms, budget=max(100, ceil(26.8*2))=100
  searchKeystrokeMs: 100,
  // pinned 2026-08-06: local p95=8.7ms, budget=max(100, ceil(8.7*2))=100
  paletteOpenMs: 100,
  // pinned 2026-08-07: local p95=29.2ms, budget=max(100, ceil(29.2*2))=100
  // The fixture carried no documents at all until this release, so the axis was
  // unmeasured and shipped unwindowed — 5,000 rows rebuilt on every tab switch.
  // FAIL-first: the same budget against the pre-window list gave p95=1107.1ms
  // (p50=777.3) — e2e/perf/.tmp/fail-first-docs-tab-switch.log. A list that
  // stops windowing lands back there, which is what this budget catches.
  docsTabSwitchMs: 100,
  // pinned 2026-08-07: local p95=15.3ms, budget=max(100, ceil(15.3*2))=100
  // FAIL-first: budget=1 failed with p95=15.3ms (p50=14.7) —
  // e2e/perf/.tmp/fail-first-docs-filter-keystroke.log. The axis is the one the
  // document screens gained a filter on: every keystroke re-scans the whole
  // page index in memory, and this is what says it stays in memory.
  docsFilterKeystrokeMs: 100,
} as const

const SAMPLES = 20

type MetricName = keyof typeof BUDGETS

test.describe('performance budgets (10k fixture)', () => {
  test.skip(!RUN, 'set GADAK_PERF=1 (npm run test:perf)')

  test('interaction metrics: warmup + 20 samples → p95 vs budget', async ({ browser }) => {
    // Measure everything first so FAIL-first / re-pin runs always print a full table.
    const results: Record<MetricName, Stats> = {
      coldBootMs: await runColdBoot(browser),
      warmBootMs: await runWarmBoot(browser),
      searchKeystrokeMs: await runSearchKeystroke(browser),
      paletteOpenMs: await runPaletteOpen(browser),
      docsTabSwitchMs: await runDocsTabSwitch(browser),
      docsFilterKeystrokeMs: await runDocsFilterKeystroke(browser),
    }

    console.log('[perf] ── summary (ms) ──')
    console.log('[perf] metric              p50     p95   budget')
    for (const name of Object.keys(BUDGETS) as MetricName[]) {
      const s = results[name]
      const b = BUDGETS[name]
      console.log(
        `[perf] ${name.padEnd(20)} ${s.p50.toFixed(1).padStart(7)} ${s.p95.toFixed(1).padStart(7)} ${String(b).padStart(7)}`,
      )
    }

    const failures: string[] = []
    for (const name of Object.keys(BUDGETS) as MetricName[]) {
      const s = results[name]
      const b = BUDGETS[name]
      if (s.p95 > b) {
        failures.push(failMsg(name, s, b))
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })
})

// ── metric runners ────────────────────────────────────────────────────────

async function runColdBoot(browser: Browser): Promise<Stats> {
  // Warmup
  {
    const { ms, context } = await measureColdBoot(browser)
    await context.close()
    console.log(`[perf] coldBoot warmup: ${ms.toFixed(1)}ms`)
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    const { ms, context } = await measureColdBoot(browser)
    samples.push(ms)
    await context.close()
    console.log(`[perf] coldBoot sample ${i + 1}/${SAMPLES}: ${ms.toFixed(1)}ms`)
  }
  const stats = summarize(samples)
  logStats('coldBoot', stats, BUDGETS.coldBootMs)
  return stats
}

async function runWarmBoot(browser: Browser): Promise<Stats> {
  const context = await browser.newContext({ locale: 'en-US' })
  const page = await context.newPage()
  await forceLocale(page)

  // Prime IndexedDB once outside the measured loop — wait until the bootstrap
  // write has actually committed (row count == fixture), not merely until the
  // UI is interactive. Interactive arrives before replaceAllIssues finishes;
  // sampling earlier measured empty-cache cold + contention with a dead write.
  await page.goto('/')
  await waitInteractive(page)
  await waitIdbPrimed(page, FIXTURE_ISSUE_COUNT)
  console.log(
    `[perf] warmBoot prime complete: IDB issues=${await countIdbIssues(page)} (expected ${FIXTURE_ISSUE_COUNT})`,
  )

  {
    await assertIdbPrimed(page, FIXTURE_ISSUE_COUNT)
    const ms = await measureWarmBoot(page)
    // After each warm navigation, bootstrap may rewrite; re-prime before samples.
    await waitIdbPrimed(page, FIXTURE_ISSUE_COUNT)
    console.log(`[perf] warmBoot warmup: ${ms.toFixed(1)}ms`)
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    // Guard: prior iteration's write must not have left an empty/partial cache.
    await assertIdbPrimed(page, FIXTURE_ISSUE_COUNT)
    const ms = await measureWarmBoot(page)
    samples.push(ms)
    await waitIdbPrimed(page, FIXTURE_ISSUE_COUNT)
    console.log(`[perf] warmBoot sample ${i + 1}/${SAMPLES}: ${ms.toFixed(1)}ms`)
  }
  await context.close()

  const stats = summarize(samples)
  logStats('warmBoot', stats, BUDGETS.warmBootMs)
  return stats
}

async function runSearchKeystroke(browser: Browser): Promise<Stats> {
  const context = await browser.newContext({ locale: 'en-US' })
  const page = await context.newPage()
  await forceLocale(page)
  await page.goto('/')
  await waitInteractive(page)
  // Let trailing boot requests settle (not networkidle — 15s poll).
  await page.waitForTimeout(400)

  {
    const ms = await measureSearchKeystroke(page)
    console.log(`[perf] searchKeystroke warmup: ${ms.toFixed(1)}ms`)
    await clearSearch(page)
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    const ms = await measureSearchKeystroke(page)
    samples.push(ms)
    console.log(`[perf] searchKeystroke sample ${i + 1}/${SAMPLES}: ${ms.toFixed(1)}ms`)
    await clearSearch(page)
  }
  await context.close()

  const stats = summarize(samples)
  logStats('searchKeystroke', stats, BUDGETS.searchKeystrokeMs)
  return stats
}

async function runPaletteOpen(browser: Browser): Promise<Stats> {
  const context = await browser.newContext({ locale: 'en-US' })
  const page = await context.newPage()
  await forceLocale(page)
  await page.goto('/')
  await waitInteractive(page)
  await page.waitForTimeout(400)

  {
    const ms = await measurePaletteOpen(page)
    console.log(`[perf] paletteOpen warmup: ${ms.toFixed(1)}ms`)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Command palette' })).toBeHidden()
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    const ms = await measurePaletteOpen(page)
    samples.push(ms)
    console.log(`[perf] paletteOpen sample ${i + 1}/${SAMPLES}: ${ms.toFixed(1)}ms`)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Command palette' })).toBeHidden()
  }
  await context.close()

  const stats = summarize(samples)
  logStats('paletteOpen', stats, BUDGETS.paletteOpenMs)
  return stats
}

async function runDocsTabSwitch(browser: Browser): Promise<Stats> {
  const context = await browser.newContext({ locale: 'en-US' })
  const page = await context.newPage()
  await forceLocale(page)
  await page.goto('/')
  await waitInteractive(page)
  await page.waitForTimeout(400)

  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible({ timeout: 60_000 })

  {
    const ms = await measureDocsTabSwitch(page)
    console.log(`[perf] docsTabSwitch warmup: ${ms.toFixed(1)}ms`)
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    const ms = await measureDocsTabSwitch(page)
    samples.push(ms)
    console.log(`[perf] docsTabSwitch sample ${i + 1}/${SAMPLES}: ${ms.toFixed(1)}ms`)
  }
  await context.close()

  const stats = summarize(samples)
  logStats('docsTabSwitch', stats, BUDGETS.docsTabSwitchMs)
  return stats
}

async function runDocsFilterKeystroke(browser: Browser): Promise<Stats> {
  const context = await browser.newContext({ locale: 'en-US' })
  const page = await context.newPage()
  await forceLocale(page)
  await page.goto('/')
  await waitInteractive(page)
  await page.waitForTimeout(400)

  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible({ timeout: 60_000 })
  // Updated lists the whole mirror, so a keystroke here is a pass over all
  // 5,000 pages — the worst case the tab offers, which is what to budget.
  await page.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
  await expect(page.getByTestId('docs-count')).toHaveText(DOCS_COUNT_TEXT, { timeout: 60_000 })

  {
    const ms = await measureDocsFilterKeystroke(page)
    console.log(`[perf] docsFilterKeystroke warmup: ${ms.toFixed(1)}ms`)
    await clearDocsFilter(page)
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    const ms = await measureDocsFilterKeystroke(page)
    samples.push(ms)
    console.log(`[perf] docsFilterKeystroke sample ${i + 1}/${SAMPLES}: ${ms.toFixed(1)}ms`)
    await clearDocsFilter(page)
  }
  await context.close()

  const stats = summarize(samples)
  logStats('docsFilterKeystroke', stats, BUDGETS.docsFilterKeystrokeMs)
  return stats
}

// ── measurements ──────────────────────────────────────────────────────────

async function measureColdBoot(
  browser: Browser,
): Promise<{ ms: number; context: Awaited<ReturnType<Browser['newContext']>> }> {
  // Fresh context ⇒ empty IndexedDB / localStorage (cold path).
  const context = await browser.newContext({ locale: 'en-US' })
  const page = await context.newPage()
  await forceLocale(page)

  // Node-side clock: navigation resets the document performance origin.
  const t0 = performance.now()
  await page.goto('/')
  await waitInteractive(page)
  const ms = performance.now() - t0
  return { ms, context }
}

async function measureWarmBoot(page: Page): Promise<number> {
  // Node-side clock: full navigation; IndexedDB hydration is the product path.
  const t0 = performance.now()
  await page.goto('/')
  await waitInteractive(page)
  return performance.now() - t0
}

async function measureSearchKeystroke(page: Page): Promise<number> {
  const input = page.getByPlaceholder(/Search issues/)
  await input.click()

  const countEl = page.locator('span', { hasText: /\d[\d,]* issues?/ }).first()
  const before = (await countEl.innerText()).trim()

  const apiDuring: string[] = []
  const onReq = (req: Request) => {
    const url = req.url()
    // Same exclusion as the docs-filter axis (see POLLED_ENDPOINTS): the sync
    // pollers fire on their own timer — while the mirror is busy, every 2s —
    // so a long enough window always catches one regardless of what was typed.
    // FAIL-first 2026-08-07: this assertion caught sync/progress/ on a quiet
    // rerun after the activity poller shipped (task #54) — an instrumentation
    // collision, not a keystroke fetch. Everything that could answer a search
    // (search/, bootstrap/, delta/, pages/) still fails here.
    if (url.includes('/api/') && !POLLED_ENDPOINTS.test(url)) apiDuring.push(url)
  }
  page.on('request', onReq)

  try {
    // Document performance.now — same page, no navigation (spec requirement).
    const t0 = await page.evaluate(() => performance.now())
    await input.press('x')
    await expect(countEl).not.toHaveText(before, { timeout: 15_000 })
    const t1 = await page.evaluate(() => performance.now())

    expect(
      apiDuring,
      `expected no /api/ traffic while typing, got:\n${apiDuring.join('\n')}`,
    ).toEqual([])

    return t1 - t0
  } finally {
    page.off('request', onReq)
  }
}

async function clearSearch(page: Page): Promise<void> {
  const input = page.getByPlaceholder(/Search issues/)
  await input.click()
  await input.fill('')
  await expect(page.getByText(ISSUE_COUNT_RE).first()).toBeVisible({ timeout: 15_000 })
}

/**
 * Alternate Updated ↔ By author and time the rebuild. Both tabs list the whole
 * mirror, so this is the moment the old code spent building 5,000 rows; the
 * count in the header is what proves the switch landed rather than the rows,
 * which are now a screenful either way.
 */
async function measureDocsTabSwitch(page: Page): Promise<number> {
  const view = page.getByTestId('docs-view')
  const updated = view.getByTestId('docs-tab').filter({ hasText: 'Updated' })
  const author = view.getByTestId('docs-tab').filter({ hasText: 'By author' })
  const pressed = (await updated.getAttribute('aria-pressed')) === 'true'
  const target = pressed ? author : updated

  const t0 = await page.evaluate(() => performance.now())
  await target.click()
  await expect(target).toHaveAttribute('aria-pressed', 'true', { timeout: 15_000 })
  await expect(view.getByTestId('doc-row').first()).toBeVisible({ timeout: 15_000 })
  const t1 = await page.evaluate(() => performance.now())
  return t1 - t0
}

/**
 * One keystroke in the document filter → the header count settles on its
 * fraction. Same shape as the issue-list keystroke above, including the claim
 * that matters most on this path: narrowing 5,000 documents asks the server
 * nothing at all.
 */
async function measureDocsFilterKeystroke(page: Page): Promise<number> {
  const input = page.getByTestId('docs-filter-input')
  await input.click()
  const countEl = page.getByTestId('docs-count')

  const apiDuring: string[] = []
  const onReq = (req: Request) => {
    const url = req.url()
    // The freshness pollers run on their own timer and will land inside any
    // window long enough to catch them; they are not on the keystroke path and
    // silencing them here would only make the sample length decide the verdict.
    // Everything that could actually serve a filter (pages, search, bootstrap,
    // delta) still fails this.
    if (url.includes('/api/') && !POLLED_ENDPOINTS.test(url)) apiDuring.push(url)
  }
  page.on('request', onReq)

  try {
    const t0 = await page.evaluate(() => performance.now())
    await input.press('e')
    // The unfiltered count is a bare total; a filtered one is "n / total", so
    // the fraction arriving is the render having landed.
    await expect(countEl).toHaveText(/\//, { timeout: 15_000 })
    const t1 = await page.evaluate(() => performance.now())

    expect(
      apiDuring,
      `expected no /api/ traffic while filtering documents, got:\n${apiDuring.join('\n')}`,
    ).toEqual([])

    return t1 - t0
  } finally {
    page.off('request', onReq)
  }
}

async function clearDocsFilter(page: Page): Promise<void> {
  const input = page.getByTestId('docs-filter-input')
  await input.click()
  await input.fill('')
  await expect(page.getByTestId('docs-count')).toHaveText(DOCS_COUNT_TEXT, { timeout: 15_000 })
}

async function measurePaletteOpen(page: Page): Promise<number> {
  const palette = page.getByRole('dialog', { name: 'Command palette' })
  const t0 = await page.evaluate(() => performance.now())
  await page.keyboard.press('ControlOrMeta+k')
  await expect(palette).toBeVisible({ timeout: 15_000 })
  const t1 = await page.evaluate(() => performance.now())
  return t1 - t0
}

async function waitInteractive(page: Page): Promise<void> {
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
  await expect(page.getByText(ISSUE_COUNT_RE).first()).toBeVisible({ timeout: 60_000 })
}

/**
 * Count rows in the issue-navigator IndexedDB `issues` store (default workspace).
 * Used only by the warmBoot prime gate — product code stays untouched.
 */
async function countIdbIssues(page: Page): Promise<number> {
  return page.evaluate(async () => {
    const DB_NAME = 'issue-navigator'
    return new Promise<number>((resolve, reject) => {
      const req = indexedDB.open(DB_NAME)
      req.onerror = () => reject(req.error ?? new Error('idb open failed'))
      req.onsuccess = () => {
        const idb = req.result
        try {
          if (!idb.objectStoreNames.contains('issues')) {
            idb.close()
            resolve(0)
            return
          }
          const tx = idb.transaction('issues', 'readonly')
          const countReq = tx.objectStore('issues').count()
          countReq.onsuccess = () => {
            const n = countReq.result
            idb.close()
            resolve(n)
          }
          countReq.onerror = () => {
            idb.close()
            reject(countReq.error ?? new Error('idb count failed'))
          }
        } catch (e) {
          idb.close()
          reject(e)
        }
      }
    })
  })
}

/** Poll until IDB issue count equals expected (bootstrap write committed). */
async function waitIdbPrimed(
  page: Page,
  expected: number,
  timeoutMs = 30_000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs
  let last = -1
  while (Date.now() < deadline) {
    try {
      last = await countIdbIssues(page)
      if (last === expected) {
        console.log(`[perf] IDB primed: issues=${last} (expected ${expected})`)
        return
      }
    } catch {
      // DB may not exist yet during first boot write.
    }
    await page.waitForTimeout(100)
  }
  throw new Error(
    `IDB prime timeout after ${timeoutMs}ms: issues=${last}, expected ${expected}`,
  )
}

/** One-shot assert before a warm sample — fail fast if cache was destroyed. */
async function assertIdbPrimed(page: Page, expected: number): Promise<void> {
  const n = await countIdbIssues(page)
  if (n !== expected) {
    throw new Error(
      `warmBoot pre-goto IDB issues=${n}, expected ${expected} (cache not ready / destroyed)`,
    )
  }
}

async function forceLocale(page: Page): Promise<void> {
  await page.addInitScript(() => {
    try {
      localStorage.setItem('gadak_locale', 'en')
    } catch {
      /* ignore */
    }
  })
}

// ── stats ─────────────────────────────────────────────────────────────────

type Stats = { p50: number; p95: number; min: number; max: number; n: number; samples: number[] }

function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0
  // Nearest-rank: ceil(p/100 * n) - 1
  const idx = Math.min(
    sorted.length - 1,
    Math.max(0, Math.ceil((p / 100) * sorted.length) - 1),
  )
  return sorted[idx]!
}

function summarize(samples: number[]): Stats {
  const sorted = [...samples].sort((a, b) => a - b)
  return {
    n: samples.length,
    min: sorted[0] ?? 0,
    max: sorted[sorted.length - 1] ?? 0,
    p50: percentile(sorted, 50),
    p95: percentile(sorted, 95),
    samples: sorted,
  }
}

function logStats(name: string, stats: Stats, budget: number): void {
  console.log(
    `[perf] ${name}: n=${stats.n} min=${stats.min.toFixed(1)} p50=${stats.p50.toFixed(1)} p95=${stats.p95.toFixed(1)} max=${stats.max.toFixed(1)} budget=${budget}ms`,
  )
  console.log(`[perf] ${name} samples_ms=${JSON.stringify(stats.samples.map((s) => Math.round(s)))}`)
}

function failMsg(name: string, stats: Stats, budget: number): string {
  return (
    `${name} p95=${stats.p95.toFixed(1)}ms exceeds budget=${budget}ms ` +
    `(p50=${stats.p50.toFixed(1)} min=${stats.min.toFixed(1)} max=${stats.max.toFixed(1)}; ` +
    `re-pin: budget=max(100, ceil(p95*2)) — see e2e/perf/README.md)`
  )
}
