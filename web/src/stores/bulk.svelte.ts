/*
 * Issue Navigator — 멀티선택(일괄 작업) 스토어
 *
 * 단일 선택(selection: 상세 열기)과 별개로, 여러 이슈를 골라 한 번에 상태/담당자를
 *  바꾸기 위한 선택 집합이다. UI(체크박스/BulkBar)가 이 스토어만 읽고 쓴다.
 *
 *  - selected: 선택된 issue_key 집합(SvelteSet — add/delete 가 반응성 트리거).
 *  - anchorKey: 마지막으로 단일 토글한 키. shift-클릭 범위 선택의 기준점.
 *  - selectRange: 앵커~현재 행을 "현재 필터·정렬된 가시 리스트 순서"로 범위 선택.
 *      가시 순서는 filters.groups(리스트가 그리는 시각 순서와 동일)에서 파생한다.
 *  - retain: 뷰 변경 등으로 가시 리스트에서 빠진 키를 정리(호출부가 가시 키를 넘김).
 *
 * ⚠️ 실제 쓰기(상태/담당자)는 여기서 하지 않는다 — write 스토어의 per-issue
 *    옵티미스틱 메서드를 BulkBar 가 배치로 호출한다.
 */

import { SvelteSet } from 'svelte/reactivity'
import { filters } from './filters.svelte'

class BulkStore {
  /** 선택된 issue_key. */
  selected = new SvelteSet<string>()
  /** 마지막 단일 토글 키(shift 범위 선택의 앵커). */
  anchorKey = $state<string | null>(null)

  /** 선택 개수(반응형). */
  count = $derived(this.selected.size)

  /** 1개 이상 선택된 "선택 모드" 여부. */
  get active(): boolean {
    return this.selected.size > 0
  }

  has(key: string): boolean {
    return this.selected.has(key)
  }

  /** 단일 토글(있으면 해제, 없으면 선택) + 앵커 갱신. */
  toggle(key: string): void {
    if (this.selected.has(key)) this.selected.delete(key)
    else this.selected.add(key)
    this.anchorKey = key
  }

  /** 강제 선택(토글 아님) + 앵커 갱신. */
  add(key: string): void {
    this.selected.add(key)
    this.anchorKey = key
  }

  /**
   * shift-클릭 범위 선택: 앵커~target 을 가시 순서로 모두 추가.
   *  앵커가 없거나 target/앵커가 가시 리스트 밖이면 단일 추가로 폴백.
   *  앵커는 유지 — 반복 shift-클릭이 같은 기준점에서 확장되도록.
   */
  selectRange(targetKey: string): void {
    const order = orderedVisibleKeys()
    const ti = order.indexOf(targetKey)
    const ai = this.anchorKey ? order.indexOf(this.anchorKey) : -1
    if (ti === -1 || ai === -1) {
      this.add(targetKey)
      return
    }
    const [lo, hi] = ai <= ti ? [ai, ti] : [ti, ai]
    for (let i = lo; i <= hi; i++) this.selected.add(order[i])
  }

  clear(): void {
    if (this.selected.size) this.selected.clear()
    this.anchorKey = null
  }

  /** 가시 키 집합에 없는 선택은 제거(뷰/필터 변경 시 호출부가 호출). */
  retain(visibleKeys: Iterable<string>): void {
    const keep = visibleKeys instanceof Set ? visibleKeys : new Set(visibleKeys)
    for (const k of this.selected) if (!keep.has(k)) this.selected.delete(k)
    if (this.anchorKey && !keep.has(this.anchorKey)) this.anchorKey = null
  }

  /** 선택 키 스냅샷(배치 실행용). */
  keys(): string[] {
    return [...this.selected]
  }
}

/** 리스트가 그리는 시각 순서(그룹 평면화)와 동일한 가시 키 목록. */
function orderedVisibleKeys(): string[] {
  const out: string[] = []
  for (const g of filters.groups) for (const it of g.items) out.push(it.issue_key)
  return out
}

/** 앱 전역 싱글턴. */
export const bulk = new BulkStore()
