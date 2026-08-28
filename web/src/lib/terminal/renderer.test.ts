import { afterEach, describe, expect, test, vi } from 'vitest'
import { fromTerminalHost, isEditableTarget, keyContext, resolveGlobalKey } from '../keymap.svelte'
import { createUtf8StreamDecoder, createRenderer, fontFamily, terminalFontSize } from './renderer'
import {
  normalizeSessionDoc,
  TERMINAL_CURSOR_BLINK_FALLBACK,
  TERMINAL_SCROLLBACK_FALLBACK,
} from './session'
import {
  TERMINAL_MIN_WIDTH_PX,
  TERMINAL_SPLIT_WITH_DETAIL_MIN_PX,
  terminalIsNarrow,
} from './layout'
import { VIEWPORT_DOCKED_MIN_PX } from '../viewport-regime'
import { COMMANDS } from '../commands'

describe('UTF-8 stream decoder', () => {
  test('a character split across two write() calls renders as one glyph', () => {
    // '한' is U+D55C → ed 95 9c. The ring replay is one 256 KiB chunk that
    // can split a multibyte character at either end.
    const bytes = new TextEncoder().encode('한')
    expect(bytes.length).toBe(3)
    const dec = createUtf8StreamDecoder()
    expect(dec.push(bytes.subarray(0, 1))).toBe('')
    expect(dec.push(bytes.subarray(1))).toBe('한')
  })

  test('a complete sequence in one push is returned whole', () => {
    const dec = createUtf8StreamDecoder()
    expect(dec.push(new TextEncoder().encode('ok'))).toBe('ok')
  })

  test('string writes pass through', () => {
    const dec = createUtf8StreamDecoder()
    expect(dec.push('한')).toBe('한')
  })
})

describe('Ctrl+` toggle chord', () => {
  test('Ctrl+` toggles the terminal even in an editable field', () => {
    expect(
      resolveGlobalKey(keyContext({ key: '`', ctrlKey: true, inEditable: true })),
    ).toEqual({ type: 'toggle-terminal' })
  })

  test('Cmd+` is not stolen (macOS window-cycle)', () => {
    expect(resolveGlobalKey(keyContext({ key: '`', metaKey: true }))).toEqual({
      type: 'ignore',
    })
  })
})

describe('data-gadak-editable', () => {
  test('a host marked data-gadak-editable is an editable target', () => {
    const host = {
      tagName: 'DIV',
      isContentEditable: false,
      closest(sel: string) {
        return sel === '[data-gadak-editable]' ? this : null
      },
    }
    expect(isEditableTarget(host as unknown as EventTarget)).toBe(true)
  })

  test('an ordinary div is not editable', () => {
    const el = {
      tagName: 'DIV',
      isContentEditable: false,
      closest() {
        return null
      },
    }
    expect(isEditableTarget(el as unknown as EventTarget)).toBe(false)
  })
})

describe('defaultPrevented is read by origin, not by command', () => {
  /*
   * 2026-08-25 — GDK-864 (lead). A VT renderer preventDefault-s nearly every
   * keystroke on purpose, so the pane needs an exception to the composer's
   * "someone already spent this key" rule (GDK-462). The exception is keyed
   * on **where the key came from**, which is what these three cases pin.
   *
   * It replaced a first form keyed on the command type
   * (`cmd.type !== 'toggle-palette'`), which would also have let a
   * preventDefault-ed Cmd+K inside a *composer* open the palette — a
   * behaviour change to a surface GDK-864 has no business touching. That is
   * reasoning about the old code, not a failure this file observed: the
   * predicate below did not exist then, so there was nothing to run red.
   */
  const terminalHost = {
    tagName: 'DIV',
    isContentEditable: false,
    closest(sel: string) {
      return sel === '[data-gadak-editable]' ? this : null
    },
  } as unknown as EventTarget

  const composer = {
    tagName: 'TEXTAREA',
    isContentEditable: false,
    closest() {
      return null
    },
  } as unknown as EventTarget

  test('a key from the terminal host escapes even when defaultPrevented', () => {
    expect(fromTerminalHost(terminalHost)).toBe(true)
  })

  test('a key from a composer does not', () => {
    expect(fromTerminalHost(composer)).toBe(false)
  })

  test('no target at all does not', () => {
    expect(fromTerminalHost(null)).toBe(false)
  })
})

