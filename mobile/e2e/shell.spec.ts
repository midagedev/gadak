// Shell tab (GDK-865). Playwright at 402×874 against `gadak demo` on
// 127.0.0.1:7899 and vite on 127.0.0.1:5182 — same fixture as viewport.spec.ts.
// Each test names the user behaviour it protects (release-audit.md axis 5).
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { mkdirSync } from 'node:fs'
import { SERVE_ORIGIN } from '../playwright.config'
// The repo-root owner of the buffer walk (GDK-1567). This file's old local
// copy did not stitch xterm's wrapped rows — the exact fork the owner exists
// to prevent. term-read.ts is import-free so it typechecks under mobile's
// own @playwright/test install too.
import { readTerm } from '../../e2e/term-read'

/*
 * Signal-first PTY wait (GDK-1552). `expect.poll(readTerm(page))` walked the
 * whole xterm buffer on every tick, and under parallel load those evaluate
 * round-trips competed with the pane's own WS message handling — the poll
 * starved the very socket it was waiting on, and gdk865-echo timed out with
 * the bytes already on the wire. Playwright hands websocket frames to the
 * test as CDP events, outside the page's runtime, so the WAIT rides those:
 * construct this before the socket can open (the pane attaches later), and
 * waitFor resolves when received frames' bytes contain the needle. Frames
 * already seen satisfy a later waitFor. Frame CONTENT is never printed —
 * PTY bytes are whatever the shell chose to say — so the timeout reports
 * socket/frame/byte counts, which is the diagnosis that matters (0 sockets
 * = attach never happened; 0 frames = connection dead; bytes but no needle
 * = echo never came). The DOM is read only after the signal, as a bounded
 * confirmation that the renderer painted what arrived — never as the wait.
 */
class PtyFrames {
  private static readonly CAP = 1 << 20
  private all = ''
  /** Bytes dropped by the cap, so cursors stay absolute across it. */
  private base = 0
  private bytes = 0
  private frames = 0
  private sockets = 0
  private waiters: { needle: string; from: number; resolve: () => void }[] = []

  constructor(page: Page) {
    page.on('websocket', (ws) => {
      this.sockets += 1
      ws.on('framereceived', (frame) => {
        // latin1, not utf8: PTY bytes are arbitrary, and the needles are
        // ASCII — a decoding error on junk bytes must not lose the needle.
        const chunk =
          typeof frame.payload === 'string' ? frame.payload : frame.payload.toString('latin1')
        this.frames += 1
        this.bytes += chunk.length
        this.all += chunk
        if (this.all.length > PtyFrames.CAP) {
          const drop = this.all.length - (PtyFrames.CAP >> 1)
          this.all = this.all.slice(drop)
          this.base += drop
        }
        this.waiters = this.waiters.filter((w) => {
          const hay = w.from <= this.base ? this.all : this.all.slice(w.from - this.base)
          if (!hay.includes(w.needle)) return true
          w.resolve()
          return false
        })
      })
    })
  }

  /** Position in the received-byte stream — waitFor({from}) matches only
   *  bytes that arrive after it, which is how a replay is told from the
   *  original delivery of the same text. */
  cursor(): number {
    return this.base + this.all.length
  }

  waitFor(needle: string, opts: { timeout?: number; from?: number } = {}): Promise<void> {
    const timeout = opts.timeout ?? 20_000
    const from = opts.from ?? 0
    const seen = () => {
      const hay = from <= this.base ? this.all : this.all.slice(from - this.base)
      return hay.includes(needle)
    }
    if (seen()) return Promise.resolve()
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.waiters = this.waiters.filter((w) => w.resolve !== resolve)
        reject(
          new Error(
            `no PTY frame carried "${needle}" after ${timeout}ms — ` +
              `${this.sockets} websocket(s), ${this.frames} frame(s), ${this.bytes} byte(s) received`,
          ),
        )
      }, timeout)
      this.waiters.push({
        needle,
        from,
        resolve: () => {
          clearTimeout(timer)
          resolve()
        },
      })
    })
  }
}

