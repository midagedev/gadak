/*
 * The terminal in a phone browser (GDK-1987, GDK-1986).
 *
 * `tailscale serve` made the web UI the zero-install front door, and a phone
 * browser is inside the overlay regime (<=899px) that was written for a
 * laptop at 900. This spec measures the shell at a phone's own viewport,
 * where the two defects that regime had no floor for live: a sheet wider
 * than the screen, and a surface a finger cannot type into.
 *
 * Playwright cannot raise an iOS keyboard, so the input contract measured
 * here is where focus lands from a real gesture — the thing iOS requires
 * before it will raise one. The keyboard itself is a phone verification.
 */
import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { drainTerminalSessions, forceLocale } from './helpers'

// iPhone 16/17 Pro's CSS viewport, the same box mobile/e2e measures at.
const PHONE = { width: 402, height: 874 }

async function bootPhone(page: Page): Promise<void> {
  await page.setViewportSize(PHONE)
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
}

/** Whether the pane's xterm helper textarea is the focused element. */
const paneHoldsFocus = (page: Page): Promise<boolean> =>
  page.evaluate(() => {
    const ta = document.querySelector('[data-testid="terminal-pane"] textarea')
    return !!ta && document.activeElement === ta
  })

async function openPane(page: Page): Promise<void> {
  // The sidebar's own verb: at this width the keyboard chord is not the
  // road a phone has, and this is the control a thumb reaches for.
  await page.getByTestId('sidebar-terminal').click()
  await expect(page.getByTestId('terminal-pane')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
}

test.afterEach(async ({ page }) => {
  await drainTerminalSessions(page)
})

test('the sheet stays inside the screen at phone width (GDK-1987)', async ({ page }) => {
  await bootPhone(page)
  await openPane(page)

  const box = await page.getByTestId('terminal-pane').evaluate((el) => {
    const r = el.getBoundingClientRect()
    return { left: r.left, right: r.right, width: r.width }
  })
  const inner = await page.evaluate(() => window.innerWidth)

  // The defect this pins: left resolved to the sidebar's 208px outright, the
  // 320px min-width then won over the 194px the viewport could offer, and the
  // box ran 208px past the right edge of a page that does not scroll
  // sideways. Measured 2026-09-17 before the fix: left 208, right 610.
  expect(inner).toBe(PHONE.width)
  expect(box.right).toBeLessThanOrEqual(inner + 1)
  expect(box.left).toBeGreaterThanOrEqual(0)

  // And the shell itself has a real column to draw in, not a sliver: the
  // pane's own 320px floor less the sheet's 1px left border. Before the fix
  // the host measured 401 wide starting at x=209 — 193 of it on screen.
  const host = await page
    .locator('[data-testid="terminal-pane"] .terminal-host')
    .evaluate((el) => el.getBoundingClientRect().width)
  expect(host).toBeGreaterThanOrEqual(319)

  // Nothing was pushed off the document either.
  const scrollW = await page.evaluate(() => document.scrollingElement?.scrollWidth ?? 0)
  expect(scrollW).toBeLessThanOrEqual(inner + 1)
})

test('the sheet stays inside the screen across the whole overlay regime (GDK-1987)', async ({
  page,
}) => {
  // The defect was never phone-only: `w-full` on a `fixed` box makes
  // width:100% of the viewport, which over-constrains left+width+right and
  // drops the right anchor — so the sheet ran past the right edge by exactly
  // the sidebar's width at every width the overlay paints at. 880 is a
  // laptop inside the regime; the phone case above is the other end.
  await page.setViewportSize({ width: 880, height: 900 })
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await openPane(page)

  const { right, left } = await page.getByTestId('terminal-pane').evaluate((el) => {
    const r = el.getBoundingClientRect()
    return { right: r.right, left: r.left }
  })
  expect(right).toBeLessThanOrEqual(880 + 1)
  // The sidebar is still standing beside it — that is what the overlay
  // regime is for, and the phone case is the one that gives it up.
  expect(left).toBeGreaterThan(0)
})

/*
 * The second defect the header names: a surface a finger cannot type into.
 *
 * `hasTouch` is what makes `(pointer: coarse)` match in Chromium (measured:
 * false by default, true with the flag), which is the one input the pane's
 * focus policy reads. Playwright still cannot raise a software keyboard, so
 * what is pinned here is the precondition iOS needs — that the tap is a
 * focus change and not a no-op — not the keyboard itself.
 */
test.describe('a finger keeps its own focus change (GDK-1986)', () => {
  test.use({ hasTouch: true })

  test('the pane does not take the keyboard when the socket attaches', async ({ page }) => {
    await bootPhone(page)
    await openPane(page)

    // The defect: `onAttached` focused the renderer unconditionally, so the
    // helper textarea already held focus by the time the user could tap —
    // and a focus that lands outside a gesture never raises an iOS keyboard.
    // The tap that followed changed nothing, so nothing raised one either.
    await expect
      .poll(() => paneHoldsFocus(page), { timeout: 5_000 })
      .toBe(false)

    // xterm's own path, the one the device probe measured raising a keyboard.
    await page.locator('[data-testid="terminal-pane"] .xterm-screen').tap()
    await expect.poll(() => paneHoldsFocus(page)).toBe(true)
  })
})

/*
 * The soft-key row (GDK-1995). A phone keyboard has no Esc, Ctrl or arrows,
 * so the pane carries them in a bar that paints exactly where a physical
 * keyboard cannot be: `(pointer: coarse) and (hover: none)`. Playwright
 * cannot raise a software keyboard, so what is measurable is the same shape
 * as GDK-1986 above: the row is present on a touch-only context, absent on
 * a hovering one, and pressing a key sends bytes. What a press *means* in
 * bytes is asserted in web/src/lib/terminal/keys.test.ts — the encoder is a
 * pure function over plain data, and its defects are invisible in a
 * screenshot. This tier proves the row exists and is wired.
 */
test.describe('the soft-key row sends bytes (GDK-1995)', () => {
  // The touch-only emulation mobile/e2e runs under (mobile/playwright.config.ts):
  // isMobile + hasTouch is what makes both halves of the query match.
  test.use({ hasTouch: true, isMobile: true })

  test('the row is present on a touch-only context', async ({ page }) => {
    await bootPhone(page)
    await openPane(page)

    // Guard against emulation drift first: if this stops matching, a missing
    // row below is the rig, not the product.
    const media = await page.evaluate(() => ({
      coarse: matchMedia('(pointer: coarse)').matches,
      hoverNone: matchMedia('(hover: none)').matches,
    }))
    expect(media).toEqual({ coarse: true, hoverNone: true })

    await expect(page.getByTestId('key-bar')).toBeVisible({ timeout: 15_000 })
  })

  test('pressing a key sends its bytes to the shell', async ({ page }) => {
    await bootPhone(page)
    await openPane(page)
    const bar = page.getByTestId('key-bar')
    await expect(bar).toBeVisible({ timeout: 15_000 })

    const rows = page.locator('[data-testid="terminal-pane"] .xterm-rows')
    const before = (await rows.textContent()) ?? ''

    // Three punctuation keys the bar owns. The shell echoes each byte it
    // receives, so the echoed run proves press -> bytes -> PTY -> paint
    // end to end; the byte values themselves are the unit tier's job.
    await bar.getByRole('button', { name: '/' }).tap()
    await bar.getByRole('button', { name: '~' }).tap()
    await bar.getByRole('button', { name: '-' }).tap()
    await expect.poll(() => rows.textContent(), { timeout: 15_000 }).toContain('/~-')

    // And the run was not already on screen — a prompt or env line carrying
    // it would fail here loudly instead of passing quietly.
    expect(before).not.toContain('/~-')
  })
})

test('the row stays out of the way where a keyboard exists (GDK-1995)', async ({
  page,
}) => {
  // Default desktop context: fine pointer that hovers. The media query does
  // not match there — a touch laptop has a physical Esc key, so a permanent
  // strip is clutter — and the row must not paint.
  await bootPhone(page)
  await openPane(page)
  await expect(page.getByTestId('key-bar')).toBeHidden({ timeout: 15_000 })
})
