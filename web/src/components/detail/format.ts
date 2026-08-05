/*
 * 상세 패널 공용 포맷 헬퍼 ([detail]).
 * 상대시간 / 상태 카테고리 정규화 / 이니셜 등 순수 함수만 모은다.
 */

import { jiraBrowseUrl } from '../../lib/config'
import type { StatusCategory } from '../../lib/types'
import { absTime as i18nAbsTime, relativeTime as i18nRelativeTime } from '../../lib/i18n'

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

/** ISO8601 → relative time (long style: "3m ago"). 파싱 실패 시 원문 반환. */
export function relativeTime(iso: string | null | undefined): string {
  return i18nRelativeTime(iso, 'long')
}

/** ISO8601 → 절대시간 라벨(툴팁용). */
export function absoluteTime(iso: string | null | undefined): string {
  return i18nAbsTime(iso)
}

/** Jira 원본 이슈 URL. Jira 사이트가 설정돼 있지 않으면 null(→ href 속성 생략). */
export function jiraUrl(issueKey: string): string | null {
  return jiraBrowseUrl(issueKey)
}
