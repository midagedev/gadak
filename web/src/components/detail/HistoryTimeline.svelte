<script lang="ts">
  /*
   * 변경 이력 타임라인 ([detail]).
   * status/assignee/priority 변경을 컴팩트하게(from→to, by, 상대시간).
   * 재오픈(해결됨 상태 → 미해결 상태로의 status 전이)은 빨간 포인트로 강조한다.
   */
  import type { HistoryEntry } from '../../lib/types'
  import { relativeTime, absoluteTime } from './format'

  let { history }: { history: HistoryEntry[] } = $props()

  // 백엔드 _effective_category 의 해결 상태 집합과 동일 취지(자체 판단용)
  const RESOLVED = new Set([
    '해결됨', 'resolved', '종료', 'closed', 'done', '완료',
    'qa testing', 'qa 테스트 중', 'qa테스트중',
  ])

  function isResolved(s: string | null): boolean {
    return !!s && RESOLVED.has(s.trim().toLowerCase())
  }

  /** status 전이가 재오픈(해결→미해결)인지. */
  function isReopen(e: HistoryEntry): boolean {
    return e.field === 'status' && isResolved(e.from) && !isResolved(e.to)
  }

  /** 필드 라벨(한국어). */
  function fieldLabel(f: string): string {
    return f === 'status' ? '상태' : f === 'assignee' ? '담당자' : f === 'priority' ? '우선순위' : f
  }
</script>

{#if history.length === 0}
  <p class="text-[12px] text-text-muted italic">변경 이력 없음</p>
{:else}
  <ol class="relative flex flex-col gap-2.5 pl-4">
    <!-- 세로 가이드 라인 -->
    <span
      class="absolute top-1 bottom-1 left-[3px] w-px bg-border-subtle"
      aria-hidden="true"
    ></span>
    {#each history as e, i (i)}
      {@const reopen = isReopen(e)}
      <li class="relative">
        <!-- 타임라인 포인트 -->
        <span
          class="absolute top-[5px] -left-4 h-[7px] w-[7px] rounded-full ring-2 ring-bg-panel"
          class:bg-status-reopen={reopen}
          class:bg-border-strong={!reopen}
          aria-hidden="true"
        ></span>
        <div class="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 text-[12px]">
          <span class="font-medium text-text-secondary">{fieldLabel(e.field)}</span>
          {#if reopen}
            <span class="rounded bg-status-reopen/15 px-1 text-[10px] font-semibold text-status-reopen">
              재오픈
            </span>
          {/if}
          <span class="text-text-muted">
            {e.from ?? '없음'}
            <span class="mx-0.5 text-text-muted">→</span>
            <span class="text-text-primary">{e.to ?? '없음'}</span>
          </span>
        </div>
        <div class="text-[11px] text-text-muted">
          {#if e.by}{e.by} · {/if}<span title={absoluteTime(e.at)}>{relativeTime(e.at)}</span>
        </div>
      </li>
    {/each}
  </ol>
{/if}
