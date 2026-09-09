import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-1735: every spec takes `test` from e2e/helpers, never from Playwright.
 *
 * The shared `test` carries one teardown — `page.unrouteAll({ behavior:
 * 'ignoreErrors' })` — and it exists because a route handler can outlive the
 * test that registered it. A spec that rewrites a response calls
 * `route.fetch()` inside its handler; the app keeps fetching after the last
 * assertion (a negative assertion resolves the instant it is made), so a
 * request still in flight reaches the handler during teardown and throws
 * "Test ended", failing a test that had already passed. It cost two CI reds
 * in two days, and the first was misread as a dead browser because the block
 * that reaches the log leads with "Response has been disposed".
 *
 * This lint is the recurrence layer, and it is the only one available: the
 * race is load-dependent, so it does not reproduce on a laptop and no
 * browser test can pin it. What can be pinned is the import — a new spec
 * that reaches for Playwright's own `test` opts out of the teardown without
 * meaning to, and that is the shape the next occurrence would take.
 *
 * Contract ↔ assertion table:
 *   S1 no spec imports `test` from '@playwright/test'
 *      → 'S1 every spec takes test from the shared helpers'
 *   S2 the shared test actually carries the teardown — a helpers file that
 *      re-exported Playwright's test unchanged would pass S1 and protect
 *      nothing → 'S2 the shared test drops routes at teardown'
 *
 * Scope is every Playwright spec in the repo, CI-run or not: the demo and
 * perf sets are the ones a person runs by hand, where a hang is worse.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = join(HERE, '..')

function specsUnder(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...specsUnder(p))
    else if (name.endsWith('.spec.ts')) out.push(p)
  }
  return out
}

const SPECS = [...specsUnder(join(ROOT, 'e2e')), ...specsUnder(join(ROOT, 'mobile', 'e2e'))]

test('S1 every spec takes test from the shared helpers', () => {
  expect(SPECS.length, 'no specs found — this lint would pass vacuously').toBeGreaterThan(50)
  const offenders: string[] = []
  for (const file of SPECS) {
    const src = readFileSync(file, 'utf8')
    for (const m of src.matchAll(/^import \{([^}]*)\} from '@playwright\/test'$/gm)) {
      // Types are fine to take from Playwright — `type Page`, `type Route`.
      // Only the runtime `test` binding carries fixtures.
      const names = m[1].split(',').map((n) => n.trim())
      if (names.some((n) => n === 'test' || n.startsWith('test as'))) {
        offenders.push(relative(ROOT, file))
      }
    }
  }
  expect(
    offenders,
    `these specs opt out of the shared teardown (import { test } from the suite's helpers instead):\n  ${offenders.join('\n  ')}`,
  ).toEqual([])
})

test('S2 the shared test drops routes at teardown', () => {
  // Both halves, because either one alone is a lint that guards nothing: the
  // extend makes it a fixture, the unrouteAll is what the fixture is for.
  const helpers = readFileSync(join(ROOT, 'e2e', 'helpers.ts'), 'utf8')
  expect(helpers).toMatch(/export const test = base\.extend/)
  expect(helpers).toContain("page.unrouteAll({ behavior: 'ignoreErrors' })")
  const mobile = join(ROOT, 'mobile', 'e2e')
  // The phone suite has its own helpers; assert the same shape there only if
  // its specs take `test` from it (S1 is what makes that true or not).
  try {
    const phone = readFileSync(join(mobile, 'helpers.ts'), 'utf8')
    if (/export const test = base\.extend/.test(phone)) {
      expect(phone).toContain("unrouteAll({ behavior: 'ignoreErrors' })")
    }
  } catch {
    /* no phone helpers file — nothing to assert */
  }
})
