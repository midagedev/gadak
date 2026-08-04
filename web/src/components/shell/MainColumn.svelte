<script lang="ts">
  /*
   * 중앙 메인 컬럼 프레임 ([foundation], flex-1).
   * 상단 도구줄(검색/필터/디스플레이) + 이슈 리스트는 [explore] 가 이 슬롯에 채운다.
   */
  import type { Snippet } from 'svelte'

  let { children }: { children?: Snippet } = $props()
</script>

<main class="issue-main-column flex min-w-0 flex-1 flex-col bg-bg-base">
  {@render children?.()}
</main>
