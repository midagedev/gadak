import { describe, expect, test } from 'vitest'
import { fromTerminalHost, isEditableTarget, keyContext, resolveGlobalKey } from '../keymap.svelte'
import {
  createUtf8StreamDecoder,
  RENDERER_STORAGE_KEY,
  resolveRendererKind,
} from './renderer'

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
