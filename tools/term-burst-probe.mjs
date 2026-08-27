#!/usr/bin/env node
/*
 * Terminal burst probe (GDK-1042, 2026-08-27). One command that answers
 * "does a large burst of small PTY reads still reach the client whole?"
 *
 *   - creates its own session, attaches over the WebSocket, and types
 *     `seq 1 N` at the shell (plus a computed end marker),
 *   - counts every binary byte that comes back,
 *   - watches for the {"t":"dropped"} frame that ends a slow client,
 *   - reads the session's own counters back from REST — backlog_max_bytes
 *     says how close the coalescing backlog came to the 4 MiB bound,
 *     coalesced_chunks says whether merging actually did work.
 *
 * --read-delay-ms throttles how fast the probe consumes frames: the
 * read-cadence knob that made the old chunk-count drop reproducible (a
 * client 256 tiny reads behind was cut at 59 KB). The delay applies to the
 * probe's handling, and the counters report whether the coalescing path was
 * genuinely exercised — an honest run prints them either way.
 *
 * Verdict:
 *   COMPLETE  the end marker arrived, no drop frame, rx >= seq's bytes
 *   DROPPED   a {"t":"dropped"} frame arrived — the client was cut
 *   SHORTFALL the marker never arrived and nothing was dropped — the
 *             stream stalled upstream of backpressure
 *
 * Diagnostic, not a gate: nothing in CI runs this. Like its sibling
 * term-stall-probe.mjs it always starts its own `gadak serve` on its own
 * port (default 7899, TERM_PROBE_PORT) via e2e/serve.sh — cold and fresh,
 * because a reused server can be serving the pre-fix binary and this probe
 * exists to catch exactly that class. Needs Node >= 21 for the global
 * WebSocket.
 *
 * Usage:
 *   node tools/term-burst-probe.mjs                        # 30000 lines, read flat out
 *   node tools/term-burst-probe.mjs --lines 10000 --read-delay-ms 5
 *   node tools/term-burst-probe.mjs --help
 *
 * Exit codes: 0 COMPLETE · 3 DROPPED · 4 SHORTFALL · 1 harness failure ·
 * 2 usage.
 */
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const PORT = process.env.TERM_PROBE_PORT || '7899'
const BASE = `http://127.0.0.1:${PORT}`

// The marker rides %s and is computed by the shell in double quotes
// OUTSIDE the format string: the echoed command line contains "$((6*7))",
// never BURST-END:42, so only real output can complete the run (the
// GDK-1045 convention).
const END_MARK = 'BURST-END:42'

const usage = () => {
  console.log(`usage: node tools/term-burst-probe.mjs [--lines N] [--read-delay-ms MS]

Runs \`seq 1 N\` in a fresh terminal session against a cold \`gadak serve\`
on :${PORT} (TERM_PROBE_PORT) and reports whether the whole burst reached
the client: rx bytes vs the seq arithmetic, any dropped frame, and the
session's backlog_max_bytes / coalesced_chunks / dropped_attachments from
REST. --read-delay-ms slows the probe's frame consumption, the read-cadence
knob that made the old chunk-count drop reproducible.

  --lines N          lines of seq output (default 30000)
  --read-delay-ms MS per-frame consumption delay (default 0)

exit 0 COMPLETE · 3 DROPPED · 4 SHORTFALL · 1 harness failure · 2 usage`)
}

