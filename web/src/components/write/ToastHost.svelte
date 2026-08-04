<script lang="ts">
  /*
   * 토스트 호스트 (쓰기). 우하단 스택. write.toasts 를 렌더한다.
   *  에러는 붉은 계열로 눈에 띄게, 그 외는 중립. 클릭 시 즉시 닫기.
   */
  import { write } from '../../stores/write.svelte'
</script>

<div class="pointer-events-none fixed bottom-4 right-4 z-[60] flex flex-col items-end gap-2">
  {#each write.toasts as t (t.id)}
    <button
      type="button"
      onclick={() => write.dismissToast(t.id)}
      class="anim-enter pointer-events-auto max-w-sm rounded-md border px-3 py-2 text-left text-[12px] shadow-xl transition-colors {t.kind ===
      'error'
        ? 'border-status-reopen/40 bg-status-reopen/15 text-status-reopen'
        : t.kind === 'success'
          ? 'border-status-done/40 bg-status-done/15 text-status-done'
          : 'border-border-strong bg-bg-elevated text-text-secondary'}"
    >
      {t.message}
    </button>
  {/each}
</div>
