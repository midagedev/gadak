import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, appConsoleErrors, drainTerminalSessions, forceLocale, gotoApp } from './helpers'
import { BOUND_ISSUE, openPaneBoundTo } from './issue-command-fixture'

/*
 * GDK-1196 / GDK-1197 — the two ways into an issue's session that are not the
 * issue body: the ⌘K palette, and the card on the board.
 *
 * Both assert the same thing, through the same channel the body's ▶ uses:
 * after the affordance is clicked, the pane is holding *that* session. The
 * pin is `data-selected` on the strip row of the bound session, and a second
 * session is opened first precisely so the assertion can fail — with one
 * shell in the table "the pane is on the bound session" is true before
 * anything is clicked.
 *
 * Nothing here reads the terminal buffer: the claim is about which session
 * the pane holds, which is DOM state, so this spec is outside the wrapped-
 * prompt class of fragility entirely.
 */

/* Wide enough that the pane is a split rather than an overlay — an overlay
 * covers the board the card lives on (same reason as issue-command.spec.ts). */
test.use({ viewport: { width: 1600, height: 900 } })

const BOARD_URL = '/#/?sc=new,inprogress,done&g=status_category&ly=board'

function stripRow(page: Page, id: string) {
  return page.locator(`[data-testid="terminal-strip-row"][data-session-id="${id}"]`)
}

test.describe('entering an issue’s session', () => {
  test.beforeEach(async ({ page }) => {
    await forceLocale(page, 'en')
  })

  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  test('the palette offers the shell bound to an issue, and enters it', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const boundId = await openPaneBoundTo(page, BOUND_ISSUE)
    await page.getByTestId('terminal-new').click()
    await expect(stripRow(page, boundId)).toHaveAttribute('data-selected', 'false')

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type(BOUND_ISSUE, { delay: 20 })

    // The row is a visible one, under the issue it belongs to — not a hidden
    // chord. It appears once the palette's poll has seen the binding.
    const row = page.getByTestId('palette-shell-row')
    await expect(row).toBeVisible({ timeout: 20_000 })
    await expect(row).toContainText(BOUND_ISSUE)
    await row.click()

    await expect(palette).toBeHidden()
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(stripRow(page, boundId)).toHaveAttribute('data-selected', 'true')
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a board card with a shell enters it without selecting the card', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.goto(BOARD_URL)
    await expect(page.getByTestId('board-card').first()).toBeVisible({ timeout: 30_000 })

    const boundId = await openPaneBoundTo(page, BOUND_ISSUE)
    await page.getByTestId('terminal-new').click()
    await expect(stripRow(page, boundId)).toHaveAttribute('data-selected', 'false')

    const card = page.locator(`[data-testid="board-card"][data-board-key="${BOUND_ISSUE}"]`)
    await expect(card).toHaveAttribute('data-shell', /needs|running|quiet/, { timeout: 20_000 })
    const enter = card.getByTestId('board-card-shell-enter')
    // Hover-revealed rather than always drawn — but it is a real element with
    // a role, not a chord, so hovering the card is all it takes to find it.
    await card.hover()
    await expect(enter).toBeVisible()
    await enter.click()

    await expect(stripRow(page, boundId)).toHaveAttribute('data-selected', 'true')
    // The click belonged to the shell, not to the card: no detail opened.
    await expect(page.getByTestId('issue-detail-panel')).not.toHaveClass(/is-open/)
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
