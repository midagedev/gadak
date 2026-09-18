// The shell owner (GDK-865, GDK-902). Playwright at 402×874 against `gadak demo` on
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
// The road to each surface has one owner (GDK-902): nav.ts. This spec used
// to wait on the Settings heading by its word, which a parallel round is
// renaming — openSettings waits on the layer's own back control instead.
import { closeSettings, openSettings } from './nav'
// The floor the bottom-most painted surface owes, from the module that owns
// the number (GDK-911) — restating 12 here would be the second owner.
import { sheetBottomInset, SHEET_INSET_FLOOR_PX } from '../src/lib/inset'

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
  await page.locator('h1 button.scope').waitFor()
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

/*
 * GDK-902 2026-09-15: pairing is the Settings push layer behind the gear,
 * and the shell is entered from the palette's Terminal row — there is no
 * tab bar to click through (DESIGN.md §2/§10).
 */
async function pairShell(page: Page, label = 'This Mac (dev)'): Promise<void> {
  await openSettings(page)
  await page.locator('#term-offer').fill(makeTerminalOffer(label))
  await page.getByRole('button', { name: 'Pair', exact: true }).click()
  await expect(page.locator('#term-offer')).toHaveCount(0)
  await closeSettings(page)
  await page.locator('h1 button.scope').waitFor()
}

