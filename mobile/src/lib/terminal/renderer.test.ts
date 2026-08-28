import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { readBufferOffset, readBufferType, readMouseTrackingMode } from './renderer'

/*
 * GDK-899 — the reads that feed the scroll router's context. They are pure
 * filters because the contract is "a term lacking the field reads the safe
 * default, never throws": `window.__gadakTerm` is typed TermHook precisely so
 * a partial double can sit there (a double that only renders), and the frozen
 * module's routing must not wobble on one. vitest runs this tree in the node
 * environment (vite.config.ts `test.environment`), so a live createRenderer
 * is out of reach by design — the guards are the part that needed pinning,
 * and the wiring is pinned as a source contract below.
 */

const here = dirname(fileURLToPath(import.meta.url))
const rendererSrc = readFileSync(join(here, 'renderer.ts'), 'utf8')

describe('GDK-899 — readMouseTrackingMode', () => {
  it("passes xterm's four tracking modes through unchanged", () => {
    for (const mode of ['x10', 'vt200', 'drag', 'any'] as const) {
      expect(readMouseTrackingMode(mode)).toBe(mode)
    }
  })

  it("reads 'none' for anything else modes could carry — a double must not throw", () => {
    expect(readMouseTrackingMode('none')).toBe('none')
    expect(readMouseTrackingMode(undefined)).toBe('none')
    // Not hypothetical strings: an unparsed double, a future xterm rename.
    expect(readMouseTrackingMode('vt200-px')).toBe('none')
    expect(readMouseTrackingMode(0)).toBe('none')
  })
})

describe('GDK-899 — readBufferType', () => {
  it("is alternate only when the active buffer says so", () => {
    expect(readBufferType('alternate')).toBe('alternate')
    expect(readBufferType('normal')).toBe('normal')
    expect(readBufferType(undefined)).toBe('normal')
  })
})

describe('GDK-899 — readBufferOffset', () => {
  it('defaults a missing or non-finite offset to 0, never NaN into the thumb', () => {
    expect(readBufferOffset(undefined)).toBe(0)
    expect(readBufferOffset(Number.NaN)).toBe(0)
    expect(readBufferOffset(Number.POSITIVE_INFINITY)).toBe(0)
    expect(readBufferOffset(17)).toBe(17)
    expect(readBufferOffset(0)).toBe(0)
  })
})

describe('GDK-899 — renderer wiring (source contract)', () => {
  it('every new read is optional-chained through the pure filters', () => {
    expect(rendererSrc).toContain('readMouseTrackingMode(term.modes?.mouseTrackingMode)')
    expect(rendererSrc).toContain('readBufferType(term.buffer?.active?.type)')
    expect(rendererSrc).toContain('readBufferOffset(term.buffer?.active?.viewportY)')
    expect(rendererSrc).toContain('readBufferOffset(term.buffer?.active?.baseY)')
  })

  it('scrollLines guards the call the same way', () => {
    expect(rendererSrc).toContain('term.scrollLines?.(n)')
  })
})
