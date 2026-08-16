<script lang="ts">
  /*
   * Column menu ([explore]). Toggle trailing fields shown on list rows.
   *  Config lives under the view's display and serializes with URL/saved views
   *  (per-view columns).
   */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { columnCatalog, defaultColumns, type ColumnKey } from '../../lib/view-config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import Icon from '../ui/Icon.svelte'

  const catalog = columnCatalog()
  const defaults = defaultColumns()
  const active = $derived(new Set<ColumnKey>(filters.display.columns))
  const isDefault = $derived(
    active.size === defaults.length && defaults.every((k) => active.has(k)),
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
    title={t('columns.title')}
  >
    <span>{t('columns.label')}</span>
    {#if !isDefault}
      <span class="rounded bg-accent-subtle/70 px-1 text-micro text-accent-text">{active.size}</span>
    {/if}
  </button>

  {#if open}
    <div
      class="anim-enter absolute right-0 top-full z-30 mt-1 w-52 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-overlay"
    >
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
      <div class="max-h-[60vh] overflow-y-auto">
        {#each catalog as col (col.key)}
          <button
            type="button"
            class="flex min-h-control-sm w-full items-center gap-2 rounded px-2 py-1 text-left text-[12px] transition-colors hover:bg-bg-hover"
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
    </div>
  {/if}
</div>
