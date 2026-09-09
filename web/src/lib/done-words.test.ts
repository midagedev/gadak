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
import { DONE_WORDS, claimStands, hasDoneWord } from './done-words'

const HERE = dirname(fileURLToPath(import.meta.url))
// The Go owner of the done-word list: internal/retro/retro.go `DoneWords`.
const RETRO_GO = join(HERE, '../../../internal/retro/retro.go')

// internal/retro/retro.go DoneWords, verbatim. Editing a word here and not
// there (or vice versa) is exactly the failure the next two tests exist to
// catch.
const GO_SLICE_TEXT = `"done", "fixed", "merged", "resolved", "shipped",
	"완료", "해결", "머지", "반영", "배포",
	"完了", "修正済み", "対応済み",
	"已完成", "已解决", "已修复",`

/** Pull the string literals out of a Go `[]string{…}` block. */
function parseGoSlice(block: string): string[] {
  return [...block.matchAll(/"([^"]*)"/g)].map((m) => m[1])
}

describe('done-words (C1: lockstep with internal/retro/retro.go)', () => {
  test('DONE_WORDS matches the embedded Go slice word for word', () => {
    expect([...DONE_WORDS]).toEqual(parseGoSlice(GO_SLICE_TEXT))
  })

  test('DONE_WORDS matches the live Go slice word for word', () => {
    const src = readFileSync(RETRO_GO, 'utf8')
    const start = src.search(/var DoneWords = \[\]string\{/)
    if (start < 0) {
      throw new Error(`${RETRO_GO} no longer owns the done-word list — update this test's citation`)
    }
    const open = src.indexOf('{', start)
    const close = src.indexOf('}', open)
    expect(close, `${RETRO_GO} done-words block must close`).toBeGreaterThan(open)
    expect([...DONE_WORDS], `${RETRO_GO} is a lockstep copy`).toEqual(
      parseGoSlice(src.slice(open + 1, close)),
    )
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

describe('claimStands — the recency guard (2026-09-07), lockstep with retro.ClaimStands', () => {
  /*
   * Truth table, row for row the Go owner's:
   *   comment unparseable                          → false
   *   status_changed_at empty / unparseable        → true  (nothing answered the claim)
   *   comment before the status change             → false (the change answered it)
   *   comment at exactly the status change         → false (equal is not newer)
   *   comment after the status change              → true
   * FAIL-first: the pre-change CommentList offered "Move to done" on the
   * "before" row — an old "merged" on an issue since moved back to review.
   */
  test.each<[string | null | undefined, string | null | undefined, boolean]>([
    [null, '2026-09-01T00:00:00.000Z', false],
    ['not a date', '2026-09-01T00:00:00.000Z', false],
    ['2026-09-02T00:00:00.000Z', null, true],
    ['2026-09-02T00:00:00.000Z', '', true],
    ['2026-09-02T00:00:00.000Z', 'garbage', true],
    ['2026-08-31T00:00:00.000Z', '2026-09-01T00:00:00.000Z', false],
    ['2026-09-01T00:00:00.000Z', '2026-09-01T00:00:00.000Z', false],
    ['2026-09-01T00:00:00.001Z', '2026-09-01T00:00:00.000Z', true],
    // +09:00 and Z spellings compare by instant, not by string.
    ['2026-09-01T09:00:00.000+09:00', '2026-09-01T00:00:00.000Z', false],
    ['2026-09-01T09:00:01.000+09:00', '2026-09-01T00:00:00.000Z', true],
  ])('claimStands(%j, %j) === %s', (comment, status, want) => {
    expect(claimStands(comment, status)).toBe(want)
  })
})

/*
 * GDK-1428, the pending-clause guard. Same table as Go's
 * TestDoneWordPendingClauses, row for row — a done word inside a clause about
 * work that has not happened yet is not a claim.
 *
 * FAIL-first: every `false` row below was `true` against the pre-guard rule.
 */
describe('hasDoneWord pending clauses (GDK-1428 parity)', () => {
  test.each([
    ['검토 완료 후 진행하겠습니다', false],
    ['QA 완료 후에 배포합니다', false],
    ['완료되면 알려주세요', false],
    ['완료하면 코멘트 남겨주세요', false],
    ['리뷰 완료 시 머지하겠습니다', false],
    ['완료 예정입니다', false],
    ['이번 주에 완료할 예정', false],
    ['내일까지 완료해야 합니다', false],
    ['완료되는 대로 공유드리겠습니다', false],
    ['배포 전에 다시 확인하겠습니다', false],
    ['完了後にリリースします', false],
    ['完了次第ご連絡します', false],
    ['完了予定です', false],
    ['완료했습니다', true],
    ['작업 완료됐습니다', true],
    ['완료되었습니다, 확인 부탁드립니다', true],
    ['머지 완료', true],
    ['배포 완료했습니다', true],
    ['対応済みです', true],
    ['完了しました', true],
    ['미완료 상태입니다', false],
    ['완료되지 않았습니다', false],
    ['not fixed yet', false],
    ['Merged and deployed, closing this.', true],
  ])('hasDoneWord(%j) === %s', (body, want) => {
    expect(hasDoneWord(body as string)).toBe(want)
  })
})