async function openShell(page: Page): Promise<void> {
  await page.locator('button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'Terminal' }).click()
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

/**
 * The PTY sessions the serve itself is holding, ids sorted.
 *
 * The witness for "leaving the owner did not kill the shell, and coming back
 * did not start a second one" is deliberately the server's list and not the
 * pane: Shell.svelte's owner effect DOES detach on leave and attach on
 * re-entry (screens/Shell.svelte:920-928), so counting sockets would count
 * the reattach and say nothing about the PTY. What must not change is the
 * session — same id, still exactly one.
 */
async function sessionIds(page: Page): Promise<string[]> {
  const res = await page.request.get(`${SERVE_ORIGIN}/api/v1/terminal/sessions/`)
  expect(res.ok(), 'terminal sessions list').toBe(true)
  const body = (await res.json()) as { sessions?: { id: string }[] }
  return (body.sessions ?? []).map((x) => x.id).sort()
}

async function drainSessions(page: Page): Promise<void> {
  const res = await page.request.get(`${SERVE_ORIGIN}/api/v1/terminal/sessions/`)
  if (!res.ok()) return
  const body = (await res.json()) as { sessions?: { id: string }[] }
  for (const s of body.sessions ?? []) {
    await page.request.delete(`${SERVE_ORIGIN}/api/v1/terminal/sessions/${s.id}/`)
  }
}

test.describe('shell owner', () => {
  test.afterEach(async ({ page }) => {
    await drainSessions(page)
  })

  test('the palette offers no Terminal row without a pairing, and one with it', async ({
    page,
  }) => {
    // GDK-902 2026-09-15. Was: "a default install shows three tabs; a
    // stored terminal pairing is what makes the fourth tab exist". There is
    // no tab bar (DESIGN.md §2) — the shell is an owner, reached from the
    // palette's Terminal row — but the claim the count was making is the
    // one that matters and is unchanged: absence until a pairing is stored,
    // never a greyed-out row.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await expect(page.locator('nav.safe-bottom')).toHaveCount(0)
    await page.locator('button.search').click()
    await page.locator('.palette-field input').waitFor()
    await expect(page.locator('button.palette-row', { hasText: 'Terminal' })).toHaveCount(0)
    await page.locator('button.palette-cancel').click()

    await pairShell(page)
    await page.locator('button.search').click()
    await page.locator('.palette-field input').waitFor()
    await expect(page.locator('button.palette-row', { hasText: 'Terminal' })).toBeVisible()
  })

  test('entering the shell owner attaches and a typed echo round-trips through a real PTY', async ({
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
      const barBox = bar?.getBoundingClientRect()
      return {
        hOverflow: document.documentElement.scrollWidth - window.innerWidth,
        barBottom: barBox ? barBox.y + barBox.height : null,
        // GDK-902 2026-09-15: the key bar used to be floored against the
        // tab bar above it. The shell owns the whole column now
        // (DESIGN.md §10), so the surface the bar must not overrun is the
        // viewport's own bottom edge — a stricter floor, not a dropped one.
        floor: window.innerHeight,
        barVisible: !!(barBox && barBox.height > 0),
      }
    })
    expect(geo.hOverflow, 'horizontal overflow').toBe(0)
    expect(geo.barVisible, 'key bar visible').toBe(true)
    expect(geo.barBottom, 'key bar bottom').not.toBeNull()
    expect(geo.barBottom! <= geo.floor + 1, 'key bar inside the column').toBe(true)

    // The keyboard-up capture needs the bar translated out of frame for one
    // shot; the whole dance is skipped when no round asked for captures.
    if (process.env.SHELL_SHOT_DIR) {
      await page.evaluate(() => {
        const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
        if (!bar) return
        bar.style.transform = 'translateY(-280px)'
        // GDK-902 2026-09-15: the translate alone is half of what a keyboard
        // does. The other half is the bottom inset going away — the bar is no
        // longer the bottom-most surface once it rides above the keys — and
        // app.css withdraws it on this attribute, which lib/keyboard.ts
        // stamps from the VisualViewport. Faking the lift without it
        // photographs a 12px strip no real keyboard-up frame has, and a
        // vision round would read that strip as the defect.
        bar.dataset.keyboardInset = ''
      })
      await shootShell(page, 'shell-keyboard-up.png')
      await page.evaluate(() => {
        const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
        if (!bar) return
        bar.style.transform = ''
        delete bar.dataset.keyboardInset
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

  test('the key bar clears the home indicator while the keyboard is down', async ({ page }) => {
    // GDK-902 2026-09-15. Protects: with the tab bar gone the key bar is the
    // bottom-most painted surface of the column, and a bottom-most surface
    // owes max(var(--safe-bottom), 12px) — the same formula .safe-bottom and
    // every .sheet compose (src/app.css, pinned by src/lib/inset.test.ts).
    // FAIL-first on this rig before the CSS landed: padding-bottom 0px.
    //
    // Playwright reports no safe-area inset, so the expectation is derived
    // from sheetBottomInset() rather than restating a number: a rig with a
    // real home indicator and this one are asserted against one formula.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    const bottom = await page.evaluate(() => {
      const bar = document.querySelector('[data-testid="key-bar"]')
      if (!bar) return null
      const px = (v: string) => Number.parseFloat(v.trim()) || 0
      return {
        paddingPx: px(getComputedStyle(bar).paddingBottom),
        safeBottomPx: px(getComputedStyle(document.documentElement).getPropertyValue('--safe-bottom')),
        // The action stamps this while the VisualViewport reports a band.
        // No OSK here, so it must be absent — the padding is owed.
        keyboardUp: bar.hasAttribute('data-keyboard-inset'),
      }
    })
    expect(bottom, 'key bar present').not.toBeNull()
    expect(bottom!.keyboardUp, 'no keyboard band on this rig').toBe(false)
    console.log(
      `[shell] key-bar padding-bottom ${bottom!.paddingPx}px (safe-bottom ${bottom!.safeBottomPx}px, floor ${SHEET_INSET_FLOOR_PX}px)`,
    )
    expect(bottom!.paddingPx, 'key bar bottom inset').toBe(sheetBottomInset(bottom!.safeBottomPx))

    // The other direction, on the built stylesheet rather than on a source
    // regex: while the action reports a band the bar has been translated
    // above it, so it is not the bottom-most surface any more and the rule
    // must stop matching. Stamping the attribute by hand is the only way to
    // stand in for a keyboard Chromium does not have.
    const stamped = await page.evaluate(() => {
      const bar = document.querySelector<HTMLElement>('[data-testid="key-bar"]')
      if (!bar) return null
      const px = (v: string) => Number.parseFloat(v.trim()) || 0
      bar.dataset.keyboardInset = ''
      const withBand = px(getComputedStyle(bar).paddingBottom)
      delete bar.dataset.keyboardInset
      const without = px(getComputedStyle(bar).paddingBottom)
      return { withBand, without }
    })
    expect(stamped!.withBand, 'no dead strip under a lifted bar').toBe(0)
    expect(stamped!.without, 'the inset comes back when the band closes').toBe(
      sheetBottomInset(bottom!.safeBottomPx),
    )
  })

  test('entering and leaving the shell is one pair: the last scope comes back and the PTY survives', async ({
    page,
  }) => {
    // GDK-902 2026-09-15. Protects DESIGN.md §2's entry/exit row for the
    // shell — "palette row Terminal | ← back in its header → the last scope"
    // — as one round trip rather than two half-tested directions: the owner
    // swap, the exit control's own contract (44pt, the catalog word), the
    // heading the list comes back wearing, and the session it comes back to.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    const scopeHeading = (await page.locator('h1 button.scope').innerText()).trim()
    await pairShell(page)
    await openShell(page)

    // One visible pane, and it is the shell's.
    await expect(page.locator('.pane:not(.off)')).toHaveCount(1)
    await expect(page.locator('.pane:not(.off) [data-testid="key-bar"]')).toHaveCount(1)
    await expect(page.locator('.pane.off h1 button.scope')).toHaveCount(1)

    // The exit control. Glyph-only by design (the header also carries the
    // session label), so the 44pt floor has to be measured on BOTH axes —
    // the viewport gate reads height only, which a narrow control passes.
    const back = page.locator('.pane:not(.off) button.back')
    await expect(back).toBeVisible()
    await expect(back).toHaveAttribute('aria-label', 'Back')
    const box = await back.boundingBox()
    expect(box, 'back control box').not.toBeNull()
    console.log(`[shell] back control ${box!.width}×${box!.height}`)
    expect(box!.width, 'back width').toBeGreaterThanOrEqual(44)
    expect(box!.height, 'back height').toBeGreaterThanOrEqual(44)

    const before = await sessionIds(page)
    expect(before, 'one PTY after the first entry').toHaveLength(1)

    await back.click()

    // The list is the owner again, wearing the same name it had.
    await expect(page.locator('.pane:not(.off) h1 button.scope')).toHaveText(scopeHeading)
    // …and the shell pane is still mounted, just hidden: leaving is not
    // unpairing, so the PTY is not torn down by a tap on back.
    await expect(page.locator('.pane.off [data-testid="key-bar"]')).toHaveCount(1)

    await openShell(page)
    await expect(page.getByRole('heading', { name: 'This Mac (dev)' })).toBeVisible()
    expect(await sessionIds(page), 'the same PTY, not a second one').toEqual(before)
  })

  test('system back from the shell owner lands on the list', async ({ page }) => {
    // GDK-902 2026-09-15. DESIGN.md §2: "system back = the same edge" as the
    // visible way out, and the root is the one screen with no exit. The shell
    // is an owner, not the root — the list is — so back from it had to stop
    // being the root no-op it was when closeTop only knew detail / layer /
    // palette. FAIL-first on this rig before the store change: the shell pane
    // stayed the visible one after page.goBack().
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)

    await page.goBack()

    await expect(page.locator('.pane:not(.off) h1 button.scope')).toHaveCount(1)
    await expect(page.locator('.pane:not(.off) [data-testid="key-bar"]')).toHaveCount(0)
    await expect(page.locator('.pane.off [data-testid="key-bar"]')).toHaveCount(1)
  })

  test('unpairing the shell takes its pane with it', async ({ page }) => {
    // GDK-902 2026-09-15. Protects: the owner cannot outlive its pairing —
    // unpairTerminal resets both the owner and the mount latch, so the pane
    // is torn down rather than left hidden with a dead socket.
    //
    // The two taps are the house arm/confirm (DESIGN.md §5), not a browser
    // dialog. Settings is reached from the LIST, because the gear lives in
    // the list heading and that heading is the hidden pane while the shell
    // owns the column — see the report's DESIGN note.
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await waitPaired(page)
    await pairShell(page)
    await openShell(page)
    await page.locator('.pane:not(.off) button.back').click()
    await expect(page.locator('.pane.off [data-testid="key-bar"]')).toHaveCount(1)

    await openSettings(page)
    const unpair = page.getByRole('button', { name: 'Unpair the shell' })
    await unpair.click()
    await expect(page.getByRole('button', { name: 'Tap again to unpair' })).toBeVisible()
    await page.getByRole('button', { name: 'Tap again to unpair' }).click()
    await expect(page.locator('#term-offer')).toBeVisible()
    await closeSettings(page)

    // The pane is gone, not hidden — and the palette no longer offers the row.
    await expect(page.locator('[data-testid="key-bar"]')).toHaveCount(0)
    await expect(page.locator('.pane:not(.off) h1 button.scope')).toHaveCount(1)
    await page.locator('button.search').click()
    await page.locator('.palette-field input').waitFor()
    await expect(page.locator('button.palette-row', { hasText: 'Terminal' })).toHaveCount(0)
  })
})
