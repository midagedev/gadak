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
// `glasskeys` (GDK-898). This file is the adapter that turns
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
} from 'glasskeys'

export { LOCK_WINDOW_MS, StickyModifiers } from 'glasskeys'
export type { Intent, ModifierId, SlotState } from 'glasskeys'

export type StickyMods = { ctrl: boolean; alt: boolean }
export type StickySlots = { ctrl: SlotState; alt: SlotState }

export type BarKey =
  | 'esc'
  | 'tab'
  | 'ctrl'
  | 'alt'
  | 'clear'
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

/**
 * DECCKM (`CSI ?1h`). Every full-screen TUI sets it, which is the whole
 * reason this bar exists, and under it the cursor keys are SS3 (`ESC O A`)
 * rather than CSI (`ESC [ A`). The renderer holds the live value; this
 * module is pure, so it arrives as an argument.
 */
export type CursorKeyMode = 'normal' | 'application'

type BarSend =
  | { kind: 'empty' }
  | { kind: 'text'; ch: string }
  | { kind: 'cursor'; final: number }

// Record<BarKey, …> is the exhaustiveness gate: a new bar key that does not
// pick a send kind fails the typecheck, not a screenshot of a missing button.
const BAR: Record<BarKey, BarSend> = {
  esc: { kind: 'text', ch: '\x1b' },
  tab: { kind: 'text', ch: '\t' },
  ctrl: { kind: 'empty' },
  alt: { kind: 'empty' },
  // `empty` here is not "a modifier" — it is "produces no bytes". `clear` is
  // the panic reset (GDK-953): glasskeys' contract says any UI that offers
  // lock must offer it. It sends nothing, so the encoder path never sees it.
  clear: { kind: 'empty' },
  up: { kind: 'cursor', final: 0x41 },
  down: { kind: 'cursor', final: 0x42 },
  right: { kind: 'cursor', final: 0x43 },
  left: { kind: 'cursor', final: 0x44 },
  home: { kind: 'cursor', final: 0x48 },
  end: { kind: 'cursor', final: 0x46 },
  pipe: { kind: 'text', ch: '|' },
  slash: { kind: 'text', ch: '/' },
  dash: { kind: 'text', ch: '-' },
  tilde: { kind: 'text', ch: '~' },
}

/*
 * Cursor keys, encoded the way the terminal the bytes are going to expects
 * them (GDK-899). Three cases, and the bar used to send the first one for
 * all three:
 *
 *   no modifier, normal mode        ESC [ A
 *   no modifier, DECCKM             ESC O A
 *   any modifier, either mode       ESC [ 1 ; <mask+1> A
 *
 * That table is not invented here. It is what the shipped xterm.js sends
 * for a hardware keyboard — `node_modules/@xterm/xterm/lib/xterm.js`,
 * `evaluateKeyboardEvent`, keyCodes 35–40:
 *
 *     case 38: … o.key = a ? ESC+"[1;"+(a+1)+"A" : t ? ESC+"OA" : ESC+"[A"
 *
 * where `t` is DECCKM and `a` is the modifier mask. The bar writes to the
 * socket directly instead of going through xterm's keyboard path, so
 * without this the same arrow sent different bytes depending on whether it
 * came from a bluetooth keyboard or from the screen — the release audit's
 * "same action, same affordance" axis, failing on the phone's own bar.
 */
function cursorKeyBytes(final: number, mods: StickyMods, cursorKeys: CursorKeyMode): Uint8Array {
  // xterm's mask: shift 1, alt 2, ctrl 4, meta 8, sent as mask+1. The bar
  // has no shift or meta slot, so only 2 and 4 can appear today; the
  // parameter is written as decimal so a third slot would not need this
  // function rewritten.
  const mask = (mods.alt ? 2 : 0) | (mods.ctrl ? 4 : 0)
  if (mask !== 0) {
    return new Uint8Array([ESC, 0x5b, 0x31, 0x3b, ...utf8.encode(String(mask + 1)), final])
  }
  return new Uint8Array([ESC, cursorKeys === 'application' ? 0x4f : 0x5b, final])
}

/**
 * Bytes a bar key sends under the current sticky modifiers.
 *
 * `cursorKeys` is required rather than defaulted: a default would be a
 * silent answer to a question only the live renderer can answer, and the
 * wrong answer is exactly the defect this argument exists to close.
 */
export function bytesForBarKey(
  key: BarKey,
  mods: StickyMods,
  cursorKeys: CursorKeyMode,
): Uint8Array {
  const send = BAR[key]
  if (send.kind === 'empty') return new Uint8Array(0)
  if (send.kind === 'text') return bytesForText(send.ch, mods)
  return cursorKeyBytes(send.final, mods, cursorKeys)
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
 * table never appear. Modifier taps do not go through here — they `tap()`;
 * neither does `clear` — the screen calls `sticky.clear()` for it.
 */
export function stepsForBarKey(
  key: BarKey,
  hasMarked: boolean,
  mods: readonly ModifierId[] = [],
): Intent[] {
  // `clear` resets every sticky slot; it is not a key press and must never
  // reach the barrier — an unguarded name would come back as an `emit-key`
  // the encoder has no bytes for (measured on the pre-fix source).
  if (key === 'clear') return []
  return barrierSteps({
    key,
    mods: [...mods],
    hasMarked,
    pending: 'not-needed',
  })
}
