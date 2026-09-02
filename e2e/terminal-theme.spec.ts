import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { TERMINAL_CHROME_VARS } from '../web/src/lib/terminal/protocol'
import { apiURL, drainTerminalSessions, forceLocale, openServerSettings } from './helpers'

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
 *
 * GDK-1357 gave the dock an appearance of its own — dark under every app
 * theme by default, `follow` to take the app palette — so the three tests
 * above run under `follow` (the mode in which the pane is supposed to move
 * with the theme), and the reference tokens are read off the pane's host,
 * where the dock's own palette computes, not off <html>. The dark default
 * and the switch have tests of their own below.
 */

const SETTINGS_URL = apiURL('/api/v1/issues/settings/')

/** The dark palette's page ground (theme.spec.ts DARK.base) — what the dock
 *  paints under a light app theme by default. */
const DARK_BASE = '#0d0e10'

type AppearanceDoc = { appearance?: { theme?: string; terminal?: string } }

/** Store the dock appearance on the server before the page boots: the app
 *  hydrates appearance from the settings GET after first paint, so a fresh
 *  context (empty localStorage) lands on whatever this wrote. */
async function setServerTerminalAppearance(
  request: APIRequestContext,
  pref: 'dark' | 'follow',
): Promise<void> {
  const res = await request.get(SETTINGS_URL)
  expect(res.ok(), `GET settings → ${res.status()}`).toBe(true)
  const doc = (await res.json()) as AppearanceDoc & Record<string, unknown>
  const put = await request.put(SETTINGS_URL, {
    data: { ...doc, appearance: { ...doc.appearance, terminal: pref } },
  })
  expect(put.ok(), `PUT settings → ${put.status()} ${await put.text()}`).toBe(true)
}

async function serverTerminalAppearance(request: APIRequestContext): Promise<string | null> {
  const res = await request.get(SETTINGS_URL)
  if (!res.ok()) return null
  const body = (await res.json()) as AppearanceDoc
  return body.appearance?.terminal ?? null
}

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

/** The same two slots as the stylesheet currently computes them — on the
 *  pane's host, which is where the dock's own palette lands (GDK-1357). The
 *  names come from the list's one owner (protocol.ts), so a token rename
 *  moves the gate with the code instead of quietly comparing '' to ''. */
async function documentChrome(page: Page) {
  const names = {
    background: TERMINAL_CHROME_VARS.background,
    foreground: TERMINAL_CHROME_VARS.foreground,
  }
  const got = await page.evaluate((n) => {
    const host = document.querySelector('[data-testid="terminal-pane"] .terminal-host')
    if (!host) throw new Error('no terminal host — the pane is not open')
    const cs = getComputedStyle(host)
    return {
      background: cs.getPropertyValue(n.background).trim(),
      foreground: cs.getPropertyValue(n.foreground).trim(),
    }
  }, names)
  expect(got.background, `${names.background} computed empty — the token moved`).not.toBe('')
  expect(got.foreground, `${names.foreground} computed empty — the token moved`).not.toBe('')
  return got
}

/** The page's own ground, off <html>: what the dock paints under `follow`
 *  and departs from under `dark`. */
