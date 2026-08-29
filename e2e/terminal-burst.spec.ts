import { test, expect, type Page } from '@playwright/test'
import {
  apiURL,
  appConsoleErrors,
  attachConsoleErrors,
  DEMO_ISSUE_COUNT_EN_RE,
  drainTerminalSessions,
  forceLocale,
} from './helpers'

/*
 * GDK-1042 (2026-08-27). A burst as small as 10k lines of `seq` (~59 KB,
 * arriving as ~230-byte PTY reads) used to end the pane with "Disconnected:
 * slow client": the per-attachment bound was a chunk COUNT, so a client
 * that was 256 tiny reads behind — one frame of not being scheduled — was
 * cut. The backlog is bytes now and chunks are coalesced (internal/term,
 * attach_backlog_test.go). This gate is the socket-level row: the unit
 * tests cannot see the browser's read cadence, and the defect only ever
 * fired through it.
 *
 * FAIL-first, same day, against the chunk-count bound: this poll timed out
 * with the buffer stuck part-way and the pane showing the dropped status —
 * survival was per-attempt probability, so a green run proved nothing
 * until the bound was bytes.
 *
 * The echoed command line contains "30000" all by itself, so completion is
 * asserted on a line that EQUALS the last output line, never on a substring;
 * and the computed markers ride %s — an arithmetic expansion inside the
 * single-quoted printf format would not expand, and a literal marker could
 * be satisfied by the echo of the command (the GDK-1045 convention).
 */

type TermHook = {
  buffer: {
    active: {
      length: number
      getLine: (y: number) => { translateToString: (trimRight?: boolean) => string } | undefined
    }
  }
}

type SessionRow = {
  id: string
  bytes_out: number
  dropped_attachments: number
  backlog_max_bytes: number
  coalesced_chunks: number
}

// 138894 = bytes of `seq 1 30000` (digits only, one \n per line). The
// server's own count of what the PTY produced — echo and prompt bytes ride
// on top, so this is a floor, not an equality.
function seqBytes(n: number): number {
  let total = 0
  for (let i = 1; i <= n; i++) total += String(i).length + 1
  return total
}
const LINES = 30000

async function boot(page: Page): Promise<string[]> {
  const errors = attachConsoleErrors(page)
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  return errors
}

async function openPane(page: Page): Promise<void> {
  await page.keyboard.press('Control+Backquote')
  await expect(page.getByTestId('terminal-pane')).toBeVisible()
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
}

async function readTermLines(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const t = (window as unknown as { __gadakTerm?: TermHook }).__gadakTerm
    if (!t) return []
    const buf = t.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buf.length; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? '')
    }
    return lines
  })
}

async function focusTerm(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) await host.first().click({ position: { x: 24, y: 24 } })
  else await pane.click({ position: { x: 24, y: 24 } })
  await page.evaluate(() => {
    const el = document.querySelector<HTMLTextAreaElement>(
      '[data-testid="terminal-pane"] textarea',
    )
    el?.focus()
  })
}

async function typeLine(page: Page, line: string): Promise<void> {
  await focusTerm(page)
  await page.keyboard.type(line, { delay: 15 })
  await page.keyboard.press('Enter')
}

async function sessions(page: Page): Promise<SessionRow[]> {
  const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
  expect(res.ok(), `session list ${res.status()}`).toBe(true)
  const body = (await res.json()) as { sessions?: SessionRow[] }
  return body.sessions ?? []
}

test.describe('terminal burst', () => {
  // This suite counts sessions, so it starts from zero rather than trusting
  // the suites before it: one that opened a pane and walked away left a live
  // session the grace never reaps, and the count failed here instead of
  // there (GDK-1127). Ordering shifts with every test added; this does not.
  test.beforeEach(async ({ page }) => {
    const leaked = await drainTerminalSessions(page)
    if (leaked > 0) console.log(`[terminal-burst] drained ${leaked} leaked session(s) on entry`)
  })

  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
    const body = (await (await page.request.get(apiURL('/api/v1/terminal/sessions/'))).json()) as {
      sessions?: unknown[]
    }
    expect(body.sessions ?? []).toEqual([])
  })

  test('a 30k-line burst is coalesced, not cut', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)
    const pageErrors: string[] = []
    page.on('pageerror', (err) => pageErrors.push(String(err).split('\n')[0]))
    await boot(page)
    await openPane(page)

    const t0 = Date.now()
    await typeLine(page, `seq 1 ${LINES}; printf 'GDK1042-END:%s\\n' "$((6*7))"`)
    // The last line of the burst, as a whole line: the echoed command
    // contains "30000" as a substring, only parsed output has it alone.
    await expect
      .poll(async () => (await readTermLines(page)).some((l) => l.trim() === String(LINES)), {
        timeout: 60_000,
      })
      .toBe(true)
    await expect.poll(async () => (await readTermLines(page)).join('\n')).toContain('GDK1042-END:42')
    const burstMs = Date.now() - t0

    // Still attached and still parsing after the burst: a client that was
    // dropped mid-burst never sees this marker.
    await typeLine(page, "printf 'GDK1042-LIVE:%s\\n' \"$((7*8))\"")
    await expect.poll(async () => (await readTermLines(page)).join('\n')).toContain('GDK1042-LIVE:56')

    // No dropped end reached the client: the disconnected state renders
    // into terminal-status, and the pane is still attached.
    await expect(page.getByTestId('terminal-status')).toHaveCount(0)
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true')
    expect(pageErrors, `uncaught page errors:\n${pageErrors.join('\n')}`).toEqual([])
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])

    // The server's own counters: the burst completed and nothing was
    // dropped. The two coalescing counters are asserted for presence and
    // numeric type only — how much merging a given run does depends on the
    // browser's read cadence, and pinning a floor here would be pinning the
    // scheduler. Their values are what tools/term-burst-probe.mjs reports.
    const rows = await sessions(page)
    // GDK-1127: this asserted a bare count, and a leaked session from another
    // suite made it fail here — the bare number named neither the leak nor
    // its owner. Printing the rows is what identified it (created_at 95s
    // before this session, attached:0, grace_extensions:1 — abandoned, alive,
    // never reaped), so the diagnosis stays in the assertion rather than
    // being rebuilt the next time.
    expect(rows.length, `sessions:\n${JSON.stringify(rows, null, 2)}`).toBe(1)
    const row = rows[0]
    expect(row.dropped_attachments).toBe(0)
    expect(row.bytes_out).toBeGreaterThanOrEqual(seqBytes(LINES))
    expect(row.backlog_max_bytes).toBeGreaterThanOrEqual(0)
    expect(row.coalesced_chunks).toBeGreaterThanOrEqual(0)
    console.log(`[terminal-burst] ${LINES} lines rendered in ${burstMs} ms`)
  })
})
