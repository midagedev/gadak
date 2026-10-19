import { type Browser, type Page } from '@playwright/test'
import { test, expect, forceLocale, gotoApp, attachConsoleErrors } from './helpers'
import { en, ko, ja } from '../web/src/lib/i18n/catalog'

/*
 * GDK-2034 (2026-10-19): switching language no longer reloads the page.
 * setLocale used to call location.reload(), which threw away the new-issue
 * draft, edits in progress, scroll and a live terminal attachment — the
 * reload was the defect, so locale.spec.ts asserts the new contract too.
 *
 * Two gates:
 *  1. Draft survives: what is typed in the new-issue dialog is still there
 *     after the switch, and the dialog's own labels are in the new language.
 *  2. Equivalence against mixed language: for each screen, the text after a
 *     live switch en -> X equals the text of a fresh boot in X. A string
 *     computed once outside a reactive context keeps the old language and
 *     shows up here as a differing line.
 */

/** The create dialog needs create-meta to render its form; the fixture
 *  serves none. Same stub shape as create-fields.spec.ts. */
async function stubCreateMeta(page: Page): Promise<void> {
  const projects = [{ key: 'NMB', name: 'Numbers', issue_types: [{ id: '10001', name: 'Task' }] }]
  await page.route('**/api/v1/issues/meta/write/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await route.fulfill({ json: { transitions: {}, create_meta: { projects }, updated_at: '2026-08-18T00:00:00.000Z' } })
  })
  await page.route('**/api/v1/issues/create-meta/**', async (route) => {
    if (route.request().method() !== 'GET' || route.request().url().includes('/create-meta/fields')) return route.continue()
    await route.fulfill({ json: { projects } })
  })
  await page.route('**/priorities/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await route.fulfill({ json: { priorities: [{ id: '3', name: 'Medium' }] } })
  })
}

const CATALOG = { en, ko, ja } as const
type Target = 'ko' | 'ja'
const TARGET_LABEL: Record<Target, string> = { ko: '한국어', ja: '日本語' }

