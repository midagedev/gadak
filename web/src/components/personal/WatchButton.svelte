<script lang="ts">
  /*
   * 워치 토글 버튼 ([personal]). 눈 아이콘 + watching 상태 표시.
   *  - 로그인: 옵티미스틱 토글(me.toggleWatch, 실패 시 롤백).
   *  - 비로그인: 클릭 시 로그인 유도 콜백(onNeedLogin) 호출.
   */
  import { me } from '../../stores/me.svelte'

  let { issueKey }: { issueKey: string } = $props()

  const watching = $derived(me.watches.has(issueKey))
  let busy = $state(false)

  async function onClick(e: MouseEvent) {
    e.stopPropagation()
    if (!me.authed) {
      me.promptLogin()
      return
    }
    if (busy) return
    busy = true
    await me.toggleWatch(issueKey)
    busy = false
  }
</script>

<button
  type="button"
  onclick={onClick}
  class="flex h-6 items-center gap-1 rounded-md px-2 text-[11px] font-medium transition-colors {watching
    ? 'bg-accent-subtle/40 text-accent-text hover:bg-accent-subtle/60'
    : 'text-text-muted hover:bg-bg-hover hover:text-text-primary'}"
  title={me.authed
    ? watching
      ? '워치 해제 — 상태 변경/코멘트/재오픈 알림을 받고 있습니다'
      : '워치 — 변경 시 Teams 알림'
    : '로그인하면 워치할 수 있습니다'}
  aria-pressed={watching}
>
  {#if watching}
    <!-- 뜬 눈 -->
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8s-2.4 4.5-6.5 4.5S1.5 8 1.5 8Z"
        stroke="currentColor"
        stroke-width="1.3"
      />
      <circle cx="8" cy="8" r="2" fill="currentColor" />
    </svg>
    <span>워치 중</span>
  {:else}
    <!-- 감은 눈 -->
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8s-2.4 4.5-6.5 4.5S1.5 8 1.5 8Z"
        stroke="currentColor"
        stroke-width="1.3"
      />
      <circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.3" />
    </svg>
    <span>워치</span>
  {/if}
</button>
