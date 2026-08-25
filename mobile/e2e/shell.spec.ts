// Shell tab (GDK-865). Playwright at 402×874 against `gadak demo` on
// 127.0.0.1:7899 and vite on 127.0.0.1:5182 — same fixture as viewport.spec.ts.
// Each test names the user behaviour it protects (release-audit.md axis 5).
import { expect, test, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { SERVE_ORIGIN } from '../playwright.config'

const SHOT_DIR = '/tmp/gadak-865c'

type TermHook = {
  buffer: {
    active: {
      length: number
      getLine: (y: number) => { translateToString: (trimRight?: boolean) => string } | undefined
    }
  }
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

function makeTerminalOffer(label: string): string {
  const doc = JSON.stringify({
    v: 1,
    endpoint: `${SERVE_ORIGIN}`,
    token: crypto.randomUUID(),
    expires_at: '',
    label,
  })
  return Buffer.from(doc).toString('base64url')
}

async function pairShell(page: Page, label = 'This Mac (dev)'): Promise<void> {
  await page.locator('nav.safe-bottom button.tab', { hasText: 'Pairing' }).click()
  await page.getByRole('heading', { name: 'Pairing' }).waitFor()
  await page.locator('#term-offer').fill(makeTerminalOffer(label))
  await page.getByRole('button', { name: 'Pair', exact: true }).click()
  await expect(page.locator('nav.safe-bottom button.tab', { hasText: 'Terminal' })).toBeVisible()
}

async function openShell(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom button.tab', { hasText: 'Terminal' }).click()
  await expect(page.getByTestId('terminal-pane')).toBeVisible()
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
}

async function readTerm(page: Page): Promise<string> {
  return page.evaluate(() => {
    const t = (window as unknown as { __gadakTerm?: TermHook }).__gadakTerm
    if (!t) return ''
    const buf = t.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buf.length; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? '')
    }
    return lines.join('\n')
  })
}

async function focusIme(page: Page): Promise<void> {
  const host = page.locator('[data-gadak-editable]')
  if ((await host.count()) > 0) {
    await host.first().click({ position: { x: 24, y: 24 } })
  }
  await page.getByTestId('shell-ime').focus()
}

async function typeLine(page: Page, line: string): Promise<void> {
  await focusIme(page)
  await page.keyboard.type(line, { delay: 15 })
  await page.keyboard.press('Enter')
}

async function drainSessions(page: Page): Promise<void> {
  const res = await page.request.get(`${SERVE_ORIGIN}/api/v1/terminal/sessions/`)
  if (!res.ok()) return
  const body = (await res.json()) as { sessions?: { id: string }[] }
  for (const s of body.sessions ?? []) {
    await page.request.delete(`${SERVE_ORIGIN}/api/v1/terminal/sessions/${s.id}/`)
  }
}

