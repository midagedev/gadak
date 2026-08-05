/*
 * Issue Navigator — 표시 헬퍼 ([explore])
 *  상대시간 / 이니셜 / 우선순위·상태 메타 등 순수 함수. 컴포넌트 곳곳에서 재사용.
 */

import type { IssueLite } from './types'
import { effectiveCategory, type StatusCategory } from './view-config'
import {
  absTime as i18nAbsTime,
  categoryLabel,
  relativeTime as i18nRelativeTime,
  t,
} from './i18n'

/* ── 상대시간 (i18n 단일 구현, compact) ── */

/** "just now · 3m · 2h · 1d …" compact relative time. */
export function relativeTime(iso: string | null): string {
  return i18nRelativeTime(iso, 'compact')
}

/** 절대시각(툴팁용). */
export function absTime(iso: string | null): string {
  return i18nAbsTime(iso)
}

/* ── 검색어 하이라이팅 ── */

/** 대소문자 무시 부분일치 조각 분해. 초성/키 단축형 매칭은 조각이 없어 하이라이트 생략. */
export function highlightSegments(text: string, q: string): { text: string; hit: boolean }[] {
  const needle = q.trim().toLowerCase()
  if (!needle) return [{ text, hit: false }]
  const lower = text.toLowerCase()
  const out: { text: string; hit: boolean }[] = []
  let i = 0
  for (;;) {
    const at = lower.indexOf(needle, i)
    if (at < 0) break
    if (at > i) out.push({ text: text.slice(i, at), hit: false })
    out.push({ text: text.slice(at, at + needle.length), hit: true })
    i = at + needle.length
  }
  if (out.length === 0) return [{ text, hit: false }]
  if (i < text.length) out.push({ text: text.slice(i), hit: false })
  return out
}

/* ── 이니셜(아바타 폴백) ── */

export function initials(name: string | null | undefined, email?: string | null): string {
  const raw = (name || email || '').trim()
  if (!raw) return '?'
  // 이메일이면 로컬 파트만 쓴다. 도메인까지 토큰으로 세면 marco@x.io 의 이니셜이
  // 사람과 무관한 'MX'(또는 TLD 를 집어 'MI')가 된다 — 아바타는 사람을 가리켜야 한다.
  const at = raw.indexOf('@')
  const src = at > 0 ? raw.slice(0, at) : raw
  // 한글이면 공백 제거 후 마지막 두 글자(성 제외 이름), 영문이면 앞 두 토큰 첫 글자.
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
  return { label: priority ?? t('common.none'), color: '#64748b', level: 0 }
}

/* ── 상태 카테고리 메타 ── */

export function categoryMetaOf(cat: StatusCategory): { label: string; color: string } {
  const color =
    cat === 'new'
      ? 'var(--color-status-new)'
      : cat === 'inprogress'
        ? 'var(--color-status-inprogress)'
        : 'var(--color-status-done)'
  return { label: categoryLabel(cat), color }
}

/** @deprecated prefer categoryMetaOf — kept as getter-style for call sites. */
export const CATEGORY_META: Record<StatusCategory, { get label(): string; color: string }> = {
  new: {
    get label() {
      return categoryLabel('new')
    },
    color: 'var(--color-status-new)',
  },
  inprogress: {
    get label() {
      return categoryLabel('inprogress')
    },
    color: 'var(--color-status-inprogress)',
  },
  done: {
    get label() {
      return categoryLabel('done')
    },
    color: 'var(--color-status-done)',
  },
}

export function categoryOf(issue: IssueLite): StatusCategory {
  return effectiveCategory(issue)
}
