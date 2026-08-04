/*
 * Issue Navigator — 한글 초성 유틸 ([explore] 로컬 검색 지능화)
 *
 * 목적: "커트백" 을 "ㅋㅌㅂ" 로 찾을 수 있게 초성 검색을 지원한다.
 *  - extractChosung: 음절을 초성열로 환원(비한글은 소문자 그대로) → 검색 대상 텍스트를 미리 초성화.
 *  - isChosungQuery: 쿼리가 "자음만" 인지 판정 → 초성 매칭을 켤지 결정.
 *
 * 성능: 순수 문자열 연산(정규식/할당 최소화). 대량(10.5K) 대상은 호출부에서 캐시한다.
 */

/* 한글 음절의 초성 19자(유니코드 조합 순서). */
const CHOSUNG = [
  'ㄱ', 'ㄲ', 'ㄴ', 'ㄷ', 'ㄸ', 'ㄹ', 'ㅁ', 'ㅂ', 'ㅃ', 'ㅅ',
  'ㅆ', 'ㅇ', 'ㅈ', 'ㅉ', 'ㅊ', 'ㅋ', 'ㅌ', 'ㅍ', 'ㅎ',
] as const

const SYLLABLE_BASE = 0xac00 // '가'
const SYLLABLE_LAST = 0xd7a3 // '힣'
const CHOSUNG_BLOCK = 588 // 21(중성) × 28(종성)

/* 호환 자음(단독 자음) 영역: ㄱ(0x3131) ~ ㅎ(0x314e). 그 뒤는 모음(ㅏ …). */
const COMPAT_CONSONANT_START = 0x3131
const COMPAT_CONSONANT_END = 0x314e

/**
 * 문자열을 초성열로 환원한다.
 *  - 한글 음절(가~힣)은 초성 1자로 치환.
 *  - 그 외(영문/숫자/기호/공백/이미 자음인 문자)는 소문자로 그대로 통과.
 *
 * 예) "커트백 A" → "ㅋㅌㅂ a"
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
 * 쿼리가 초성 쿼리인지(자음만으로 구성) 판정한다.
 *  공백을 제외한 모든 문자가 단독 자음(ㄱ~ㅎ)이고, 최소 1자 이상일 때 true.
 *  "ㅋㅌㅂ" → true, "커트백"/"den"/"ㅋ초" → false.
 */
export function isChosungQuery(str: string): boolean {
  let seen = false
  for (let i = 0; i < str.length; i++) {
    const code = str.charCodeAt(i)
    if (code === 0x20) continue // 공백은 허용(무시)
    if (code < COMPAT_CONSONANT_START || code > COMPAT_CONSONANT_END) return false
    seen = true
  }
  return seen
}
