import { expect, type ConsoleMessage, type Locator, type Page } from '@playwright/test'

/**
 * Seed locale only when unset so catalog assertions match en.ts by default,
 * without clobbering a user-driven setLocale() across reloads (locale.spec).
 */
export async function forceLocale(page: Page, locale: 'en' | 'ko' = 'en'): Promise<void> {
  await page.addInitScript((loc) => {
    try {
      if (!localStorage.getItem('gadak_locale')) {
        localStorage.setItem('gadak_locale', loc)
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
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
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

/**
 * Every row of a windowed list, in list order.
 *
 * The document lists render only the rows in view, so counting the DOM answers
 * "how tall is the window", not "what is in the list" — and "grouping regroups,
 * it does not filter" is a claim about the list. This walks the scroller to the
 * bottom and accumulates rows by their key, which asks the original question of
 * a list that no longer renders itself all at once.
 */
export async function walkRows(
  scroller: Locator,
  opts: { rowTestId?: string; keyAttr?: string } = {},
): Promise<{ key: string; title: string }[]> {
  const rowTestId = opts.rowTestId ?? 'doc-row'
  const keyAttr = opts.keyAttr ?? 'data-doc-key'
  const seen = new Map<string, string>()
  await scroller.evaluate((el) => {
    el.scrollTop = 0
  })
  // Bounded so a list that never reports its end fails loudly instead of hanging.
  for (let step = 0; step < 500; step++) {
    const batch = await scroller.evaluate(
      (el, sel) => ({
        rows: [...el.querySelectorAll(`[data-testid="${sel.t}"]`)].map((r) => ({
          key: r.getAttribute(sel.k) ?? '',
          title: r.querySelector('span')?.textContent?.trim() ?? '',
        })),
        atEnd: el.scrollTop + el.clientHeight >= el.scrollHeight - 1,
      }),
      { t: rowTestId, k: keyAttr },
    )
    for (const r of batch.rows) if (!seen.has(r.key)) seen.set(r.key, r.title)
    if (batch.atEnd) break
    await scroller.evaluate((el) => {
      el.scrollTop += el.clientHeight * 0.8
    })
    await scroller.page().waitForTimeout(30)
  }
  await scroller.evaluate((el) => {
    el.scrollTop = 0
  })
  return [...seen].map(([key, title]) => ({ key, title }))
}
