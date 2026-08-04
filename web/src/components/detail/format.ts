/*
 * 상세 패널 공용 포맷 헬퍼 ([detail]).
 * 상대시간 / 상태 카테고리 정규화 / 이니셜 등 순수 함수만 모은다.
 */

import { jiraBrowseUrl } from '../../lib/config'
import type { StatusCategory } from '../../lib/types'

/** Jira 원본 status_category 문자열을 UI 3분류로 정규화한다. */
export function normalizeCategory(raw: string | null | undefined): StatusCategory {
  const c = (raw ?? '').toLowerCase()
  if (c === 'done' || c === 'complete' || c === 'completed') return 'done'
  if (c === 'new' || c === 'todo' || c === 'to do') return 'new'
  // "indeterminate"(진행중) 및 그 외 미분류는 진행중으로 취급
  return 'inprogress'
}

/** 상태 카테고리 → 시맨틱 색 토큰 이름(app.css @theme 의 --color-status-*). */
export function categoryColor(cat: StatusCategory): string {
  return cat === 'done' ? 'done' : cat === 'new' ? 'new' : 'inprogress'
}

/** ISO8601 → "3분 전" 류 상대시간(한국어). 파싱 실패 시 원문 반환. */
export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return iso
  const diff = Date.now() - t
  const sec = Math.round(diff / 1000)
  if (sec < 0) return '방금'
  if (sec < 60) return '방금'
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}분 전`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}시간 전`
  const day = Math.round(hr / 24)
  if (day === 1) return '어제'
  if (day < 7) return `${day}일 전`
  const wk = Math.round(day / 7)
  if (wk < 5) return `${wk}주 전`
  const mo = Math.round(day / 30)
  if (mo < 12) return `${mo}개월 전`
  return `${Math.round(day / 365)}년 전`
}

/** ISO8601 → 절대시간 라벨(툴팁용). */
export function absoluteTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return iso
  return new Date(t).toLocaleString('ko-KR', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/** 이름/이메일에서 아바타 이니셜(최대 2자)을 뽑는다. */
export function initials(name: string | null | undefined, email?: string | null): string {
  const src = (name ?? '').trim() || (email ?? '').trim()
  if (!src) return '?'
  // 한글 이름은 마지막 두 글자, 영문은 각 단어 첫 글자
  if (/[가-힣]/.test(src)) return src.slice(-2)
  const parts = src.split(/[\s@._-]+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return src.slice(0, 2).toUpperCase()
}

/** Jira 원본 이슈 URL. Jira 사이트가 설정돼 있지 않으면 null(→ href 속성 생략). */
export function jiraUrl(issueKey: string): string | null {
  return jiraBrowseUrl(issueKey)
}
