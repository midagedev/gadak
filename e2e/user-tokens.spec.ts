import { type APIRequestContext, type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { apiURL, attachConsoleErrors, gotoApp } from './helpers'

const SETTINGS_URL = apiURL('/api/v1/issues/settings/')

// `accent` is a validated token whose only rule is hex (catalog
// "rules":["hex"]), so this value clears the write gate in every palette
// without a per-theme companion — the assertions below are about delivery,
// not color math. LIGHT.accent in theme.spec is the value we start from.
const DEFAULT_LIGHT_ACCENT = '#2e4560'
const ACCENT = '#7a4bd0'
const STATUS_NEW_INK = '#c03030'

// The poll cycle is 500ms (keys-focus.spec pins it); the GDK-791 contract is
// p95 ≤ 1s from write to repaint. The e2e budget adds one poll cycle of
// scheduling slack plus the config refetch for CI variance.
const LIVE_REFLECT_BUDGET_MS = 1_500

/** GET→PUT replace, the same write path the settings dialog uses. */
async function putUI(
  request: APIRequestContext,
  ui: Record<string, unknown>,
): Promise<{ status: number; body: Record<string, unknown> }> {
  const res = await request.get(SETTINGS_URL)
  const body = (await res.json()) as Record<string, unknown>
  const put = await request.put(SETTINGS_URL, { data: { ...body, ui } })
  let parsed: Record<string, unknown> = {}
  try {
    parsed = (await put.json()) as Record<string, unknown>
  } catch {
    /* non-JSON failure bodies assert on status only */
  }
  return { status: put.status(), body: parsed }
}

async function readToken(page: Page, name: string): Promise<string> {
  return page.evaluate(
    (n) => getComputedStyle(document.documentElement).getPropertyValue(n).trim().toLowerCase(),
    name,
  )
}

test.describe('user tokens', () => {
  test.afterEach(async ({ request }) => {
    // `ui: {}` is a valid empty block: omitempty drops it from config.json,
    // so later specs (theme.spec asserts the stock accent) start clean.
    const res = await request.get(SETTINGS_URL)
    if (!res.ok()) return
    const body = (await res.json()) as Record<string, unknown>
    await request.put(SETTINGS_URL, {
      data: { ...body, appearance: { theme: 'system' }, ui: {} },
    })
    await expect
      .poll(async () => {
        const check = await request.get(SETTINGS_URL)
        if (!check.ok()) return null
        const doc = (await check.json()) as {
          appearance?: { theme?: string }
          ui?: { tokens?: unknown }
        }
        return doc.appearance?.theme === 'system' && doc.ui?.tokens === undefined
          ? 'reset'
          : null
      })
      .toBe('reset')
  })

  test('PUT settings re-tints an open tab without a reload, inside the poll budget', async ({
    page,
    request,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)
    expect(await readToken(page, '--color-accent')).toBe(DEFAULT_LIGHT_ACCENT)

    // A reload would clear this; the reflection path must not need one.
    await page.evaluate(() => {
      ;(window as { __gadakNoReload?: string }).__gadakNoReload = 'sentinel'
    })

    const started = Date.now()
    const { status } = await putUI(request, {
      tokens: { colors: { accent: ACCENT } },
      dataColors: { status: { new: STATUS_NEW_INK } },
    })
    expect(status).toBe(200)

    // Timing: from the PUT answer to the repainted var.
    await expect.poll(() => readToken(page, '--color-accent')).toBe(ACCENT)
    const elapsed = Date.now() - started
    expect(elapsed, `live reflect took ${elapsed}ms; budget ${LIVE_REFLECT_BUDGET_MS}ms`).toBeLessThan(
      LIVE_REFLECT_BUDGET_MS,
    )
    expect(
      await page.evaluate(() => (window as { __gadakNoReload?: string }).__gadakNoReload),
    ).toBe('sentinel')

    // dataColors re-tint through the same poll: every "New" status dot is an
    // inline style read from the $state snapshot, so the repaint is reactive.
    // exact: the row's own button folds the dot's aria-label into a longer
    // accessible name, and non-exact getByRole matches substrings.
    const dot = page
      .getByRole('button', { name: 'Filter by category New', exact: true })
      .first()
    await expect
      .poll(async () => dot.evaluate((el) => getComputedStyle(el).backgroundColor))
      .toBe('rgb(192, 48, 48)')

    // Write-through boot cache: the next cold boot must not flash the stock
    // palette while config.json loads.
    expect(await page.evaluate(() => localStorage.getItem('gadak:user-tokens'))).toContain(ACCENT)
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    expect(await readToken(page, '--color-accent')).toBe(ACCENT)
    expect(
      await page.evaluate(
        () => document.querySelector('style[data-gadak-user-tokens]') !== null,
      ),
    ).toBe(true)

    expect(errors).toEqual([])
  })

  // GDK-858 (user decision 2026-08-25): judgment violations warn and SAVE;
  // only parse/shape/derived refusals remain. This spec was revised with the
  // round — FAIL-first against the pre-change source: the old test asserted
  // 400 + config unchanged for exactly this locked write.
  test('locked tokens warn via uiWarnings and still apply; unparseable values refuse', async ({
    page,
    request,
  }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)

    const { status } = await putUI(request, { tokens: { colors: { accent: ACCENT } } })
    expect(status).toBe(200)
    await expect.poll(() => readToken(page, '--color-accent')).toBe(ACCENT)

    // Locked tier: a judgment — 200, the warning rides the response, and the
    // value renders in the open tab.
    const warned = await putUI(request, {
      tokens: { colors: { accent: ACCENT, 'bg-base': '#000000' } },
    })
    expect(warned.status).toBe(200)
    const uiWarnings = (warned.body.uiWarnings ?? []) as { rule?: string; token?: string }[]
    expect(
      uiWarnings.some((w) => w.rule === 'locked' && w.token === 'bg-base'),
      `uiWarnings carried the locked verdict: ${JSON.stringify(uiWarnings)}`,
    ).toBe(true)
    await expect.poll(() => readToken(page, '--color-bg-base')).toBe('#000000')
    const doc = (await (await request.get(SETTINGS_URL)).json()) as {
      ui?: { tokens?: { colors?: Record<string, string> } }
    }
    expect(doc.ui?.tokens?.colors?.['bg-base']).toBe('#000000')

    // Machine check: a non-hex value still refuses, config unchanged.
    // lozenge-red is free tier (hex-only rule), so this is a pure parse case.
    const refused = await putUI(request, {
      tokens: { colors: { accent: ACCENT, 'bg-base': '#000000', 'lozenge-red': 'not-a-color' } },
    })
    expect(refused.status).toBe(400)
    expect(String(refused.body.error)).toContain('hex')
    // one poll cycle + slack: if the refusal had leaked through, the next
    // GET after a cycle is where the wrongly-applied write would show.
    await page.waitForTimeout(650)
    const after = (await (await request.get(SETTINGS_URL)).json()) as {
      ui?: { tokens?: { colors?: Record<string, string> } }
    }
    expect(after.ui?.tokens?.colors?.['bg-base']).toBe('#000000')
    expect(after.ui?.tokens?.colors?.['lozenge-red']).toBeUndefined()
  })

  // GDK-769 R1: the list column's width is settable the way the sidebar's
  // already is. --layout-list has no default value — app.css consumes it as
  // var(--layout-list, <the track that shipped>) — so this test pins both
  // halves: the measured column is exactly what was asked for while the
  // token is set, and the measured layout returns to the untouched one when
  // it is cleared. Measuring the painted grid (getComputedStyle's resolved
  // gridTemplateColumns is in used px) rather than the declaration is what
  // makes this an assertion about the layout and not about the CSS text.
  test('ui.tokens.layout.list pins the list column, and clearing it restores the layout', async ({
    page,
    request,
  }) => {
    await gotoApp(page)
    const layout = page.getByTestId('issue-layout')
    const columns = async () =>
      layout.evaluate((el) => getComputedStyle(el).gridTemplateColumns)
    const listWidth = async () =>
      page
        .locator('.issue-main-column')
        .first()
        .evaluate((el) => Math.round(el.getBoundingClientRect().width))

    const stockColumns = await columns()
    const stockList = await listWidth()
    expect(stockList, 'the unpinned list fills the track it always had').not.toBe(520)

    expect((await putUI(request, { tokens: { layout: { list: '520px' } } })).status).toBe(200)
    await expect.poll(listWidth, { timeout: LIVE_REFLECT_BUDGET_MS }).toBe(520)
    // The pin lands on the list track and nowhere else: the sidebar track is
    // untouched and the freed width goes to the third track (measured: stock
    // "272px 1008px 0px" → pinned "272px 520px 488px"), so the used widths
    // still sum to the same grid.
    const stock = stockColumns.split(' ').map(parseFloat)
    const pinnedTracks = (await columns()).split(' ').map(parseFloat)
    expect(pinnedTracks[0], 'the sidebar track is untouched').toBe(stock[0])
    expect(pinnedTracks[1], 'the list track is the pinned width').toBe(520)
    expect(
      pinnedTracks.reduce((a, b) => a + b, 0),
      'the grid keeps its total width — the pin moves space, it does not add any',
    ).toBe(stock.reduce((a, b) => a + b, 0))

    expect((await putUI(request, { tokens: {} })).status).toBe(200)
    await expect.poll(listWidth, { timeout: LIVE_REFLECT_BUDGET_MS }).toBe(stockList)
    expect(await columns()).toBe(stockColumns)
  })
})
