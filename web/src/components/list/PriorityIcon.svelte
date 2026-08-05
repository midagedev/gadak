<script lang="ts">
  /*
   * Priority icon ([explore]). Linear-style 4 bars (fill by level).
   *  Level 0 (none) is a faint dashed bar. Color from priorityMeta.
   */
  import { t } from '../../lib/i18n'
  import { priorityMeta } from '../../lib/format'

  let { priority }: { priority: string | null } = $props()
  const meta = $derived(priorityMeta(priority))
  // 4 bars: levels 1–5 → how many filled (0 = none)
  const bars = [1, 2, 3, 4]
  const filled = $derived(Math.min(4, Math.max(0, meta.level - 1)))
</script>

<span
  class="inline-flex h-3.5 flex-none items-end gap-[2px]"
  title={meta.label || t('list.priorityNone')}
  aria-label={t('list.priorityLabel', { label: meta.label || t('common.none') })}
>
  {#each bars as b (b)}
    <span
      class="w-[3px] rounded-[1px]"
      style:height="{b * 2.5 + 2}px"
      style:background={b <= filled ? meta.color : 'var(--color-border-strong)'}
    ></span>
  {/each}
</span>
