/**
 * Interaction performance budgets (10k-issue fixture).
 *
 * Budgets are pinned from local p95 with CI headroom — see README.md.
 * FAIL-first: pin only after a deliberately tight budget has failed once.
 *
 * Guard: the main e2e suite discovers *.spec.ts under e2e/. Without SCRY_PERF
 * these tests skip immediately so `npm run test:e2e` stays green and does not
 * need a main-config edit (ownership: e2e/perf only).
 */
import { test, expect, type Browser, type Page, type Request } from '@playwright/test'
import { performance } from 'node:perf_hooks'

const RUN = !!process.env.SCRY_PERF

/** Expected issue count text on the 10k fixture (en-US locale). */
const ISSUE_COUNT_RE = /10,000 issues|10000 issues/

/**
 * Performance budgets (ms). Each pin: max(100, ceil(local_p95 * 2)).
 *
 * FAIL-first (2026-08-06): budgets temporarily set to 1ms; all four metrics
 * failed with the measured p95 values below. Then re-pinned to 2× headroom.
 * Evidence: e2e/perf/.tmp/fail-first-run.log (gitignored; see README).
 */
const BUDGETS = {
  // pinned 2026-08-06: local p95=971.3ms, budget=max(100, ceil(971.3*2))=1943
  coldBootMs: 1943,
  // pinned 2026-08-06: local p95=4453.3ms, budget=max(100, ceil(4453.3*2))=8907
  // Note: warm > cold on this fixture — IDB hydrate of 10k + navigation dominates;
  // still a separate product path (cache boot) so it stays its own metric.
  warmBootMs: 8907,
  // pinned 2026-08-06: local p95=26.8ms, budget=max(100, ceil(26.8*2))=100
  searchKeystrokeMs: 100,
  // pinned 2026-08-06: local p95=8.7ms, budget=max(100, ceil(8.7*2))=100
  paletteOpenMs: 100,
} as const

const SAMPLES = 20

type MetricName = keyof typeof BUDGETS

test.describe('performance budgets (10k fixture)', () => {
  test.skip(!RUN, 'set SCRY_PERF=1 (npm run test:perf)')

  test('four interaction metrics: warmup + 20 samples → p95 vs budget', async ({ browser }) => {
    // Measure everything first so FAIL-first / re-pin runs always print a full table.
    const results: Record<MetricName, Stats> = {
      coldBootMs: await runColdBoot(browser),
      warmBootMs: await runWarmBoot(browser),
      searchKeystrokeMs: await runSearchKeystroke(browser),
      paletteOpenMs: await runPaletteOpen(browser),
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

  // Prime IndexedDB once outside the measured loop.
  await page.goto('/')
  await waitInteractive(page)

  {
    const ms = await measureWarmBoot(page)
    console.log(`[perf] warmBoot warmup: ${ms.toFixed(1)}ms`)
  }

  const samples: number[] = []
  for (let i = 0; i < SAMPLES; i++) {
    const ms = await measureWarmBoot(page)
    samples.push(ms)
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
    if (url.includes('/api/')) apiDuring.push(url)
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

async function forceLocale(page: Page): Promise<void> {
  await page.addInitScript(() => {
    try {
      localStorage.setItem('scry_locale', 'en')
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