async function pageBase(page: Page): Promise<string> {
  return page.evaluate(
    (v) => getComputedStyle(document.documentElement).getPropertyValue(v).trim(),
    TERMINAL_CHROME_VARS.background,
  )
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
  // These three are about the pane moving with the app theme, which is the
  // `follow` mode's contract (GDK-1357); the default `dark` is tested below.
  test.beforeEach(async ({ request }) => {
    await setServerTerminalAppearance(request, 'follow')
  })
  test.afterEach(async ({ page, request }) => {
    await drainTerminalSessions(page)
    await setServerTerminalAppearance(request, 'dark')
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
    await expect(page.locator('html')).toHaveAttribute('data-terminal-theme', 'follow')
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

/*
 * GDK-1357: the dock's own appearance. Dark by default under every app
 * theme — xterm's ANSI palette is a dark-ground palette (on paper, 9 of its
 * 16 colours fall under 3.0:1), and the band of ink is the seam the dock
 * otherwise lacks under a light page. `follow` hands it the app palette.
 * Asserted off the live terminal's applied theme and the host's computed
 * tokens, like the tests above.
 */
test.describe('the dock has an appearance of its own (GDK-1357)', () => {
  test.afterEach(async ({ page, request }) => {
    await drainTerminalSessions(page)
    await setServerTerminalAppearance(request, 'dark')
  })

  test('dark under a light app theme, by default', async ({ page, request }) => {
    await setServerTerminalAppearance(request, 'dark')
    await page.emulateMedia({ colorScheme: 'light' })
    await boot(page)
    await (await themePicker(page)).selectOption('light')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    await page.keyboard.press('Escape')
    await openPane(page)

    // No `follow` attribute, so the stylesheet's dock scope applies.
    await expect(page.locator('html')).toHaveAttribute('data-terminal-theme', 'dark')
    const host = await documentChrome(page)
    expect(host.background).toBe(DARK_BASE)
    expect(await pageBase(page)).not.toBe(DARK_BASE)
    // The live terminal painted what the host computes — not the page.
    await expect.poll(async () => await appliedChrome(page), { timeout: 5_000 }).toEqual(host)
    // And the whole dock — roster, status line — is one dark surface.
    const dock = await page.evaluate(() => {
      const pane = document.querySelector('[data-testid="terminal-pane"]')!
      const roster = document.querySelector('[data-testid="terminal-chrome"]')!
      return {
        scheme: getComputedStyle(pane).colorScheme,
        pane: getComputedStyle(pane).backgroundColor,
        roster: getComputedStyle(roster).backgroundColor,
      }
    })
    expect(dock.scheme).toBe('dark')
    expect(dock.pane).toBe('rgb(13, 14, 16)')
    expect(dock.roster).toBe('rgb(19, 20, 23)')
  })

  test('the Terminal settings tab switches it to follow and back, live', async ({
    page,
    request,
  }) => {
    await setServerTerminalAppearance(request, 'dark')
    await page.emulateMedia({ colorScheme: 'light' })
    await boot(page)
    await (await themePicker(page)).selectOption('light')
    await page.keyboard.press('Escape')
    await openPane(page)
    expect((await documentChrome(page)).background).toBe(DARK_BASE)

    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Terminal', exact: true }).click()
    const picker = page.getByTestId('terminal-appearance-picker')
    await expect(picker).toHaveValue('dark')
    await picker.selectOption('follow')
    await expect(page.locator('html')).toHaveAttribute('data-terminal-theme', 'follow')
    // The open pane retints without a reload — the watcher sees the attribute.
    const paper = await pageBase(page)
    await expect
      .poll(async () => (await appliedChrome(page)).background, { timeout: 5_000 })
      .toBe(paper)
    expect((await documentChrome(page)).background).toBe(paper)
    // Written through, keeping the sibling theme field.
    await expect.poll(() => serverTerminalAppearance(request)).toBe('follow')
    const doc = (await (await request.get(SETTINGS_URL)).json()) as AppearanceDoc
    expect(doc.appearance?.theme).toBe('light')

    await picker.selectOption('dark')
    await expect
      .poll(async () => (await appliedChrome(page)).background, { timeout: 5_000 })
      .toBe(DARK_BASE)
    await expect.poll(() => serverTerminalAppearance(request)).toBe('dark')
  })

  test('a theme click does not reset the dock appearance', async ({ page, request }) => {
    await setServerTerminalAppearance(request, 'follow')
    await boot(page)
    await (await themePicker(page)).selectOption('ember')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'ember')
    // The theme write-through spreads the block; `terminal` survives.
    await expect
      .poll(async () => {
        const doc = (await (await request.get(SETTINGS_URL)).json()) as AppearanceDoc
        return `${doc.appearance?.theme}/${doc.appearance?.terminal}`
      })
      .toBe('ember/follow')
    await (await themePicker(page)).selectOption('system')
  })
})
