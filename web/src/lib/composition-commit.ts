/*
 * GDK-169: mid-composition states are never committed as queries.
 *
 * One owner for SearchBox and CommandPalette. While a composition session
 * is active, commit() is not called. compositionend is the commit point:
 * Safari can fire the final input with isComposing still true after
 * compositionend, so that input must not be the only path that commits.
 * Non-IME typing (no composition session, isComposing false) still commits
 * on every input, including deletions after a composition.
 */

export type CompositionCommitEvent = {
  isComposing?: boolean
  data?: string | null
  currentTarget?: EventTarget | null
  target?: EventTarget | null
}

function inputValue(el: EventTarget | null | undefined): string | undefined {
  if (el && typeof el === 'object' && 'value' in el && typeof (el as { value: unknown }).value === 'string') {
    return (el as { value: string }).value
  }
  return undefined
}

function readValue(event: CompositionCommitEvent, text?: string): string {
  const fromEl = inputValue(event.currentTarget) ?? inputValue(event.target)
  if (fromEl !== undefined) return fromEl
  if (typeof text === 'string') return text
  if (typeof event.data === 'string') return event.data
  return ''
}

export function createCompositionCommit(commit: (text: string) => void) {
  let composing = false

  return {
    oninput(event: CompositionCommitEvent, text?: string) {
      if (composing || event.isComposing) return
      commit(readValue(event, text))
    },
    oncompositionstart() {
      composing = true
    },
    oncompositionend(event: CompositionCommitEvent, text?: string) {
      composing = false
      commit(readValue(event, text))
    },
    get composing() {
      return composing
    },
  }
}
