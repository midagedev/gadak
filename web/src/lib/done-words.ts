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

/**
 * Suffixes that turn a done word into a clause about work that has NOT
 * happened yet: "완료되면 알려주세요", "완료 예정입니다", "完了次第".
 * GDK-1428 — on a Korean corporate Jira the mismatch row ran 44–201 hits a
 * week against 54–244 closures while the English OSS mirror stayed at 0–23,
 * because scheduling and requesting are ordinary office vocabulary and both
 * carry a done word. Lockstep with Go's `pendingSuffixes`.
 *
 * GDK-1943 halved the list: only the verbal endings — condition, obligation,
 * futurity, politeness — remain, because those are word morphology and have
 * to be listed one by one. The marker-plus-particle spellings that used to
 * live here ("후에", "뒤에", "후까지"…) moved into KOREAN_PARTICLES, where one
 * table composes with every marker at once; a list of compound spellings is a
 * list that is short by one again, and "뒤부터" was exactly that one.
 */
const PENDING_SUFFIXES = [
  '되면',
  '하면',
  '면 ',
  '되는 대로',
  '되는대로',
  '되면서',
  '해야',
  '하여야',
  '되어야',
  '예정',
  '되기 전',
  '하기 전',
  '할',
  '될',
  '하겠',
  '드리겠',
  '합니다',
  '해주',
  '해 주',
  '하시',
  '부탁',
  '요청',
  '次第',
  'したら',
  'すれば',
  '予定',
]

/**
 * One-character clause markers: bare "후"/"시"/"전" is a clause only when
 * clauseMarkerFollows says so — a Korean particle after it keeps it one, a
 * Hangul/Han syllable after it means it headed a longer word. Lockstep with
 * Go's `pendingSingles`.
 */
const PENDING_SINGLES = ['후', '뒤', '시', '전', '後', '前']

/**
 * The closed class of Korean particles that attach directly to a bare
 * temporal noun: 후에, 뒤에서, 시부터, 전까지, 후로… (GDK-1943). Korean glues
 * particles onto the noun in the same script as the words, which is why the
 * rune guard alone cannot tell "뒤부터" (a clause) from "후반전" (a longer
 * word) — 부 and 반 are both Hangul. The table holds the class, not the
 * spellings seen so far; any particle prefix settles, so stacked particles
 * (후에는, 뒤에서도) need no entries of their own. Japanese particles are kana
 * — a script the rune guard already separates — so they are absent. Lockstep
 * with Go's `koreanParticles`.
 */
const KOREAN_PARTICLES = ['에서', '부터', '까지', '으로', '보다', '에', '엔', '로', '도', '만']

/**
 * Endings that let a done word modify a following noun instead of claiming
 * anything: "반영된 뒤부터 시작됨" — the verb is 시작됨 and 반영 is a past
 * modifier inside its temporal phrase. The bridge only counts when a clause
 * marker follows it; "반영된 부분만 남겼습니다" keeps its claim. Lockstep
 * with Go's `adnominalBridges`.
 */
const ADNOMINAL_BRIDGES = ['된', '됐', '되고', '되어', '한', '하고', '하여']

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
    const after = text.slice(i + w.length)
    if (!prefixed && !negatedSuffix(after) && !pendingSuffix(after)) return true
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

/**
 * Anchored the same way negatedSuffix is: the marker starts right after the
 * word, spaces aside, so a conditional in a later sentence is never borrowed.
 * GDK-1943 made this the one owner of the decision, as a rule in three steps
 * — a verbal ending, an adnominal bridge into a marker, or a bare marker
 * carrying a particle — where it used to be a suffix list, a marker list and
 * a rune class that disagreed at the edges ("뒤부터" fell through all three
 * and the row measured the language, not the work). Lockstep with Go's
 * `pendingSuffix`.
 */
function pendingSuffix(after: string): boolean {
  const rest = after.replace(/^[ \t]+/, '')
  if (PENDING_SUFFIXES.some((p) => rest.startsWith(p))) return true
  for (const b of ADNOMINAL_BRIDGES) {
    if (rest.startsWith(b) && clauseMarkerFollows(rest.slice(b.length))) return true
  }
  return clauseMarkerFollows(rest)
}

/**
 * The single owner of "a bare clause marker is actually carrying a clause":
 * a Korean particle right after the marker keeps it one ("뒤부터", "후로",
 * "완료 후에"), and failing that a Hangul or Han rune means the marker was
 * the head of a longer word ("완료 후반전", "준비 시점"), so the claim stands.
 * End of text, a space, punctuation or another script leaves it a clause —
 * "완료 후 진행" schedules, and kana after 後/前 is Japanese's own particle.
 * Lockstep with Go's `clauseMarkerFollows`.
 */
function clauseMarkerFollows(rest: string): boolean {
  const r = rest.replace(/^[ \t]+/, '')
  for (const p of PENDING_SINGLES) {
    if (!r.startsWith(p)) continue
    const tail = r.slice(p.length)
    if (KOREAN_PARTICLES.some((x) => tail.startsWith(x))) return true
    return tail === '' || !isCjkSyllable(tail[0])
  }
  return false
}

/** A Hangul syllable or a Han character — the runes that glue into words. */
function isCjkSyllable(ch: string): boolean {
  return /[\u3400-\u9fff\uac00-\ud7af\uf900-\ufaff]/.test(ch)
}
