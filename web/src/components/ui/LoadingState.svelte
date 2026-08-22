<script lang="ts">
  /*
   * Transient loading copy. Same geometry as EmptyState (center column, px-6,
   * centered) with pulse on the label. Callers pass a translated `label`;
   * the default is common.loading.
   */
  import { t } from '../../lib/i18n'

  let { label = t('common.loading') }: { label?: string } = $props()
</script>

<div class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
  <span class="animate-pulse text-body font-medium text-text-secondary">{label}</span>
</div>