/**
 * Evidence captures for this file are env-gated (v0.21 release audit,
 * capture-hygiene finding): they used to write /tmp/gadak-865c/ on every run
 * — a machine-local path no other consumer can rely on. Set SHELL_SHOT_DIR
 * to take them.
 */
async function shootShell(page: Page, file: string): Promise<void> {
  const dir = process.env.SHELL_SHOT_DIR
  if (!dir) return
  mkdirSync(dir, { recursive: true })
  await page.screenshot({ path: `${dir}/${file}`, fullPage: true })
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
    const frames = new PtyFrames(page)
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)
    await expect(page.getByRole('heading', { name: 'This Mac (dev)' })).toBeVisible()

    await typeLine(page, "printf 'gdk865-echo\\n'")
    await frames.waitFor('gdk865-echo')
    // The one DOM confirmation (GDK-1552): the signal says the bytes arrived
    // over the socket; this bounded poll says the renderer painted them.
    await expect.poll(async () => readTerm(page), { timeout: 5_000 }).toContain('gdk865-echo')

    await shootShell(page, 'shell-keyboard-down.png')
  })

  test('a key-bar Ctrl+c interrupts a running command', async ({ page }) => {
    // Protects: sticky Ctrl looks armed, then a letter sends the control
    // byte, and a running command is interrupted rather than typed through.
    const frames = new PtyFrames(page)
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    await typeLine(page, 'sleep 60')
    // Signal-only (GDK-1552): this wait just proves the command started —
    // the PTY's own echo of the typed line is the signal. The DOM below is
    // read once, for the printf the interrupt must have made possible.
    await frames.waitFor('sleep 60', { timeout: 10_000 })

    const ctrl = page.getByRole('button', { name: 'Ctrl' })
    await ctrl.click()
    await expect(ctrl).toHaveAttribute('aria-pressed', 'true')
    await focusIme(page)
    await page.keyboard.type('c')
    await expect(ctrl).toHaveAttribute('aria-pressed', 'false')

    await typeLine(page, "printf 'gdk865-int\\n'")
    await frames.waitFor('gdk865-int')
    await expect.poll(async () => readTerm(page), { timeout: 5_000 }).toContain('gdk865-int')
  })

  test('the pane recovers after the socket drops and replayed scrollback still holds earlier output', async ({
    page,
  }) => {
    // Protects: a dropped socket (the normal case on a phone) reattaches
    // inside the grace, and the ring replay still shows what was typed.
    const frames = new PtyFrames(page)
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    await typeLine(page, "printf 'gdk865-keep\\n'")
    await frames.waitFor('gdk865-keep')

    // Cursor BEFORE the drop, not after reattach (GDK-1552): the replay can
    // start the instant the new socket opens, so a cursor read after
    // `data-attached` would already be past it. The dying socket sends
    // nothing between here and the drop, so everything past this cursor is
    // reattach traffic — and the replay arrives on the NEW socket as the
    // SAME text a second time, which is exactly what `from` must isolate.
    const replayFrom = frames.cursor()
    await page.evaluate(() => {
      ;(window as unknown as { __gadakShellDrop?: () => void }).__gadakShellDrop?.()
    })
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'false')
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    // The DOM confirm is the test's actual claim: the ring replay paints,
    // not just arrives.
    await frames.waitFor('gdk865-keep', { from: replayFrom })
    await expect.poll(async () => readTerm(page), { timeout: 5_000 }).toContain('gdk865-keep')
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

    // The keyboard-up capture needs the bar translated out of frame for one
    // shot; the whole dance is skipped when no round asked for captures.
    if (process.env.SHELL_SHOT_DIR) {
      await page.evaluate(() => {
        const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
        if (bar) bar.style.transform = 'translateY(-280px)'
      })
      await shootShell(page, 'shell-keyboard-up.png')
      await page.evaluate(() => {
        const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
        if (bar) bar.style.transform = ''
      })
    }
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

    await shootShell(page, 'shell-ended.png')
  })
})