function die(code, msg) {
  console.error(`[term-burst] ${msg}`)
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

/** Bytes of `seq 1 n` output: digits plus one newline per line. */
function seqBytes(n) {
  let total = 0
  for (let i = 1; i <= n; i++) total += String(i).length + 1
  return total
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

function parseArgs(argv) {
  const opts = { lines: 30000, readDelayMs: 0 }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    if (a === '-h' || a === '--help') return { help: true, opts }
    if (a !== '--lines' && a !== '--read-delay-ms') {
      die(2, `unknown argument ${a}`)
    }
    const raw = argv[++i]
    if (raw === undefined) die(2, `${a} needs a value`)
    if (!/^[0-9]+$/.test(raw)) die(2, `${a} must be a non-negative integer, got ${raw}`)
    const v = Number(raw)
    if (a === '--lines') {
      if (v < 1) die(2, '--lines must be at least 1')
      opts.lines = v
    } else {
      opts.readDelayMs = v
    }
  }
  return { help: false, opts }
}

async function main() {
  const { help, opts } = parseArgs(process.argv.slice(2))
  if (help) {
    usage()
    return 0
  }
  if (typeof WebSocket === 'undefined') {
    die(1, 'this probe needs Node >= 21 (global WebSocket)')
  }

  const stopServer = await startServer()
  let exitCode = 1
  let sid = null
  try {
    const created = await fetch(`${BASE}/api/v1/terminal/sessions/`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ cols: 80, rows: 24 }),
    })
    if (!created.ok) throw new Error(`session create ${created.status}`)
    sid = (await created.json()).id

    const ws = new WebSocket(`ws://127.0.0.1:${PORT}/api/v1/terminal/sessions/${sid}/ws/`)
    ws.binaryType = 'arraybuffer'
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('probe ws open timeout')), 8_000)
      ws.onopen = () => clearTimeout(timer) || resolve()
      ws.onerror = () => reject(new Error('probe ws error'))
    })

    // rx accounting and verdict inputs. Frames are consumed through a
    // serial queue so --read-delay-ms throttles the read cadence; the
    // socket stays open and the queue drains in order.
    let rx = 0
    let dropReason = null
    let exitFrame = null
    let text = ''
    let done = false
    let sawMark = false
    const queue = []
    let draining = false
    const drain = async () => {
      if (draining) return
      draining = true
      while (queue.length > 0) {
        const ev = queue.shift()
        if (opts.readDelayMs > 0) await sleep(opts.readDelayMs)
        if (ev.data instanceof ArrayBuffer) {
          rx += ev.data.byteLength
          text += Buffer.from(ev.data).toString('latin1')
          if (!sawMark && text.includes(END_MARK)) sawMark = true
        } else {
          try {
            const msg = JSON.parse(ev.data)
            if (msg.t === 'dropped') dropReason = msg.reason || 'unspecified'
            if (msg.t === 'exit') exitFrame = msg.code
          } catch {
            /* not JSON; ignore */
          }
        }
      }
      draining = false
    }
    ws.onmessage = (ev) => {
      queue.push(ev)
      void drain()
    }
    ws.onclose = () => {
      done = true
    }

    // One command line: the burst, then the computed end marker. The
    // command is submitted as PTY stdin; the echo comes back on the same
    // attachment and is counted in rx (which is why expected is a floor).
    const cmd = `seq 1 ${opts.lines}; printf 'BURST-END:%s\\n' "$((6*7))"`
    ws.send(new TextEncoder().encode(cmd + '\n'))
    const expected = seqBytes(opts.lines)

    // Completion is the marker, a positive signal — never a bare timeout.
    // The deadline scales with the delay: worst case every line is its own
    // frame, so a throttled reader legitimately takes lines x delay longer.
    const deadline = 60_000 + opts.readDelayMs * opts.lines
    const t0 = Date.now()
    try {
      await waitFor('the end marker', async () => {
        await drain()
        return sawMark || dropReason || done
      }, deadline, 100)
    } catch {
      /* handled by the verdict below */
    }
    await drain()
    const elapsed = Date.now() - t0

    // The server's own row: what the PTY produced, what the backlog did.
    const list = await (await fetch(`${BASE}/api/v1/terminal/sessions/`)).json()
    const row = (list.sessions ?? []).find((s) => s.id === sid) ?? {}

    console.log(`[term-burst] lines: ${opts.lines} (seq alone is ${expected} bytes), read delay: ${opts.readDelayMs} ms`)
    console.log(`[term-burst] rx: ${rx} bytes in ${elapsed} ms (marker ${sawMark ? 'seen' : 'NOT seen'})`)
    console.log(
      `[term-burst] server: bytes_out=${row.bytes_out ?? '?'} dropped_attachments=${row.dropped_attachments ?? '?'} ` +
        `backlog_max_bytes=${row.backlog_max_bytes ?? '?'} coalesced_chunks=${row.coalesced_chunks ?? '?'}`,
    )
    if (dropReason) {
      console.log(`[term-burst] dropped frame: ${dropReason}`)
      console.log('[term-burst] verdict: DROPPED — the client was cut; backpressure fired')
      exitCode = 3
    } else if (!sawMark) {
      if (exitFrame !== null) console.log(`[term-burst] exit frame: code ${exitFrame}`)
      console.log(
        '[term-burst] verdict: SHORTFALL — the marker never arrived and nothing was dropped; the stream stalled upstream of backpressure',
      )
      exitCode = 4
    } else {
      const short = rx < expected
      if (short) console.log(`[term-burst] rx ${rx} < seq's own ${expected} bytes`)
      console.log(
        `[term-burst] verdict: ${short ? 'SHORTFALL — the marker arrived but bytes are missing' : 'COMPLETE — the whole burst reached the client'}`,
      )
      exitCode = short ? 4 : 0
    }

    ws.close()
  } finally {
    if (sid) {
      await fetch(`${BASE}/api/v1/terminal/sessions/${sid}/`, { method: 'DELETE' }).catch(() => {})
    }
    stopServer()
  }
  return exitCode
}

main().then(
  (code) => process.exit(code),
  (err) => die(1, err?.stack ?? String(err)),
)
