/*
 * C1 is the whole reason this file exists: the TS list must stay
 * byte-identical to the Go owner (internal/retro/retro.go DoneWords), and the
 * matching rule must stay behaviour-identical to HasDoneWord.
 *
 * Contract table (spec C1–C7; only C1 lives here — C2–C7 are pinned by
 * e2e/detail-coaching.spec.ts, which names its own rows):
 *   assertion                          | fails first when
 *   -----------------------------------+------------------------------------------
 *   DONE_WORDS ≡ embedded Go slice     | either side edits a word without the other
 *   DONE_WORDS ≡ live parsed retro.go  | the Go owner changes in any later commit
 *   retroHasDoneWord parity cases      | TS changes folding/blank/substring handling
 *
 * FAIL-first: against the pre-change tree this file cannot even import —
 * done-words.ts did not exist — and the parity rows stay red against any
 * future drift in either direction (a word list is only lockstep if a test
 * can see both copies). The embedded slice below is the Go source's own
 * block, quoted verbatim, so the diff a reviewer reads is word-for-word.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { DONE_WORDS, hasDoneWord } from './done-words'

const HERE = dirname(fileURLToPath(import.meta.url))
// The Go owner of the done-word list moved mid-round (uncommitted, parallel
// work in this tree): cmd/gadak/retro.go `retroDoneWords` → internal/retro/
// retro.go `DoneWords`, word list unchanged. Parse every candidate that
// exists so the lockstep holds on both sides of that landing.
const RETRO_GO_CANDIDATES = [
  join(HERE, '../../../internal/retro/retro.go'),
  join(HERE, '../../../cmd/gadak/retro.go'),
] as const

// internal/retro/retro.go:57 (cmd/gadak/retro.go:53 pre-move), verbatim.
// Editing a word here and not there (or vice versa) is exactly the failure
// the next two tests exist to catch.
const GO_SLICE_TEXT = `"done", "fixed", "merged", "resolved", "shipped",
	"완료", "해결", "머지", "반영", "배포",
	"完了", "修正済み", "対応済み",
	"已完成", "已解决", "已修复",`

/** Pull the string literals out of a Go `[]string{…}` block. */
function parseGoSlice(block: string): string[] {
  return [...block.matchAll(/"([^"]*)"/g)].map((m) => m[1])
}

describe('done-words (C1: lockstep with cmd/gadak/retro.go)', () => {
  test('DONE_WORDS matches the embedded Go slice word for word', () => {
    expect([...DONE_WORDS]).toEqual(parseGoSlice(GO_SLICE_TEXT))
  })

  test('DONE_WORDS matches the live Go slice word for word', () => {
    const decl = /var (?:retroDoneWords|DoneWords) = \[\]string\{/
    let checked = 0
    for (const path of RETRO_GO_CANDIDATES) {
      let src: string
      try {
        src = readFileSync(path, 'utf8')
      } catch {
        continue // candidate absent in this tree state
      }
      const start = src.search(decl)
      if (start < 0) continue // this file does not own the list
      const open = src.indexOf('{', start)
      const close = src.indexOf('}', open)
      expect(close, `${path} done-words block must close`).toBeGreaterThan(open)
      expect([...DONE_WORDS], `${path} is a lockstep copy`).toEqual(
        parseGoSlice(src.slice(open + 1, close)),
      )
      checked++
    }
    expect(checked, 'the Go owner must exist in exactly one of the candidates').toBeGreaterThan(0)
  })
})

describe('hasDoneWord (HasDoneWord parity)', () => {
  /*
   * Guard for guard with the Go owner. The negation rows are the ones that
   * matter: every `false` below was `true` under the first shipped rule
   * (plain substring containment, 2026-09-06), which pointed the signal at
   * exactly the comments saying the work is NOT finished.
   */
  test.each([
    // claims
    ['Merged and deployed, closing this.', true],
    ['this PR was MERGED a moment ago', true], // case folds like strings.ToLower
    ['작업 완료 — QA까지 확인했습니다', true],
    ['対応済み as of comment 4', true],
    ['已解决，已上线', true], // Simplified Chinese, added with the guards
    ['스테이징에 반영했습니다', true],
    ['resolved the crash in 1.2', true],
    // English word boundaries
    ['abandoned this approach', false], // contains "done"
    ['UNDONE — reconsidering the approach', false],
    ['unresolved, still digging', false],
    ['unmerged as of this morning', false],
    // English negators
    ['not fixed yet', false],
    ['it isn\'t fixed', false],
    ['never merged', false],
    // CJK negation, prefix and suffix
    ['미완료', false],
    ['아직 미완료입니다', false],
    ['완료되지 않음', false],
    ['未完了のまま', false],
    ['対応済みではない', false],
    ['해결 안 됨', false],
    // a question is not a claim
    ['is this done?', false],
    ['이거 완료됐나요?', false],
    // quoted and fenced text is someone else's word
    ['> done\nnot from my side', false],
    ['```\ndone\n```', false],
    // unchanged
    ['incomplete', false],
    ['still fixing the edge case', false],
    ['not yet — waiting on review', false],
    ['', false],
    ['   ', false],
  ])('hasDoneWord(%j) === %s', (body, want) => {
    expect(hasDoneWord(body)).toBe(want)
  })
})
