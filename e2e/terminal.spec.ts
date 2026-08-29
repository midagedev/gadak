import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'
import { test, expect, type Page } from '@playwright/test'
import {
  apiURL,
  appConsoleErrors,
  attachConsoleErrors,
  drainTerminalSessions,
  DEMO_ISSUE_COUNT_EN,
  DEMO_ISSUE_COUNT_EN_RE,
  forceLocale,
} from './helpers'

const SHOT_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', 'scratch', 'terminal-shots')

type TermHook = {
  buffer: {
    active: {
      length: number
      getLine: (y: number) => { translateToString: (trimRight?: boolean) => string } | undefined
    }
  }
}

async function boot(page: Page): Promise<string[]> {
  const errors = attachConsoleErrors(page)
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_EN)
  return errors
}

async function openPane(page: Page): Promise<void> {
  await page.keyboard.press('Control+Backquote')
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

async function focusTerm(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  // The renderer helper textarea is positioned off-canvas (xterm
  // `.xterm-helper-textarea`). Click the host, then focus the textarea.
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) {
    await host.first().click({ position: { x: 24, y: 24 } })
  } else {
    await pane.click({ position: { x: 24, y: 24 } })
  }
  await page.evaluate(() => {
    const el = document.querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')
    el?.focus()
  })
}

async function typeLine(page: Page, line: string): Promise<void> {
  await focusTerm(page)
  await page.keyboard.type(line, { delay: 15 })
  await page.keyboard.press('Enter')
}

function lastStty(text: string): string | null {
  const matches = [...text.matchAll(/(\d+)\s+(\d+)/g)]
  if (matches.length === 0) return null
  return matches[matches.length - 1][0]
}

/** Same dataset surface esc-negotiate.spec reads: what did the keystroke do? */
function lastKeyCmd(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-last-key-cmd')
}

/** Focus to nowhere: blur whatever holds it so Esc arrives at the shell. */
async function blurFocus(page: Page): Promise<void> {
  await page.evaluate(() => {
    const active = document.activeElement
    if (active instanceof HTMLElement) active.blur()
  })
}

