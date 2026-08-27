#!/usr/bin/env node
/*
 * Terminal stall probe (GDK-1045 follow-up, 2026-08-27). One command that
 * answers "bytes arrive but the pane never changes" by separating the two:
 *
 *   - attaches a SECOND WebSocket to the pane's own PTY session and counts
 *     the output bytes the browser receives (the serve broadcasts to every
 *     attachment),
 *   - reads the xterm buffer through the pane's __gadakTerm hook,
 *   - collects page errors (the GDK-1045 parser death surfaced as
 *     `ReferenceError: i is not defined` inside xterm's requestMode).
 *
 * Verdict:
 *   RENDER  bytes arrived AND the buffer changed — the pane path is alive
 *   STALL   bytes arrived, buffer unchanged — renderer/parser died mid-chunk
 *           (GDK-1045 class: DECRQM hit the downleveled `||=`)
 *   NO-RX   bytes never arrived — look upstream of the renderer (serve, PTY,
 *           the command itself)
 *
 * Diagnostic, not a gate: nothing in CI runs this. It always starts its own
 * `gadak serve` on its own port (default 7899, TERM_PROBE_PORT) via
 * e2e/serve.sh — cold and fresh on purpose, because a reused server can be
 * serving a stale bundle and this probe exists to catch exactly that class.
 * Needs Node >= 21 for the global WebSocket.
 *
 * Usage:
 *   node tools/term-stall-probe.mjs                 # default: DECRQM reproducer
 *   node tools/term-stall-probe.mjs crush            # probe any command
 *   node tools/term-stall-probe.mjs --help
 *
 * Exit codes: 0 RENDER · 3 STALL · 4 NO-RX · 1 harness failure · 2 usage.
 * The command is typed at the pane's shell prompt; output bytes are counted
 * from the moment the command line is submitted.
 */
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { chromium } from '@playwright/test'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const PORT = process.env.TERM_PROBE_PORT || '7899'
const BASE = `http://127.0.0.1:${PORT}`

// The known stall reproducer: two DECRQM probes and a marker in one write
// chunk (GDK-1045). The marker rides %s — an arithmetic expansion inside
// the single-quoted format would not expand, and a literal marker could be
// satisfied by the echoed command line instead of parsed output.
const DEFAULT_CMD = `printf '\\033[?2026$p\\033[?1$pSTALL-MARK:%s\\n' "$((6*7))"`
const MARKER = 'STALL-MARK:42'

const usage = () => {
  console.log(`usage: node tools/term-stall-probe.mjs [command...]

Separates "output bytes arrived in the browser" from "the xterm buffer
changed" for the terminal pane — the one-command answer to a TUI that shows
nothing in the pane. Runs the command (default: the GDK-1045 DECRQM
reproducer) in a fresh pane against a cold \`gadak serve\` on :${PORT}
(TERM_PROBE_PORT), then prints rx bytes, buffer delta, page errors, and a
RENDER / STALL / NO-RX verdict.

exit 0 RENDER · 3 STALL · 4 NO-RX · 1 harness failure · 2 usage`)
}

function die(code, msg) {
  console.error(`[term-stall] ${msg}`)
  process.exit(code)
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

/** Poll until fn() returns truthy; state-based, never a bare sleep. */
async function waitFor(what, fn, timeoutMs = 30_000, intervalMs = 200) {
  const t0 = Date.now()
  for (;;) {
    const v = await fn()
    if (v) return v
    if (Date.now() - t0 > timeoutMs) throw new Error(`timeout waiting for ${what}`)
    await sleep(intervalMs)
  }
}

async function startServer() {
  const child = spawn('bash', [join(ROOT, 'e2e', 'serve.sh')], {
    cwd: ROOT,
    env: { ...process.env, GADAK_E2E_PORT: PORT },
    stdio: ['ignore', 'inherit', 'inherit'],
  })
  const stop = () => {
    try {
      child.kill('SIGTERM')
    } catch {
      /* already gone */
    }
  }
  process.on('SIGINT', () => {
    stop()
    process.exit(130)
  })
  await waitFor(
    `serve on :${PORT} (building binary + UI first — this can take a minute)`,
    async () => {
      try {
        const r = await fetch(`${BASE}/healthz`)
        return r.ok
      } catch {
        return false
      }
    },
    240_000,
    500,
  )
  return stop
}

async function readTerm(page) {
  return page.evaluate(() => {
    const t = window.__gadakTerm
    if (!t) return ''
    const buf = t.buffer.active
    const lines = []
    for (let i = 0; i < buf.length; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? '')
    }
    return lines.join('\n')
  })
}

async function focusTerm(page) {
  const pane = page.getByTestId('terminal-pane')
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) await host.first().click({ position: { x: 24, y: 24 } })
  else await pane.click({ position: { x: 24, y: 24 } })
  await page.evaluate(() => {
    document
      .querySelector('[data-testid="terminal-pane"] textarea')
      ?.focus()
  })
}

