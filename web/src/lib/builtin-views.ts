/*
 * 기본 제공 뷰 ([explore])
 *
 * 사이드바 "기본 뷰" 섹션에 노출되는 프리셋. 각 뷰는 완전한 ViewConfig(필터+디스플레이)를
 * 갖고, 클릭 시 filters.applyConfig 로 통째 적용된다.
 *
 * 규율: 프리셋은 **테넌트 중립적인 축만** 쓴다.
 *  - status_category(new/inprogress/done)·unassigned·reopened·stale·updated_from 은
 *    Jira 어디서나 같은 의미다.
 *  - status 이름, priority 이름, issue_type 이름, 프로젝트 키는 사이트마다(심지어 계정
 *    언어마다) 달라서 프리셋에 박으면 다른 사이트에서 빈 결과가 된다. 그런 축은 사용자가
 *    직접 필터를 걸어 저장 뷰로 만들면 된다.
 *
 * 날짜 의존 뷰(이번 주 해결됨)는 매 호출 시점 기준으로 재계산하므로 함수로 노출한다.
 */

import { emptyConfig, type ViewConfig } from './view-config'

export interface BuiltinView {
  id: string // 사이드바 활성 표시 및 안정 키
  icon: string // 이모지 마커
  name: string
  hint?: string
  config: ViewConfig
}

/** 이번 주 월요일 00:00 (로컬) 의 ISO 문자열(YYYY-MM-DD). */
function startOfWeekISO(): string {
  const now = new Date()
  const day = (now.getDay() + 6) % 7 // 월=0
  const mon = new Date(now.getFullYear(), now.getMonth(), now.getDate() - day)
  return `${mon.getFullYear()}-${String(mon.getMonth() + 1).padStart(2, '0')}-${String(mon.getDate()).padStart(2, '0')}`
}

/** config 조립 헬퍼 — 빈 config 에서 부분만 덮어쓴다. */
function make(over: {
  filters?: Partial<ViewConfig['filters']>
  display?: Partial<ViewConfig['display']>
}): ViewConfig {
  const c = emptyConfig()
  if (over.filters) Object.assign(c.filters, over.filters)
  if (over.display) Object.assign(c.display, over.display)
  return c
}

export function builtinViews(): BuiltinView[] {
  return [
    {
      id: 'all-open',
      icon: '📋',
      name: '전체 미해결',
      hint: '신규 + 진행중',
      config: make({ filters: { status_category: ['new', 'inprogress'] } }),
    },
    {
      id: 'unassigned-new',
      icon: '🆕',
      name: '미할당 신규',
      hint: '담당자 없는 신규',
      config: make({
        filters: { unassigned: true, status_category: ['new'] },
        display: { sort: 'created', dir: 'desc' },
      }),
    },
    {
      id: 'reopened',
      icon: '🔁',
      name: '재오픈 이슈',
      hint: '완료 후 다시 열린 이슈',
      config: make({
        filters: { reopened: true },
        display: { sort: 'reopen_count', dir: 'desc' },
      }),
    },
    {
      id: 'stale',
      icon: '⏳',
      name: '정체 이슈',
      hint: '한 상태에 오래 머문 이슈',
      config: make({ filters: { stale: true }, display: { sort: 'updated', dir: 'asc' } }),
    },
    {
      id: 'recently-updated',
      icon: '⚡',
      name: '최근 갱신',
      hint: '미해결 · 갱신 최신순',
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
    {
      id: 'resolved-week',
      icon: '✅',
      name: '이번 주 해결됨',
      hint: '월요일 이후 갱신',
      config: make({
        filters: { status_category: ['done'], updated_from: startOfWeekISO() },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
  ]
}
