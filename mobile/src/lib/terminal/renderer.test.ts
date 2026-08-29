import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { readBufferOffset, readBufferType, readMouseTrackingMode } from './renderer'
import { TERMINAL_CHROME_VARS } from '../../../../web/src/lib/terminal/protocol'

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

/*
 * GDK-1109 — the chrome variable names have one owner,
 * web/src/lib/terminal/protocol.ts TERMINAL_CHROME_VARS. The phone used to
 * spell its own copy of the list, so a web rename dropped the phone's chrome
 * to fallbacks with nothing red. The list is pinned against app.css on the
 * web side (protocol.test.ts); this pin is the phone's half — this renderer
 * reads the names through the shared list and never re-spells one.
 */
describe('GDK-1109 — chrome variable names come from the shared protocol list', () => {
  it('every chrome slot is read through TERMINAL_CHROME_VARS', () => {
    for (const [slot, name] of Object.entries(TERMINAL_CHROME_VARS)) {
      expect(rendererSrc, `${slot} must be read via TERMINAL_CHROME_VARS (${name})`).toContain(
        `TERMINAL_CHROME_VARS.${slot}`,
      )
    }
  })

  it('no chrome name is re-spelled as a literal in this renderer', () => {
    // A '--color-*' literal here is the copy drift this gate exists to
    // catch. Font and size tokens (--font-mono, --text-terminal) are not
    // chrome and stay as literals.
    expect(rendererSrc.includes("'--color-")).toBe(false)
  })
})

/*
 * GDK-900 — the real iOS zoom contract, and why the terminal grid is free of
 * it.
 *
 * Safari zooms the page when a focused form control's font-size is under
 * 16px. The token comment used to read that as a tax on the grid: shrink the
 * glyphs for more columns, and iOS zooms on the xterm helper textarea. It
 * does not, because that textarea is never focused here — the renderer runs
 * `disableStdin: true` (display only; the PTY echo paints), and its `focus()`
 * has no caller anywhere in this app. Every focus in the shell goes to the
 * app's own IME field, which is 16px by token.
 *
 * So the invariant worth pinning is not the grid size — it is those two
 * facts. While they hold, `--text-terminal` is a reading decision (measured
 * on an iPhone 17 Pro simulator: 16px → 39 columns, 13px → 48). Break either
 * and the sub-16px sink joins the focus path, which is when the zoom the old
 * comment feared becomes real.
 */
describe('GDK-900 — nothing focusable in the shell is under the iOS zoom floor', () => {
  const IOS_ZOOM_FLOOR_PX = 16

  it('the terminal is display-only, so its sub-16px sink cannot be focused', () => {
    expect(rendererSrc).toContain('disableStdin: true')
    // Wiring term.focus() to anything is the change this test exists to
    // catch: it would put xterm's helper textarea — sized from the grid
    // token, below the floor — one tap away from the keyboard.
    const shellSrc = readFileSync(join(here, '..', '..', 'screens', 'Shell.svelte'), 'utf8')
    expect(shellSrc).not.toMatch(/\b(renderer|term)\??\.focus\(\)/)
  })

  it('the field that does take focus is at or above the floor', () => {
    const css = readFileSync(join(here, '..', '..', 'app.css'), 'utf8')
    const body = Number.parseFloat(/--text-body:\s*([\d.]+)px/.exec(css)?.[1] ?? 'NaN')
    expect(body, '--text-body backs the IME field (.ime in Shell.svelte)').toBeGreaterThanOrEqual(
      IOS_ZOOM_FLOOR_PX,
    )
  })
})
