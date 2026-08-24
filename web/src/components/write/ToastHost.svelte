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
  import { onMount } from 'svelte'
  import { write, type ToastKind } from '../../stores/write.svelte'
  import { mediaViewer } from '../../stores/media-viewer.svelte'
  import { openIssueOrigin } from '../../lib/desktop-links'
  import { isEditableTarget } from '../../lib/keymap.svelte'
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

  // Capture, and stopPropagation, for the same reasons MediaViewer does (it
  // mounts after App's keymap, this host after App's window template): the
  // dialogs' <svelte:window> Esc handlers do not check defaultPrevented, so
  // only stopping the event keeps one Esc from closing a toast *and* the
  // dialog under it. Declines: an already-spent key, an Esc typed into a
  // field (the keymap's own convention), the media viewer (z-70 sits above
  // this stack and registers its capture listener later), and an empty
  // stack — then the Esc flows through the existing chain untouched.
  onMount(() => {
    function onWin(e: KeyboardEvent) {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      if (isEditableTarget(e.target)) return
      if (mediaViewer.attachment) return
      const top = write.toasts[write.toasts.length - 1]
      if (!top) return
      e.preventDefault()
      e.stopPropagation()
      write.dismissToast(top.id)
    }
    window.addEventListener('keydown', onWin, true)
    return () => window.removeEventListener('keydown', onWin, true)
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
