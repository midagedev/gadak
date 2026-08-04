<script lang="ts">
  /*
   * 우선순위 아이콘 ([explore]). Linear 식 막대 4단(레벨에 따라 채움).
   *  level 0(없음)은 옅은 점선 막대. 색은 priorityMeta.
   */
  import { t } from '../../lib/i18n'
  import { priorityMeta } from '../../lib/format'

  let { priority }: { priority: string | null } = $props()
  const meta = $derived(priorityMeta(priority))
  // 막대 4개: level 1~5 → 채워지는 개수(0=없음)
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
