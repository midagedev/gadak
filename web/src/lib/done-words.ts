/*
 * Done vocabulary for the structure/prose mismatch signal (THEORY.md T5;
 * "Writing a done-word comment" in Seven moments): a comment that claims the
 * work is finished while the issue's status does not.
 *
 * Lockstep copy of the Go owner — internal/retro/retro.go `DoneWords` and
 * `HasDoneWord`. The Go side is the owner: a word or a guard changed there
 * must arrive here in the same commit, and done-words.test.ts enforces the
 * word half by parsing the Go source.
 *
 * Matching is guarded, not plain containment. Measured on the first shipped
 * rule (2026-09-06): "미완료", "완료되지 않음", "未完了", "not fixed",
 * "unresolved", "unmerged", "abandoned" and "is this done?" all came back
 * true — every one of them a comment saying the work is NOT done. English
 * words now have to stand alone, CJK words must not carry a negation, quoted
 * and fenced text is stripped, and a question is not a claim.
 *
 * The result is a candidate, never a fact. It is precise enough for an
 * affordance that costs one dismissal when wrong; a count shown to a steward
 * needs more than this.
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
  '반영',
  '배포',
  '完了',
  '修正済み',
  '対応済み',
  '已完成',
  '已解决',
  '已修复',
] as const

/** One-character CJK negations that glue to the front and reverse the word. */
const NEGATION_PREFIXES = ['미', '未', '불', '非', '无', '無']

/** Negations that follow the word and reverse it: "완료되지 않았다". */
const NEGATION_SUFFIXES = [
  '되지 않',
  '하지 않',
  '지 않',
  '안 됨',
  '안됨',
  'ではない',
  'されていない',
  'していない',
  'ていない',
]

/** English negators that cancel a done word sitting just after them. */
const ENGLISH_NEGATORS = ['not', "n't", 'no', 'never', "isn't", "wasn't", "aren't", 'yet']

/** The mismatch test — same rule, guard for guard, as Go's HasDoneWord. */
export function hasDoneWord(text: string): boolean {
  const body = stripQuotedAndCode(text)
  if (body.trim() === '') return false
  if (endsWithQuestion(body)) return false
  const low = body.toLowerCase()
  for (const w of DONE_WORDS) {
    if (isAsciiWord(w)) {
      if (matchEnglishWord(low, w)) return true
      continue
    }
    if (matchCjkWord(body, w)) return true
  }
  return false
}

/**
 * A done-word comment is a live claim only while nothing has answered it: it
 * must be newer than the issue's last status change. A comment that predates
 * the change was read and acted on — the structure moved after the prose, so
 * it is not a mismatch now. No recorded status change means nothing answered
 * the claim, so it stands; a comment with no usable stamp cannot be shown to
 * be newer, so it does not. Lockstep with Go's retro.ClaimStands, truth table
 * for truth table (2026-09-07).
 */
export function claimStands(
  commentAt: string | null | undefined,
  statusChangedAt: string | null | undefined,
): boolean {
  const c = Date.parse(commentAt ?? '')
  if (!Number.isFinite(c)) return false
  const s = Date.parse(statusChangedAt ?? '')
  if (!Number.isFinite(s)) return true
  return c > s
}

/** Markdown quote lines and fenced code blocks are someone else's words. */
function stripQuotedAndCode(body: string): string {
  const out: string[] = []
  let inFence = false
  for (const line of body.split('\n')) {
    const trimmed = line.trim()
    if (trimmed.startsWith('```') || trimmed.startsWith('~~~')) {
      inFence = !inFence
      continue
    }
    if (inFence || trimmed.startsWith('>')) continue
    out.push(line)
  }
  return out.join('\n')
}

/** "is this done?" asks; it does not claim. */
function endsWithQuestion(text: string): boolean {
  const t = text.trim().replace(/[ \t)\]"'”’]+$/, '')
  return t.endsWith('?') || t.endsWith('？')
}

function isAsciiWord(w: string): boolean {
  // eslint-disable-next-line no-control-regex
  return /^[\x00-\x7f]*$/.test(w)
}

function isWordChar(ch: string): boolean {
  return /[a-z0-9]/i.test(ch)
}

/** w standing on its own, and not preceded by a negator. */
function matchEnglishWord(low: string, w: string): boolean {
  let i = low.indexOf(w)
  while (i >= 0) {
    const beforeOk = i === 0 || !isWordChar(low[i - 1])
    const end = i + w.length
    const afterOk = end >= low.length || !isWordChar(low[end])
    if (beforeOk && afterOk && !englishNegatedBefore(low.slice(0, i))) return true
    i = low.indexOf(w, i + 1)
  }
  return false
}

/**
 * The last three words before the match. Three is the window: "not yet
 * fixed" and "is not fixed" both negate; a clause two sentences back does not.
 */
function englishNegatedBefore(before: string): boolean {
  const fields = before.split(/\s+/).filter(Boolean).slice(-3)
  for (const raw of fields) {
    const f = raw.replace(/^[.,;:!?()[\]"']+|[.,;:!?()[\]"']+$/g, '')
    for (const n of ENGLISH_NEGATORS) {
      if (f === n || (n.endsWith("n't") && f.endsWith("n't"))) return true
    }
  }
  return false
}

/** w without a negation prefix before it or a negation anchored after it. */
function matchCjkWord(text: string, w: string): boolean {
  let i = text.indexOf(w)
  while (i >= 0) {
    const prefixed = i > 0 && NEGATION_PREFIXES.includes(text[i - 1])
    if (!prefixed && !negatedSuffix(text.slice(i + w.length))) return true
    i = text.indexOf(w, i + 1)
  }
  return false
}

/**
 * Anchored: the negation has to start right after the word, spaces aside.
 * Anchoring is the whole bound, which is what lets Go (byte indices) and this
 * file (UTF-16) express the same rule.
 */
function negatedSuffix(after: string): boolean {
  const rest = after.replace(/^[ \t]+/, '')
  return NEGATION_SUFFIXES.some((n) => rest.startsWith(n))
}
