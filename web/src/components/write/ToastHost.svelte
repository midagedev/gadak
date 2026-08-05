<script lang="ts">
  /*
   * Toast host (write). Bottom-right stack over write.toasts.
   *  Errors in red; others neutral. Click dismisses immediately.
   */
  import { write } from '../../stores/write.svelte'
</script>

<div
  class="pointer-events-none fixed bottom-4 right-4 z-[60] flex flex-col items-end gap-2"
  data-testid="toast-host"
>
  {#each write.toasts as toast (toast.id)}
    <button
      type="button"
      role={toast.kind === 'error' ? 'alert' : 'status'}
      aria-live={toast.kind === 'error' ? 'assertive' : 'polite'}
      onclick={() => write.dismissToast(toast.id)}
      data-testid="toast"
      class="anim-toast pointer-events-auto max-w-sm rounded-md border px-3 py-2 text-left text-[12px] shadow-xl transition-colors {toast.kind ===
      'error'
        ? 'border-status-reopen/40 bg-status-reopen/15 text-status-reopen'
        : toast.kind === 'success'
          ? 'border-status-done/40 bg-status-done/15 text-status-done'
          : 'border-border-strong bg-bg-elevated text-text-secondary'}"
    >
      {toast.message}
    </button>
  {/each}
</div>
