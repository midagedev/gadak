// What a key-bar press means in bytes (DESIGN.md §10.3). A phone keyboard
// has no Esc, Ctrl, or arrows, so a bar above it carries them. This module
// owns the mapping — CSI sequences, control bytes — and none of the DOM.
// Defects here are invisible in a screenshot and obvious in a table of
// inputs, which is why the encoder is a pure function over plain data.
//
// The control-byte table is explicit, not `code & 0x1f`: that bitmask would
// also map `@ { | } ~`, which have no control byte and must go out unchanged
// (dropping the keystroke would be worse than ignoring the modifier).
//
// Sticky slots, the composition gate, and the flush barrier live in
// `touch-remote-input` (GDK-898). This file is the adapter that turns
// `activeModifiers()` into the `{ctrl, alt}` the encoder takes, and the
// PTY's permanent `pending: 'not-needed'` answer for the barrier. The
// encoder itself stays ours: naru-remote wraps the same decisions in X11
// keysyms over VNC, and there is no shared encoding of "Ctrl-C".

import {
  barrierSteps,
  type Intent,
  type ModifierId,
  type SlotState,
  type StickyModifiers,
} from 'touch-remote-input'

export { LOCK_WINDOW_MS, StickyModifiers } from 'touch-remote-input'
export type { Intent, ModifierId, SlotState } from 'touch-remote-input'

export type StickyMods = { ctrl: boolean; alt: boolean }
export type StickySlots = { ctrl: SlotState; alt: SlotState }

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

/** `control`/`alt` become the encoder's booleans; `shift`/`meta` do not. */
export function encoderMods(ids: readonly ModifierId[]): StickyMods {
  let ctrl = false
  let alt = false
  for (const id of ids) {
    if (id === 'control') ctrl = true
    else if (id === 'alt') alt = true
  }
  return { ctrl, alt }
}

export function stickySlots(sticky: StickyModifiers): StickySlots {
  return { ctrl: sticky.slot('control'), alt: sticky.slot('alt') }
}

export function modifierIdForBarKey(key: BarKey): ModifierId | null {
  if (key === 'ctrl') return 'control'
  if (key === 'alt') return 'alt'
  return null
}

/**
 * Ordered steps for a non-modifier bar key. A PTY write cannot fail, so
 * `pending` is always `'not-needed'` and the failure rows of the barrier
 * table never appear. Modifier taps do not go through here — they `tap()`.
 */
export function stepsForBarKey(
  key: BarKey,
  hasMarked: boolean,
  mods: readonly ModifierId[] = [],
): Intent[] {
  return barrierSteps({
    key,
    mods: [...mods],
    hasMarked,
    pending: 'not-needed',
  })
}
