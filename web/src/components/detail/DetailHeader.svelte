<script lang="ts">
  /*
   * 상세 패널 헤더 ([detail]).
   * 로컬 풀의 IssueLite 만으로 즉시 렌더 가능(레이턴시 은닉). detail 로드 여부와 무관.
   * 키/제목/상태/우선순위/심각도/담당자/라벨/버전/그룹/재오픈 배지 + 닫기.
   */
  import type { IssueLite } from '../../lib/types'
  import { selection } from '../../stores/selection.svelte'
  import { me } from '../../stores/me.svelte'
  import { presence } from '../../stores/presence.svelte'
  import { feature } from '../../lib/config'
  import { jiraUrl } from './format'
  import WatchButton from '../personal/WatchButton.svelte'
  import StatusTransition from '../write/StatusTransition.svelte'
  import AssigneePicker from '../write/AssigneePicker.svelte'
  import ViewerStack from '../presence/ViewerStack.svelte'

  let { issue }: { issue: IssueLite } = $props()

  const isFavorite = $derived(me.favorites.has(issue.issue_key))
  // 본인 제외, 지금 이 이슈를 같이 보고 있는 사람들.
  const viewers = $derived(presence.viewersOf(issue.issue_key))
</script>

<header class="border-b border-border-strong/70 px-5 pt-4 pb-4">
  <!-- 상단 줄: 이슈 키(Jira 링크) + 프레즌스 + 닫기 -->
  <div class="mb-2 flex items-center justify-between gap-2">
    <div class="flex min-w-0 items-center gap-2">
      <a
        href={jiraUrl(issue.issue_key)}
        target="_blank"
        rel="noopener noreferrer"
        class="flex-none font-mono text-[12px] font-medium text-accent-text hover:underline"
        title="Jira 원본 열기"
      >
        {issue.issue_key}
      </a>

      <!-- 같이 보는 중: 초록 점 + 아바타 스택 + 라벨 (미묘한 실시간 표시) -->
      {#if feature('presence') && viewers.length > 0}
        <span class="flex min-w-0 items-center gap-1.5">
          <span
            class="h-1.5 w-1.5 flex-none rounded-full bg-status-done shadow-[0_0_5px_var(--color-status-done)]"
            aria-hidden="true"
          ></span>
          <ViewerStack {viewers} size={20} max={3} ringClass="ring-bg-panel" />
          <span class="flex-none text-[11px] text-text-muted">보는 중</span>
        </span>
      {/if}
    </div>
    <div class="flex flex-none items-center gap-1">
      <!-- 즐겨찾기 토글 -->
      <button
        type="button"
        onclick={() => me.toggleFavorite(issue.issue_key)}
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-[13px] transition-colors hover:bg-bg-hover {isFavorite
          ? 'text-status-stale'
          : 'text-text-muted hover:text-text-primary'}"
        aria-pressed={isFavorite}
        aria-label={isFavorite ? '즐겨찾기 해제' : '즐겨찾기'}
        title={isFavorite ? '즐겨찾기 해제' : '즐겨찾기'}
      >
        {isFavorite ? '★' : '☆'}
      </button>
      <!-- 워치 -->
      <WatchButton issueKey={issue.issue_key} />
      <!-- 닫기 -->
      <button
        type="button"
        onclick={() => selection.clear()}
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        aria-label="닫기"
        title="닫기 (Esc)"
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
          <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
        </svg>
      </button>
    </div>
  </div>

  <!-- 제목 -->
  <h2 class="mb-3 text-[16px] leading-snug font-semibold text-text-primary">
    {issue.summary}
  </h2>

  <!-- 메타 칩 줄 -->
  <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
    <!-- 상태 (클릭 → 전환 드롭다운) -->
    <StatusTransition {issue} />

    {#if issue.issue_type}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{issue.issue_type}</span>
    {/if}
    {#if issue.priority}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">우선 {issue.priority}</span>
    {/if}
    {#if issue.severity}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">심각도 {issue.severity}</span>
    {/if}
    {#if issue.d1_group}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{issue.d1_group}</span>
    {/if}

    <!-- 재오픈 배지 -->
    {#if issue.reopen_count > 0}
      <span
        class="inline-flex items-center gap-1 rounded-md bg-status-reopen/15 px-2 py-0.5 font-semibold text-status-reopen"
        title={issue.reopen_reason ?? '재오픈됨'}
      >
        재오픈 ×{issue.reopen_count}
      </span>
    {/if}
  </div>

  <!-- 담당자 + 라벨/버전 -->
  <div class="mt-3 flex flex-col gap-2 text-[12px] text-text-muted">
    <!-- 담당자 (클릭 → 지정 팝오버, 미할당도 지정 가능) -->
    <AssigneePicker {issue} />
    {#if issue.fix_versions.length > 0}
      <div class="flex items-start gap-1.5">
        <span class="w-12 flex-none pt-0.5 text-text-muted">버전</span>
        <span class="flex flex-wrap gap-1">
          {#each issue.fix_versions as v (v)}
            <span class="rounded bg-bg-elevated px-1.5 py-0.5 text-text-secondary">{v}</span>
          {/each}
        </span>
      </div>
    {/if}
    {#if issue.labels.length > 0}
      <div class="flex items-start gap-1.5">
        <span class="w-12 flex-none pt-0.5 text-text-muted">라벨</span>
        <span class="flex flex-wrap gap-1">
          {#each issue.labels as l (l)}
            <span class="rounded bg-bg-elevated px-1.5 py-0.5 text-text-secondary">{l}</span>
          {/each}
        </span>
      </div>
    {/if}
  </div>
</header>
