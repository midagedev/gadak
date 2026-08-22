<script lang="ts">
  /*
   * Group header ([explore]). Fixed 42px. Label + total + status-category mini tallies.
   *  The list is the summary dashboard (plan §5.1) — aggregates live in the header.
   */
  import { t } from '../../lib/i18n'
  import type { IssueGroup } from '../../stores/filters.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { categoryMetaOf } from '../../lib/format'
  import type { StatusCategory } from '../../lib/view-config'

  let {
    group,
    floating = false,
    showCategoryCounts = true,
  }: { group: IssueGroup; floating?: boolean; showCategoryCounts?: boolean } = $props()

  const order: StatusCategory[] = ['new', 'inprogress', 'done']
</script>

<!--
  Section rhythm: header recedes as a "label line", not a filled card —
  microcap label + thin rule. Virtual scroll fixes the slot at 42px, so keep
  height but bottom-align the label to make space above the group.
  (Background/blur only when sticky-floating over rows below.)
-->
<div
  data-testid="group-header"
  class="flex h-row items-end gap-2 px-4 pb-1.5 text-micro
    {floating
      ? 'bg-bg-base/95 backdrop-blur border-b border-border-subtle'
      : ''}"
>
  {#if group.prefix}
    <!-- Epic key stays as typed (mono, no uppercase transform) so it reads as an
         identifier next to the uppercased section label. An epic is a Jira issue
         itself — unlike every other group axis — so the key opens it. -->
    <button
      type="button"
      class="flex-none font-mono text-micro font-semibold text-accent-text transition-colors hover:underline"
      title={t('group.openEpic')}
      onclick={() => selection.select(group.key)}
    >{group.prefix}</button>
  {/if}
  <span class="truncate text-micro font-medium uppercase tracking-wide text-text-muted">
    {group.label || t('common.all')}
  </span>
  <span class="flex-none text-micro tabular-nums text-text-muted">
    {group.counts.total}
  </span>
  <span class="h-px flex-1 self-center bg-border-subtle"></span>

  <!-- Status-category mini tallies -->
  {#if showCategoryCounts}
    <span class="flex flex-none items-center gap-2">
      {#each order as c (c)}
        {#if group.counts.category[c] > 0}
          <span class="flex items-center gap-1 text-micro text-text-muted" title={categoryMetaOf(c).label}>
            <span class="h-1.5 w-1.5 rounded-full" style:background={categoryMetaOf(c).color}></span>
            {group.counts.category[c]}
          </span>
        {/if}
      {/each}
    </span>
  {/if}
</div>