async function main() {
  const args = process.argv.slice(2)
  if (args[0] === '-h' || args[0] === '--help') {
    usage()
    return 0
  }
  if (args[0] !== undefined && args[0].startsWith('-')) {
    usage()
    return 2
  }
  if (typeof WebSocket === 'undefined') {
    die(1, 'this probe needs Node >= 21 (global WebSocket)')
  }
  const cmd = args.length > 0 ? args.join(' ') : DEFAULT_CMD

  const stopServer = await startServer()
  const browser = await chromium.launch()
  let exitCode = 1
  try {
    const page = await browser.newPage()
    const pageErrors = []
    page.on('pageerror', (err) => pageErrors.push(String(err).split('\n')[0]))

    await page.addInitScript(() => {
      try {
        if (!localStorage.getItem('gadak_locale')) localStorage.setItem('gadak_locale', 'en')
      } catch {
        /* ignore */
      }
    })
    await page.goto(`${BASE}/`)
    await page.getByTestId('issue-layout').waitFor({ state: 'visible', timeout: 30_000 })
    // Same shape as e2e gotoApp's toHaveURL(/[#?&]sc=/): the view state can
    // land as `#sc=`, `#…?sc=` or `&sc=` — dropping the `?` hangs the boot.
    await waitFor('startup view hash', () => page.url().match(/[#?&]sc=/))

    await page.keyboard.press('Control+Backquote')
    await waitFor('terminal pane attach', () =>
      page
        .getByTestId('terminal-pane')
        .getAttribute('data-attached')
        .then((v) => v === 'true'),
    )

    // Second attachment to the same PTY: everything the shell writes is
    // broadcast here too, so rx counts exactly the bytes the browser COULD
    // render — independent of whether the pane's xterm actually does.
    const list = await (await fetch(`${BASE}/api/v1/terminal/sessions/`)).json()
    const sid = list.sessions?.[0]?.id
    // Throw, never die(), inside the try: process.exit would skip the
    // finally and orphan the serve child.
    if (!sid) throw new Error('no live terminal session after opening the pane')
    const ws = new WebSocket(`ws://127.0.0.1:${PORT}/api/v1/terminal/sessions/${sid}/ws/`)
    ws.binaryType = 'arraybuffer'
    let rx = 0
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('probe ws open timeout')), 8_000)
      ws.onopen = () => clearTimeout(timer) || resolve()
      ws.onerror = () => reject(new Error('probe ws error'))
    })
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) rx += ev.data.byteLength
    }

    const before = await readTerm(page)
    await focusTerm(page)
    await page.keyboard.type(cmd, { delay: 15 })
    const rxAtSubmit = rx
    await page.keyboard.press('Enter')
    const tSubmit = Date.now()

    // Quiescence by observation: rx must stop growing for 2s, observed for
    // at least 4s — a TUI cold start (crush drew its first frame ~2s after
    // launch when measured) must not be declared silent while booting. The
    // probe exists precisely for stalls where no render state ever flips,
    // so there is nothing else to wait ON — the interval is the
    // measurement, not a boot sleep.
    let stable = 0
    let last = rx
    for (let i = 0; i < 150 && stable < 10; i++) {
      await sleep(200)
      if (Date.now() - tSubmit >= 4_000) {
        if (rx === last) stable++
        else stable = 0
      }
      last = rx
    }
    const after = await readTerm(page)
    const rxBytes = rx - rxAtSubmit

    // The render signal must exclude the ECHO of the typed command — the
    // echo changes the buffer all by itself and would read as RENDER for a
    // TUI that drew nothing. Echo lines are substrings of the typed command.
    const oldLines = new Set(before.split('\n'))
    const fresh = after
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !oldLines.has(l) && !cmd.includes(l))
      .slice(0, 5)
      .map((l) => l.slice(0, 60))

    console.log(`[term-stall] cmd: ${cmd}`)
    console.log(`[term-stall] rx after submit: ${rxBytes} bytes`)
    console.log(
      `[term-stall] buffer: ${fresh.length ? 'changed' : 'UNCHANGED'} ` +
        `(${before.length} -> ${after.length} chars${after.includes(MARKER) ? `, contains ${MARKER}` : ''})`,
    )
    if (fresh.length) console.log(`[term-stall] new lines:\n  ${fresh.join('\n  ')}`)
    console.log(
      pageErrors.length
        ? `[term-stall] page errors (${pageErrors.length}):\n  - ${pageErrors.join('\n  - ')}`
        : '[term-stall] page errors: none',
    )

    if (rxBytes === 0) {
      console.log('[term-stall] verdict: NO-RX — bytes never arrived; look upstream (serve, PTY, the command)')
      exitCode = 4
    } else if (fresh.length) {
      console.log('[term-stall] verdict: RENDER — output beyond the echo was parsed and rendered')
      exitCode = 0
    } else {
      console.log(
        '[term-stall] verdict: STALL — bytes arrived, nothing beyond the echo rendered; ' +
          'the renderer/parser died mid-chunk (GDK-1045 class). Page errors above are the prime suspect.',
      )
      exitCode = 3
    }

    ws.close()
    await fetch(`${BASE}/api/v1/terminal/sessions/${sid}/`, { method: 'DELETE' }).catch(() => {})
  } finally {
    await browser.close().catch(() => {})
    stopServer()
  }
  return exitCode
}

main().then(
  (code) => process.exit(code),
  (err) => die(1, err?.stack ?? String(err)),
)
