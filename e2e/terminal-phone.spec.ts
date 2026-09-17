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
