<script lang="ts">
  /*
   * 가상 이슈 리스트 ([explore]) — 자체 구현.
   *  - 고정 행높이 42px(헤더/이슈 동일) → 균일 가상화(DOM 노드 = 뷰포트 행 수).
   *  - 그룹 헤더는 상단 고정(floating) + 다음 헤더가 밀어올리는 push 효과.
   *  - 키보드 보조: j/k 커서 이동, Enter 선택, Esc 선택 해제(입력 포커스 중엔 무시).
   *  성능 규율: 스크롤은 top(translate) 계산만, 재계산 없음.
   */
  import { onMount } from 'svelte'
  import { filters, type IssueGroup } from '../../stores/filters.svelte'
  import { selection } from '../../stores/selection.svelte'
  import type { IssueLite } from '../../lib/types'
  import IssueRow from './IssueRow.svelte'
  import GroupHeader from './GroupHeader.svelte'

  const ROW_H = 42
  const OVERSCAN = 8

  type RowItem =
    | { type: 'header'; group: IssueGroup }
    | { type: 'issue'; issue: IssueLite }

  const grouped = $derived(filters.display.group_by !== 'none')

  // 시각 순서 평면화: (헤더) + 그룹 아이템. group_by=none 이면 단일 그룹(헤더 없음).
  const rows = $derived.by(() => {
    const out: RowItem[] = []
    for (const g of filters.groups) {
      if (grouped) out.push({ type: 'header', group: g })
      for (const it of g.items) out.push({ type: 'issue', issue: it })
    }
    return out
  })

  // 커서 내비게이션용 이슈 키(시각 순서)와 행 인덱스 매핑
  const issueRowIndex = $derived.by(() => {
    const m = new Map<string, number>()
    rows.forEach((r, i) => {
      if (r.type === 'issue') m.set(r.issue.issue_key, i)
    })
    return m
  })
  const issueKeys = $derived([...issueRowIndex.keys()])

  let scrollTop = $state(0)
  let viewportH = $state(0)
  let scroller = $state<HTMLDivElement | null>(null)
  let cursorKey = $state<string | null>(null)

  const total = $derived(rows.length * ROW_H)
  const start = $derived(Math.max(0, Math.floor(scrollTop / ROW_H) - OVERSCAN))
  const end = $derived(
    Math.min(rows.length, Math.ceil((scrollTop + viewportH) / ROW_H) + OVERSCAN),
  )
  const slice = $derived(rows.slice(start, end))

  // ── floating 그룹 헤더(현재 최상단 그룹 + push 효과) ──
  const firstRow = $derived(Math.floor(scrollTop / ROW_H))
  const floatingGroup = $derived.by(() => {
    if (!grouped || rows.length === 0) return null
    for (let i = Math.min(firstRow, rows.length - 1); i >= 0; i--) {
      const r = rows[i]
      if (r.type === 'header') return { group: r.group, headerIndex: i }
    }
    return null
  })
  // 다음 헤더가 다가오면 위로 밀어 올린다.
  const floatingOffset = $derived.by(() => {
    if (!floatingGroup) return 0
    for (let i = floatingGroup.headerIndex + 1; i < rows.length; i++) {
      if (rows[i].type === 'header') {
        const nextTop = i * ROW_H
        const delta = nextTop - scrollTop
        return delta < ROW_H ? delta - ROW_H : 0
      }
    }
    return 0
  })

  function onScroll(e: Event) {
    scrollTop = (e.currentTarget as HTMLDivElement).scrollTop
  }

  function scrollToRow(rowIndex: number) {
    if (!scroller) return
    const top = rowIndex * ROW_H
    const bottom = top + ROW_H
    // 그룹 모드면 상단 고정 헤더(1행) 높이만큼 여유
    const pad = grouped ? ROW_H : 0
    if (top - pad < scroller.scrollTop) scroller.scrollTop = top - pad
    else if (bottom > scroller.scrollTop + viewportH) scroller.scrollTop = bottom - viewportH
  }

  function moveCursor(delta: number) {
    if (issueKeys.length === 0) return
    let idx = cursorKey ? issueKeys.indexOf(cursorKey) : -1
    idx = idx === -1 ? (delta > 0 ? 0 : issueKeys.length - 1) : idx + delta
    idx = Math.max(0, Math.min(issueKeys.length - 1, idx))
    cursorKey = issueKeys[idx]
    const rowIdx = issueRowIndex.get(cursorKey)
    if (rowIdx != null) scrollToRow(rowIdx)
  }

  function inEditable(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null
    if (!el) return false
    const tag = el.tagName
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable
  }

  function onKey(e: KeyboardEvent) {
    if (inEditable(e.target)) return
    if (e.key === 'j') {
      e.preventDefault()
      moveCursor(1)
    } else if (e.key === 'k') {
      e.preventDefault()
      moveCursor(-1)
    } else if (e.key === 'Enter') {
      if (cursorKey) {
        e.preventDefault()
        selection.select(cursorKey)
      }
    } else if (e.key === 'Escape' || e.key === 'x') {
      if (selection.selectedKey) {
        e.preventDefault()
        selection.clear()
      }
    }
  }

  // 뷰(필터/그룹/정렬)가 실제로 바뀌면 최상단으로 리셋(데이터 delta 에는 반응 안 함).
  $effect(() => {
    void filters.viewKey
    if (scroller) scroller.scrollTop = 0
    scrollTop = 0
    cursorKey = null
  })

  onMount(() => {
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
</script>

<div class="relative h-full">
  <!-- 고정 그룹 헤더(floating) -->
  {#if floatingGroup}
    <div
      class="pointer-events-none absolute inset-x-0 top-0 z-10"
      style:transform="translateY({floatingOffset}px)"
    >
      <div class="pointer-events-auto border-b border-border-subtle shadow-sm shadow-black/20">
        <GroupHeader
          group={floatingGroup.group}
          floating
          showCategoryCounts={filters.display.group_by !== 'status_category'}
        />
      </div>
    </div>
  {/if}

  <div
    bind:this={scroller}
    bind:clientHeight={viewportH}
    onscroll={onScroll}
    data-testid="issue-list-scroller"
    class="h-full overflow-y-auto"
  >
    <div class="relative" style:height="{total}px">
      {#each slice as row, i (start + i + (row.type === 'issue' ? row.issue.issue_key : 'h' + row.group.key))}
        <div class="absolute inset-x-0" style:top="{(start + i) * ROW_H}px" style:height="{ROW_H}px">
          {#if row.type === 'header'}
            <GroupHeader
              group={row.group}
              showCategoryCounts={filters.display.group_by !== 'status_category'}
            />
          {:else}
            <IssueRow
              issue={row.issue}
              active={selection.selectedKey === row.issue.issue_key}
              cursor={cursorKey === row.issue.issue_key}
            />
          {/if}
        </div>
      {/each}
    </div>
  </div>
</div>
