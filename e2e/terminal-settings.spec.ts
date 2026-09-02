import { expect, test, type APIRequestContext } from '@playwright/test'
import { apiURL, gotoApp, openServerSettings } from './helpers'

/*
 * GDK-1357: the Terminal settings tab. Scrollback, cursor blink and the two
 * text tokens ride the form's Save; the server merges the display fields
 * onto the stored terminal block and replaces the `ui` block whole. Shell
 * and working directory are on the tab as a read-only note with the CLI
 * command, and never in the document (GDK-1069) — the Go test holds that
 * half; this one holds what the tab writes.
 */

const SETTINGS_URL = apiURL('/api/v1/issues/settings/')

type Doc = {
  terminal?: { scrollback: number; cursorBlink: boolean }
  ui?: { tokens?: { type?: Record<string, string>; fonts?: Record<string, string> } }
} & Record<string, unknown>

async function getDoc(request: APIRequestContext): Promise<Doc> {
  const res = await request.get(SETTINGS_URL)
  expect(res.ok()).toBe(true)
  return (await res.json()) as Doc
}

/** Back to defaults, so a later spec's terminal is sized and scrolled the
 *  way the fixture assumes (the font size is a real cell size). */
async function resetTerminalSettings(request: APIRequestContext): Promise<void> {
  const doc = await getDoc(request)
  const ui = { ...(doc.ui ?? {}) }
  const tokens = { ...(ui.tokens ?? {}) }
  delete tokens.type
  delete tokens.fonts
  ui.tokens = tokens
  const put = await request.put(SETTINGS_URL, {
    data: { ...doc, terminal: { scrollback: 0, cursorBlink: false }, ui },
  })
  expect(put.ok(), await put.text()).toBe(true)
}

test.describe('terminal settings tab (GDK-1357)', () => {
  test.afterEach(async ({ request }) => {
    await resetTerminalSettings(request)
  })

  test('scrollback, cursor blink and the text tokens save through the form', async ({
    page,
    request,
  }) => {
    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Terminal', exact: true }).click()
    await expect(page.getByTestId('terminal-settings')).toBeVisible()

    // Untouched: placeholders, not the defaults pinned into the form.
    await expect(page.getByTestId('terminal-scrollback')).toHaveValue('')
    await expect(page.getByTestId('terminal-font-size')).toHaveValue('')
    // The CLI-only pair is named, with its command, and has no input.
    await expect(page.getByTestId('terminal-shell-commands')).toContainText(
      'gadak config set terminal.shell',
    )
    expect(await page.getByTestId('terminal-settings').locator('input').count()).toBe(4)

    await page.getByTestId('terminal-scrollback').fill('20000')
    await page.getByTestId('terminal-cursor-blink').check()
    await page.getByTestId('terminal-font-size').fill('15')
    await page.getByTestId('terminal-font-family').fill('Menlo, monospace')
    await dialog.getByRole('button', { name: 'Save', exact: true }).click()

    await expect
      .poll(async () => {
        const doc = await getDoc(request)
        return {
          terminal: doc.terminal,
          size: doc.ui?.tokens?.type?.terminal ?? null,
          family: doc.ui?.tokens?.fonts?.['mono-terminal'] ?? null,
        }
      })
      .toEqual({
        terminal: { scrollback: 20000, cursorBlink: true },
        size: '15px',
        family: 'Menlo, monospace',
      })

    // Reopened after the reload, the form shows what is stored.
    await gotoApp(page)
    await openServerSettings(page)
    await page
      .getByRole('dialog', { name: 'Settings' })
      .getByRole('button', { name: 'Terminal', exact: true })
      .click()
    await expect(page.getByTestId('terminal-scrollback')).toHaveValue('20000')
    await expect(page.getByTestId('terminal-cursor-blink')).toBeChecked()
    await expect(page.getByTestId('terminal-font-size')).toHaveValue('15')
    await expect(page.getByTestId('terminal-font-family')).toHaveValue('Menlo, monospace')
  })

  test('an out-of-range scrollback is refused by the server, by name', async ({ page }) => {
    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Terminal', exact: true }).click()
    await page.getByTestId('terminal-scrollback').fill('5')
    await dialog.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(dialog.getByText(/terminal\.scrollback must be/)).toBeVisible()
  })
})
