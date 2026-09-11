import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-1758: the gate over e2e/mutating-specs.txt, the mutating census.
 *
 * `workers: 1` is not a Playwright quirk here — it is load-bearing. One
 * suite shares ONE serve (one GADAK_E2E_PORT → one home `e2e/.tmp/home-<port>`
 * → one mirror DB → one terminal-session pool). A spec that writes through
 * the app's API surface is sharing all of them with every other spec; two
 * workers running mutating specs concurrently would delete each other's
 * terminal sessions (drainTerminalSessions below drains ALL of them) and
 * race dashboard/view ids.
 *
 * The LIST lives in e2e/mutating-specs.txt — one file, readable by humans
 * and by the shell that assembles a read-only bundle (its header documents
 * the shared-state axes and the measurement command). This test is the
 * gate half: it parses that file's uncommented lines as spec filenames and
 * drifts red when the file and the specs disagree, in either direction.
 *
 * Five mechanical markers, one axis each (a spec hits the census if any
 * fires):
 *   1. `(page.)request.(post|put|delete|patch)(` — API mutation via the
 *      Playwright request context. `searchParams.delete(` cannot match:
 *      the receiver must be `request`.
 *   2. `method: 'POST'|'PUT'|'DELETE'|'PATCH'` — a fetch inside
 *      page.evaluate mutating the API.
 *   3. `.objectStore(...).put(` — a page-side cache write (IndexedDB).
 *   4. `drainTerminalSessions(` — helper-mediated session deletion.
 *   5. `e2eHomeDir(` — a spec touching the shared home directly: the CLI
 *      spawns and ui-focus.json writes this catches (jql, keys-focus,
 *      nav-issue-list) mutate no API surface, so the first four markers
 *      are blind to them — they still share one home with every worker.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const CENSUS_FILE = join(E2E_DIR, 'mutating-specs.txt')

const API_MUTATION = /(?:^|[^\w.])(?:page\.)?request\.(?:post|put|delete|patch)\(/
const EVAL_FETCH_MUTATION = /method:\s*'(?:POST|PUT|DELETE|PATCH)'/
const PAGE_STORAGE_WRITE = /\.objectStore\([^)]*\)\.put\(/
const HELPER_MUTATION = /\bdrainTerminalSessions\(/
const HOME_TOUCH = /\be2eHomeDir\(/

/** The census, parsed from e2e/mutating-specs.txt. A spec here mutates shared suite state. */
function mutatingSpecs(): string[] {
  return readFileSync(CENSUS_FILE, 'utf8')
    .split('\n')
    .map((line) => line.replace(/#.*$/, '').trim())
    .filter((line) => line !== '')
}

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
  if (HOME_TOUCH.test(src)) return 'shared-home touch'
  return null
}

test('every mutating spec is censused, and every censused spec still mutates', () => {
  const MUTATING = mutatingSpecs()
  const drift: string[] = []
  for (const f of specFiles()) {
    const src = readFileSync(join(E2E_DIR, f), 'utf8')
    const hit = mutatingHit(src)
    const listed = MUTATING.includes(f)
    if (hit && !listed) drift.push(`${f}: ${hit} but not censused`)
    if (!hit && listed) drift.push(`${f}: censused but no mutation marker fires`)
  }
  const ghosts = MUTATING.filter((f) => !specFiles().includes(f))
  for (const g of ghosts) drift.push(`${g}: censused but no such spec file`)
  // A census line that is not exactly a spec filename (a typo, a stray word)
  // would silently pass as a ghost of nothing — the ghosts check above is
  // what makes the parse honest.
  expect(drift, 'mutation census drift — update e2e/mutating-specs.txt (GDK-1758)').toEqual([])
  // The census must stay non-trivial: an empty list would pass the loop
  // above only if no spec mutated, which today is not the world we serve.
  expect(MUTATING.length, 'the mutating census went empty — workers>1 maths changed').toBeGreaterThan(
    0,
  )
})

test('the census file and the workers>1 design agree on the axes they name', () => {
  // The census header names the shared-state axes. If the design section is
  // rewritten, keep the axes and this check in step — the point of the gate
  // is that the list IS the design input.
  const src = readFileSync(CENSUS_FILE, 'utf8')
  for (const axis of [
    'the served app instance',
    'the mirror DB',
    'terminal PTY pool',
    "the shared home's loose files",
    'IndexedDB cache',
  ]) {
    expect(src, `the shared-state axis "${axis}" left the census header`).toContain(axis)
  }
})
