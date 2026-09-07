import { test, expect, type Page } from '@playwright/test'
import { DEBUG_ATTRS_KEY } from '../web/src/lib/debug-attrs'
import {
  DEMO_ISSUE_COUNT_EN,
  DEMO_ISSUE_COUNT_EN_RE,
  drainTerminalSessions,
  forceLocale,
  readTerm,
} from './helpers'

/*
 * GDK-1186 — the click that focuses the pane is not a link click.
 *
 * A detail panel closed on camera came back with nobody touching a card, and
 * stood over the board's Done column for a whole take. The cause is this
 * file's subject: xterm activates a link on mouseup wherever the pointer
 * happens to be, and a terminal is something people click *at* — to start
 * typing, to put the caret back. The recording rig clicks the same fixed
 * point in the pane before every command it types; when an issue key had
 * scrolled under that point (GDK-1172 made a key live the instant it prints,
 * with no pointer motion needed), that focus click opened it.
 *
 * The two halves of the rule are both here, because either alone is a trap:
 * a pane that never opens links has lost GDK-1160, and a pane that opens them
 * from any click has this defect back.
 *
 * The pane must hold a shell for this to mean anything, so the suite's own
 * session hygiene applies (drain before and after).
 */

async function boot(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  // The panel's own account of who opened it (stores/panel) — read at the end
  // so a failure says which surface fired, not just that something did.
  await page.addInitScript((key) => {
    try {
      localStorage.setItem(key, '1')
    } catch {
      /* blocked storage: the assertions below still stand on the DOM */
    }
  }, DEBUG_ATTRS_KEY)
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_EN)
}

async function openPane(page: Page): Promise<void> {
  await page.keyboard.press('Control+Backquote')
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 30_000,
  })
}

/**
 * The rig's focus gesture, copied from e2e/demo/roundtrip.spec.ts: a click at
 * a fixed point in the pane's host, then the helper textarea focused
 * explicitly (the renderer paints on a canvas, so the click alone lands on
 * something that cannot hold a caret). This is the gesture under test — keep
 * it identical to the recording's, or this spec stops reproducing it.
 */
async function focusPane(page: Page): Promise<void> {
  const host = page.getByTestId('terminal-pane').locator('[data-gadak-editable]')
  await host.first().click({ position: { x: 24, y: 24 } })
  await page.evaluate(() => {
    document.querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')?.focus()
  })
}

async function typeLine(page: Page, line: string): Promise<void> {
  await focusPane(page)
  await page.keyboard.type(line, { delay: 8 })
  await page.keyboard.press('Enter')
}

/** The cell under the pane host's (24,24) — what the focus click lands on. */
async function keyUnderFocusPoint(page: Page): Promise<string> {
  return page.evaluate(() => {
    const t = (window as unknown as {
      __gadakTerm?: {
        rows: number
        cols: number
        buffer: {
          active: {
            viewportY: number
            getLine: (y: number) => { translateToString: (t?: boolean) => string } | undefined
          }
        }
      }
    }).__gadakTerm
    const host = document.querySelector<HTMLElement>(
      '[data-testid="terminal-pane"] [data-gadak-editable]',
    )
    const screen = document.querySelector<HTMLElement>(
      '[data-testid="terminal-pane"] .xterm-screen',
    )
    if (!t || !host || !screen) return ''
    const hb = host.getBoundingClientRect()
    const sb = screen.getBoundingClientRect()
    const col = Math.floor(((hb.left + 24 - sb.left) / sb.width) * t.cols)
    const row = Math.floor(((hb.top + 24 - sb.top) / sb.height) * t.rows)
    if (col < 0 || row < 0) return ''
    const text = t.buffer.active.getLine(t.buffer.active.viewportY + row)?.translateToString(true) ?? ''
    // Is that column inside an issue key on that line?
    for (const m of text.matchAll(/\b[A-Z][A-Z0-9]+-\d+\b/g)) {
      if (col >= m.index && col < m.index + m[0].length) return m[0]
    }
    return ''
  })
}

function detailOpen(page: Page): Promise<string | null> {
  return page.getByTestId('issue-layout').getAttribute('data-detail-open')
}

function panelTrace(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-panel-open')
}

test.describe('terminal issue links and the focus click (GDK-1186)', () => {
  test.use({ viewport: { width: 1440, height: 900 } })
  test.beforeEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  test('the click that focuses the pane does not open the issue under it', async ({ page }) => {
    test.setTimeout(180_000)
    await boot(page)
    await openPane(page)

    // `gadak`-shaped output: the key at column 0 of every row, which is what
    // every write verb's confirmation line looks like (summaryLine). Filling
    // the screen makes the fixed focus point land on a key deterministically
    // instead of by the luck the recording had.
    await typeLine(
      page,
      "for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do printf 'NM''B-140\\tthe tax column is missing\\n'; done",
    )
    await expect.poll(async () => readTerm(page), { timeout: 20_000 }).toContain('NMB-140')
    await expect
      .poll(() => keyUnderFocusPoint(page), {
        message: 'the focus point must sit on an issue key, or this spec proves nothing',
        timeout: 20_000,
      })
      .toBe('NMB-140')

    // Nothing is open, and the keyboard is outside the pane — a strip row is
    // exactly where the recording left it before each of its focus clicks.
    await expect(page.getByTestId('issue-layout')).toHaveAttribute('data-detail-open', 'false')
    await page.getByTestId('terminal-strip-row').first().click()
    await expect
      .poll(() => page.evaluate(() => document.activeElement?.getAttribute('data-testid') ?? ''))
      .toBe('terminal-strip-row')

    // The gesture. Nothing else.
    await focusPane(page)
    // Let the app paint. The link fires synchronously on mouseup and the
    // panel is a store read away, so two frames is the whole window in which
    // it could appear — a poll would only re-assert the same instant.
    await page.evaluate(
      () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
    )
    expect(
      await detailOpen(page),
      `a click that only focused the pane opened a detail panel. Panel trace: ${await panelTrace(page)}`,
    ).toBe('false')

    // …and the pane took the keyboard, which is the whole job of that click.
    expect(
      await page.evaluate(() => {
        const el = document.querySelector('[data-testid="terminal-pane"] .xterm')
        return !!el && el.contains(document.activeElement)
      }),
      'the focus click must still focus the pane',
    ).toBe(true)
  })

  test('a click in a pane that already holds the keyboard opens the key (GDK-1160)', async ({
    page,
  }) => {
    test.setTimeout(180_000)
    await boot(page)
    await openPane(page)
    await typeLine(
      page,
      "for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do printf 'NM''B-140\\tthe tax column is missing\\n'; done",
    )
    await expect.poll(async () => readTerm(page), { timeout: 20_000 }).toContain('NMB-140')
    await expect
      .poll(() => keyUnderFocusPoint(page), { timeout: 20_000 })
      .toBe('NMB-140')
    // typeLine's own focus click left the keyboard in the pane; the second
    // click at the same point is the one that means "open this".
    await focusPane(page)

    const panel = page.getByTestId('issue-detail-panel')
    await expect(
      panel,
      `the key under the pointer should have opened. Panel trace: ${await panelTrace(page)}`,
    ).toContainText('NMB-140', { timeout: 20_000 })
    expect(await panelTrace(page)).toContain('via terminal-link')
  })
})
