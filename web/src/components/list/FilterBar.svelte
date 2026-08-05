<script lang="ts">
  /*
   * Filter bar ([explore]). Removable chips + "Add filter" dropdown (2-step:
   * field→value) + flag toggles. With active filters, shows "Save as view" /
   * "Reset". Every value toggle writes the filters store (URL) → local refilter.
   */
  import { filters, type FacetValue } from '../../stores/filters.svelte'
  import { views } from '../../stores/views.svelte'
  import { filterFields, type MultiField } from '../../lib/view-config'
  import { t, fieldLabel } from '../../lib/i18n'

  /** One pickable axis: a static field or a discovered custom-field alias. */
  interface Axis {
    key: string
    label: string
    dynamic: boolean
  }

  // Static fields (disabled features omitted) + discovered axes for the scope.
  const axes = $derived.by<Axis[]>(() => [
    ...filterFields().map((f) => ({ key: f as string, label: fieldLabel(f), dynamic: false })),
    ...filters.dynamicAxes.map((a) => ({ key: a.key, label: a.label, dynamic: true })),
  ])

  let open = $state(false)
  let field = $state<Axis | null>(null)
  let valueQuery = $state('')
  let saveOpen = $state(false)
  let saveName = $state('')
  let rootEl = $state<HTMLDivElement | null>(null)

  const activeSet = $derived.by(() => {
    // Selected values for the open field (for checkmarks)
    if (!field) return new Set<string>()
    if (field.dynamic) return new Set(filters.filters.fields[field.key] ?? [])
    return new Set(filters.filters[field.key as MultiField])
  })

  const values = $derived.by<FacetValue[]>(() => {
    if (!field) return []
    const list = field.dynamic
      ? (filters.dynamicFacets[field.key] ?? [])
      : filters.facets[field.key as MultiField]
    const q = valueQuery.trim().toLowerCase()
    return q ? list.filter((v) => v.label.toLowerCase().includes(q)) : list
  })

  function openMenu() {
    open = true
    field = null
    valueQuery = ''
  }
  function pickField(f: Axis) {
    field = f
    valueQuery = ''
  }
  function toggle(axis: Axis, value: string) {
    if (axis.dynamic) filters.toggleFieldValue(axis.key, value)
    else filters.toggleValue(axis.key as MultiField, value)
  }
  function closeAll() {
    open = false
    field = null
    saveOpen = false
  }

  function onDocClick(e: MouseEvent) {
    if (!rootEl) return
    const path = e.composedPath()
    if (!path.includes(rootEl)) closeAll()
  }

  async function doSave(scope: 'personal' | 'team') {
    const name = saveName.trim()
    if (!name) return
    const config = filters.currentConfig()
    if (scope === 'personal') views.addPersonal(name, config)
    else await views.addTeam(name, config).catch((e) => console.warn('[filterbar] 팀 뷰 저장 실패', e))
    saveName = ''
    saveOpen = false
  }
</script>

<svelte:document onclick={onDocClick} />

