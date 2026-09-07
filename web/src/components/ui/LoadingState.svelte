<script lang="ts">
  /*
   * Transient loading copy. Same geometry as EmptyState (center column, px-6,
   * centered) with pulse on the label. Callers pass a translated `label`;
   * the default is common.loading.
   *
   * `compact` is the dropdown-menu row variant (GDK-1566): the px-3 py-2
   * text-micro muted row the bulk bar and the pickers used to hand-roll —
   * one markup owner now, so the grace/skeleton sweep sees every wait.
   */
  import { t } from '../../lib/i18n'

  let { label = t('common.loading'), compact = false }: { label?: string; compact?: boolean } =
    $props()
</script>

{#if compact}
  <div class="px-3 py-2 text-micro text-text-muted">{label}</div>
{:else}
  <div class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
    <span class="animate-pulse text-body font-medium text-text-secondary">{label}</span>
  </div>
{/if}
