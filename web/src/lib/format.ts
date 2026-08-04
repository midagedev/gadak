/*
 * Issue Navigator — 표시 헬퍼 ([explore])
 *  상대시간 / 이니셜 / 우선순위·상태 메타 등 순수 함수. 컴포넌트 곳곳에서 재사용.
 */

import type { IssueLite } from './types'
import { effectiveCategory, type StatusCategory } from './view-config'

/* ── 상대시간 (ko) ── */

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR

/** "방금 · N분 · N시간 · N일 · N주 · N달 · N년" 형태의 압축 상대시간. */
export function relativeTime(iso: string | null): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const diff = Date.now() - t
  if (diff < MIN) return '방금'
  if (diff < HOUR) return `${Math.floor(diff / MIN)}분`
  if (diff < DAY) return `${Math.floor(diff / HOUR)}시간`
  const days = Math.floor(diff / DAY)
  if (days < 7) return `${days}일`
  if (days < 30) return `${Math.floor(days / 7)}주`
  if (days < 365) return `${Math.floor(days / 30)}달`
  return `${Math.floor(days / 365)}년`
}

/** 절대시각(툴팁용). */
export function absTime(iso: string | null): string {
  if (!iso) return ''
  const t = new Date(iso)
  if (Number.isNaN(t.getTime())) return ''
  return t.toLocaleString('ko-KR', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/* ── 이니셜(아바타 폴백) ── */

export function initials(name: string | null | undefined, email?: string | null): string {
  const src = (name || email || '').trim()
  if (!src) return '?'
  // 한글이면 마지막 두 글자(성 제외 이름), 영문이면 앞 두 단어 첫 글자
  if (/[가-힣]/.test(src)) {
    const nm = src.replace(/\s+/g, '')
    return nm.length >= 2 ? nm.slice(-2) : nm
  }
  const parts = src.split(/[\s._-]+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return src.slice(0, 2).toUpperCase()
}

/** 이메일/이름 문자열을 안정적인 색 인덱스로(아바타 배경). */
export function colorIndex(seed: string | null | undefined, buckets = 8): number {
  const s = seed ?? ''
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h) % buckets
}

/* ── 우선순위 메타 ── */

export interface PriorityMeta {
  label: string
  /** 시맨틱 색(Tailwind 임의값용 hex). */
  color: string
  /** 막대 채움 단계 1~5 (아이콘 렌더). */
  level: number
}

/** priority 문자열 → 표시 메타. 한/영 명칭 모두 대응. */
export function priorityMeta(priority: string | null): PriorityMeta {
  const p = (priority ?? '').toLowerCase()
  if (/highest|긴급|가장 높음|blocker/.test(p)) return { label: priority ?? '', color: '#ef4444', level: 5 }
  if (/high|높음|major/.test(p)) return { label: priority ?? '', color: '#f97316', level: 4 }
  if (/medium|보통|normal/.test(p)) return { label: priority ?? '', color: '#eab308', level: 3 }
  if (/lowest|가장 낮음|매우 ?낮음|trivial/.test(p)) return { label: priority ?? '', color: '#64748b', level: 1 }
  if (/low|낮음|minor/.test(p)) return { label: priority ?? '', color: '#3b82f6', level: 2 }
  return { label: priority ?? '없음', color: '#64748b', level: 0 }
}

/* ── 상태 카테고리 메타 ── */

export const CATEGORY_META: Record<StatusCategory, { label: string; color: string }> = {
  new: { label: '신규', color: 'var(--color-status-new)' },
  inprogress: { label: '진행중', color: 'var(--color-status-inprogress)' },
  done: { label: '완료', color: 'var(--color-status-done)' },
}

export function categoryOf(issue: IssueLite): StatusCategory {
  return effectiveCategory(issue)
}

/* ── 심각도 색(집계 표시용, 값 형식이 다양하므로 휴리스틱) ── */

export function severityColor(sev: string | null): string {
  const s = (sev ?? '').toLowerCase()
  if (/s1|critical|치명|blocker/.test(s)) return '#ef4444'
  if (/s2|major|높음|high/.test(s)) return '#f97316'
  if (/s3|minor|보통|medium/.test(s)) return '#eab308'
  if (/s4|trivial|낮음|low/.test(s)) return '#64748b'
  return '#9ba1a9'
}