test.describe('terminal pane', () => {
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
    const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
    const body = (await res.json()) as { sessions?: unknown[] }
    expect(body.sessions ?? []).toEqual([])
  })

  test('echo, resize, Esc, replay, exit', async ({ page }) => {
    test.setTimeout(90_000)
    const errors = await boot(page)
    await openPane(page)

    await typeLine(page, "printf 'spike-864-%s\\n' ok")
    await expect.poll(async () => readTerm(page)).toContain('spike-864-ok')

    await typeLine(page, 'stty size')
    let sizeBefore: string | null = null
    await expect
      .poll(async () => {
        sizeBefore = lastStty(await readTerm(page))
        return sizeBefore
      })
      .toMatch(/\d+\s+\d+/)

    const viewport = page.viewportSize() ?? { width: 1280, height: 720 }
    await page.setViewportSize({
      width: viewport.width + 280,
      height: viewport.height + 80,
    })
    await typeLine(page, 'stty size')
    await expect
      .poll(async () => {
        const after = lastStty(await readTerm(page))
        return after !== null && after !== sizeBefore
      })
      .toBe(true)

    await page.keyboard.press('Escape')
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(page.getByTestId('shortcuts-dialog')).toHaveCount(0)
    await expect(page.locator('[data-testid="issue-layout"]')).toHaveAttribute(
      'data-detail-open',
      'false',
    )

    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    await expect.poll(async () => readTerm(page)).toContain('spike-864-ok')

    await focusTerm(page)
    await page.keyboard.press('Control+C')
    await typeLine(page, 'exit')
    await expect(page.getByTestId('terminal-status')).toContainText('Shell exited', {
      timeout: 20_000,
    })

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1154 (2026-08-29). The size the PTY believes and the size the pane
   * draws were allowed to disagree for the life of a session, and nothing
   * on screen said so: xterm renders at its own width, so only a child
   * asking the kernel — which is every TUI — sees the difference. Measured
   * on the phone pane, which carried the same six lines: sessions created
   * at cols 10 / rows 5 under a pane rendering 48x34.
   *
   * The cause was a cache that ran ahead of the send. `renderer.onResize`
   * advanced lastCols/lastRows and *then* called `socket?.resize(...)`,
   * a no-op before the socket is live — so the pane recorded a size the
   * server had never been told, and every later check found "no change".
   *
   * The existing case above proves a *change* propagates. This one proves
   * the starting value is right, which is the half that was wrong: ask the
   * PTY through stty, ask the renderer through its own dims, and require
   * them to agree with no resize in between.
   */
  test('the PTY agrees with the pane about its size from the first prompt (GDK-1154)', async ({
    page,
  }) => {
    test.setTimeout(60_000)
    const errors = await boot(page)
    await openPane(page)

    await typeLine(page, 'stty size')
    await expect.poll(async () => lastStty(await readTerm(page))).toMatch(/\d+\s+\d+/)
    const stty = lastStty(await readTerm(page))
    const [ptyRows, ptyCols] = (stty ?? '').split(/\s+/).map(Number) // stty prints rows cols

    const drawn = await page.evaluate(() => {
      const t = (window as unknown as { __gadakTerm?: { cols: number; rows: number } }).__gadakTerm
      return t ? { cols: t.cols, rows: t.rows } : null
    })
    expect(drawn, 'the renderer hook is missing').not.toBeNull()

    // A laid-out pane, not a collapsed one: 10x5 was the measured failure.
    expect(ptyCols, `stty said "${stty}"`).toBeGreaterThan(20)
    expect(ptyRows, `stty said "${stty}"`).toBeGreaterThan(5)
    expect(
      { cols: ptyCols, rows: ptyRows },
      `the PTY and the pane disagree: stty "${stty}" vs renderer ${JSON.stringify(drawn)}`,
    ).toEqual(drawn)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * 2026-08-28 — GDK-939. Svelte component identity is the template branch:
   * a MainColumn inside a terminal-keyed {#if} is destroyed and recreated on
   * every toggle, and the collateral is real (ListView teardown resets the
   * cursor, dashboard iframes reload, docs virtual scroll resets). An expando
   * stamped on the column's DOM node is the cheapest observable — a recreated
   * <main> loses it. FAIL-first, measured: against the split-branch template
   * the first read-back after opening the pane reported the stamp gone.
   */
  test('opening the terminal never remounts the main column (GDK-939)', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await page.setViewportSize({ width: 1440, height: 900 })

    const columnStamped = () =>
      page.evaluate(() => {
        const el = document.querySelector('[data-testid="main-column"]')
        return !!el && (el as HTMLElement & { __gdk939?: boolean }).__gdk939 === true
      })
    const stampColumn = () =>
      page.evaluate(() => {
        const el = document.querySelector('[data-testid="main-column"]')
        if (!el) throw new Error('main-column not found')
        ;(el as HTMLElement & { __gdk939?: boolean }).__gdk939 = true
      })

    await stampColumn()
    await openPane(page)
    expect(await columnStamped(), 'column identity across open').toBe(true)

    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
    expect(await columnStamped(), 'column identity across close').toBe(true)

    await openPane(page)
    expect(await columnStamped(), 'column identity across reopen').toBe(true)

    // The narrow threshold with the pane open used to swap in a second
    // TerminalPane template branch (WASM renderer + socket remount). The
    // pane now takes overlay as a prop, so its node survives the crossing.
    await page.evaluate(() => {
      const el = document.querySelector('[data-testid="terminal-pane"]')
      if (!el) throw new Error('terminal-pane not found')
      ;(el as HTMLElement & { __gdk939?: boolean }).__gdk939 = true
    })
    const paneStamped = () =>
      page.evaluate(() => {
        const el = document.querySelector('[data-testid="terminal-pane"]')
        return !!el && (el as HTMLElement & { __gdk939?: boolean }).__gdk939 === true
      })
    await page.setViewportSize({ width: 820, height: 900 })
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-overlay', 'true')
    expect(await columnStamped(), 'column identity across the narrow crossing').toBe(true)
    expect(await paneStamped(), 'pane identity across the narrow crossing').toBe(true)

    await page.setViewportSize({ width: 1440, height: 900 })
    await expect(page.getByTestId('terminal-pane')).not.toHaveAttribute('data-overlay', 'true')
    expect(await paneStamped(), 'pane identity across the wide crossing').toBe(true)

    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-945: the overlay terminal joins the Esc ladder — on axis B only. Esc
   * closes the pane when the VT does not hold focus; a focused terminal's Esc
   * is the PTY's (vim, less, fzf eat it), and chrome may not take it. A
   * docked split never joins the ladder: it is not covering anything.
   *
   * FAIL-first, measured 2026-08-29 against the pre-change tree: case 1
   * failed at toHaveCount(0) — "34 × locator resolved to 1 element", the
   * pane never closed. Cases 2 and 3 were already green there; they are the
   * axis-B pins (an always-phase Esc command — rejected axis A — turns case
   * 2 red, and treating a split as an overlay turns case 3 red).
   */
  test('overlay: Esc with focus outside the terminal closes the pane (GDK-945)', async ({
    page,
  }) => {
    test.setTimeout(90_000)
    await page.setViewportSize({ width: 820, height: 900 })
    const errors = await boot(page)
    await openPane(page)
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-overlay', 'true')

    // onOpen focuses the VT; walk focus off it so the Escape belongs to the
    // shell, not the terminal.
    await blurFocus(page)
    await page.keyboard.press('Escape')

    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
    expect(await lastKeyCmd(page)).toBe('close-terminal-overlay')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('overlay: Esc with focus in the terminal reaches the PTY, not the pane (GDK-945)', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    await page.setViewportSize({ width: 820, height: 900 })
    const errors = await boot(page)
    await openPane(page)
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-overlay', 'true')

    // cat -v prints the ESC byte as ^[, so the buffer itself is the witness
    // that the keystroke crossed chrome and reached the PTY. The printf
    // marker is the readiness handshake: once it is rendered, cat is the
    // foreground reader the Escape must land in.
    await typeLine(page, "printf 'GDK945RDY\\n'; cat -v")
    await expect.poll(async () => readTerm(page)).toContain('GDK945RDY')

    await focusTerm(page)
    await page.keyboard.press('Escape')

    await expect
      .poll(async () => readTerm(page), 'ESC should reach the PTY')
      .toContain('^[', {
        timeout: 10_000,
      })
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')

    // Leave no reader behind the pane for the drain below.
    await page.keyboard.press('Control+c')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('split: Esc does not close the pane (GDK-945)', async ({ page }) => {
    test.setTimeout(90_000)
    await page.setViewportSize({ width: 1280, height: 900 })
    const errors = await boot(page)
    await openPane(page)
    await expect(page.getByTestId('terminal-pane')).not.toHaveAttribute('data-overlay', 'true')

    // Even with focus off the VT: a split pane is not an overlay, so Esc
    // stays the column's and never becomes a close.
    await blurFocus(page)
    await page.keyboard.press('Escape')

    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('terminal shots', () => {
  test.use({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
  })

  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  /*
   * 2026-08-25 — GDK-864 (lead review). The default width is 44% of the
   * *window*, but the pane is a flex child of the layout's main track, which
   * is capped at 1360px. On a wide display those two disagree and the issue
   * list is squeezed. TERMINAL_SPLIT_MAX_PCT is the ceiling; this pins it
   * where the disagreement is largest.
   *
   * FAIL-first, measured: with the max-width removed from TerminalPane's
   * split style, this test failed here at `pane took 71% of the track`
   * (ratio 0.7118) — the list kept 392px of a 1360px track.
   */
  test('a wide display does not let the pane eat the list', async ({ page }) => {
    await page.setViewportSize({ width: 2200, height: 900 })
    await boot(page)
    await openPane(page)
    const trackBox = await page.getByTestId('terminal-split').boundingBox()
    const paneBox = await page.getByTestId('terminal-pane').boundingBox()
    expect(trackBox, 'split wrapper box').not.toBeNull()
    expect(paneBox, 'pane box').not.toBeNull()
    const ratio = paneBox!.width / trackBox!.width
    expect(ratio, `pane took ${Math.round(ratio * 100)}% of the track`).toBeLessThanOrEqual(0.62)
    expect(trackBox!.width - paneBox!.width).toBeGreaterThan(400)
  })

  /*
   * 2026-08-25 — GDK-864 (lead, entry-point pass). ⌘K is this app's answer to
   * "how do I do anything", so a surface reachable only by a chord is
   * reachable only by someone who already knows the chord. The row carries
   * Ctrl+` so the palette is also where you learn it.
   */
  test('the command palette opens the terminal, and shows its chord', async ({ page }) => {
    await boot(page)
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await palette.getByRole('combobox').fill('terminal')
    const row = palette.getByTestId('palette-action-terminal')
    await expect(row).toBeVisible()
    await expect(row).toContainText('Ctrl+`')
    await row.click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    // And the pane can be closed from inside itself: the chord that opens it
    // is swallowed by the VT on purpose, so a visible way out has to exist.
    await page.getByTestId('terminal-close').click()
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
  })

  /*
   * 2026-08-25 — GDK-864 (lead). Four surfaces want this row: sidebar, list,
   * terminal, detail panel. Below the sum of their minimums the split stops
   * being a split. FAIL-first, measured: with the rule disabled the pane
   * stayed a split and this assertion read `Expected: "true", Received: ""` —
   * a 1100px row where the pane's 320px min-width beat the percentage cap and
   * the list was left 70px of its own 390px floor.
   */
  test('a docked detail panel pushes the terminal to overlay when the row is too narrow', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1100, height: 900 })
    await boot(page)
    await openPane(page)
    await expect(page.getByTestId('terminal-pane')).not.toHaveAttribute('data-overlay', 'true')
    // Open an issue: the detail panel docks at 1100px (VIEWPORT_DOCKED_MIN_PX).
    await page.locator('[data-testid="issue-list-scroller"] [role="button"]').first().click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-overlay', 'true')
  })

  test('capture split, exited, overlay, dark', async ({ page }) => {
    test.setTimeout(90_000)
    mkdirSync(SHOT_DIR, { recursive: true })
    const hash = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
    /*
     * A commit hash alone is a claim the captures cannot keep: a round shoots
     * from the working tree, and a working tree with edits in it is not that
     * commit. A judge reading MANIFEST.md has no other way to know, and a
     * verdict on pixels that were never the named source is the expensive
     * failure here. Name the dirt.
     */
    const dirty = execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim()
    const source = dirty ? `${hash} + uncommitted edits (${dirty.split('\n').length} files)` : hash

    const shoot = async (name: string) => {
      await page.screenshot({
        path: join(SHOT_DIR, name),
        fullPage: false,
      })
    }

    await boot(page)
    await openPane(page)
    await typeLine(page, 'ls -la')
    await expect.poll(async () => readTerm(page)).toMatch(/total\s+\d+/)
    await shoot('01-split.png')

    await focusTerm(page)
    await page.keyboard.press('Control+C')
    await typeLine(page, 'exit')
    await expect(page.getByTestId('terminal-status')).toContainText('Shell exited')
    await shoot('02-exited.png')

    await boot(page)
    await openPane(page)
    await typeLine(page, "printf 'overlay\\n'")
    await page.setViewportSize({ width: 820, height: 900 })
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-overlay', 'true')
    await shoot('03-overlay.png')
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)

    await page.setViewportSize({ width: 1440, height: 900 })
    await boot(page)
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme', 'dark')
    })
    await openPane(page)
    await typeLine(page, 'ls -la')
    await expect.poll(async () => readTerm(page)).toMatch(/total\s+\d+/)
    await shoot('04-dark.png')

    writeFileSync(
      join(SHOT_DIR, 'MANIFEST.md'),
      [
        '# terminal pane captures (GDK-864; single renderer)',
        '',
        `- source: \`${source}\``,
        `- viewport: 1440×900, deviceScaleFactor 2 (03 is 820×900)`,
        '',
        '| file | notes |',
        '| --- | --- |',
        '| `01-split.png` | pane + list, `ls -la` |',
        '| `02-exited.png` | exited status line |',
        '| `03-overlay.png` | 820px overlay |',
        '| `04-dark.png` | `data-theme=dark` |',
        '',
      ].join('\n'),
      'utf8',
    )
  })
})
