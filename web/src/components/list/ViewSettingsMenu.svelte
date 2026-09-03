<script lang="ts">
  /*
   * View settings (GDK-1391). ViewDisplay is one object — layout, sort,
   * direction, columns — so it gets one editor. Until this menu the toolbar
   * spent three controls on it (a list⇄board toggle, a columns menu and a sort
   * menu) and two of them wore the same glyph. The breakdown axis stays on
   * BreakdownBar, beside the chips it governs.
   *
   * The panel stays open across clicks: it is a settings panel, and a reader
   * adjusting sort and columns together should not reopen it per change.
   */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import { hasServerVerb } from '../../lib/config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import {
    LAYOUT_VALUES,
    columnCatalog,
    defaultColumns,
    type ColumnKey,
    type Layout,
    type SortKey,
  } from '../../lib/view-config'
  import Icon, { type IconName } from '../ui/Icon.svelte'

  // Not `columns` for the board: that glyph names the column checklist below
  // (GDK-1391).
  const LAYOUT_ICON: Record<Layout, IconName> = { list: 'list', board: 'kanban' }
  const layoutLabel = (l: Layout): string => (l === 'list' ? t('board.asList') : t('board.asBoard'))

  const BASE_SORTS: { k: SortKey; l: string }[] = [
    { k: 'updated', l: t('sort.updated') },
    { k: 'created', l: t('sort.created') },
    { k: 'due', l: t('sort.due') },
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
  const autoSort = $derived(filters.effectiveSort === 'keys' || filters.effectiveSort === 'relevance')
  const sortLabel = $derived(
    [KEYS_SORT, RELEVANCE, ...BASE_SORTS].find((s) => s.k === filters.effectiveSort)?.l,
  )

  const catalog = columnCatalog()
  const defaults = defaultColumns()
  const active = $derived(new Set<ColumnKey>(filters.display.columns))
  const isDefault = $derived(
    active.size === defaults.length && defaults.every((k) => active.has(k)),
  )

  let open = $state(false)
  let saveOpen = $state(false)
  let saveName = $state('')

  // GDK-1343: saving a view is a display decision as much as a filter one —
  // "All open, grouped by epic, by priority" has no chip to hang the door on.
  // GDK-437: the product picks the store. A server behind this bundle is
  // where a view belongs (it follows the user across devices). The hosted
  // demo has no server to write to, so it stays in this browser and says so.
  const saveToServer = hasServerVerb('settings')

  async function doSave() {
    const name = saveName.trim()
    if (!name) return
    const config = filters.currentConfig()
    if (saveToServer) {
      try {
        await views.addTeam(name, config)
      } catch (e) {
        // Never lose the view quietly: keep it in this browser and say so.
        views.addPersonal(name, config)
        write.toast(t('filter.saveServerFailed'), 'error')
        console.warn('[view-settings] 서버 뷰 저장 실패, 브라우저 저장으로 폴백', e)
      }
    } else {
      views.addPersonal(name, config)
    }
    saveName = ''
    saveOpen = false
    open = false
  }

  function close() {
    open = false
    saveOpen = false
  }

  // Spend Esc so one keystroke cannot also clear the detail panel.
  // preventDefault is what DetailPanel declines; stopPropagation is what the
  // shell keymap needs — it does not read defaultPrevented, and its
  // svelte:window listener is registered first. The delegated onkeydown
  // below reaches the event while it still walks the focused trigger.
  function onEsc(e: KeyboardEvent) {
    if (e.key !== 'Escape' || !open) return
    e.preventDefault()
    e.stopPropagation()
    close()
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative"
  onkeydown={onEsc}
  use:onEscape={onEsc}
  use:onOutsideClick={{ handler: close, enabled: open }}
>
  <button
    type="button"
    data-testid="view-settings"
    class="inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 text-body text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    onclick={() => (open = !open)}
    title={t('view.settings')}
    aria-expanded={open}
  >
    <Icon name="sliders" size={14} class="text-text-muted" />
    <!-- The sort value is state, the door is wayfinding: when the toolbar
         runs short the value goes first (GDK-1343). The auto-promoted orders
         (given order, relevance) stay: they explain why the list is not in
         its usual order. No aria-label: the value is the accessible name
         while it shows (keys-order.spec reads "Given order" off this button);
         the title names the door once the value has folded. -->
    <span class="text-text-muted {autoSort ? '' : '@max-[1120px]:hidden'}">{sortLabel}</span>
    {#if !isDefault}
      <span class="rounded bg-accent-subtle/70 px-1 text-micro text-accent-text">{active.size}</span>
    {/if}
  </button>

  {#if open}
    <div
      class="anim-enter absolute right-0 top-full z-30 mt-1 max-h-[80vh] w-64 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-overlay"
    >
      <!-- Save first, not last: the panel is one screen tall once the column
           catalog is in it, and "keep this view" should not need a scroll. -->
      <div class="mb-1 flex items-center justify-between">
        <span class="text-micro font-medium text-text-muted">{t('view.settings')}</span>
        {#if !saveOpen}
          <button
            type="button"
            class="inline-flex h-control-sm items-center rounded px-1.5 text-micro text-accent-text transition-colors hover:bg-accent-subtle/40"
            onclick={() => (saveOpen = true)}
          >
            {t('filter.saveAsView')}
          </button>
        {/if}
      </div>
      {#if saveOpen}
        <div data-testid="filter-save-popover">
          <input
            type="text"
            bind:value={saveName}
            placeholder={t('filter.viewName')}
            class="mb-2 h-control-sm w-full rounded bg-bg-base px-2 text-body text-text-primary placeholder:text-text-muted focus:outline-none"
            onkeydown={(e) => e.key === 'Enter' && doSave()}
          />
          <button
            type="button"
            class="h-control-sm w-full rounded bg-accent px-2 text-body font-medium text-white hover:opacity-90 disabled:opacity-40"
            disabled={!saveName.trim()}
            data-testid="filter-save-view"
            onclick={() => doSave()}
          >
            {t('filter.saveAsView')}
          </button>
          {#if !saveToServer}
            <div class="mt-1.5 text-micro text-text-muted" data-testid="filter-save-local-hint">
              {t('filter.saveDemoLocal')}
            </div>
          {/if}
        </div>
      {/if}
      <div class="my-2 border-t border-border-subtle"></div>
      <div class="mb-1 text-micro font-medium text-text-muted">{t('board.layout')}</div>
      <div
        class="flex h-control items-center gap-px rounded-md border border-border-strong p-px"
        role="group"
        aria-label={t('board.layout')}
      >
        {#each LAYOUT_VALUES as l (l)}
          <button
            type="button"
            data-testid="layout-{l}"
            aria-pressed={filters.display.layout === l}
            class="flex h-full flex-1 items-center justify-center gap-1.5 rounded text-body transition-colors duration-150
              {filters.display.layout === l
                ? 'bg-bg-active text-text-primary'
                : 'text-text-muted hover:bg-bg-hover hover:text-text-secondary'}"
            onclick={() => filters.setLayout(l)}
          >
            <Icon name={LAYOUT_ICON[l]} size={14} />
            {layoutLabel(l)}
          </button>
        {/each}
      </div>

      <div class="my-2 border-t border-border-subtle"></div>
      <div class="mb-1 text-micro font-medium text-text-muted">{t('sort.label')}</div>
      <div class="flex flex-wrap gap-1">
        {#each sorts as s (s.k)}
          <button
            type="button"
            class="inline-flex h-control-sm items-center rounded px-2 text-body transition-colors {filters.effectiveSort === s.k
              ? 'bg-accent text-white'
              : 'bg-bg-base text-text-secondary hover:bg-bg-hover'}"
            onclick={() => filters.setSort(s.k)}
          >
            {s.l}
          </button>
        {/each}
        <button
          type="button"
          class="ml-auto inline-flex h-control-sm items-center rounded px-2 text-body text-text-secondary transition-colors hover:bg-bg-hover"
          onclick={() => filters.toggleDir()}
          title={t('sort.direction')}
        >
          {filters.display.dir === 'desc' ? t('sort.desc') : t('sort.asc')}
        </button>
      </div>

      <!-- Columns are list-row fields; a board card does not read them. -->
      {#if filters.display.layout === 'list'}
      <div class="my-2 border-t border-border-subtle"></div>
      <div class="mb-1 flex items-center justify-between">
        <span class="text-micro font-medium text-text-muted">{t('columns.exposed')}</span>
        <button
          type="button"
          class="inline-flex h-control-sm items-center rounded px-1.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover disabled:opacity-40"
          onclick={() => filters.resetColumns()}
          disabled={isDefault}
          title={t('columns.reset')}
        >
          {t('columns.defaults')}
        </button>
      </div>
      {#each catalog as col (col.key)}
        <button
          type="button"
          data-testid={`column-toggle-${col.key}`}
          class="flex min-h-control-sm w-full items-center gap-2 rounded px-2 py-1 text-left text-body transition-colors hover:bg-bg-hover"
          onclick={() => filters.toggleColumn(col.key)}
          aria-pressed={active.has(col.key)}
        >
          <span
            class="flex h-3.5 w-3.5 flex-none items-center justify-center rounded border transition-colors {active.has(
              col.key,
            )
              ? 'border-accent bg-accent text-white'
              : 'border-border-strong'}"
          >
            {#if active.has(col.key)}<Icon name="check" size={10} />{/if}
          </span>
          <span class={active.has(col.key) ? 'text-text-primary' : 'text-text-secondary'}>
            {col.label}
          </span>
        </button>
      {/each}
      {/if}

    </div>
  {/if}
</div>
