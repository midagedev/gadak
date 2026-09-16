<script lang="ts">
  /*
   * The phone's one toast surface (GDK-1504), mounted once in App.svelte.
   * It sits at the TOP (GDK-1969): every bottom surface — the sheet, the
   * composer, the tab bar — is thumb territory, and a write's confirmation
   * must not share the sheet's exit lane. Detail's onWritten closes the
   * sheet and announces on the next line, so a bottom-anchored toast rose
   * exactly where the sheet was falling out (fly y:320) and read as part
   * of its exit. The inset rides the --safe-top token — app.css owns env()
   * alone (DESIGN.md §4.1).
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
      transition:fly={{ y: reduceMotion ? 0 : -10, duration: reduceMotion ? 0 : 120 }}
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
    /* 8px below the notch/status bar, never the bare token: the same idea
       as the tab bar's .safe-bottom floor, so the pill never touches the
       system chrome on an inset-less canvas. */
    top: calc(var(--safe-top) + 8px);
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
    border: 1px solid transparent;
    /* Inverted ink (GDK-1969): --color-bg-elevated on --color-bg-base
       measured ≈1.2:1 — invisible. text-primary on bg-base is ≈15:1 light,
       ≈16:1 dark, for both the pill-on-page and the text-on-pill pair. */
    background: var(--color-text-primary);
    font-size: var(--text-micro);
    color: var(--color-bg-base);
    text-align: center;
  }
  .toast[data-kind='error'] .pill {
    /* The spine marks the error kind; reopen red as text on the inverted
       ground fails contrast in dark, so the red is geometry, not ink. */
    box-shadow: inset 4px 0 0 var(--color-status-reopen);
    padding-left: 20px;
  }
</style>
