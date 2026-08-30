#!/usr/bin/env node
/*
 * "Is a shell attached to this issue — and if not, why not?" (GDK-1162 /
 * GDK-1164, 2026-08-30).
 *
 * Promoted from the scratch curl this round was debugged with. Two questions
 * kept coming back and neither had a one-line answer:
 *
 *   - the ▶ is dead on GDK-nnn: is there no session, is the session bound to
 *     something else, or is it bound and exited?
 *   - the "No shell here" mark is on an issue somebody is clearly working on:
 *     which serve is being asked, and what does *it* see?
 *
 * Both are the same table — GET /api/v1/terminal/sessions/ — read through the
 * one join the UI uses. Printing the whole table alongside the verdict is the
 * point: "no shell on GDK-3" is not actionable, "no shell on GDK-3; two live
 * shells, bound to GDK-1 and nothing" is.
 *
 * Read-only, and deliberately so. It cannot unclaim, transition, or write
 * anything: knowing that no shell is on a claim is not the same as knowing
 * the claim is dead — a laptop that slept is a serve that knows nothing, and
 * a tool that "cleaned up" on that evidence would delete the claim of an
 * agent still running elsewhere. Detection and recording are different
 * layers; this is the detection one.
 *
 * Usage:
 *   node tools/issue-shell.mjs GDK-1162
 *   node tools/issue-shell.mjs            # just dump what the serve sees
 *   node tools/issue-shell.mjs GDK-1162 --base http://127.0.0.1:7878
 *   GADAK_SERVE_BASE=http://127.0.0.1:7878 node tools/issue-shell.mjs GDK-1
 *
 * Exit codes: 0 a live shell is bound · 3 none is · 1 the serve could not be
 * reached · 2 usage.
 */

const args = process.argv.slice(2)
if (args.includes('--help') || args.includes('-h')) {
  console.log(
    [
      'usage: node tools/issue-shell.mjs [ISSUE-KEY] [--base URL]',
      '',
      '  ISSUE-KEY   the issue to ask about; omit to only list what the serve sees',
      '  --base URL  the serve to ask (default $GADAK_SERVE_BASE or http://127.0.0.1:7777)',
      '',
      'exit: 0 bound · 3 not bound · 1 unreachable · 2 usage',
    ].join('\n'),
  )
  process.exit(0)
}

const baseIdx = args.indexOf('--base')
if (baseIdx !== -1 && !args[baseIdx + 1]) {
  console.error('issue-shell: --base needs a URL')
  process.exit(2)
}
const base = (
  baseIdx !== -1 ? args[baseIdx + 1] : process.env.GADAK_SERVE_BASE || 'http://127.0.0.1:7777'
).replace(/\/$/, '')
// The guard on baseIdx matters: with no --base, baseIdx is -1 and a bare
// `i !== baseIdx + 1` would swallow argv[0] — the key itself, silently
// turning `issue-shell.mjs GDK-1162` into the list-only mode that exits 0.
const key = args.filter((a, i) => !a.startsWith('--') && (baseIdx === -1 || i !== baseIdx + 1))[0] ?? null

const url = `${base}/api/v1/terminal/sessions/`
let sessions
try {
  const res = await fetch(url)
  if (!res.ok) {
    // A gate refusal is a different diagnosis from a dead serve, and the
    // caller needs to see which: 403 here means this host is not loopback.
    console.error(`issue-shell: ${url} answered ${res.status} ${await res.text()}`)
    process.exit(1)
  }
  sessions = (await res.json()).sessions ?? []
} catch (err) {
  console.error(`issue-shell: cannot reach ${url} — ${err.message}`)
  console.error('  is `gadak serve` running? pass --base for a non-default port.')
  process.exit(1)
}

const live = sessions.filter((s) => !s.exited)
const describe = (s) =>
  `  ${s.id.slice(0, 8)}…  ${s.exited ? 'exited ' : 'live   '} ${
    s.issue_key ? s.issue_key : '(unbound)'
  }  pid ${s.pid}  ${s.cols}x${s.rows}  attached ${s.attached}`

console.log(`serve: ${base}`)
console.log(`sessions: ${sessions.length} (${live.length} live)`)
for (const s of sessions) console.log(describe(s))

if (!key) process.exit(0)

const bound = live.find((s) => s.issue_key === key)
if (bound) {
  console.log(`\n${key}: bound to ${bound.id} — the ▶ in the issue body is live.`)
  process.exit(0)
}

// The "why not", in the order that actually distinguishes the cases.
const exitedOnKey = sessions.find((s) => s.issue_key === key && s.exited)
const reason = exitedOnKey
  ? `session ${exitedOnKey.id.slice(0, 8)}… was bound to it but has exited`
  : live.length === 0
    ? 'this serve has no live session at all — open the terminal pane'
    : `none of the ${live.length} live session(s) carries this key — run \`gadak claim ${key}\` inside the pane`
console.log(`\n${key}: no live shell — ${reason}.`)
console.log(
  'note: a binding is runtime state that dies with the serve. "Not here" is not "not anywhere".',
)
process.exit(3)