/** Switch through the palette row ("Switch language to 한국어"). */
async function switchViaPalette(
  page: Page,
  target: Target,
  from: 'en' | Target = 'en',
  opts: { alreadyOpen?: boolean } = {},
) {
  if (!opts.alreadyOpen) await page.keyboard.press('ControlOrMeta+k')
  const palette = page.getByRole('dialog').filter({ has: page.getByRole('option') }).last()
  await expect(palette).toBeVisible()
  const label = CATALOG[from]['palette.actionLocale'].replace('{lang}', TARGET_LABEL[target])
  await page.keyboard.type(label.slice(0, 12))
  await palette.getByRole('option', { name: label }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', target === 'ko' ? 'ko-KR' : 'ja-JP')
}

/** Reads, whitespace-normalised and stripped of digits (counts, dates, clock
 *  text), until two reads 400 ms apart agree. */
async function settledLines(page: Page): Promise<string[]> {
  const read = async () =>
    (
      await page.evaluate(() => {
        // The session strip shows only on a page that has a previous session
        // on record — state of the live page, not of its language (its label
        // is a $derived of t()). Hidden on both sides so the two boots
        // compare the same screen.
        document
          .querySelectorAll<HTMLElement>('[data-testid="session-strip"]')
          .forEach((e) => (e.style.display = 'none'))
        return document.body.innerText
      })
    )
      .replace(/[0-9０-９]/g, '')
      .split('\n')
      .map((l) => l.replace(/\s+/g, ' ').trim())
      .filter(Boolean)
  let prev = await read()
  for (let i = 0; i < 20; i++) {
    // why: the contract is "two reads this far apart agree" — a settle window for async fetches, not a wait for one event
    await page.waitForTimeout(400)
    const next = await read()
    if (next.join('\n') === prev.join('\n')) return next
    prev = next
  }
  return prev
}

function firstDiff(live: string[], fresh: string[]): string {
  const out: string[] = []
  const a = new Set(live)
  const b = new Set(fresh)
  for (const l of live) if (!b.has(l)) out.push(`  only after live switch: ${l}`)
  for (const l of fresh) if (!a.has(l)) out.push(`  only in fresh boot:     ${l}`)
  return out.slice(0, 60).join('\n')
}

async function freshPage(browser: Browser, baseURL: string, target: Target): Promise<Page> {
  const ctx = await browser.newContext({ baseURL })
  const page = await ctx.newPage()
  await page.addInitScript((loc) => {
    try {
      localStorage.setItem('gadak_locale', loc)
    } catch {
      /* ignore */
    }
  }, target)
  return page
}

type Screen = {
  name: string
  /** Bring a booted page (any locale) to the screen. */
  open: (page: Page, locale: 'en' | Target) => Promise<void>
  /** The switch itself, when the screen owns how it happens. */
  switch?: (page: Page, target: Target) => Promise<void>
}

const SCREENS: Screen[] = [
  {
    name: 'issue list',
    open: async (page) => {
      await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    },
  },
  {
    name: 'detail panel',
    open: async (page) => {
      await page.goto('/#/?issue=NMB-110')
      await expect(page.getByTestId('issue-detail-panel')).toBeVisible({ timeout: 30_000 })
      await expect(page.getByTestId('issue-detail-panel').getByText('NMB-110').first()).toBeVisible()
    },
  },
  {
    name: 'board',
    open: async (page) => {
      await page.goto('/#/?sc=new,inprogress,done&g=status_category&ly=board')
      await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
      await expect(page.getByTestId('board-card').first()).toBeVisible({ timeout: 30_000 })
    },
  },
  {
    name: 'settings dialog (general)',
    open: async (page, locale) => {
      const name = CATALOG[locale]['sidebar.settings']
      await page.getByRole('button', { name, exact: true }).click()
      await expect(page.getByTestId('settings-dialog')).toBeVisible()
    },
    switch: async (page, target) => {
      const dialog = page.getByTestId('settings-dialog')
      await dialog.getByLabel(en['settings.locale']).selectOption(target)
      await expect(page.locator('html')).toHaveAttribute('lang', target === 'ko' ? 'ko-KR' : 'ja-JP')
    },
  },
  {
    name: 'command palette open',
    open: async (page) => {
      await page.keyboard.press('ControlOrMeta+k')
      await expect(page.getByRole('dialog').filter({ has: page.getByRole('option') }).last()).toBeVisible()
    },
    switch: async (page, target) => {
      // The row closes the palette; the screen under test is the palette, so
      // reopen it in the new language.
      await switchViaPalette(page, target, 'en', { alreadyOpen: true })
      await page.keyboard.press('ControlOrMeta+k')
      await expect(page.getByRole('dialog').filter({ has: page.getByRole('option') }).last()).toBeVisible()
    },
  },
]

test.describe('GDK-2034 locale switches in place', () => {
  test('no reload: the page document survives a switch', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.evaluate(() => {
      ;(window as unknown as { __noReload: number }).__noReload = 1
    })
    await switchViaPalette(page, 'ko')
    await expect(page.getByRole('button', { name: ko['sidebar.settings'], exact: true })).toBeVisible()
    expect(await page.evaluate(() => (window as unknown as { __noReload?: number }).__noReload)).toBe(1)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  for (const target of ['ko', 'ja'] as const) {
    test(`new-issue draft survives a switch to ${target}`, async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await stubCreateMeta(page)
      await gotoApp(page)
      await page.getByRole('button', { name: en['write.newIssue'], exact: true }).click()
      const dialog = page.getByRole('dialog', { name: en['write.newIssue'] })
      await expect(dialog).toBeVisible()
      await dialog.getByPlaceholder(en['write.issueTitle']).fill('Draft title GDK-2034')
      await dialog.getByPlaceholder(en['write.descriptionPlain']).fill('Draft body line\nsecond line')

      await switchViaPalette(page, target)

      // Same dialog, same values, new labels.
      const after = page.getByRole('dialog', { name: CATALOG[target]['write.newIssue'] })
      await expect(after).toBeVisible()
      await expect(after.getByPlaceholder(CATALOG[target]['write.issueTitle'])).toHaveValue('Draft title GDK-2034')
      await expect(after.getByPlaceholder(CATALOG[target]['write.descriptionPlain'])).toHaveValue(
        'Draft body line\nsecond line',
      )
      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  }

  for (const target of ['ko', 'ja'] as const) {
    for (const screen of SCREENS) {
      test(`${screen.name}: live switch to ${target} equals a fresh ${target} boot`, async ({ page, browser, baseURL }) => {
        test.setTimeout(120_000)
        // Live: boot en, reach the screen, switch without reload.
        await forceLocale(page, 'en')
        await gotoApp(page)
        await screen.open(page, 'en')
        await (screen.switch ?? ((p, t) => switchViaPalette(p, t)))(page, target)
        const live = await settledLines(page)

        // Fresh: boot in the target and reach the same screen the same way.
        const fresh = await freshPage(browser, baseURL!, target)
        try {
          await gotoApp(fresh)
          await screen.open(fresh, target)
          const freshLines = await settledLines(fresh)
          const same = live.join('\n') === freshLines.join('\n')
          expect(same, `mixed language on "${screen.name}" (${target}):\n${firstDiff(live, freshLines)}`).toBe(true)
        } finally {
          await fresh.context().close()
        }
      })
    }
  }
})
