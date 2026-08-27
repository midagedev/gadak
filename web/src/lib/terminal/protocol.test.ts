import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

import { coerceDroppedReason, DROPPED_REASONS } from './protocol'

// GDK-932: the wire vocabulary is the serve's own (internal/term/session.go),
// and protocol.ts promises "neither side re-spells them." An unknown reason is
// coerced at runtime, so a Go rename or addition that TS never followed would
// silently degrade to 'closed' instead of failing anything. This pins the two
// sets against each other, both directions, so the divergence is caught here.
describe('GDK-932 dropped-reason parity (Go ⟷ TS)', () => {
  // The Go source is the source of truth; scrape its Reason* string literals.
  const goReasons = (): Set<string> => {
    const src = readFileSync(
      resolve(dirname(fileURLToPath(import.meta.url)), '../../../../internal/term/session.go'),
      'utf8',
    )
    const out = new Set<string>()
    // Matches:  ReasonSlow = "slow_client"
    const re = /^\s*Reason\w+\s*=\s*"([^"]+)"/gm
    for (let m = re.exec(src); m !== null; m = re.exec(src)) out.add(m[1])
    return out
  }

  test('the scrape finds the constants (guards the regex, not just the data)', () => {
    // If the constant style in session.go ever changes shape, this fails loudly
    // rather than letting an empty set pass the parity checks below.
    expect(goReasons().size).toBeGreaterThanOrEqual(5)
  })

  test('every Go reason is a known TS DroppedReason', () => {
    for (const r of goReasons()) {
      expect(DROPPED_REASONS.has(r), `Go reason ${r} missing from protocol.ts`).toBe(true)
      // And it survives coercion as itself, not degraded to 'closed'.
      expect(coerceDroppedReason(r)).toBe(r)
    }
  })

  test('every TS DroppedReason is a Go Reason constant', () => {
    const go = goReasons()
    for (const r of DROPPED_REASONS) {
      expect(go.has(r), `TS reason ${r} has no Go Reason* constant`).toBe(true)
    }
  })

  test('an unknown reason coerces to closed, not through', () => {
    expect(coerceDroppedReason('reboot')).toBe('closed')
    expect(coerceDroppedReason(undefined)).toBe('closed')
    expect(coerceDroppedReason(42)).toBe('closed')
  })
})
