import { expect, test, type Page } from '@playwright/test'
import { gotoApp, openServerSettings } from './helpers'

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

const DARK = {
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
    expect(bg).toBe('rgb(22, 15, 4)')
  })

  test('picker dark survives reload; system follows the emulated scheme', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await gotoApp(page)
    const picker = await themePicker(page)
    await picker.selectOption('dark')
    expect((await readTokens(page)).base).toBe(DARK.base)
    expect(await page.evaluate(() => localStorage.getItem('gadak:theme'))).toBe('dark')

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
  })

  test('palette action applies dark and persists', async ({ page }) => {
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
})
