<script lang="ts">
  /*
   * 좌측 사이드바 프레임 (desktop 272px, [foundation]).
   * 상단 로고 + `children` 스니펫 슬롯.
   * 내비게이션 콘텐츠(My Issues / 피드 / 최근 본 이슈 / 뷰 목록)는
   *  [explore]·[personal] 이 `components/sidebar/` 에 만들어 이 슬롯에 주입한다.
   */
  import type { Snippet } from 'svelte'

  let { children }: { children?: Snippet } = $props()
</script>

<aside
  class="issue-sidebar flex h-full flex-none flex-col border-r border-border-subtle bg-bg-panel"
>
  <!-- 로고 워드마크 (헤더 없음 — 밀도 우선) -->
  <div class="flex h-12 flex-none items-center gap-2 px-4">
    <span
      class="inline-block h-2.5 w-2.5 rounded-[3px] bg-accent"
      aria-hidden="true"
    ></span>
    <span class="text-[13px] font-semibold tracking-tight text-text-primary">scry</span>
  </div>

  <!-- 내비게이션 슬롯 -->
  <nav class="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
    {@render children?.()}
  </nav>
</aside>
