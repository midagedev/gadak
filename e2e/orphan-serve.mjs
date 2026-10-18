/*
 * orphan-serve.mjs — single owner of "this listener is an abandoned gate serve"
 * (GDK-1951).
 *
 * A stopped Playwright run takes its parent down but not the serve it spawned:
 * the serve is re-parented to pid 1 and keeps its port for days. The stamp
 * checks (e2e/helpers.ts assertServedArtifact, mobile/e2e/serve.ts) refuse to
 * test against such a server, which is right, but left a person to find the
 * orphan with lsof and kill it. Both gates now call reapAbandonedServe(port)
 * at the point they would refuse.
 *
 * Abandoned = ppid 1 (the run that started it is gone) AND a command line this
 * repo's gates start. Anything else on the port — a developer's `gadak serve`,
 * a live run's serve (living parent) — is never touched.
 *
 * Lookup is lsof by port + ps by pid; the only signalled pid is the one lsof
 * returned for that port. Never pgrep/pkill by pattern.
 */
import { spawnSync } from 'node:child_process'

const GATE_SERVE_SHAPES = [
  // e2e/serve.sh: <worktree>/e2e/.tmp/gadak[-<port>] serve …
  { name: 'e2e serve', re: /^\S*\/e2e\/\.tmp\/gadak(-\d+)? serve(\s|$)/ },
  // mobile/e2e/gate-serve.sh api: $TMPDIR/gadak-mobile-api-<port> demo …
  { name: 'phone gate demo', re: /^\S*\/gadak-mobile-api-\d+ demo(\s|$)/ },
  // mobile/e2e/gate-serve.sh ui: npx vite preview --config <…>/mobile/vite.config.ts …
  { name: 'phone gate vite preview', re: /\bvite(\.js)? preview\b.*--config \S*\/mobile\/vite\.config\.ts(\s|$)/ },
]

/**
 * Pure verdict over one listener row {pid, ppid, command}.
 * Returns {abandoned, reason}.
 */
export function classifyListener(row) {
  if (!row || row.ppid !== 1) return { abandoned: false, reason: 'parent is alive' }
  const shape = GATE_SERVE_SHAPES.find((s) => s.re.test(row.command ?? ''))
  if (!shape) return { abandoned: false, reason: 'not a gate serve command' }
  return { abandoned: true, reason: shape.name }
}

/** Listener pids on a port, via lsof (empty when lsof is absent or nothing listens). */
export function listenerPids(port) {
  const res = spawnSync('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t'], { encoding: 'utf8' })
  if (res.status !== 0 || !res.stdout) return []
  return [...new Set(res.stdout.split('\n').map((l) => Number(l.trim())).filter((n) => Number.isInteger(n) && n > 0))]
}

/** {pid, ppid, etime, command} for a pid, or null when it is gone. */
export function describePid(pid) {
  const res = spawnSync('ps', ['-o', 'ppid=,etime=,command=', '-p', String(pid)], { encoding: 'utf8' })
  if (res.status !== 0 || !res.stdout.trim()) return null
  const m = res.stdout.trim().match(/^(\d+)\s+(\S+)\s+(.*)$/s)
  if (!m) return null
  return { pid, ppid: Number(m[1]), etime: m[2], command: m[3] }
}

function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms)
}

function alive(pid) {
  try {
    process.kill(pid, 0)
    return true
  } catch (err) {
    return err.code !== 'ESRCH'
  }
}

function terminate(pid, waitMs = 5000) {
  process.kill(pid, 'SIGTERM')
  const deadline = Date.now() + waitMs
  while (Date.now() < deadline) {
    if (!alive(pid)) return 'SIGTERM'
    sleepSync(100)
  }
  try {
    process.kill(pid, 'SIGKILL')
  } catch {}
  sleepSync(200)
  return 'SIGKILL'
}

/**
 * If the listener on `port` is an abandoned gate serve, kill that pid only.
 * Returns the one-line report, or null when nothing was reaped.
 * `deps` is injectable for tests.
 */
export function reapAbandonedServe(port, deps = {}) {
  const pids = (deps.listenerPids ?? listenerPids)(port)
  const describe = deps.describePid ?? describePid
  const kill = deps.terminate ?? terminate
  for (const pid of pids) {
    const row = describe(pid)
    if (!row || !classifyListener(row).abandoned) continue
    kill(pid)
    return `reaped abandoned gate serve pid ${pid} on :${port} (up ${row.etime}, parent gone): ${row.command}`
  }
  return null
}