<div bind:this={rootEl} class="flex flex-wrap items-center gap-1.5">
  <!-- Active chips -->
  {#each filters.activeChips as chip (chip.field + (chip.value ?? ''))}
    <button
      type="button"
      class="group inline-flex items-center gap-1 rounded-md border border-border-subtle bg-bg-elevated px-2.5 py-1 text-[12px] text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
      onclick={() => {
        if (chip.kind === 'multi') filters.removeValue(chip.field as MultiField, chip.value!)
        else if (chip.kind === 'field') filters.removeFieldValue(chip.field, chip.value!)
        else if (chip.kind === 'flag') filters.toggleFlag(chip.field as 'reopened' | 'unassigned' | 'stale')
        else filters.setRange(chip.field as 'created' | 'updated', null, null)
      }}
      title={t('filter.remove')}
    >
      <span class="truncate max-w-[180px]">{chip.label}</span>
      <span class="text-text-muted transition-colors group-hover:text-status-reopen">✕</span>
    </button>
  {/each}

  <!-- Add filter -->
  <div class="relative">
    <button
      type="button"
      class="inline-flex items-center gap-1 rounded-md border border-dashed border-border-strong px-2.5 py-1 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => (open ? closeAll() : openMenu())}
    >
      {t('filter.add')}
    </button>

    {#if open}
      <div
        class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-[70vh] w-64 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-xl shadow-black/40"
      >
        {#if !field}
          <!-- Step 1: field pick + flags -->
          <div class="px-2 py-1 text-[11px] font-medium text-text-muted">{t('filter.properties')}</div>
          {#each axes as f (f.key)}
            <button
              type="button"
              class="flex w-full items-center justify-between rounded px-2 py-1 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              onclick={() => pickField(f)}
            >
              <span>{f.label}</span>
              <span class="text-text-muted">›</span>
            </button>
          {/each}
          <div class="my-1 border-t border-border-subtle"></div>
          <div class="px-2 py-1 text-[11px] font-medium text-text-muted">{t('filter.quick')}</div>
          {#each [{ k: 'reopened' as const, l: t('filter.flagReopened') }, { k: 'unassigned' as const, l: t('filter.flagUnassigned') }, { k: 'stale' as const, l: t('filter.flagStale') }] as flag (flag.k)}
            <button
              type="button"
              class="flex w-full items-center justify-between rounded px-2 py-1 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              onclick={() => filters.toggleFlag(flag.k as 'reopened' | 'unassigned' | 'stale')}
            >
              <span>{flag.l}</span>
              {#if filters.filters[flag.k as 'reopened' | 'unassigned' | 'stale']}
                <span class="text-accent-text">✓</span>
              {/if}
            </button>
          {/each}
        {:else}
          <!-- Step 2: value pick -->
          <div class="flex items-center gap-1 px-1 pb-1">
            <button
              type="button"
              class="rounded px-1 text-[12px] text-text-muted hover:text-text-primary"
              onclick={() => (field = null)}
            >
              ‹
            </button>
            <input
              type="text"
              bind:value={valueQuery}
              placeholder={t('filter.searchField', { field: field.label })}
              class="min-w-0 flex-1 rounded bg-bg-base px-2 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none"
            />
          </div>
          <div class="max-h-64 overflow-y-auto">
            {#if values.length === 0}
              <div class="px-2 py-3 text-center text-[12px] text-text-muted">{t('common.noValues')}</div>
            {/if}
            {#each values as v (v.value)}
              <button
                type="button"
                class="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
                onclick={() => toggle(field!, v.value)}
              >
                <span
                  class="flex h-3.5 w-3.5 flex-none items-center justify-center rounded border {activeSet.has(
                    v.value,
                  )
                    ? 'border-accent bg-accent text-white'
                    : 'border-border-strong'}"
                >
                  {#if activeSet.has(v.value)}<span class="text-[9px]">✓</span>{/if}
                </span>
                <span class="min-w-0 flex-1 truncate">{v.label}</span>
                <span class="flex-none text-[11px] text-text-muted">{v.count}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  {#if filters.hasFilters}
    <div class="relative">
      <button
        type="button"
        class="rounded-md px-2 py-0.5 text-[12px] text-accent-text transition-colors hover:bg-accent-subtle/40"
        onclick={() => (saveOpen = !saveOpen)}
      >
        {t('filter.saveAsView')}
      </button>
      {#if saveOpen}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 w-60 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-xl shadow-black/40"
        >
          <input
            type="text"
            bind:value={saveName}
            placeholder={t('filter.viewName')}
            class="mb-2 w-full rounded bg-bg-base px-2 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none"
            onkeydown={(e) => e.key === 'Enter' && doSave('personal')}
          />
          <div class="flex gap-1.5">
            <button
              type="button"
              class="flex-1 rounded bg-accent px-2 py-1 text-[12px] font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-40"
              disabled={!saveName.trim()}
              onclick={() => doSave('personal')}
            >
              {t('filter.savePersonal')}
            </button>
            <button
              type="button"
              class="flex-1 rounded border border-border-strong px-2 py-1 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover disabled:opacity-40"
              disabled={!saveName.trim()}
              onclick={() => doSave('team')}
            >
              {t('filter.saveTeam')}
            </button>
          </div>
        </div>
      {/if}
    </div>
    <button
      type="button"
      class="rounded-md px-2 py-0.5 text-[12px] text-text-muted transition-colors hover:text-status-reopen"
      onclick={() => filters.clearAll()}
    >
      {t('filter.clear')}
    </button>
  {/if}
</div>
