<script lang="ts">
  /*
   * 우측 상세 패널 프레임 ([foundation]).
   * `open` 이 true 일 때만 폭을 차지한다(선택 이슈 있을 때). 넓은 화면에서는
   *  grid track 을, 좁은 화면에서는 오버레이를 사용한다.
   *
   * selection 계약: 열림 여부는 [explore] 의 selection 스토어(`selectedKey`)를 구독해
   *  App.svelte 에서 `open` 으로 내려준다 — 이 컴포넌트는 표시만 담당한다.
   */
  import type { Snippet } from 'svelte'

  let { open = false, children }: { open?: boolean; children?: Snippet } = $props()
</script>

<aside
  class="issue-detail-panel h-full overflow-hidden bg-bg-panel"
  class:is-open={open}
  class:border-l={open}
  class:border-border-subtle={open}
  aria-hidden={!open}
  data-testid="issue-detail-panel"
>
  <div class="h-full w-full min-w-0 overflow-y-auto">
    {@render children?.()}
  </div>
</aside>
