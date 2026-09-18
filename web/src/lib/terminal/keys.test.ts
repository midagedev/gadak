import { describe, expect, it } from 'vitest'
import {
  bytesForBarKey,
  bytesForText,
  type BarKey,
  type CursorKeyMode,
  type StickyMods,
} from './keys'

const NONE: StickyMods = { ctrl: false, alt: false }
const CTRL: StickyMods = { ctrl: true, alt: false }
const ALT: StickyMods = { ctrl: false, alt: true }
const BOTH: StickyMods = { ctrl: true, alt: true }

function bytesOf(got: Uint8Array): number[] {
  return [...got]
}

/** `esc('[1;5A')` — the readable spelling of an escape sequence's bytes. */
function esc(rest: string): number[] {
  return [0x1b, ...[...rest].map((c) => c.charCodeAt(0))]
}

describe('bytesForBarKey', () => {
  // Record<BarKey, …> is the "every BarKey" gate: a new union member that
  // is not in this table fails the typecheck.
  const expectNone = {
    esc: [0x1b],
    tab: [0x09],
    ctrl: [],
    alt: [],
    // GDK-953: the panic exit sends nothing, under any modifier mix.
    clear: [],
    up: [0x1b, 0x5b, 0x41],
    down: [0x1b, 0x5b, 0x42],
    right: [0x1b, 0x5b, 0x43],
    left: [0x1b, 0x5b, 0x44],
    home: [0x1b, 0x5b, 0x48],
    end: [0x1b, 0x5b, 0x46],
    pipe: [0x7c],
    slash: [0x2f],
    dash: [0x2d],
    tilde: [0x7e],
  } satisfies Record<BarKey, readonly number[]>

  for (const key of Object.keys(expectNone) as BarKey[]) {
    it(`${key}`, () => {
      const got = bytesForBarKey(key, NONE, 'normal')
      expect(got).toBeInstanceOf(Uint8Array)
      expect(bytesOf(got)).toEqual([...expectNone[key]])
    })
  }

  it('literal keys match bytesForText of the same ASCII', () => {
    expect(bytesOf(bytesForBarKey('pipe', NONE, 'normal'))).toEqual(bytesOf(bytesForText('|', NONE)))
    expect(bytesOf(bytesForBarKey('slash', NONE, 'normal'))).toEqual(
      bytesOf(bytesForText('/', NONE)),
    )
    expect(bytesOf(bytesForBarKey('dash', NONE, 'normal'))).toEqual(bytesOf(bytesForText('-', NONE)))
    expect(bytesOf(bytesForBarKey('tilde', NONE, 'normal'))).toEqual(
      bytesOf(bytesForText('~', NONE)),
    )
  })

  /*
   * Rewritten 2026-08-27 for GDK-899. The assertion that stood here pinned
   * the defect: it said Alt-Up is `ESC ESC [ A` and Ctrl-Up is a plain
   * `ESC [ A` — i.e. that the bar drops Ctrl entirely. Both were the
   * encoder's behaviour, neither is what the terminal on the other end is
   * told by a hardware keyboard. The replacement is not a preference; it
   * is the shipped xterm.js evaluator's own table (see cursorKeyBytes for
   * the citation), and it failed on the pre-fix source.
   */
  it('a modifier makes a cursor key the CSI parameter form, in both modes', () => {
    expect(bytesOf(bytesForBarKey('up', ALT, 'normal'))).toEqual(esc('[1;3A'))
    expect(bytesOf(bytesForBarKey('up', CTRL, 'normal'))).toEqual(esc('[1;5A'))
    expect(bytesOf(bytesForBarKey('up', BOTH, 'normal'))).toEqual(esc('[1;7A'))
    // DECCKM does not change the modified form — the mask branch wins.
    expect(bytesOf(bytesForBarKey('up', CTRL, 'application'))).toEqual(esc('[1;5A'))
  })

  it('Ctrl on a literal with no control byte sends the ASCII unchanged', () => {
    expect(bytesOf(bytesForBarKey('pipe', CTRL, 'normal'))).toEqual([0x7c])
    expect(bytesOf(bytesForBarKey('tilde', CTRL, 'normal'))).toEqual([0x7e])
  })

  /*
   * DECCKM parity, GDK-899. Every row below is what
   * `node_modules/@xterm/xterm/lib/xterm.js` `evaluateKeyboardEvent`
   * produces for the same physical key — keyCodes 38/40/39/37/36/35, the
   * `t ? ESC+"OA" : ESC+"[A"` branch. The bar bypasses that evaluator by
   * writing to the socket itself, so parity has to be asserted here or it
   * is asserted nowhere: the failure is invisible on screen and shows up
   * as an arrow that "does nothing" inside a full-screen TUI.
   */
  const cursorParity: Record<string, { normal: string; application: string }> = {
    up: { normal: '[A', application: 'OA' },
    down: { normal: '[B', application: 'OB' },
    right: { normal: '[C', application: 'OC' },
    left: { normal: '[D', application: 'OD' },
    home: { normal: '[H', application: 'OH' },
    end: { normal: '[F', application: 'OF' },
  }

  for (const [key, want] of Object.entries(cursorParity)) {
    it(`${key} follows DECCKM with no modifier`, () => {
      for (const mode of ['normal', 'application'] as CursorKeyMode[]) {
        expect(bytesOf(bytesForBarKey(key as BarKey, NONE, mode))).toEqual(esc(want[mode]))
      }
    })
  }

  it('a non-cursor key ignores DECCKM entirely', () => {
    for (const key of ['esc', 'tab', 'pipe', 'clear'] as BarKey[]) {
      expect(bytesOf(bytesForBarKey(key, NONE, 'application'))).toEqual(
        bytesOf(bytesForBarKey(key, NONE, 'normal')),
      )
    }
  })
})

