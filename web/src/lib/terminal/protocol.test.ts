import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

import { coerceDroppedReason, DROPPED_REASONS, TERMINAL_CHROME_VARS } from './protocol'

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

/*
 * GDK-1109: TERMINAL_CHROME_VARS is the single owner of the chrome variable
 * *names* because two renderers read them — this one and the phone's, which
 * imports this module directly. Before the list had an owner, each renderer
 * spelled its own copy, so a rename in app.css (or in the web renderer) left
 * the phone's chrome silently on fallbacks. Two pins close both directions:
 * the list against app.css, and both renderers against the list.
 */
describe('GDK-1109 chrome-variable parity (protocol ⟷ app.css ⟷ renderers)', () => {
  const HERE = dirname(fileURLToPath(import.meta.url))

  // Every custom property app.css declares, @theme and the theme overrides
  // alike (same names, different values). Anchored to a declaration shape,
  // not a block structure a CSS refactor could move.
  const declaredTokens = (): Set<string> => {
    const css = readFileSync(resolve(HERE, '../../app.css'), 'utf8')
    const out = new Set<string>()
    const re = /^[ \t]*(--[a-z0-9-]+)\s*:/gm
    for (let m = re.exec(css); m !== null; m = re.exec(css)) out.add(m[1])
    return out
  }

  test('the scrape finds the custom properties (guards the regex, not the data)', () => {
    expect(declaredTokens().size).toBeGreaterThanOrEqual(20)
  })

  test('every chrome variable the list names is declared in app.css', () => {
    const declared = declaredTokens()
    for (const [slot, name] of Object.entries(TERMINAL_CHROME_VARS)) {
      expect(
        declared.has(name),
        `${slot} reads ${name}, which app.css no longer declares — update TERMINAL_CHROME_VARS (protocol.ts is the list's one owner)`,
      ).toBe(true)
    }
  })

  test('the web renderer reads the names through the list, not a re-spelled copy', () => {
    const src = readFileSync(resolve(HERE, 'renderer.ts'), 'utf8')
    for (const slot of Object.keys(TERMINAL_CHROME_VARS)) {
      expect(src, `renderer.ts must read its ${slot} via TERMINAL_CHROME_VARS`).toContain(
        `TERMINAL_CHROME_VARS.${slot}`,
      )
    }
    // Chrome names re-spelled as literals are the drift this gate exists to
    // catch. Font and size tokens (--font-*, --text-*) are not chrome and
    // stay as literals.
    expect(src.includes("'--color-")).toBe(false)
  })
})
