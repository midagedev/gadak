import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from './helpers'
import { gotoApp } from './helpers'

/*
 * GDK-1598: the dashboard tab must not flash white on the way in.
 *
 * Measured on the ja hero take: the body column's mean luminance sits at
 * 200-215 on the list and in the 70s on the dashboard, but two frames at
 * 40.97s and 41.00s read 232 — about 70ms of pure white between the cream
 * list and the dark dashboard. The cause is in the host, not the authored
 * document: DashboardView painted the <iframe> element itself `bg-white`,
 * so the element's own background showed for the span between the frame
 * being attached and its first paint. In a cream-and-dark UI a white frame
 * is the one colour that reads as a fault.
 *
 * The assertion is on the resolved background, never on frame timing — the
 * flash is a paint race and a timing assertion would be non-deterministic.
 * A themed background makes the race invisible whether it happens or not,
 * which is the actual fix: there is no longer a wrong colour to catch.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const PREFIX = 'gdk1598'

/** rgb(255,255,255) in any of the forms getComputedStyle may hand back. */
function isWhite(css: string): boolean {
  const m = css.match(/rgba?\(([^)]+)\)/)
  if (!m) return /^#(fff|ffffff)$/i.test(css.trim())
  const [r, g, b, a] = m[1].split(/[,/]/).map((p) => parseFloat(p))
  if (a !== undefined && a === 0) return false
  return r === 255 && g === 255 && b === 255
}

test('the dashboard frame carries a theme background, never white', async ({ page, request }) => {
  const html = readFileSync(join(E2E_DIR, '..', 'examples', 'dashboards', 'triage.html'), 'utf8')
  const name = `${PREFIX}-${Date.now()}`
  const created = await request.post('/api/v1/dashboards/', {
    data: { name, config: { html, datasources: {} } },
  })
  expect(created.ok(), await created.text()).toBeTruthy()
  const saved = (await created.json()) as { id: string }

  try {
    await gotoApp(page)
    await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
    const view = page.getByTestId('dashboard-view')
    await expect(view).toBeVisible()

    const frame = page.getByTestId('dashboard-frame')
    await expect(frame).toBeVisible()

    const bg = await frame.evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(isWhite(bg), `dashboard frame background is ${bg} — a white frame is the flash`).toBe(
      false,
    )

    // ...and it is the app's own background, not merely "not white": the
    // container behind the frame and the frame itself resolve to the same
    // colour, so no seam appears at any point during the swap.
    const hostBg = await view.evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(bg, 'frame and dashboard column share one background token').toBe(hostBg)
  } finally {
    await request.delete(`/api/v1/dashboards/${saved.id}/`)
  }
})
