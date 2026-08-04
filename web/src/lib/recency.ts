/*
 * Issue Navigator — 최근 사용 기록 (개인화 정렬용)
 *
 * 모든 선택 UI(담당자/상태전환/새 이슈)에 "조용한 제안"을 주기 위한 localStorage 기반 헬퍼.
 *  - kind 별로 최근 사용값을 최신순·중복제거·최대 10개 보관한다.
 *  - 순서 자체가 제안이다(별도 배지 없음). 성공한 액션만 기록한다(호출부 책임).
 *
 * kind 예: 'assignee', 'transition:<project>', 'create-project',
 *          'create-type:<project>', 'label'
 */

const PREFIX = 'issue-nav:recent:'
const MAX = 10

/** 최근 사용값 목록(최신순). 없으면 빈 배열. */
export function recentOf(kind: string): string[] {
  try {
    const raw = localStorage.getItem(PREFIX + kind)
    if (!raw) return []
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return []
  }
}

/** 사용값을 맨 앞에 기록(중복 제거, 최대 MAX). 빈 값은 무시. */
export function recordRecent(kind: string, value: string): void {
  if (!value) return
  try {
    const next = [value, ...recentOf(kind).filter((v) => v !== value)].slice(0, MAX)
    localStorage.setItem(PREFIX + kind, JSON.stringify(next))
  } catch {
    /* localStorage 불가 환경 — 무시 */
  }
}

/**
 * 최근 순위 인덱스(0 = 가장 최근, 없으면 Infinity). 정렬 비교자에 바로 쓴다.
 */
export function recentRank(kind: string, value: string): number {
  const i = recentOf(kind).indexOf(value)
  return i === -1 ? Infinity : i
}