test.describe('shell tab', () => {
  test.afterEach(async ({ page }) => {
    await drainSessions(page)
  })

  test('the Shell tab is absent with no terminal pairing, and present with one', async ({
    page,
  }) => {
    // Protects: a default install shows three tabs; a stored terminal
    // pairing is what makes the fourth tab exist, never a greyed-out one.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await expect(page.locator('nav.safe-bottom button.tab')).toHaveCount(3)
    await expect(page.locator('nav.safe-bottom button.tab', { hasText: 'Terminal' })).toHaveCount(0)

    await pairShell(page)
    await expect(page.locator('nav.safe-bottom button.tab')).toHaveCount(4)
    await expect(page.locator('nav.safe-bottom button.tab', { hasText: 'Terminal' })).toBeVisible()
  })

  test('opening the Shell tab attaches and a typed echo round-trips through a real PTY', async ({
    page,
  }) => {
    // Protects: first activation creates a session, attaches, and typed
    // bytes come back as PTY echo — not a fake local terminal.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)
    await expect(page.getByRole('heading', { name: 'This Mac (dev)' })).toBeVisible()

    await typeLine(page, "printf 'gdk865-echo\\n'")
    await expect.poll(async () => readTerm(page), { timeout: 20_000 }).toContain('gdk865-echo')

    mkdirSync(SHOT_DIR, { recursive: true })
    await page.screenshot({ path: `${SHOT_DIR}/shell-keyboard-down.png`, fullPage: true })
  })

  test('a key-bar Ctrl+c interrupts a running command', async ({ page }) => {
    // Protects: sticky Ctrl looks armed, then a letter sends the control
    // byte, and a running command is interrupted rather than typed through.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    await typeLine(page, 'sleep 60')
    await expect.poll(async () => readTerm(page), { timeout: 10_000 }).toMatch(/sleep 60/)

    const ctrl = page.getByRole('button', { name: 'Ctrl' })
    await ctrl.click()
    await expect(ctrl).toHaveAttribute('aria-pressed', 'true')
    await focusIme(page)
    await page.keyboard.type('c')
    await expect(ctrl).toHaveAttribute('aria-pressed', 'false')

    await typeLine(page, "printf 'gdk865-int\\n'")
    await expect.poll(async () => readTerm(page), { timeout: 20_000 }).toContain('gdk865-int')
  })

  test('the pane recovers after the socket drops and replayed scrollback still holds earlier output', async ({
    page,
  }) => {
    // Protects: a dropped socket (the normal case on a phone) reattaches
    // inside the grace, and the ring replay still shows what was typed.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    await typeLine(page, "printf 'gdk865-keep\\n'")
    await expect.poll(async () => readTerm(page), { timeout: 20_000 }).toContain('gdk865-keep')

    await page.evaluate(() => {
      ;(window as unknown as { __gadakShellDrop?: () => void }).__gadakShellDrop?.()
    })
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'false')
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    await expect.poll(async () => readTerm(page), { timeout: 20_000 }).toContain('gdk865-keep')
  })

  test('no horizontal overflow, and the key bar sits above the bottom chrome', async ({
    page,
  }) => {
    // Protects: the key bar is not under the keyboard band (Playwright
    // Chromium has no OSK, so the measured stand-in is the tab bar — the
    // bar rides keyboardInset when VisualViewport reports an inset).
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)
    await focusIme(page)

    const geo = await page.evaluate(() => {
      const bar = document.querySelector('[data-testid="key-bar"]')
      const nav = document.querySelector('nav.safe-bottom')
      const barBox = bar?.getBoundingClientRect()
      const navBox = nav?.getBoundingClientRect()
      return {
        hOverflow: document.documentElement.scrollWidth - window.innerWidth,
        barBottom: barBox ? barBox.y + barBox.height : null,
        navTop: navBox ? navBox.y : null,
        barVisible: !!(barBox && barBox.height > 0),
      }
    })
    expect(geo.hOverflow, 'horizontal overflow').toBe(0)
    expect(geo.barVisible, 'key bar visible').toBe(true)
    expect(geo.barBottom, 'key bar bottom').not.toBeNull()
    expect(geo.navTop, 'nav top').not.toBeNull()
    expect(geo.barBottom! <= geo.navTop! + 1, 'key bar above the tab bar').toBe(true)

    mkdirSync(SHOT_DIR, { recursive: true })
    await page.evaluate(() => {
      const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
      if (bar) bar.style.transform = 'translateY(-280px)'
    })
    await page.screenshot({ path: `${SHOT_DIR}/shell-keyboard-up.png`, fullPage: true })
    await page.evaluate(() => {
      const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
      if (bar) bar.style.transform = ''
    })
  })

  test('an ended shell is a calm line with a next action, never a toast', async ({ page }) => {
    // Protects: exit is a status line on the pane, with the catalog restart
    // hint, not a toast that disappears.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    await typeLine(page, 'exit')
    await expect(page.getByTestId('terminal-status')).toContainText('Shell exited', {
      timeout: 20_000,
    })
    await expect(page.getByTestId('terminal-status')).toContainText('Press Enter to start a new shell')
    // One line, and it is the control. This used to assert role="status"
    // count 1; GDK-908 made the ended state a real <button> because tapping
    // it is what starts the next shell, and role="status" is an announcement,
    // not a control. The live-region roles now belong to connecting and
    // reconnecting — states with nothing to tap — so an ended pane having
    // none is the contract, not a regression.
    await expect(page.getByTestId('terminal-status')).toHaveCount(1)
    await expect(page.getByTestId('terminal-status')).toHaveJSProperty('tagName', 'BUTTON')
    await expect(page.locator('[role="status"]')).toHaveCount(0)

    mkdirSync(SHOT_DIR, { recursive: true })
    await page.screenshot({ path: `${SHOT_DIR}/shell-ended.png`, fullPage: true })
  })
})
