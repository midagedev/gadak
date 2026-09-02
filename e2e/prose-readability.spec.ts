/*
 * The description is prose, and prose has its own rules (GDK-1311).
 *
 * Measured before this landed, on a 2.5KB Korean description at 1440px:
 * body 13px on a 421px column (≈30 Hangul per line), paragraph gap 7px
 * against a 21px line, and h3 the body's own size in text-secondary —
 * a heading dimmer than the paragraph it titles. The spec pins the three
 * things a reader needs: a heading ramp that stands above the body, a
 * paragraph gap wider than the line gap, and a way to give the body the
 * width of the window.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

// A default-view fixture whose description carries several paragraphs.
const KEY = 'NMB-139'

const row = (page: Page, key: string) =>
  page.getByTestId('issue-list-scroller').locator(`[data-issue-key="${key}"]`)

async function openIssue(page: Page): Promise<void> {
  await gotoApp(page)
  // The search box offers the key's row with "Open with Enter" — the same
  // door a person takes; clicking the list row under the open suggestion
  // was flaky at 1440px.
  await searchInput(page).fill(KEY)
  await expect(row(page, KEY)).toBeVisible()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)
  await expect(page.getByTestId('issue-body').locator('p').first()).toBeVisible()
}

/** Computed metrics of the rendered body — and of a heading rendered under
 *  the same stylesheet, since the fixture's descriptions carry none. */
async function metrics(page: Page) {
  return page.evaluate(() => {
    const body = document.querySelector('[data-testid="issue-body"]') as HTMLElement
    const p = body.querySelector('p') as HTMLElement
    const h3 = document.createElement('h3')
    h3.textContent = 'probe'
    body.appendChild(h3)
    const ps = getComputedStyle(p)
    const hs = getComputedStyle(h3)
    const out = {
      bodyWidth: body.getBoundingClientRect().width,
      pSize: parseFloat(ps.fontSize),
      pLine: parseFloat(ps.lineHeight),
      pGap: parseFloat(ps.marginTop) + parseFloat(ps.marginBottom),
      pColor: ps.color,
      h3Size: parseFloat(hs.fontSize),
      h3Color: hs.color,
    }
    h3.remove()
    return out
  })
}

test.describe('description prose (GDK-1311)', () => {
  test('headings stand above the body, paragraphs read as units', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await openIssue(page)
    const m = await metrics(page)

    // The body is prose-sized, not chip-sized: the list token is 13px.
    expect(m.pSize).toBeGreaterThanOrEqual(14)
    // Paragraphs separate: the gap between two paragraphs is wider than the
    // gap between two lines inside one (line-height minus font-size).
    expect(m.pGap).toBeGreaterThan(m.pLine - m.pSize)
    // A heading is not dimmer than its paragraph, and it is not body-size.
    expect(m.h3Color).toBe(m.pColor)
    expect(m.h3Size).toBeGreaterThanOrEqual(m.pSize)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('wide reading hands the body the window and is remembered', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await openIssue(page)
    const layout = page.getByTestId('issue-layout')
    await expect(layout).toHaveAttribute('data-detail-wide', 'false')
    const narrow = (await metrics(page)).bodyWidth

    const toggle = page.getByTestId('issue-detail-wide')
    await expect(toggle).toHaveAttribute('aria-pressed', 'false')
    await toggle.click()
    await expect(layout).toHaveAttribute('data-detail-wide', 'true')
    await expect(toggle).toHaveAttribute('aria-pressed', 'true')
    // The list keeps its minimum track; the body takes the rest — at 1440
    // that is well over one and a third of the docked width.
    await expect
      .poll(async () => (await metrics(page)).bodyWidth / narrow)
      .toBeGreaterThan(1.3)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()

    // A display preference of this browser: it survives a reload and never
    // enters the URL.
    expect(page.url()).not.toMatch(/wide/)
    await page.reload()
    await expect(page.getByTestId('issue-body').locator('p').first()).toBeVisible()
    await expect(layout).toHaveAttribute('data-detail-wide', 'true')

    await page.getByTestId('issue-detail-wide').click()
    await expect(layout).toHaveAttribute('data-detail-wide', 'false')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
