<script module lang="ts">
  /** Published once on mount. BrowsePane reads this instead of querySelector. */
  export const toastHostSlot: { el: HTMLElement | null } = { el: null }
</script>

<script lang="ts">
  /*
   * Toast host (write). Bottom-right stack over write.toasts.
   *  Each kind carries a registry glyph so success and error stay
   *  distinguishable when the done/reopen tokens collapse under
   *  deuteranopia. Click dismisses immediately.
   */
  import { write, type ToastKind } from '../../stores/write.svelte'
  import { openIssueOrigin } from '../../lib/desktop-links'
  import Icon, { type IconName } from '../ui/Icon.svelte'

  const TOAST_ICON: Record<ToastKind, IconName> = {
    success: 'check-circle',
    error: 'warning',
    info: 'info',
  }

  let hostEl = $state<HTMLDivElement | null>(null)
  $effect(() => {
    toastHostSlot.el = hostEl
    return () => {
      if (toastHostSlot.el === hostEl) toastHostSlot.el = null
    }
  })
</script>

<div
  bind:this={hostEl}
  class="pointer-events-none fixed bottom-4 right-4 z-[60] flex flex-col items-end gap-2"
  data-testid="toast-host"
>
  {#each write.toasts as toast (toast.id)}
    <div
      role={toast.kind === 'error' ? 'alert' : 'status'}
      aria-live={toast.kind === 'error' ? 'assertive' : 'polite'}
      data-testid="toast"
      class="anim-toast pointer-events-auto inline-flex max-w-sm items-center gap-1.5 rounded-lg border px-3 py-2 text-left text-[12px] shadow-overlay {toast.kind ===
      'error'
        ? 'border-status-reopen/40 bg-status-reopen/15 text-status-reopen'
        : toast.kind === 'success'
          ? 'border-status-done/40 bg-status-done/15 text-status-done'
          : 'border-border-strong bg-bg-elevated text-text-secondary'}"
    >
      <button
        type="button"
        onclick={() => write.dismissToast(toast.id)}
        class="inline-flex min-w-0 items-center gap-1.5 text-left"
      >
        <span class="flex-none" data-testid="toast-icon" data-icon={TOAST_ICON[toast.kind]}>
          <Icon name={TOAST_ICON[toast.kind]} size={14} />
        </span>
        <span class="min-w-0">{toast.message}</span>
      </button>
      {#if toast.action}
        <button
          type="button"
          data-testid="toast-action"
          class="flex-none text-micro font-medium underline"
          onclick={() => {
            const action = toast.action
            if (action) openIssueOrigin(action.openIssueKey)
            write.dismissToast(toast.id)
          }}
        >
          {toast.action.label}
        </button>
      {/if}
    </div>
  {/each}
</div>
