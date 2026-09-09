/*
 * The announce threshold for a running pass, moved from e2e/docs-ux.spec.ts
 * by the GDK-1702 cost ladder. The browser case cost 5.2 s of CI wall clock
 * (a 1.5 s young-pass window plus up to 15 s waiting for the same stubbed
 * pass to cross 4 s) to observe one number the source states outright.
 *
 * What moved here is the rule: a pass is narrated only once it has run
 * ACTIVITY_MIN_VISIBLE_MS, and an unparseable started_at errs toward
 * showing the pass (a pass the clock cannot date must not be invisible).
 * What stays in e2e is the running half — a real chip crossing from quiet
 * to narrated — which the kept document-pass case covers end to end.
 *
 * Source scan, not import: the store is a runes module (*.svelte.ts) and
 * the vitest unit project loads no svelte plugin (FeaturesTab.test.ts
 * idiom). The assertions pin the gate's shape; rewording the comment
 * around it stays green.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const STORE = join(HERE, 'issues.svelte.ts')

describe('a pass is announced only once it has run long enough to wonder about', () => {
  const store = readFileSync(STORE, 'utf8')

  test('the threshold is four seconds — past the every-minute incremental, under the backfill', () => {
    // Blocks: the threshold being tuned away in either direction. The
    // watch loop's incrementals finish in a second or two, every minute —
    // narrating those puts a blinking status in front of someone all day;
    // the six-minute backfill is what needed saying.
    expect(store).toMatch(/ACTIVITY_MIN_VISIBLE_MS = 4_000/)
  })

  test('visibility is elapsed-time gated: running AND old enough', () => {
    // Blocks: a pass announced in its first poll (no rule at all — the
    // young half the e2e case asserted), which is the `a.running && old`
    // shape: running alone must not reach the UI.
    expect(store).toMatch(/running: a\.running && old/)
    expect(store).toMatch(/Date\.now\(\) - startedMs >= ACTIVITY_MIN_VISIBLE_MS/)
  })

  test('a started_at the clock cannot parse errs toward showing the pass', () => {
    // Blocks: a malformed date making `old` false and hiding a pass that
    // is really running — Number.isFinite false ⇒ true is the fail-open
    // branch of the ternary.
    expect(store).toMatch(/Number\.isFinite\(startedMs\)/)
    expect(store).toMatch(/: true/)
  })
})
