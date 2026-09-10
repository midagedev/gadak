import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-1758: the read-only/mutating census of e2e specs, as a gate.
 *
 * `workers: 1` is not a Playwright quirk here — it is load-bearing. One
 * suite shares ONE serve (one GADAK_E2E_PORT → one home `e2e/.tmp/home-<port>`
 * → one mirror DB → one terminal-session pool). A spec that writes through
 * the app's API surface is sharing all four with every other spec; two
 * workers running mutating specs concurrently would delete each other's
 * terminal sessions (drainTerminalSessions below drains ALL of them) and
 * race dashboard/view ids. This file is the enumeration any future
 * `workers > 1` design has to answer for, kept honest mechanically:
 *
 *   shared state under one port     | isolation a worker would need
 *   --------------------------------+------------------------------------
 *   the served app instance         | per-worker port (homes are port-keyed)
 *   the mirror DB (writes)          | per-worker home → per-worker serve
 *   terminal PTY pool               | per-worker drain scope, not "all"
 *   IndexedDB cache                 | already per-context (Playwright) ✔
 *
 * Enabling workers>1 is NOT this round (GDK-1757 first: port assignment
 * must fail loud before more ports exist to assign). What this gate buys
 * now: a spec cannot start mutating without this list knowing, and a
 * listed spec cannot silently stop mutating (then be assumed still
 * quarantined when the real isolation design lands).
 *
 * Four mechanical markers, one axis each — a spec hits the census if any
 * fires (helpers.ts itself is the fifth surface: drainTerminalSessions
 * deletes terminal sessions, so its callers are census entries by name):
 *   1. `(page.)request.(post|put|delete|patch)(` — API mutation via the
 *      Playwright request context. `searchParams.delete(` cannot match:
 *      the receiver must be `request`.
 *   2. `method: 'POST'|'PUT'|'DELETE'|'PATCH'` — a fetch inside
 *      page.evaluate mutating the API.
 *   3. `.objectStore(...).put(` — a page-side cache write (IndexedDB).
 *   4. `drainTerminalSessions(` — helper-mediated session deletion.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))

const API_MUTATION = /(?:^|[^\w.])(?:page\.)?request\.(?:post|put|delete|patch)\(/
const EVAL_FETCH_MUTATION = /method:\s*'(?:POST|PUT|DELETE|PATCH)'/
const PAGE_STORAGE_WRITE = /\.objectStore\([^)]*\)\.put\(/
const HELPER_MUTATION = /\bdrainTerminalSessions\(/

/** The hand-kept classification. A spec here mutates shared suite state. */
const MUTATING: string[] = [
  'cache-upgrade.spec.ts', // IndexedDB cache writes (page-side)
  'dashboard-frame-bg.spec.ts',
  'dashboard-legend-overflow.spec.ts',
  'dashboards.spec.ts',
  'issue-command.spec.ts',
  'linear.spec.ts', // writes through the linear mock origin
  'session-entry.spec.ts', // drainTerminalSessions
  'settings.spec.ts',
  'sidebar-sections.spec.ts',
  'terminal-burst.spec.ts', // drainTerminalSessions
  'terminal-link-focus.spec.ts', // drainTerminalSessions
  'terminal-modes.spec.ts', // drainTerminalSessions
  'terminal-settings.spec.ts',
  'terminal-strip.spec.ts',
  'terminal-theme.spec.ts',
  'terminal.spec.ts', // drainTerminalSessions
  'theme.spec.ts',
  'user-tokens.spec.ts',
  'view-cross-device.spec.ts', // evaluate fetch DELETE of views
  'workspaces-manage.spec.ts', // destroy_origin=1 — the heaviest mutation
]

function specFiles(): string[] {
  return readdirSync(E2E_DIR)
    .filter((f) => f.endsWith('.spec.ts'))
    .sort()
}

function mutatingHit(src: string): string | null {
  if (API_MUTATION.test(src)) return 'request mutation'
  if (EVAL_FETCH_MUTATION.test(src)) return 'evaluate fetch mutation'
  if (PAGE_STORAGE_WRITE.test(src)) return 'page storage write'
  if (HELPER_MUTATION.test(src)) return 'drainTerminalSessions'
  return null
}

test('every mutating spec is censused, and every censused spec still mutates', () => {
  const drift: string[] = []
  for (const f of specFiles()) {
    const src = readFileSync(join(E2E_DIR, f), 'utf8')
    const hit = mutatingHit(src)
    const listed = MUTATING.includes(f)
    if (hit && !listed) drift.push(`${f}: ${hit} but not in MUTATING`)
    if (!hit && listed) drift.push(`${f}: listed but no mutation marker fires`)
  }
  const ghosts = MUTATING.filter((f) => !specFiles().includes(f))
  for (const g of ghosts) drift.push(`${g}: listed but no such spec file`)
  expect(
    drift,
    'mutation census drift — update MUTATING in e2e/mutation-census.unit.ts (GDK-1758)',
  ).toEqual([])
  // The census must stay non-trivial: an empty list would pass the loop
  // above only if no spec mutated, which today is not the world we serve.
  expect(MUTATING.length, 'the mutating census went empty — workers>1 maths changed').toBeGreaterThan(
    0,
  )
})

test('the census and the workers>1 design agree on the axes they name', () => {
  // The header names the shared-state axes. If the design section is
  // rewritten, keep the axes and this check in step — the point of the gate
  // is that the list IS the design input.
  const src = readFileSync(new URL(import.meta.url), 'utf8')
  for (const axis of [
    'the served app instance',
    'the mirror DB',
    'terminal PTY pool',
    'IndexedDB cache',
  ]) {
    expect(src, `the shared-state axis "${axis}" left this file's design section`).toContain(axis)
  }
})
