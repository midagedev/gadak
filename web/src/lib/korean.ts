/*
 * Issue Navigator — Hangul chosung helpers ([explore] local-search intelligence)
 *
 * Purpose: support chosung (initial-consonant) search so "커트백" matches "ㅋㅌㅂ".
 *  - extractChosung: reduce syllables to a chosung string (non-Hangul lowercased as-is)
 *    → precompute chosung form of searchable text.
 *  - isChosungQuery: true when the query is "consonants only" → decide whether to enable
 *    chosung matching.
 *
 * Perf: pure string ops (minimize regex/allocations). Callers cache over large (10.5K) sets.
 */

/* 19 Hangul syllable initials in Unicode composition order. */
const CHOSUNG = [
  'ㄱ', 'ㄲ', 'ㄴ', 'ㄷ', 'ㄸ', 'ㄹ', 'ㅁ', 'ㅂ', 'ㅃ', 'ㅅ',
  'ㅆ', 'ㅇ', 'ㅈ', 'ㅉ', 'ㅊ', 'ㅋ', 'ㅌ', 'ㅍ', 'ㅎ',
] as const

const SYLLABLE_BASE = 0xac00 // '가' (first Hangul syllable)
const SYLLABLE_LAST = 0xd7a3 // '힣' (last Hangul syllable)
const CHOSUNG_BLOCK = 588 // 21 (jungseong) × 28 (jongseong)

/* Compatibility jamo (standalone consonants): ㄱ (0x3131) ~ ㅎ (0x314e). Vowels follow (ㅏ …). */
const COMPAT_CONSONANT_START = 0x3131
const COMPAT_CONSONANT_END = 0x314e

/**
 * Reduce a string to its chosung form.
 *  - Hangul syllables (가~힣) → single initial consonant.
 *  - Everything else (Latin/digits/symbols/space/already-jamo) passes through lowercased.
 *
 * e.g. "커트백 A" → "ㅋㅌㅂ a"
 */
export function extractChosung(str: string): string {
  let out = ''
  for (let i = 0; i < str.length; i++) {
    const code = str.charCodeAt(i)
    if (code >= SYLLABLE_BASE && code <= SYLLABLE_LAST) {
      out += CHOSUNG[Math.floor((code - SYLLABLE_BASE) / CHOSUNG_BLOCK)]
    } else {
      out += str[i].toLowerCase()
    }
  }
  return out
}

/**
 * Whether the query is a chosung-only query (consonants only).
 *  True when every non-space char is a standalone jamo (ㄱ~ㅎ) and there is ≥1 such char.
 *  "ㅋㅌㅂ" → true, "커트백"/"den"/"ㅋ초" → false.
 */
export function isChosungQuery(str: string): boolean {
  let seen = false
  for (let i = 0; i < str.length; i++) {
    const code = str.charCodeAt(i)
    if (code === 0x20) continue // spaces allowed (ignored)
    if (code < COMPAT_CONSONANT_START || code > COMPAT_CONSONANT_END) return false
    seen = true
  }
  return seen
}
