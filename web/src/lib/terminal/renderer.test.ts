import { describe, expect, test } from 'vitest'
import { fromTerminalHost, isEditableTarget, keyContext, resolveGlobalKey } from '../keymap.svelte'
import {
  createUtf8StreamDecoder,
  RENDERER_STORAGE_KEY,
  resolveRendererKind,
  terminalFontSize,
} from './renderer'
import {
  TERMINAL_MIN_WIDTH_PX,
  TERMINAL_SPLIT_WITH_DETAIL_MIN_PX,
  terminalIsNarrow,
} from './layout'
import { VIEWPORT_DOCKED_MIN_PX } from '../viewport-regime'
import { COMMANDS } from '../commands'

describe('resolveRendererKind owner', () => {
  test('query beats localStorage beats default', () => {
    const storage = {
      store: new Map<string, string>([[RENDERER_STORAGE_KEY, 'xterm']]),
      getItem(key: string) {
        return this.store.get(key) ?? null
      },
    }
    expect(resolveRendererKind({ search: '', storage })).toBe('xterm')
    expect(resolveRendererKind({ search: '?term=ghostty', storage })).toBe('ghostty')
    expect(resolveRendererKind({ search: '?term=xterm', storage })).toBe('xterm')
    expect(resolveRendererKind({ search: '', storage: { getItem: () => null } })).toBe('ghostty')
    expect(resolveRendererKind({ search: '?foo=1', storage: { getItem: () => null } })).toBe(
      'ghostty',
    )
  })

  test('unknown query values fall through to storage then default', () => {
    expect(
      resolveRendererKind({
        search: '?term=kitty',
        storage: { getItem: () => 'xterm' },
      }),
    ).toBe('xterm')
    expect(
      resolveRendererKind({
        search: '?term=kitty',
        storage: { getItem: () => 'nope' },
      }),
    ).toBe('ghostty')
  })
})

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
   * `gadak config set ui.tokens.type.terminal 15px` — with no new surface.
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
