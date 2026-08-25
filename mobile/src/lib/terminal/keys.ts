// What a key-bar press means in bytes (DESIGN.md §10.3). A phone keyboard
// has no Esc, Ctrl, or arrows, so a bar above it carries them. This module
// owns the mapping — CSI sequences, sticky Ctrl/Alt, control bytes — and
// none of the DOM. The screen is a thin adapter from taps to these calls.
// Defects here are invisible in a screenshot and obvious in a table of
// inputs, which is why the whole thing is a pure function over plain data.
//
// The control-byte table is explicit, not `code & 0x1f`: that bitmask would
// also map `@ { | } ~`, which have no control byte and must go out unchanged
// (dropping the keystroke would be worse than ignoring the modifier). Sticky
// state lives here rather than as two booleans in a component so a modifier
// press toggles and every other press — bar key or typed letter — consumes.
//
// Deliberately a subset, and not the final contract. ~/repo/naru-remote has
// a matured version of the same UX — sticky slots are idle/armed/**locked**
// with a 400 ms double-tap window, held keys repeat at 400 ms then 45 ms,
// and a control key waits behind a flush barrier so it cannot overtake
// in-flight IME composition — all measured against a real terminal
// (`docs/research/orca-mobile-input-reference.md` there). Its wire is X11
// keysyms over VNC, so the *encoder* below stays ours; the state machines do
// not, and GDK-898 lifts them out into a repo both apps run the same golden
// vectors against. Add behaviour here only if it also belongs there.

export type StickyMods = { ctrl: boolean; alt: boolean }

export type BarKey =
  | 'esc'
  | 'tab'
  | 'ctrl'
  | 'alt'
  | 'up'
  | 'down'
  | 'left'
  | 'right'
  | 'home'
  | 'end'
  | 'pipe'
  | 'slash'
  | 'dash'
  | 'tilde'

const utf8 = new TextEncoder()
const ESC = 0x1b

/** Letters a–z / A–Z plus the punctuation the spec names. Anything else is null. */
function controlByte(text: string): number | null {
  if (text.length !== 1) return null
  const code = text.charCodeAt(0)
  if (code >= 65 && code <= 90) return code - 64
  if (code >= 97 && code <= 122) return code - 96
  switch (text) {
    case '[':
      return 0x1b
    case '\\':
      return 0x1c
    case ']':
      return 0x1d
    case '^':
      return 0x1e
    case '_':
      return 0x1f
    case '?':
      return 0x7f
    case ' ':
      return 0x00
    default:
      return null
  }
}

function prefixEsc(bytes: Uint8Array): Uint8Array {
  const out = new Uint8Array(bytes.length + 1)
  out[0] = ESC
  out.set(bytes, 1)
  return out
}

function withMods(bytes: Uint8Array, mods: StickyMods, ctrlText: string | null): Uint8Array {
  let out = bytes
  if (mods.ctrl && ctrlText !== null) {
    const mapped = controlByte(ctrlText)
    if (mapped !== null) out = new Uint8Array([mapped])
  }
  if (mods.alt && out.length > 0) out = prefixEsc(out)
  return out
}

type BarSend = { kind: 'empty' } | { kind: 'text'; ch: string } | { kind: 'csi'; final: number }

// Record<BarKey, …> is the exhaustiveness gate: a new bar key that does not
// pick a send kind fails the typecheck, not a screenshot of a missing button.
const BAR: Record<BarKey, BarSend> = {
  esc: { kind: 'text', ch: '\x1b' },
  tab: { kind: 'text', ch: '\t' },
  ctrl: { kind: 'empty' },
  alt: { kind: 'empty' },
  up: { kind: 'csi', final: 0x41 },
  down: { kind: 'csi', final: 0x42 },
  right: { kind: 'csi', final: 0x43 },
  left: { kind: 'csi', final: 0x44 },
  home: { kind: 'csi', final: 0x48 },
  end: { kind: 'csi', final: 0x46 },
  pipe: { kind: 'text', ch: '|' },
  slash: { kind: 'text', ch: '/' },
  dash: { kind: 'text', ch: '-' },
  tilde: { kind: 'text', ch: '~' },
}

/** Bytes a bar key sends under the current sticky modifiers. */
export function bytesForBarKey(key: BarKey, mods: StickyMods): Uint8Array {
  const send = BAR[key]
  if (send.kind === 'empty') return new Uint8Array(0)
  if (send.kind === 'text') return bytesForText(send.ch, mods)
  /*
   * CSI (`ESC [ x`) unconditionally, which is wrong under DECCKM and is
   * tracked as GDK-899: an application that set `CSI ?1h` — every full-screen
   * TUI, which is the whole reason this bar exists — expects SS3
   * (`ESC O x`) for the cursor keys, and readline in application mode does
   * not accept the CSI form. Fixing it needs the renderer's live
   * `modes.applicationCursorKeysMode` threaded in as an argument, which is
   * the same wiring GDK-899 needs for the touch-vs-mouse decision, so both
   * land together rather than growing two parameters a week apart.
   *
   * CSI sequences have no control byte — send them unchanged, still honor Alt.
   */
  const seq = new Uint8Array([ESC, 0x5b, send.final])
  return withMods(seq, mods, null)
}

/** An ordinary typed character with the sticky modifiers applied. */
export function bytesForText(text: string, mods: StickyMods): Uint8Array {
  return withMods(utf8.encode(text), mods, text)
}

/**
 * A modifier press toggles that modifier; any other press (bar key or typed
 * letter) consumes every sticky back to false. `key` is `string` so a letter
 * from the OS keyboard can consume without inventing a dummy bar key.
 */
export function applyStickyPress(mods: StickyMods, key: string): StickyMods {
  if (key === 'ctrl') return { ctrl: !mods.ctrl, alt: mods.alt }
  if (key === 'alt') return { ctrl: mods.ctrl, alt: !mods.alt }
  return { ctrl: false, alt: false }
}
