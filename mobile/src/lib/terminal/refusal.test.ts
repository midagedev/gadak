import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { en } from '../../../../web/src/lib/i18n/catalog'
import { ApiError, errorMessage } from '../api'
import { classifyRefusal, REFUSAL_KEYS } from './refusal'

/*
 * Recurrence layer for GDK-1121: each "no" the server can say keeps its own
 * sentence, measured per status+code. The class this closes has one
 * measured incident (GDK-1120): every POST from a packaged phone answered
 * 403 forbidden_origin and the screen showed the network line, because the
 * shared classifier folds every 401/403 into 'forbidden' and (worse) the
 * pre-GDK-1121 errorMessage() had no forbidden_origin case at all — it fell
 * to the default refusal line. The server leaves one guard log line, so
 * these sentences are the only user-visible diagnostic.
 *
 * FAIL-first evidence: at HEAD~ (git show HEAD:mobile/src/lib/api.ts) the
 * switch had cases for pairing_rejected / forbidden_host / scope_rejected
 * and none for forbidden_origin; Shell.svelte rendered
 * t(UNAVAILABLE_KEYS[status.cause]) with no refusal field — both measured
 * red the moment this file's assertions are stated.
 */

const HERE = dirname(fileURLToPath(import.meta.url))

describe('GDK-1121 classifyRefusal: (status, code) → kind', () => {
  it('403 forbidden_origin is the origin guard, not a generic refusal', () => {
    expect(classifyRefusal(403, 'forbidden_origin')).toBe('origin')
  })

  it('401 pairing_rejected is the pairing', () => {
    expect(classifyRefusal(401, 'pairing_rejected')).toBe('pairing')
  })

  it('403 forbidden_host and scope_rejected are the scope', () => {
    expect(classifyRefusal(403, 'forbidden_host')).toBe('scope')
    expect(classifyRefusal(403, 'scope_rejected')).toBe('scope')
  })

  it('any other 401/403 — no code, or an unknown one — is the generic refusal', () => {
    expect(classifyRefusal(403, null)).toBe('other')
    expect(classifyRefusal(401, null)).toBe('other')
    expect(classifyRefusal(403, 'not_the_guard')).toBe('other')
  })

  it('non-refusal statuses are none of this module\'s business', () => {
    expect(classifyRefusal(500, 'forbidden_origin')).toBeNull()
    expect(classifyRefusal(404, 'not_found')).toBeNull()
    expect(classifyRefusal(0, 'network')).toBeNull()
    expect(classifyRefusal(409, 'credential_required')).toBeNull()
  })
})

describe('GDK-1121 each refusal kind owns one distinct catalog sentence', () => {
  it('every REFUSAL_KEYS value exists in the catalog', () => {
    for (const key of Object.values(REFUSAL_KEYS)) {
      expect(en[key], key).toBeTruthy()
    }
  })

  it('the four sentences are pairwise distinct', () => {
    const values = Object.values(REFUSAL_KEYS).map((k) => en[k])
    expect(new Set(values).size).toBe(values.length)
  })
})

describe('GDK-1121 errorMessage() renders the owner’s sentence per status', () => {
  it('403 forbidden_origin names the origin guard, never the generic line', () => {
    expect(errorMessage(new ApiError('forbidden_origin', 403))).toBe(en['terminal.refusal.origin'])
    expect(en['terminal.refusal.origin']).not.toBe(en['terminal.refusal.other'])
  })

  it('401 pairing_rejected', () => {
    expect(errorMessage(new ApiError('pairing_rejected', 401))).toBe(en['terminal.refusal.pairing'])
  })

  it('403 forbidden_host', () => {
    expect(errorMessage(new ApiError('forbidden_host', 403))).toBe(en['terminal.refusal.scope'])
  })

  it('403 scope_rejected', () => {
    expect(errorMessage(new ApiError('scope_rejected', 403))).toBe(en['terminal.refusal.scope'])
  })

  it('403 with no code stays the generic refusal sentence', () => {
    expect(errorMessage(new ApiError('', 403, null))).toBe(en['terminal.refusal.other'])
  })

  it('non-refusal codes keep their existing copy (spot: network)', () => {
    expect(errorMessage(new ApiError('network', 0))).toBe('Cannot reach the server.')
  })
})

describe('GDK-1121 the terminal strip renders the refusal sentence', () => {
  // Source scan, same family as gdk-908.test.ts: the strip is inside a
  // component this suite does not mount, so the contract is stated against
  // the markup that renders it.
  const shell = readFileSync(join(HERE, '../../screens/Shell.svelte'), 'utf8')

  it('onCreateFail records classifyRefusal’s verdict on the status', () => {
    const onCreate = shell.slice(shell.indexOf('function onCreateFail'), shell.indexOf('function onCreateFail') + 700)
    expect(onCreate).toContain('classifyRefusal(err.status, err.code)')
    expect(onCreate).toMatch(/\{ kind: 'unavailable', cause, \.\.\.\(refusal/)
  })

  it('the unavailable pane renders the refusal key ahead of the cause line', () => {
    const strip = shell.slice(shell.indexOf("status.kind === 'exited'"))
    expect(strip).toContain('{#if status.refusal}')
    expect(strip.indexOf('{#if status.refusal}')).toBeLessThan(
      strip.indexOf("status.cause === 'failed'"),
    )
    expect(strip).toContain('t(REFUSAL_KEYS[status.refusal])')
  })
})