describe('overlay thresholds', () => {
  /*
   * 2026-08-25 — GDK-864 (lead, entry-point/layout pass). Two reasons the
   * split becomes an overlay, and neither number is invented: 899 is the
   * pane's own breakpoint, and the upper one is
   * VIEWPORT_DOCKED_MIN_PX + the pane's min-width — the width at which
   * sidebar, list, detail and terminal can no longer all stand at their
   * documented minimums. FAIL-first: before this rule, 1100px with the detail
   * panel docked kept the split, and the list was left 70px of a 390px track
   * because the pane's min-width beat the percentage cap.
   */
  test('a small viewport is an overlay whatever the detail panel is doing', () => {
    expect(terminalIsNarrow(820, false)).toBe(true)
    expect(terminalIsNarrow(820, true)).toBe(true)
  })

  test('with no docked detail panel a wide-enough viewport stays a split', () => {
    expect(terminalIsNarrow(900, false)).toBe(false)
    expect(terminalIsNarrow(1100, false)).toBe(false)
    expect(terminalIsNarrow(1440, false)).toBe(false)
  })

  test('a docked detail panel raises the floor to the four minimums', () => {
    expect(TERMINAL_SPLIT_WITH_DETAIL_MIN_PX).toBe(
      VIEWPORT_DOCKED_MIN_PX + TERMINAL_MIN_WIDTH_PX,
    )
    expect(terminalIsNarrow(TERMINAL_SPLIT_WITH_DETAIL_MIN_PX - 1, true)).toBe(true)
    expect(terminalIsNarrow(TERMINAL_SPLIT_WITH_DETAIL_MIN_PX, true)).toBe(false)
    // The 1100px case that used to leave the list 70px.
    expect(terminalIsNarrow(1100, true)).toBe(true)
  })
})

describe('the terminal has a palette door, not only a chord', () => {
  /*
   * A surface reachable only by a shortcut is reachable only by someone who
   * already knows the shortcut. ⌘K is this app's "how do I do anything", so
   * the row must exist there — carrying the chord, which is how the palette
   * teaches it.
   */
  test('COMMANDS carries a palette spec for the terminal, with its chord', () => {
    const cmd = COMMANDS.find((c) => c.id === 'toggle-terminal')
    expect(cmd?.palette?.id).toBe('a:terminal')
    expect(cmd?.palette?.kind).toBe('always')
    expect(cmd?.palette?.kbd).toBe('Ctrl+`')
  })
})

describe('the terminal size is a token, not a literal', () => {
  /*
   * 2026-08-25 — GDK-864 (lead). A terminal is personal: everyone who uses
   * one sets its size. Rather than a settings field of its own, it is a
   * dimension token on the same path as every other dimension in this app,
   * which is what makes it settable by an agent —
   * `gadak config set ui.tokens.type.terminal 15px` — with no new surface
   * (the scalar leaf path landed in GDK-853).
   * Catalog parity is pinned on the Go side (tokencheck); this pins the reader.
   */
  test('the token wins', () => {
    expect(terminalFontSize(() => '17px')).toBe(17)
    expect(terminalFontSize(() => '9px')).toBe(9)
  })

  test('a missing or unusable token falls back rather than rendering nothing', () => {
    expect(terminalFontSize(() => '')).toBe(13)
    expect(terminalFontSize(() => 'inherit')).toBe(13)
    expect(terminalFontSize(() => '0px')).toBe(13)
  })
})

