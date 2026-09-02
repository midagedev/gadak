<script lang="ts">
  /*
   * Sort menu ([explore]). Breakdown is changed directly on BreakdownBar.
   */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import { hasServerVerb } from '../../lib/config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import type { SortKey } from '../../lib/view-config'
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
        console.warn('[display-menu] 서버 뷰 저장 실패, 브라우저 저장으로 폴백', e)
      }
    } else {
      views.addPersonal(name, config)
    }
    saveName = ''
    saveOpen = false
    open = false
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
    open = false
    saveOpen = false
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative"
  onkeydown={onEsc}
  use:onEscape={onEsc}
  use:onOutsideClick={{
    handler: () => {
      open = false
      saveOpen = false
    },
    enabled: open,
  }}
>
  <button
    type="button"
    class="inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 text-body text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    onclick={() => (open = !open)}
    title={t('sort.options')}
    data-testid="display-menu"
  >
    <span>{t('sort.label')}</span>
    <!-- The value is state, the palette door beside it is wayfinding: when the
         toolbar runs short the value goes first, and the menu still says it.
         The auto-promoted orders (given order, relevance) stay: they explain
         why the list is not in its usual order. -->
    <span
      class="text-text-muted {filters.effectiveSort === 'keys' || filters.effectiveSort === 'relevance'
        ? ''
        : '@max-[1120px]:hidden'}"
    >
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

      <div class="my-2 border-t border-border-subtle"></div>
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
      {:else}
        <button
          type="button"
          class="flex h-control-sm w-full items-center rounded px-2 text-body text-accent-text hover:bg-accent-subtle/40"
          onclick={() => (saveOpen = true)}
        >
          {t('filter.saveAsView')}
        </button>
      {/if}
    </div>
  {/if}
</div>
