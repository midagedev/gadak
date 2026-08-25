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

export type ImeEvent =
  | { kind: 'compositionstart' }
  | { kind: 'compositionupdate'; data: string }
  | { kind: 'compositionend'; data: string }
  | { kind: 'input'; data: string; isComposing: boolean }

export type ImeState = { composing: boolean }

export function imeReduce(state: ImeState, ev: ImeEvent): { state: ImeState; emit: string } {
  switch (ev.kind) {
    case 'compositionstart':
      return { state: { composing: true }, emit: '' }
    case 'compositionupdate':
      // Updates are the half-assembled syllables. Never emit; do not touch
      // `composing` — start/end own that flag, so a stray update cannot
      // latch it true with no matching end.
      return { state: { composing: state.composing }, emit: '' }
    case 'compositionend':
      // Always clear, even with no matching start (defensive: must not leave
      // composing stuck true). Empty data is a dismissed candidate.
      return { state: { composing: false }, emit: ev.data }
    case 'input':
      // Browsers fire input *and* composition events for the same syllable.
      // Either flag means we are inside a composition; emit nothing.
      if (state.composing || ev.isComposing) {
        return { state: { composing: state.composing }, emit: '' }
      }
      return { state: { composing: false }, emit: ev.data }
  }
}

/** Attributes the keystroke-taking element must carry so iOS does not
 *  rewrite what the user typed. */
export const IME_INPUT_ATTRS: Readonly<Record<string, string>> = Object.freeze({
  autocapitalize: 'off',
  autocorrect: 'off',
  autocomplete: 'off',
  spellcheck: 'false',
})
