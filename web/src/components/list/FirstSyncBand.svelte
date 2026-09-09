<script lang="ts">
  /*
   * First-sync band (GDK-1677) — the one line that says a half-filled list
   * is still filling. A first full sync commits pages newest-first, so the
   * list grows while it runs; without this band a 1,200-of-3,514 screen
   * reads as "this project has 1,200 issues". The store is the only
   * decider: issues.firstSync is null whenever the server does not send
   * first_sync (older server, no first sync), so absence renders nothing —
   * there is deliberately no "done" state here; the finished sync speaks
   * through the freshness chip like any other.
   */
  import { issues } from '../../stores/issues.svelte'
  import { firstSyncLine } from './first-sync-band'

  const fs = $derived(issues.firstSync)
  const line = $derived(fs ? firstSyncLine(fs) : '')
</script>

{#if fs}
  <div
    class="flex w-full flex-none items-center gap-1.5 px-3 py-1.5 text-micro text-text-muted"
    data-testid="first-sync-band"
    data-phase={fs.phase}
    role="status"
    aria-live="polite"
  >
    <!-- FreshnessChip's syncing spinner, reused as-is; motion-safe so a
         reduced-motion preference keeps the line and drops the spin. -->
    <svg
      class="h-3 w-3 flex-none motion-safe:animate-spin"
      viewBox="0 0 12 12"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="6" cy="6" r="4.5" stroke="currentColor" stroke-width="1.5" opacity="0.3" />
      <path
        d="M10.5 6A4.5 4.5 0 0 0 6 1.5"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linecap="round"
      />
    </svg>
    <span class="truncate tabular-nums">{line}</span>
  </div>
{/if}
