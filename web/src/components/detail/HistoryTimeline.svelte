<script lang="ts">
  /*
   * 변경 이력 타임라인 ([detail]).
   * status/assignee/priority 변경을 컴팩트하게(from→to, by, 상대시간).
   * 재오픈(해결됨 상태 → 미해결 상태로의 status 전이)은 빨간 포인트로 강조한다.
   */
  import { t } from '../../lib/i18n'
  import type { HistoryEntry } from '../../lib/types'
  import { RESOLVED_STATUS_NAMES } from '../../lib/view-config'
  import { relativeTime, absoluteTime } from './format'

  let { history }: { history: HistoryEntry[] } = $props()

  function isResolved(s: string | null): boolean {
    return !!s && RESOLVED_STATUS_NAMES.has(s.trim().toLowerCase())
  }

  /**
   * status 전이가 재오픈(해결→미해결)인지. 서버가 전/후 카테고리를 주면 그걸 쓰고,
   * 없을 때만 상태 이름으로 추정한다(사이트마다 이름이 달라 폴백일 뿐).
   */
  function isReopen(e: HistoryEntry): boolean {
    if (e.field !== 'status') return false
    if (e.from_category || e.to_category) return e.from_category === 'done' && e.to_category !== 'done'
    return isResolved(e.from) && !isResolved(e.to)
  }

  /** 필드 라벨(한국어). */
  function fieldLabel(f: string): string {
    return f === 'status' ? t('common.status') : f === 'assignee' ? t('common.assignee') : f === 'priority' ? t('common.priority') : f
  }
</script>

{#if history.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noHistory')}</p>
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
              {t('feed.kindReopen')}
            </span>
          {/if}
          <span class="text-text-muted">
            {e.from ?? t('common.none')}
            <span class="mx-0.5 text-text-muted">→</span>
            <span class="text-text-primary">{e.to ?? t('common.none')}</span>
          </span>
        </div>
        <div class="text-[11px] text-text-muted">
          {#if e.by}{e.by} · {/if}<span title={absoluteTime(e.at)}>{relativeTime(e.at)}</span>
        </div>
      </li>
    {/each}
  </ol>
{/if}
