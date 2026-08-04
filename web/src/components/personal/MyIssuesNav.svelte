<script lang="ts">
  /*
   * My Issues 사이드바 섹션 ([personal]).
   *  항목: 내 담당 N / 내가 보고 N / 나를 멘션(피드) N.
   *   - 담당: filters.applyConfig(assignee + 활성 상태) → 메인 리스트.
   *   - 보고: reporter 는 explore 필터 스키마에 없어(계약상 assignee_email 만 지원),
   *           개인 피드의 "보고" 초점 탭으로 연다(피드가 reporter 관계를 직접 계산).
   *   - 멘션/피드: 개인 피드(전체 초점) 열기.
   *  카운트는 로컬 풀에서 $derived(멘션은 API 결과 수).
   *  비로그인 시엔 로그인 유도 문구만 노출.
   */
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { effectiveCategory, emptyConfig, type ViewConfig } from '../../lib/view-config'

  const myEmail = $derived(me.email)

  // 활성(미완료) 기준 카운트.
  const assignedCount = $derived(
    myEmail
      ? issues.allIssues.filter(
          (i) => i.assignee_email === myEmail && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const reportedCount = $derived(
    myEmail
      ? issues.allIssues.filter(
          (i) => i.reporter_email === myEmail && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const feedUnreadCount = $derived(me.feedUnread.all)

  function assigneeConfig(): ViewConfig {
    const c = emptyConfig()
    c.filters.assignee_email = [myEmail!]
    c.filters.status_category = ['new', 'inprogress']
    return c
  }

  function applyAssignee() {
    me.closeFeed()
    filters.applyConfig(assigneeConfig())
  }
</script>

<div class="mb-3">
  <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
    My Issues
  </div>

  {#if me.authed}
    <button
      type="button"
      class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={applyAssignee}
    >
      <span class="flex-none">🙋</span>
      <span class="min-w-0 flex-1 truncate">내 담당</span>
      <span class="flex-none text-[11px] text-text-muted">{assignedCount}</span>
    </button>

    <button
      type="button"
      class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {me.feedOpen &&
      me.feedFocus === 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      onclick={() => me.openFeed('reporter')}
    >
      <span class="flex-none">✍️</span>
      <span class="min-w-0 flex-1 truncate">내가 보고</span>
      <span class="flex-none text-[11px] text-text-muted">{reportedCount}</span>
    </button>

    <button
      type="button"
      class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {me.feedOpen &&
      me.feedFocus !== 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      onclick={() => me.openFeed('all')}
      title="내 이슈 변화 + 나를 멘션한 코멘트"
    >
      <span class="flex-none">📣</span>
      <span class="min-w-0 flex-1 truncate">피드</span>
      {#if feedUnreadCount}
        <span
          class="min-w-5 flex-none rounded-full bg-accent px-1.5 py-0.5 text-center text-[10px] font-semibold text-white"
        >{feedUnreadCount > 99 ? '99+' : feedUnreadCount}</span>
      {/if}
    </button>
  {:else}
    <button
      type="button"
      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-secondary"
      onclick={() => me.promptLogin()}
    >
      로그인하면 내 담당·보고·멘션이 여기 모입니다 →
    </button>
  {/if}
</div>
