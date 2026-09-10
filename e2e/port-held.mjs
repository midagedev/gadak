#!/usr/bin/env node
/*
 * port-held.mjs — is 127.0.0.1:<port> already listening, and if so, by whom?
 *
 * GDK-1757: e2e/serve.sh calls this before building anything. A port held by
 * another worktree's serve (or any stray listener) used to surface only when
 * `gadak serve` hit EADDRINUSE minutes later, naming nobody — after the go
 * build, the UI build and the fixture seeding had all run. The healthy-reuse
 * case never reaches this probe: playwright's reuseExistingServer adopts a
 * healthz-answering serve before running its command, and the served-artifact
 * stamp check in e2e/helpers.ts is that path's honesty gate. This script owns
 * the other half — a listener that did NOT satisfy reuseExistingServer — and
 * its whole job is to name port, pid, command and stamped worktree, then
 * exit 1 so the build never starts.
 *
 * Exit 0: nothing listens (or the probe cannot reach it) — serve.sh proceeds.
 * Exit 1: something listens; the report on stderr says who.
 */
import { createConnection } from 'node:net'
import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'

const PROBE_TIMEOUT_MS = 500

function probe(port) {
  return new Promise((resolve) => {
    const socket = createConnection({ host: '127.0.0.1', port })
    const done = (held) => {
      socket.destroy()
      resolve(held)
    }
    socket.setTimeout(PROBE_TIMEOUT_MS)
    socket.on('connect', () => done(true))
    socket.on('error', () => done(false))
    // On loopback a connect either lands or is refused immediately; a timeout
    // is not a listener anyone can bind over, so treat it as free.
    socket.on('timeout', () => done(false))
  })
}

function listenersOn(port) {
  // status null is ENOENT (no lsof on this host) — the pid-less report below
  // is still useful, so absence degrades instead of failing.
  const res = spawnSync('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN'], { encoding: 'utf8' })
  if (res.status !== 0 || !res.stdout) return []
  return res.stdout
    .split('\n')
    .slice(1)
    .filter((line) => line.trim() !== '')
    .map((line) => {
      const cols = line.trim().split(/\s+/)
      return { command: cols[0], pid: cols[1] }
    })
}

function stampFor(port) {
  const path = join(process.env.TMPDIR || '/tmp', `gadak-e2e-served-${port}.json`)
  if (!existsSync(path)) return null
  try {
    const value = JSON.parse(readFileSync(path, 'utf8'))
    if (!value || typeof value !== 'object') return null
    return value
  } catch {
    return null
  }
}

const port = process.argv[2]
if (!port || !/^[1-9][0-9]*$/.test(port) || Number(port) > 65535) {
  console.error(`[e2e] port-held.mjs: expected a port argument 1-65535, got ${JSON.stringify(process.argv[2] ?? '')}`)
  process.exit(2)
}

if (!(await probe(Number(port)))) process.exit(0)

const lines = [`[e2e] port ${port} is already listening — this serve cannot bind it (GDK-1757).`]
for (const l of listenersOn(port)) {
  lines.push(`[e2e]   holder: pid ${l.pid} (${l.command}) — lsof -nP -iTCP:${port} -sTCP:LISTEN`)
}
const stamp = stampFor(port)
if (stamp && typeof stamp.worktree === 'string') {
  const pid = typeof stamp.pid === 'number' ? ` pid ${stamp.pid}` : ''
  lines.push(`[e2e]   served-artifact stamp: worktree ${stamp.worktree}${pid} digest ${stamp.digest ?? '?'}`)
  lines.push(`[e2e]   if that is a serve you own: kill ${stamp.pid ?? `$(lsof -t -nP -iTCP:${port} -sTCP:LISTEN)`}, or pkill -f '${stamp.worktree}/e2e/.tmp/gadak'.`)
} else {
  lines.push(`[e2e]   no served-artifact stamp for this port — the holder is not this e2e suite's serve.`)
  lines.push(`[e2e]   stop it (lsof -nP -iTCP:${port} -sTCP:LISTEN names it) or move GADAK_E2E_PORT.`)
}
console.error(lines.join('\n'))
process.exit(1)
