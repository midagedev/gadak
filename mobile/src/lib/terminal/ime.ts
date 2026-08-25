// IME gate for the shell (DESIGN.md §10.3). Between compositionstart and
// compositionend, nothing reaches the PTY: half-assembled 자모 cannot be
// taken back once the shell has acted on them. Modelled as a reducer over
// a description of the event, never a DOM event, so a Korean sequence is a
// unit test rather than a screenshot. The input-element attributes live
// here so the screen cannot forget to disable autocorrect — a smart quote
// in a shell command is a defect, and this is the cheapest place to make
// its absence a gate.
//
// The sibling on the desktop (web/src/lib/composition-commit.ts) commits a
// search-box *value* and is idempotent on Chrome's trailing input-after-end.
// A PTY appends; the same trailing event would type the syllable twice. This
// reducer therefore owns a different contract and does not import that file
// (mobile vocabulary: only lib/i18n.ts may reach into web/).

import { CompositionGate, type CompositionEvent, type Intent, type ModifierId } from 'glasskeys'

export type ImeEvent =
  | { kind: 'compositionstart' }
  | { kind: 'compositionupdate'; data: string }
  | { kind: 'compositionend'; data: string }
  | { kind: 'input'; data: string; isComposing: boolean }

export type ImeState = { composing: boolean }

function toCompositionEvent(ev: ImeEvent): CompositionEvent {
  switch (ev.kind) {
    case 'compositionstart':
      return { type: 'compose-start' }
    case 'compositionupdate':
      return { type: 'compose-update', text: ev.data }
    case 'compositionend':
      return { type: 'compose-end', text: ev.data }
    case 'input':
      // `isComposing` without a start is a platform that skipped the
      // start event. Map it to an update so it withholds and does not
      // latch the gate — the same rule the library's update has.
      return ev.isComposing
        ? { type: 'compose-update', text: ev.data }
        : { type: 'plain', text: ev.data }
  }
}

function emitOf(intents: Intent[]): string {
  for (const intent of intents) {
    if (intent.op === 'emit-text') return intent.text
  }
  return ''
}

/**
 * DOM-shaped events in, the library gate underneath. `{state, emit}` stays
 * so the screen's call sites stay readable; `intents` is the gate's own
 * answer (withhold vs empty vs emit-text), which the seam tests pin.
 */
export function imeReduce(
  state: ImeState,
  ev: ImeEvent,
  mods: readonly ModifierId[] = [],
): { state: ImeState; emit: string; intents: Intent[] } {
  const gate = new CompositionGate()
  if (state.composing) gate.next({ type: 'compose-start' })
  const intents = gate.next(toCompositionEvent(ev), [...mods])
  return { state: { composing: gate.composing }, emit: emitOf(intents), intents }
}

/** Attributes the keystroke-taking element must carry so iOS does not
 *  rewrite what the user typed. */
export const IME_INPUT_ATTRS: Readonly<Record<string, string>> = Object.freeze({
  autocapitalize: 'off',
  autocorrect: 'off',
  autocomplete: 'off',
  spellcheck: 'false',
})