describe('the terminal font stack is its own token (GDK-1043)', () => {
  /*
   * 2026-08-28 — GDK-1043. WebKit (the desktop pane and the phone, both
   * WKWebView) resolves ui-monospace to SF Mono, whose box-glyph ink
   * (15.31css) is shorter than the 16css cell xterm derives at 13px — a 1px
   * seam at every row boundary, with └┴┘ arms floating above the cell
   * bottom. Menlo joins by overshoot (15.14 ≥ 15.00) on both engines, so the
   * pane leads with it through a token of its own; --font-mono stays the
   * app-wide face (code chips, tables) where box grids never occur. The
   * stack's order is pinned as text in font-stack.test.ts — these pin the
   * reader's preference, not the stack itself.
   */
  const vars = (map: Record<string, string>) => (name: string) => map[name] ?? ''

  test('the terminal token wins over the app-wide mono', () => {
    const read = vars({
      '--font-mono-terminal': 'Menlo, monospace',
      '--font-mono': 'ui-monospace, monospace',
    })
    expect(fontFamily(read)).toBe('Menlo, monospace')
  })

  test('an empty terminal token falls back to the app-wide mono', () => {
    const read = vars({
      '--font-mono-terminal': '',
      '--font-mono': 'ui-monospace, monospace',
    })
    expect(fontFamily(read)).toBe('ui-monospace, monospace')
  })

  test('neither token set keeps the hardcoded stack (units ship no stylesheet)', () => {
    expect(fontFamily(() => '')).toBe('ui-monospace, SFMono-Regular, Menlo, Consolas, monospace')
  })
})

/** The readback slice of the xterm options the behavior seam writes. */
type TerminalBehaviorShape = { scrollback: number; cursorBlink: boolean }

describe('the pane behavior comes from the create response (GDK-896 R2)', () => {
  /*
   * 2026-08-28 — GDK-896 R2. scrollback/cursorBlink used to be literals in
   * termOptions(); now the Go config owns them (EffectiveTerminal) and the
   * create response is the one road they reach the pane on — the settings
   * dialog is deliberately not a second road (GDK-1069). These pin the two
   * halves of that wiring: the values survive the trip into the live xterm
   * options, and an older server that never sent them still renders the
   * same 5000/false the literals used to hardcode.
   *
   * xterm constructs and takes options without a DOM, so this runs the real
   * renderer — window is stubbed only because exposeTerm stores its test
   * hook there.
   */
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  test('applyBehavior reaches the live xterm options, and keeps taking later applies', async () => {
    vi.stubGlobal('window', globalThis)
    const renderer = await createRenderer()
    const term = () =>
      (globalThis as unknown as { __gadakTerm?: { options: TerminalBehaviorShape } }).__gadakTerm
    try {
      renderer.applyBehavior({ scrollback: 9000, cursorBlink: true })
      expect(term()?.options.scrollback).toBe(9000)
      expect(term()?.options.cursorBlink).toBe(true)
      // Not frozen at first apply: a create response that arrives after
      // the fallback still wins.
      renderer.applyBehavior({ scrollback: 20000, cursorBlink: false })
      expect(term()?.options.scrollback).toBe(20000)
      expect(term()?.options.cursorBlink).toBe(false)
    } finally {
      renderer.dispose()
    }
  })

  test('a response without behavior fields falls back to the server defaults', () => {
    const doc = normalizeSessionDoc({ id: 's1', cols: 90, rows: 30 })
    expect(doc.scrollback).toBe(TERMINAL_SCROLLBACK_FALLBACK)
    expect(doc.cursorBlink).toBe(TERMINAL_CURSOR_BLINK_FALLBACK)
    expect(TERMINAL_SCROLLBACK_FALLBACK).toBe(5000)
  })

  test('a response carrying behavior passes the values through', () => {
    const doc = normalizeSessionDoc({ id: 's1', cols: 90, rows: 30, scrollback: 9000, cursorBlink: true })
    expect(doc).toMatchObject({ id: 's1', cols: 90, rows: 30, scrollback: 9000, cursorBlink: true })
  })

  test('a zero or non-finite scrollback reads as absent, not as a budget of 0', () => {
    expect(normalizeSessionDoc({ id: 's', cols: 1, rows: 1, scrollback: 0 }).scrollback).toBe(
      TERMINAL_SCROLLBACK_FALLBACK,
    )
    expect(
      normalizeSessionDoc({ id: 's', cols: 1, rows: 1, scrollback: Number.NaN }).scrollback,
    ).toBe(TERMINAL_SCROLLBACK_FALLBACK)
  })
})