describe('bytesForText', () => {
  it('Ctrl+a..z at both cases', () => {
    for (let i = 0; i < 26; i++) {
      const lower = String.fromCharCode(97 + i)
      const upper = String.fromCharCode(65 + i)
      const want = i + 1
      expect(bytesOf(bytesForText(lower, CTRL)), lower).toEqual([want])
      expect(bytesOf(bytesForText(upper, CTRL)), upper).toEqual([want])
    }
  })

  const punct = [
    ['[', 0x1b],
    ['\\', 0x1c],
    [']', 0x1d],
    ['^', 0x1e],
    ['_', 0x1f],
    ['?', 0x7f],
    [' ', 0x00],
  ] as const

  for (const [ch, want] of punct) {
    it(`Ctrl+${JSON.stringify(ch)} → 0x${want.toString(16)}`, () => {
      expect(bytesOf(bytesForText(ch, CTRL))).toEqual([want])
    })
  }

  it('Alt prefixes ESC', () => {
    expect(bytesOf(bytesForText('c', ALT))).toEqual([0x1b, 0x63])
    expect(bytesOf(bytesForText('C', ALT))).toEqual([0x1b, 0x43])
  })

  it('Ctrl+Alt together: ESC then the control byte', () => {
    expect(bytesOf(bytesForText('c', BOTH))).toEqual([0x1b, 0x03])
    expect(bytesOf(bytesForText('C', BOTH))).toEqual([0x1b, 0x03])
    expect(bytesOf(bytesForText('a', BOTH))).toEqual([0x1b, 0x01])
  })

  it('Korean character round-trips to three UTF-8 bytes, not charCodeAt', () => {
    const han = '한'
    const got = bytesForText(han, NONE)
    expect(got).toBeInstanceOf(Uint8Array)
    expect(got).toHaveLength(3)
    expect(bytesOf(got)).toEqual([0xed, 0x95, 0x9c])
    expect(bytesOf(got)).toEqual([...new TextEncoder().encode(han)])
    // charCodeAt would be 0xd55c — a value that does not fit in one byte
    // and is not any of the three UTF-8 bytes.
    expect(got[0]).not.toBe(han.charCodeAt(0) & 0xff)
  })

  it('Ctrl with a character that has no control byte sends it unchanged', () => {
    expect(bytesOf(bytesForText('1', CTRL))).toEqual([0x31])
    expect(bytesOf(bytesForText('|', CTRL))).toEqual([0x7c])
    expect(bytesOf(bytesForText('@', CTRL))).toEqual([0x40])
    expect(bytesOf(bytesForText('한', CTRL))).toEqual([0xed, 0x95, 0x9c])
  })

  it('plain Latin is the UTF-8 of the character', () => {
    expect(bytesOf(bytesForText('c', NONE))).toEqual([0x63])
    expect(bytesOf(bytesForText('ab', NONE))).toEqual([0x61, 0x62])
  })
})


