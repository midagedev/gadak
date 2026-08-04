import { expect, type ConsoleMessage, type Page } from '@playwright/test'

/**
 * Seed locale only when unset so catalog assertions match en.ts by default,
 * without clobbering a user-driven setLocale() across reloads (locale.spec).
 */
export async function forceLocale(page: Page, locale: 'en' | 'ko' = 'en'): Promise<void> {
  await page.addInitScript((loc) => {
    try {
      if (!localStorage.getItem('scry_locale')) {
        localStorage.setItem('scry_locale', loc)
      }
    } catch {
      /* ignore */
    }
  }, locale)
}

/** Boot the SPA and wait until the issue list is hydrated. */
export async function gotoApp(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(/519 issues/).first()).toBeVisible({ timeout: 30_000 })
}

export function attachConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg: ConsoleMessage) => {
    if (msg.type() === 'error') {
      errors.push(msg.text())
    }
  })
  page.on('pageerror', (err) => {
    errors.push(String(err))
  })
  return errors
}

export async function openServerSettings(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible()
}

/** Local client-side search box (placeholder from en.ts). */
export function searchInput(page: Page) {
  return page.getByPlaceholder(/Search issues/)
}
