import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import { gotoApp, openServerSettings } from './helpers'
import { THEMES } from '../web/src/lib/theme'

const SETTINGS_URL = 'http://127.0.0.1:7877/api/v1/issues/settings/'

async function waitServerTheme(request: APIRequestContext, theme: string): Promise<void> {
  await expect
    .poll(async () => {
      const res = await request.get(SETTINGS_URL)
      if (!res.ok()) return null
      const body = (await res.json()) as { appearance?: { theme?: string } }
      return body.appearance?.theme ?? null
    })
    .toBe(theme)
}

/** Settings dialog select — same handle the sidebar picker used to carry. */
async function themePicker(page: Page) {
  const picker = page.getByTestId('theme-picker')
  if (await picker.isVisible()) return picker
  // After a reload, issue-layout paints before boot overlays (credential
  // form, etc.) finish. Wait for the pool label — same signal locale.spec
  // uses — so Settings is not clicked through a z-50 presentation layer.
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
  await openServerSettings(page)
  await expect(picker).toBeVisible()
  return picker
}

const LIGHT = {
  base: '#f4efe4',
  text: '#1c1812',
  accent: '#2e4560',
} as const

// The system-default dark: neutral-cool charcoal since GDK-190.
const DARK = {
  base: '#0d0e10',
  text: '#eae6dd',
  accent: '#3a5b80',
} as const

const INK = {
  base: '#101621',
  text: '#e0e7f2',
  accent: '#2f6285',
} as const

// ember carries the warm dark gadak shipped through 0.14 forward under its own
// name. These three hexes are the ones DARK held before GDK-190 — if they drift,
// the continuity promise made to users who chose that ground is broken.
const EMBER = {
  base: '#160f04',
  text: '#e7e0d4',
  accent: '#365375',
} as const

async function readTokens(page: Page) {
  return page.evaluate(() => {
    const s = getComputedStyle(document.documentElement)
    return {
      base: s.getPropertyValue('--color-bg-base').trim().toLowerCase(),
      text: s.getPropertyValue('--color-text-primary').trim().toLowerCase(),
      accent: s.getPropertyValue('--color-accent').trim().toLowerCase(),
      scheme: s.colorScheme,
      dataTheme: document.documentElement.getAttribute('data-theme'),
    }
  })
}

