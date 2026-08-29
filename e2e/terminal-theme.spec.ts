import { test, expect, type Page } from '@playwright/test'
import { TERMINAL_CHROME_VARS } from '../web/src/lib/terminal/protocol'
import { drainTerminalSessions, forceLocale, openServerSettings } from './helpers'

/*
 * GDK-1156: the terminal's chrome has to follow the theme it is sitting in.
 *
 * The renderer reads the chrome tokens once, at construction (the list is
 * TERMINAL_CHROME_VARS in web/src/lib/terminal/protocol.ts — --color-bg-base
 * and friends), and hands them to xterm as its `theme`. Nothing re-read them
 * afterwards, so every live path that moves those variables left the pane
 * painted in the palette it was born in:
 *
 *   - the theme picker (data-theme on <html>: light / dark / ink / ember)
 *   - the OS, when the preference is `system` (prefers-color-scheme)
 *   - `gadak config set ui.tokens` (a stylesheet swap, no reload — the whole
 *     point of that feature is that an open tab retints)
 *
 * Measured on a phone first (the 0.19 hero shoot): iOS flipped to dark at
 * sunset mid-session and the pane went black with the previous scheme's ink
 * still in it — the command output that had just landed read as an empty
 * terminal. The web pane has four themes and a picker, so it is the cheaper
 * and stricter place to hold the contract.
 *
 * What is asserted is the LIVE terminal's own applied theme, read back off
 * the test hook, against the computed variables — not a screenshot. A
 * screenshot would pass on a pane that happens to be mid-repaint, and it
 * cannot tell "followed the theme" from "was constructed in it".
 */

type ThemeHook = {
  options: {
    theme?: {
      background?: string
      foreground?: string
      cursor?: string
      cursorAccent?: string
      selectionBackground?: string
    }
  }
}

/** The live terminal's applied chrome, as xterm holds it. */
async function appliedChrome(page: Page) {
  return page.evaluate(() => {
    const t = (window as unknown as { __gadakTerm?: ThemeHook }).__gadakTerm
    if (!t) throw new Error('no __gadakTerm — the pane never created a terminal')
    const th = t.options.theme
    if (!th) throw new Error('the live terminal carries no theme')
    return { background: th.background ?? '', foreground: th.foreground ?? '' }
  })
}

/** The same two slots as the stylesheet currently computes them. The names
 *  come from the list's one owner (protocol.ts), so a token rename moves the
 *  gate with the code instead of quietly comparing '' to ''. */
async function documentChrome(page: Page) {
  const names = {
    background: TERMINAL_CHROME_VARS.background,
    foreground: TERMINAL_CHROME_VARS.foreground,
  }
  const got = await page.evaluate((n) => {
    const cs = getComputedStyle(document.documentElement)
    return {
      background: cs.getPropertyValue(n.background).trim(),
      foreground: cs.getPropertyValue(n.foreground).trim(),
    }
  }, names)
  expect(got.background, `${names.background} computed empty — the token moved`).not.toBe('')
  expect(got.foreground, `${names.foreground} computed empty — the token moved`).not.toBe('')
  return got
}

async function boot(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
}

/** Settings dialog select — the picker's only handle (see theme.spec.ts). */
async function themePicker(page: Page) {
  const picker = page.getByTestId('theme-picker')
  if (await picker.isVisible()) return picker
  await openServerSettings(page)
  await expect(picker).toBeVisible()
  return picker
}

async function openPane(page: Page): Promise<void> {
  await page.keyboard.press('Control+Backquote')
  await expect(page.getByTestId('terminal-pane')).toBeVisible()
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
}

test.describe('terminal chrome follows the theme', () => {
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  test('the picker retints the open pane (GDK-1156)', async ({ page }) => {
    await boot(page)
    await openPane(page)

    // Start from an explicit light so the run does not depend on the CI
    // machine's OS preference.
    await (await themePicker(page)).selectOption('light')
    await expect
      .poll(async () => (await appliedChrome(page)).background)
      .toBe((await documentChrome(page)).background)
    const light = await appliedChrome(page)

    await (await themePicker(page)).selectOption('dark')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    const wantDark = await documentChrome(page)
    expect(wantDark.background).not.toBe(light.background)

    // The contract. Before the fix this held the light palette forever.
    await expect
      .poll(async () => await appliedChrome(page), { timeout: 5_000 })
      .toEqual({ background: wantDark.background, foreground: wantDark.foreground })

    // A third palette, so the assertion cannot pass by a light/dark coin flip.
    await (await themePicker(page)).selectOption('ember')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'ember')
    const wantEmber = await documentChrome(page)
    await expect
      .poll(async () => await appliedChrome(page), { timeout: 5_000 })
      .toEqual({ background: wantEmber.background, foreground: wantEmber.foreground })
  })

  /*
   * The third live path. `gadak config set ui.tokens` reaches the page as a
   * <style> element in <head> whose textContent is replaced on every write
   * (web/src/lib/user-tokens.ts applyUserTokens) — no reload, no attribute,
   * nothing on <html> at all. This drives that same mutation directly rather
   * than through the CLI: the round that would stand a config-write fixture
   * up is bigger than this fix, and what is under test here is whether the
   * renderer notices a stylesheet swap, which is exactly what this is.
   */
  test('a token stylesheet swap retints the open pane (GDK-1156)', async ({ page }) => {
    await boot(page)
    await openPane(page)
    await (await themePicker(page)).selectOption('light')
    await expect
      .poll(async () => (await appliedChrome(page)).background)
      .toBe((await documentChrome(page)).background)

    const bgVar = TERMINAL_CHROME_VARS.background
    await page.evaluate((v) => {
      const el = document.createElement('style')
      el.setAttribute('data-gadak-user-tokens-probe', '')
      el.textContent = `:root { ${v}: #123456; }`
      document.head.appendChild(el)
    }, bgVar)
    await expect.poll(async () => (await documentChrome(page)).background).toBe('#123456')
    await expect
      .poll(async () => (await appliedChrome(page)).background, { timeout: 5_000 })
      .toBe('#123456')

    // And again by replacing the element's text, which is how every write
    // after the first one arrives.
    await page.evaluate(() => {
      const el = document.querySelector('style[data-gadak-user-tokens-probe]')
      if (!el) throw new Error('probe stylesheet vanished')
      el.textContent = el.textContent!.replace('#123456', '#654321')
    })
    await expect
      .poll(async () => (await appliedChrome(page)).background, { timeout: 5_000 })
      .toBe('#654321')
  })

  test('the OS scheme retints it when the preference is system (GDK-1156)', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await boot(page)
    await openPane(page)
    await (await themePicker(page)).selectOption('system')
    await expect(page.locator('html')).not.toHaveAttribute('data-theme', /.+/)

    await expect
      .poll(async () => (await appliedChrome(page)).background)
      .toBe((await documentChrome(page)).background)
    const light = await appliedChrome(page)

    // The phone path, in the one place a test can drive it: no attribute
    // changes, no picker, only the media query moving under the app.
    await page.emulateMedia({ colorScheme: 'dark' })
    const wantDark = await documentChrome(page)
    expect(wantDark.background).not.toBe(light.background)
    await expect
      .poll(async () => await appliedChrome(page), { timeout: 5_000 })
      .toEqual({ background: wantDark.background, foreground: wantDark.foreground })
  })
})
