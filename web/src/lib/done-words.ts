/*
 * Done vocabulary for the structure/prose mismatch signal (THEORY.md T5;
 * "Writing a done-word comment" in Seven moments): a comment that says done
 * while the issue's status does not.
 *
 * Lockstep copy of the Go owner — internal/retro/retro.go `DoneWords` (:57)
 * and `HasDoneWord` (:646). (Pre-move owner: cmd/gadak/retro.go
 * `retroDoneWords`; the word list did not change in the move.) The Go side
 * is the owner: a word changed there must arrive here in the same commit,
 * and done-words.test.ts enforces that by parsing the Go source. Matching
 * is substring containment on the lowercased body — the Go side's own note:
 * CJK words are long enough that false hits inside other words are rare —
 * so there is no word-boundary handling to keep in step, and "UNDONE"
 * matches exactly as it does in `gadak retro`.
 */

export const DONE_WORDS = [
  'done',
  'fixed',
  'merged',
  'resolved',
  'shipped',
  '완료',
  '해결',
  '머지',
  '完了',
  '修正済み',
  '対応済み',
] as const

/** The mismatch test — same rule as retroHasDoneWord: a blank body never
 *  matches, lowercasing folds the English words, CJK matches as-is. */
export function hasDoneWord(text: string): boolean {
  if (text.trim() === '') return false
  const low = text.toLowerCase()
  return DONE_WORDS.some((w) => low.includes(w))
}
