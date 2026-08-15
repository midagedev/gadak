import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * Keyboard triage (v0.7): clearing a sprint without the mouse. The suite runs
 * against the demo mirror with a fake credential, so it stops where a real write
 * would start — the popovers render, nothing is applied.
 */

/** URL + list-count + cursor, for a failure log that does not need a local repro. */
async function triageState(page: Page): Promise<string> {
  const list = await page.getByTestId('list-count').textContent().catch(() => '?')
  const key = await page.evaluate(() => {
    const el = document.querySelector('[data-cursor="true"]')
    return el?.getAttribute('data-issue-key') ?? null
  })
  return `url=${page.url()} list=${list} cursor=${key ?? '-'}`
}

/** Issue key of the row under the cursor, once the row is painted. */
async function cursorKey(page: Page): Promise<string> {
  let key = ''
  await expect
    .poll(async () => {
      key = await page.evaluate(() => {
        const rows = document.querySelectorAll('[data-cursor="true"]')
        if (rows.length !== 1) return ''
        return rows[0].getAttribute('data-issue-key') ?? ''
      })
      return key
    })
    .not.toEqual('')
  return key
}

/** Press a key and wait until the list cursor is an observable row. */
async function pressUntilCursor(page: Page, key: string): Promise<string> {
  await page.keyboard.press(key)
  return cursorKey(page)
}

test.describe('keyboard triage', () => {
  test.afterEach(async ({ page }, info) => {
    if (info.status === info.expectedStatus) return
    console.error(`[triage] ${info.title} ${await triageState(page)}`)
  })

  test('j/k move the cursor, x builds a selection, s opens the status popover', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // ── j/k ──
    const first = await pressUntilCursor(page, 'j')
    expect(first).not.toEqual('')

    const second = await pressUntilCursor(page, 'j')
    expect(second).not.toEqual(first)

    await page.keyboard.press('k')
    expect(await cursorKey(page)).toEqual(first)

    // ── x on two rows ──
    const bar = page.getByTestId('bulk-bar')
    await expect(bar).toBeHidden()

    await page.keyboard.press('x')
    await expect(bar).toBeVisible()
    await expect(bar.getByText('1 selected')).toBeVisible()

    // Assert the cursor actually moved before the second x — otherwise x
    // toggles the same row and "2 selected" never appears (GDK-39).
    const afterFirst = await cursorKey(page)
    const afterMove = await pressUntilCursor(page, 'j')
    expect(afterMove, await triageState(page)).not.toEqual(afterFirst)
    await page.keyboard.press('x')
    await expect(bar.getByText('2 selected')).toBeVisible()

    // ── s opens the status popover over the selection ──
    const statusMenu = page.getByTestId('bulk-status-menu')
    await expect(statusMenu).toBeHidden()
    await page.keyboard.press('s')
    await expect(statusMenu).toBeVisible()
    // The fixture mirror has never talked to Jira, so its transition map is empty
    // and the popover says so. Either way the batch surface is what `s` opened.
    await expect(
      statusMenu.getByRole('option').first().or(statusMenu.getByText('No shared transitions.')),
    ).toBeVisible()

    // Esc closes the popover but keeps the selection — the batch is still armed.
    await page.keyboard.press('Escape')
    await expect(statusMenu).toBeHidden()
    await expect(bar.getByText('2 selected')).toBeVisible()

    // ── a opens the assignee popover on the same selection ──
    await page.keyboard.press('a')
    const assigneeMenu = page.getByTestId('bulk-assignee-menu')
    await expect(assigneeMenu).toBeVisible()
    await expect(assigneeMenu.getByText('Unassigned')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('bulk-assignee-menu')).toBeHidden()

    // ── l opens the labels popover on the same selection ──
    await page.keyboard.press('l')
    const labelsMenu = page.getByTestId('bulk-labels-menu')
    await expect(labelsMenu).toBeVisible()
    await expect(labelsMenu.getByPlaceholder('Type a label')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('bulk-labels-menu')).toBeHidden()

    // ── Esc gives the selection back before it closes anything ──
    await page.keyboard.press('Escape')
    await expect(bar).toBeHidden()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Esc clears the selection before closing the detail panel', async ({ page }) => {
    await gotoApp(page)

    await pressUntilCursor(page, 'j')
    await page.keyboard.press('Enter')
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()

    // Select a second row while the panel is open.
    const before = await cursorKey(page)
    const moved = await pressUntilCursor(page, 'j')
    expect(moved, await triageState(page)).not.toEqual(before)
    await page.keyboard.press('x')
    await expect(page.getByTestId('bulk-bar')).toBeVisible()

    // First Esc: selection only. The panel must survive it.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('bulk-bar')).toBeHidden()
    await expect(panel).toBeVisible()

    // Second Esc: now the panel.
    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()
  })

  test('s on a bare cursor row promotes it into the selection first', async ({ page }) => {
    await gotoApp(page)

    const key = await pressUntilCursor(page, 'j')

    await page.keyboard.press('s')
    const bar = page.getByTestId('bulk-bar')
    await expect(bar).toBeVisible()
    await expect(bar.getByText('1 selected')).toBeVisible()
    await expect(page.getByTestId('bulk-status-menu')).toBeVisible()
    // The promoted row is the one the cursor was on.
    await expect(page.locator(`[data-issue-key="${key}"][aria-pressed="true"]`)).toHaveCount(0)
    await expect(
      page.locator(`[data-issue-key="${key}"] button[aria-pressed="true"]`),
    ).toHaveCount(1)
  })

  test('c composes on the cursor row without opening the detail panel', async ({ page }) => {
    await gotoApp(page)

    const key = await pressUntilCursor(page, 'j')

    await page.keyboard.press('c')
    const dialog = page.getByTestId('quick-comment')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText(key, { exact: true })).toBeVisible()
    await expect(dialog.getByTestId('comment-composer')).toBeFocused()
    // The list stayed where it was.
    await expect(page.getByTestId('issue-detail-panel')).toBeHidden()

    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })

  test('the palette carries the same actions, named for their target', async ({ page }) => {
    await gotoApp(page)

    await pressUntilCursor(page, 'j')
    await page.keyboard.press('x')
    const key = await cursorKey(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await expect(palette.getByText('Change status · 1 selected')).toBeVisible()
    await expect(palette.getByText('Change assignee · 1 selected')).toBeVisible()
    await expect(palette.getByText('Change labels · 1 selected')).toBeVisible()
    await expect(palette.getByText(`Comment on ${key}`)).toBeVisible()

    // Running it from the palette lands in the same popover the key opens.
    await palette.getByText('Change status · 1 selected').click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('bulk-status-menu')).toBeVisible()
  })

  test('the cheat sheet documents the triage keys', async ({ page }) => {
    await gotoApp(page)

    await page.keyboard.press('?')
    const sheet = page.getByTestId('shortcuts-dialog')
    await expect(sheet).toBeVisible()
    for (const label of [
      'Select the row under the cursor',
      'Change status (selection, or the cursor row)',
      'Change assignee (selection, or the cursor row)',
      'Change labels (selection, or the cursor row)',
      'Comment on the row under the cursor',
      'Clear the selection, then close the detail panel',
    ]) {
      await expect(sheet.getByText(label, { exact: true })).toBeVisible()
    }
  })
})
