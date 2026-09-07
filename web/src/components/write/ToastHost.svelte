<script module lang="ts">
  /** Published once on mount. BrowsePane reads this instead of querySelector. */
  export const toastHostSlot: { el: HTMLElement | null } = { el: null }
</script>

<script lang="ts">
  /*
   * Toast host (write). Bottom-right stack over write.toasts.
   *  Each kind carries a registry glyph so success and error stay
   *  distinguishable when the done/reopen tokens collapse under
   *  deuteranopia. Click dismisses immediately; Esc dismisses the
   *  top toast and spends the key there (GDK-829), so the surface
   *  underneath keeps its own Esc.
   */
  import { write, type ToastKind } from '../../stores/write.svelte'
  import { mediaViewer } from '../../stores/media-viewer.svelte'
  import { openIssueOrigin } from '../../lib/desktop-links'
  import { isEditableTarget } from '../../lib/keymap.svelte'
  import { ESC_TIER, isEscapeKey, onEscape } from '../../lib/dom-actions'
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

  // A capture claim, one tier under the media viewer's, on the shared Esc
  // stack (lib/dom-actions.ts). stopPropagation stays this handler's own
  // choice: stopping the phase at window keeps the bubble walk (and so the
  // dialogs below) from hearing the key — one Esc dismisses the toast and
  // nothing else. Declines: an already-spent key, an Esc typed into a field
  // (the keymap's own convention), the media viewer (tier-redundant now that
  // overlay outranks toast, kept because the handler owns its own declines),
  // and an empty stack — then the Esc flows through the existing chain
  // untouched.
  function onToastEsc(e: KeyboardEvent) {
    if (!isEscapeKey(e) || e.defaultPrevented) return
    if (isEditableTarget(e.target)) return
    if (mediaViewer.attachment) return
    const top = write.toasts[write.toasts.length - 1]
    if (!top) return
    e.preventDefault()
    e.stopPropagation()
    write.dismissToast(top.id)
  }
</script>

<div
  bind:this={hostEl}
  class="pointer-events-none fixed bottom-4 right-4 z-[60] flex flex-col items-end gap-2"
  data-testid="toast-host"
  use:onEscape={{ handler: onToastEsc, phase: 'capture', priority: ESC_TIER.toast, label: 'toast-host' }}
>
  {#each write.toasts as toast (toast.id)}
    <div
      role={toast.kind === 'error' ? 'alert' : 'status'}
      aria-live={toast.kind === 'error' ? 'assertive' : 'polite'}
      data-testid="toast"
      class="anim-toast pointer-events-auto inline-flex max-w-sm items-center gap-1.5 rounded-lg border px-3 py-2 text-left text-body shadow-overlay {toast.kind ===
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
