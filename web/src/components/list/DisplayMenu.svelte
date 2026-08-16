<script lang="ts">
  /*
   * Sort menu ([explore]). Breakdown is changed directly on BreakdownBar.
   */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import type { SortKey } from '../../lib/view-config'
  const BASE_SORTS: { k: SortKey; l: string }[] = [
    { k: 'updated', l: t('sort.updated') },
    { k: 'created', l: t('sort.created') },
    { k: 'priority', l: t('sort.priority') },
    { k: 'reopen_count', l: t('sort.reopenCount') },
  ]
  const RELEVANCE = { k: 'relevance' as SortKey, l: t('sort.relevance') }
  const KEYS_SORT = { k: 'keys' as SortKey, l: t('sort.keys') }

  // Show relevance only while searching (or relevance is active) so auto-promote is visible.
  // keys-order is the same kind of auto label — not a picker unless it is already on.
  const sorts = $derived(
    filters.effectiveSort === 'keys'
      ? [KEYS_SORT, ...BASE_SORTS]
      : filters.filters.q.trim() || filters.effectiveSort === 'relevance'
        ? [RELEVANCE, ...BASE_SORTS]
        : BASE_SORTS,
  )

  let open = $state(false)

  // Spend Esc so one keystroke cannot also clear the detail panel.
  // preventDefault is what DetailPanel declines; stopPropagation is what the
  // shell keymap needs — it does not read defaultPrevented, and its
  // svelte:window listener is registered first. The delegated onkeydown
  // below reaches the event while it still walks the focused trigger.
  function onEsc(e: KeyboardEvent) {
    if (e.key !== 'Escape' || !open) return
    e.preventDefault()
    e.stopPropagation()
    open = false
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative"
  onkeydown={onEsc}
  use:onEscape={onEsc}
  use:onOutsideClick={{ handler: () => (open = false), enabled: open }}
>
  <button
    type="button"
    class="inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 text-[12px] text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    onclick={() => (open = !open)}
    title={t('sort.options')}
  >
    <span>{t('sort.label')}</span>
    <span class="text-text-muted">
      {[KEYS_SORT, RELEVANCE, ...BASE_SORTS].find((s) => s.k === filters.effectiveSort)?.l}
    </span>
  </button>

  {#if open}
    <div
      class="anim-enter absolute right-0 top-full z-30 mt-1 w-56 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-overlay"
    >
      <div class="mb-1 text-micro font-medium text-text-muted">{t('sort.label')}</div>
      <div class="flex flex-wrap gap-1">
        {#each sorts as s (s.k)}
          <button
            type="button"
            class="inline-flex h-control-sm items-center rounded px-2 text-[12px] transition-colors {filters.effectiveSort === s.k
              ? 'bg-accent text-white'
              : 'bg-bg-base text-text-secondary hover:bg-bg-hover'}"
            onclick={() => filters.setSort(s.k)}
          >
            {s.l}
          </button>
        {/each}
        <button
          type="button"
          class="ml-auto inline-flex h-control-sm items-center rounded px-2 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
          onclick={() => filters.toggleDir()}
          title={t('sort.direction')}
        >
          {filters.display.dir === 'desc' ? t('sort.desc') : t('sort.asc')}
        </button>
      </div>
    </div>
  {/if}
</div>
