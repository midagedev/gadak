import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { fontFamily, readBufferOffset, readBufferType, readMouseTrackingMode } from './renderer'
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

/*
 * GDK-1131 — the phone terminal reads the terminal font token.
 *
 * GDK-1043 split --font-mono-terminal off from --font-mono on the web for a
 * measured reason: WebKit resolves ui-monospace to SF Mono, whose box-glyph
 * ink (15.31css at 13px) undershoots the 16css cell xterm derives, leaving a
 * 1px seam at every row boundary; Menlo joins by overshoot on both engines.
 * --font-mono still leads with ui-monospace, on purpose — it is the app-wide
 * face, where box grids never occur.
 *
 * The phone's renderer kept reading --font-mono, so the surface where box
 * drawing actually matters — a full-screen TUI on a 48-column phone grid —
 * was the one surface still riding the seam, and nothing was red about it.
 * The token is imported into this bundle already (mobile/src/app.css line 13
 * imports web/src/app.css through the same Tailwind pipeline), so this was
 * only ever a missing read.
 *
 * Pinned as an injectable reader, the same shape as terminalFontSize: the
 * unit project runs in node with no stylesheet, so the token cannot be
 * measured here — but which token is asked for, in which order, can.
 */
describe('GDK-1131 — the terminal font stack comes from --font-mono-terminal', () => {
  it('prefers the terminal token over the app-wide mono face', () => {
    const read = (name: string): string =>
      name === '--font-mono-terminal'
        ? 'Menlo, ui-monospace, monospace'
        : 'ui-monospace, SFMono-Regular, monospace'
    expect(fontFamily(read)).toBe('Menlo, ui-monospace, monospace')
  })

  it('falls back to --font-mono when the terminal token is unset', () => {
    // An older stylesheet, or a build that has not picked up app.css yet.
    const read = (name: string): string =>
      name === '--font-mono-terminal' ? '' : 'ui-monospace, SFMono-Regular, monospace'
    expect(fontFamily(read)).toBe('ui-monospace, SFMono-Regular, monospace')
  })

  it('falls back to a literal stack when neither token resolves', () => {
    const stack = fontFamily(() => '')
    expect(stack).not.toBe('')
    expect(stack).toContain('monospace')
  })

  it('the live renderer asks for the font through this reader, not a private literal', () => {
    // The construction site is what shipped the defect: `fontFamily()` used
    // to read --font-mono directly and hand xterm a hardcoded SF Mono stack
    // beneath it.
    //
    // GDK-1597 wrapped the call rather than replaced it — the stack still
    // comes from this reader, and installCjkMetricFaces only prepends the
    // advance-corrected CJK families to whatever it returned. The assertion
    // keeps its subject (the reader supplies the stack) and follows the
    // shape.
    expect(rendererSrc).toContain('installCjkMetricFaces({ stack: fontFamily() })')
    expect(rendererSrc).toContain("read('--font-mono-terminal')")
  })

  it('shares the CJK cell fit with the web renderer instead of copying it', () => {
    // GDK-1597: xterm pads a CJK cell with letterSpacing rather than
    // scaling the glyph, and the correction is a ratio to whichever Latin
    // face the platform resolved. One owner for it, in the module the phone
    // already imports — a phone-local copy is how the chrome-variable
    // defect (GDK-1109) happened one directory over.
    expect(rendererSrc).toContain("from '../../../../web/src/lib/terminal/cjk-metric'")
    expect(rendererSrc).toContain('installCjkMetricFaces')
    expect(rendererSrc).not.toMatch(/@font-face|size-adjust/)
  })

  it('the token this file asks for is the one app.css actually declares', () => {
    // Guards the rename class: a web-side token rename would otherwise drop
    // the phone silently back to the fallback, which is the whole shape of
    // GDK-1109 one directory over.
    const appCss = readFileSync(join(here, '..', '..', '..', '..', 'web', 'src', 'app.css'), 'utf8')
    expect(appCss).toMatch(/--font-mono-terminal:/)
  })
})
