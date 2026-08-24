import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
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

  test('locked tokens are refused with the reason and the open tab is untouched', async ({
    page,
    request,
  }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)

    const { status } = await putUI(request, { tokens: { colors: { accent: ACCENT } } })
    expect(status).toBe(200)
    await expect.poll(() => readToken(page, '--color-accent')).toBe(ACCENT)

    const refused = await putUI(request, {
      tokens: { colors: { accent: ACCENT, 'bg-base': '#000000' } },
    })
    expect(refused.status).toBe(400)
    expect(String(refused.body.error)).toContain('locked')

    // A refused write must not clobber what the tab already shows. Wait one
    // poll cycle so a buggy refetch would have had its chance.
    await page.waitForTimeout(650)
    expect(await readToken(page, '--color-accent')).toBe(ACCENT)
    const doc = (await (await request.get(SETTINGS_URL)).json()) as {
      ui?: { tokens?: { colors?: Record<string, string> } }
    }
    expect(doc.ui?.tokens?.colors?.['bg-base']).toBeUndefined()

    // And the refusal left no boot-cache pollution behind.
    expect(await page.evaluate(() => localStorage.getItem('gadak:user-tokens'))).toContain(ACCENT)
  })
})
