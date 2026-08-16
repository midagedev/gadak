import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, type ConsoleMessage, type Locator, type Page } from '@playwright/test'

const E2E_DIR = dirname(fileURLToPath(import.meta.url))

/** Fail when reuseExistingServer attached to a binary built at another HEAD. */
export function assertServedGitSha(): void {
  const head = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: join(E2E_DIR, '..'),
    encoding: 'utf8',
  }).trim()
  const stamp = join(E2E_DIR, '.tmp', 'served-sha')
  if (!existsSync(stamp)) {
    throw new Error(
      `stale dev server: missing ${stamp}. reuseExistingServer picked up a process that was not started by e2e/serve.sh. Stop it (pkill -f 'e2e/.tmp/gadak') and re-run.`,
    )
  }
  const served = readFileSync(stamp, 'utf8').trim()
  if (served !== head) {
    throw new Error(
      `stale dev server: serve.sh built ${served} but HEAD is ${head}. reuseExistingServer picked up an old one. Stop it (pkill -f 'e2e/.tmp/gadak') and re-run.`,
    )
  }
}

export default function globalSetup(): void {
  assertServedGitSha()
}

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
  // Sidebar pool size — visible as soon as bootstrap lands, *before* the
  // startup view. A "534 issues" match is not "the list is ready for keys".
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
  // applyStartupView waits for me.authChecked (favorites + GET auth/me/ +
  // personal loads) and then writes the default all-open filter into the
  // hash. IssueList's viewKey effect resetCursor()s on that write, so a
  // j/k/x that landed on the unfiltered 534-row list is wiped. Wait on the
  // hash *and* the refiltered count — Playwright auto-wait, not a sleep —
  // so keyboard tests start after that commit. (GDK-39)
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  await expect(page.getByTestId('list-count')).not.toHaveText('534 issues')
}

/**
 * Boot until the unfiltered list paints — and stop there.
 *
 * gotoApp waits for applyStartupView's hash write (GDK-39) so ordinary
 * keyboard specs never see this window. GDK-46 is about the window itself,
 * so those specs must bypass that wait on purpose.
 */
export async function gotoAppBeforeStartup(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
}

/**
 * Hold GET auth/me/ until both `delayMs` and `released` have settled.
 *
 * applyStartupView waits on me.authChecked, which flips in the `finally`
 * after this request. A delay alone is racy on a slow machine (keys land
 * after the commit). Gating continue() on the test's own release is what
 * keeps the keystrokes inside the window.
 */
export async function holdAuthMe(
  page: Page,
  opts: { delayMs: number; released: Promise<void> },
): Promise<void> {
  await page.route('**/api/v1/auth/me/**', async (route) => {
    await Promise.all([
      new Promise<void>((r) => setTimeout(r, opts.delayMs)),
      opts.released,
    ])
    await route.continue()
  })
}

/** Console noise the fixture credential produces (writes 409). Not an app bug. */
export function appConsoleErrors(errors: string[]): string[] {
  return errors.filter((e) => !e.includes('409'))
}

export function attachConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg: ConsoleMessage) => {
    if (msg.type() === 'error') {
      const text = msg.text()
      // Fake e2e token → write endpoints 409; Chromium logs that as a console error.
      if (text.includes('409')) return
      errors.push(text)
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

/** Local client-side search box (`data-testid` — placeholder copy is not the contract). */
export function searchInput(page: Page) {
  return page.getByTestId('search-input')
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