test.describe('theme', () => {
  test.afterEach(async ({ request }) => {
    const res = await request.get(SETTINGS_URL)
    if (!res.ok()) return
    const body = (await res.json()) as Record<string, unknown>
    await request.put(SETTINGS_URL, {
      data: { ...body, appearance: { theme: 'system' } },
    })
  })

  test('light default is unchanged without a stored choice', async ({ page }) => {
    // Pin light so a dark host OS cannot flip system-default. The contract is
    // "no stored preference + light scheme = today's hex".
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)
    const tokens = await readTokens(page)
    expect(tokens.base).toBe(LIGHT.base)
    expect(tokens.text).toBe(LIGHT.text)
    expect(tokens.accent).toBe(LIGHT.accent)
    expect(tokens.scheme).toMatch(/light/i)
    expect(tokens.dataTheme === null || tokens.dataTheme === 'light').toBeTruthy()
  })

  test('prefers-color-scheme dark paints dark; color-scheme follows', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await gotoApp(page)
    const tokens = await readTokens(page)
    expect(tokens.base).toBe(DARK.base)
    expect(tokens.text).toBe(DARK.text)
    expect(tokens.accent).toBe(DARK.accent)
    expect(tokens.scheme).toMatch(/dark/i)
    expect(tokens.dataTheme).toBeNull()
    const bg = await page
      .getByTestId('issue-layout')
      .evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(bg).toBe('rgb(13, 14, 16)')
  })

  test('every registered palette is in the picker and paints its own ground', async ({
    page,
    request,
  }) => {
    // The registry contract is "token block in app.css + entry in THEMES".
    // This is the end of that promise: a name in THEMES with no CSS block would
    // sit in the picker and select to nothing, which no unit test can see.
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)
    const picker = await themePicker(page)
    const seen = new Map<string, string>()
    for (const theme of THEMES) {
      await picker.selectOption(theme.name)
      const { base } = await readTokens(page)
      expect(base, `${theme.name} must define --color-bg-base`).toMatch(/^#[0-9a-f]{6}$/)
      const collision = [...seen].find(([, hex]) => hex === base)
      expect(collision, `${theme.name} and ${collision?.[0]} paint the same ground ${base}`).toBeUndefined()
      seen.set(theme.name, base)
    }
    expect(seen.get('dark')).toBe(DARK.base)
    expect(seen.get('ink')).toBe(INK.base)
    expect(seen.get('ember')).toBe(EMBER.base)
    await waitServerTheme(request, THEMES[THEMES.length - 1].name)
  })

  test('ember preserves the warm dark, ink is its own blue-black', async ({ page, request }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)
    const picker = await themePicker(page)

    await picker.selectOption('ember')
    const ember = await readTokens(page)
    expect(ember.base).toBe(EMBER.base)
    expect(ember.text).toBe(EMBER.text)
    expect(ember.accent).toBe(EMBER.accent)
    expect(ember.scheme).toMatch(/dark/i)

    await picker.selectOption('ink')
    const ink = await readTokens(page)
    expect(ink.base).toBe(INK.base)
    expect(ink.text).toBe(INK.text)
    expect(ink.accent).toBe(INK.accent)
    expect(ink.scheme).toMatch(/dark/i)

    await waitServerTheme(request, 'ink')
    // Survives a cold boot through the blocking boot script, not just hydration.
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    const afterReload = await readTokens(page)
    expect(afterReload.base).toBe(INK.base)
    expect(afterReload.dataTheme).toBe('ink')
  })

  test('picker dark survives reload; system follows the emulated scheme', async ({
    page,
    request,
  }) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await gotoApp(page)
    const picker = await themePicker(page)
    await picker.selectOption('dark')
    expect((await readTokens(page)).base).toBe(DARK.base)
    expect(await page.evaluate(() => localStorage.getItem('gadak:theme'))).toBe('dark')
    await waitServerTheme(request, 'dark')

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    const afterReload = await readTokens(page)
    expect(afterReload.base).toBe(DARK.base)
    expect(afterReload.dataTheme).toBe('dark')
    expect(afterReload.scheme).toMatch(/dark/i)

    await themePicker(page)
    await page.getByTestId('theme-picker').selectOption('system')
    const asSystem = await readTokens(page)
    expect(asSystem.base).toBe(DARK.base)
    expect(asSystem.dataTheme).toBeNull()
    expect(await page.evaluate(() => localStorage.getItem('gadak:theme'))).toBe('system')
    await waitServerTheme(request, 'system')
  })

  test('palette action applies dark and persists', async ({ page, request }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await palette.getByRole('combobox').fill('dark')
    const option = palette.getByRole('option', { name: /Switch theme to Dark/i })
    await expect(option).toBeVisible()
    await option.click()
    await expect(palette).toBeHidden()
    await waitServerTheme(request, 'dark')

    const tokens = await readTokens(page)
    expect(tokens.base).toBe(DARK.base)
    expect(tokens.text).toBe(DARK.text)
    expect(tokens.accent).toBe(DARK.accent)
    expect(tokens.scheme).toMatch(/dark/i)
    expect(tokens.dataTheme).toBe('dark')
    expect(await page.evaluate(() => localStorage.getItem('gadak:theme'))).toBe('dark')

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    const afterReload = await readTokens(page)
    expect(afterReload.base).toBe(DARK.base)
    expect(afterReload.dataTheme).toBe('dark')
    expect(afterReload.scheme).toMatch(/dark/i)
    expect(await page.evaluate(() => localStorage.getItem('gadak:theme'))).toBe('dark')
  })

  test('picker theme survives wiping localStorage (server is the source)', async ({
    page,
    request,
  }) => {
    // Origin is this workspace's settings document, not the boot mirror.
    // After a picker change, clearing localStorage and reloading must still
    // paint the chosen palette — hydrate reads appearance.theme.
    await page.emulateMedia({ colorScheme: 'light' })
    await gotoApp(page)
    const picker = await themePicker(page)
    await picker.selectOption('ink')
    expect((await readTokens(page)).dataTheme).toBe('ink')

    // Write-through is GET→PUT; wait until the server actually has it so a
    // wipe+reload cannot race an in-flight persist (and so FAIL-first on the
    // current local-only path is "server never took the theme").
    await waitServerTheme(request, 'ink')

    await page.evaluate(() => localStorage.clear())
    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect.poll(async () => (await readTokens(page)).dataTheme).toBe('ink')
    const afterWipe = await readTokens(page)
    expect(afterWipe.base).toBe(INK.base)
    expect(afterWipe.dataTheme).toBe('ink')
  })
})
