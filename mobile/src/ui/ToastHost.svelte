<script lang="ts">
  /*
   * The phone's one toast surface (GDK-1504), mounted once in App.svelte.
   * Geometry inherited from the pill it replaces: centered, above whatever
   * bottom chrome is up (composer slab in Detail, tab bar in the tabs),
   * over the detail layer (z 20). The inset rides the --safe-bottom token
   * — app.css owns env() alone (DESIGN.md §4.1).
   *
   * Auto-dismiss is the primary path (lib/toast.svelte.ts); a tap is the
   * early way out, so the pill is a real button at the 44pt floor (§4.8),
   * not a smaller-than-floor convenience target.
   */
  import { fly } from 'svelte/transition'
  import { dismissToast, toastHost } from '../lib/toast.svelte'

  const reduceMotion =
    typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches
</script>

<div class="toast-host" data-testid="toast-host">
  {#each toastHost.toasts as toast (toast.id)}
    <div
      class="toast"
      data-kind={toast.kind}
      data-testid="toast"
      role={toast.kind === 'error' ? 'alert' : 'status'}
      aria-live={toast.kind === 'error' ? 'assertive' : 'polite'}
      transition:fly={{ y: reduceMotion ? 0 : 10, duration: reduceMotion ? 0 : 120 }}
    >
      <button type="button" class="pill" onclick={() => dismissToast(toast.id)}>
        {toast.message}
      </button>
    </div>
  {/each}
</div>

<style>
  .toast-host {
    position: fixed;
    left: 0;
    right: 0;
    /* max(), not the token alone: the same floor the tab bar's .safe-bottom
       uses, so the pill never rides below it on a home-indicator-less
       canvas. */
    bottom: calc(max(var(--safe-bottom), 12px) + 76px);
    z-index: 40;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
  }
  .pill {
    display: flex;
    align-items: center;
    min-height: var(--spacing-control);
    padding: 0 16px;
    border-radius: 9999px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    font-size: var(--text-micro);
    color: var(--color-text-primary);
    text-align: center;
  }
  .toast[data-kind='error'] .pill {
    color: var(--color-status-reopen);
    border-color: var(--color-status-reopen);
  }
</style>
