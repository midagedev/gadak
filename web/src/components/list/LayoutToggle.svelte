<script lang="ts">
  /*
   * List ⇄ board (GDK-1175). A two-state segmented control rather than a
   * menu item: it is the one display choice you flip back and forth, and it
   * sits beside the grouping control because the two are one sentence —
   * "these issues, grouped by X, read across."
   */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { LAYOUT_VALUES, type Layout } from '../../lib/view-config'
  import Icon, { type IconName } from '../ui/Icon.svelte'

  // Not `columns`: the columns menu beside this control wears that glyph, and
  // two identical marks a finger apart read as one control (GDK-1391).
  const ICON: Record<Layout, IconName> = { list: 'list', board: 'kanban' }
  const label = (l: Layout): string => (l === 'list' ? t('board.asList') : t('board.asBoard'))

  const current = $derived(filters.display.layout)
</script>

<div
  class="flex h-control flex-none items-center gap-px rounded-md border border-border-strong p-px"
  role="group"
  aria-label={t('board.layout')}
>
  {#each LAYOUT_VALUES as l (l)}
    <button
      type="button"
      data-testid="layout-{l}"
      aria-pressed={current === l}
      title={label(l)}
      aria-label={label(l)}
      class="flex h-full items-center rounded px-2 transition-colors duration-150
        {current === l
          ? 'bg-bg-active text-text-primary'
          : 'text-text-muted hover:bg-bg-hover hover:text-text-secondary'}"
      onclick={() => filters.setLayout(l)}
    >
      <Icon name={ICON[l]} size={14} />
    </button>
  {/each}
</div>
