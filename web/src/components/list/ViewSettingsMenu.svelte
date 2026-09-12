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
  import { saveCurrentView, savesToServer } from './save-current-view'
  import { saveViewRequest } from './save-view-request.svelte'
  import { ESC_TIER, isEscapeKey, onEscape, onOutsideClick } from '../../lib/dom-actions'
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

  // GDK-832: one label per sort key, keyed by SortKey — a key added to
  // SORT_KEY_VALUES without a label here is a compile error, not a menu that
  // silently lacks the new sort.
  const SORT_LABEL: Record<SortKey, string> = {
    updated: t('sort.updated'),
    created: t('sort.created'),
    // Aging axis (my-work pack): asc = longest in status first.
    status_changed: t('sort.statusChanged'),
    // Work item age (flow canon): asc = longest underway first.
    started: t('sort.started'),
    due: t('sort.due'),
    priority: t('sort.priority'),
    reopen_count: t('sort.reopenCount'),
    relevance: t('sort.relevance'),
    keys: t('sort.keys'),
  }
  // relevance and keys are auto-promoted labels, not pickable rows — they
  // join the menu only while they apply (below), so the always-on list is
  // every other key. The exhaustiveness check names any key that is neither
  // listed here nor deliberately conditional.
  const BASE_SORT_ORDER = [
    'updated',
    'created',
    'status_changed',
    'started',
    'due',
    'priority',
    'reopen_count',
  ] as const satisfies readonly SortKey[]
  type ConditionalSort = 'relevance' | 'keys'
  type MissingFromSortMenu = Exclude<SortKey, (typeof BASE_SORT_ORDER)[number] | ConditionalSort>
  const _sortMenuCoversEveryKey: [MissingFromSortMenu] extends [never]
    ? true
    : MissingFromSortMenu = true
  void _sortMenuCoversEveryKey

  const BASE_SORTS: { k: SortKey; l: string }[] = BASE_SORT_ORDER.map((k) => ({
    k,
    l: SORT_LABEL[k],
  }))
  const RELEVANCE = { k: 'relevance' as SortKey, l: SORT_LABEL.relevance }
  const KEYS_SORT = { k: 'keys' as SortKey, l: SORT_LABEL.keys }

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

  // Local intent, plus the palette's pending hand-off (GDK-732). Derived
  // rather than assigned from an $effect: GDK-692 forbids an effect writing
  // this file's own $state, and the request is authoritative until something
  // closes the panel, so a derived read is also the honest shape.
  let openLocal = $state(false)
  let saveOpenLocal = $state(false)
  const open = $derived(openLocal || saveViewRequest.pending)
  const saveOpen = $derived(saveOpenLocal || saveViewRequest.pending)
  let saveName = $state('')

  /** Every close path goes through here so the hand-off cannot re-open. */
  function setOpen(next: boolean): void {
    saveViewRequest.clear()
    openLocal = next
    if (!next) saveOpenLocal = false
  }

  // Where a save lands, and the save itself, are list/save-current-view's —
  // the palette offers the same action and must not re-decide the policy
  // (GDK-732).
  const saveToServer = savesToServer()

  async function doSave() {
    await saveCurrentView(saveName)
    if (!saveName.trim()) return
    saveName = ''
    setOpen(false)
  }

  function close() {
    setOpen(false)
  }

  // Menu-tier claim on the Esc stack (GDK-1565): the open header menu
  // outranks the surface under it, and acting spends the key so the detail
  // panel keeps its own Esc. The delegated onkeydown below is the same
  // handler one phase early — it sees the key while it still walks the
  // trigger, where its stopPropagation shields the shell keymap.
  function onEsc(e: KeyboardEvent) {
    if (!isEscapeKey(e) || !open) return
    e.preventDefault()
    e.stopPropagation()
    close()
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- defer on the outside-click listener: the palette runs its rows on
     mousedown (CommandPalette.svelte's row handler), and the save hand-off
     opens this menu inside that same dispatch — a listener attached now still
     catches the very mousedown that asked for it, at window, and closes what
     just opened. Deferring the attach by a tick is the fix BulkBar's menu
     already carries. Measured (GDK-732): the latch went request-then-clear
     inside one click. -->
<div
  class="relative"
  onkeydown={onEsc}
  use:onEscape={{ handler: onEsc, priority: ESC_TIER.menu, label: 'view-settings' }}
  use:onOutsideClick={{ handler: close, enabled: open, defer: true }}
>
  <button
    type="button"
    data-testid="view-settings"
    class="inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 text-body text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    onclick={() => setOpen(!open)}
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
            onclick={() => (saveOpenLocal = true)}
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
      <!-- GDK-1745's rule, on this menu: the wall's height belongs to the panel.
           The popover used to carry `max-h-[80vh]` and scroll as one piece, so
           at a 640px-tall window the cap landed at 512px — inside the ninth
           column row, which the web demo published with its checkbox and its
           glyphs cut through the middle and no scrollbar or fade to say there
           was more (vision round, 2026-09-12). The catalog scrolls on its own
           now and stops on a row boundary: rows are `h-7` (28px, declared
           rather than emergent so the arithmetic below cannot drift) and the
           box holds eight of them exactly. -->
      <div class="max-h-[224px] overflow-y-auto overscroll-contain">
      {#each catalog as col (col.key)}
        <button
          type="button"
          data-testid={`column-toggle-${col.key}`}
          class="flex h-7 w-full items-center gap-2 rounded px-2 text-left text-body transition-colors hover:bg-bg-hover"
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
      </div>
      {/if}

    </div>
  {/if}
</div>
