<script lang="ts">
  /*
   * Filter bar ([explore]). Removable chips + "Add filter" dropdown (2-step:
   * field→value) + flag toggles. With active filters, shows "Save as view" /
   * "Reset". Every value toggle writes the filters store (URL) → local refilter.
   */
  import { filters, type FacetValue } from '../../stores/filters.svelte'
  import { filterFields, negationOf, type MultiField, type NegationField, type RangeField } from '../../lib/view-config'
  import { t, fieldLabel } from '../../lib/i18n'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import Icon from '../ui/Icon.svelte'

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

  const DATE_AXES: { key: RangeField; label: string }[] = [
    { key: 'created', label: t('field.created') },
    { key: 'updated', label: t('field.updated') },
    { key: 'due', label: t('field.due') },
  ]

  let open = $state(false)
  let field = $state<Axis | null>(null)
  let dateField = $state<RangeField | null>(null)
  let valueQuery = $state('')

  /** Whether the open axis has a negation twin (drives the per-value ⊘). */
  const negatable = $derived(!!field && !field.dynamic && !!negationOf(field.key as MultiField))

  // Per-value tri-state (GDK-771): a row is neutral, included (check), or
  // excluded (⊘). The old modal "Exclude" toggle made picks land differently
  // depending on invisible state; both lists now show at once.
  const includeSet = $derived.by(() => {
    if (!field) return new Set<string>()
    if (field.dynamic) return new Set(filters.filters.fields[field.key] ?? [])
    return new Set(filters.filters[field.key as MultiField] as string[])
  })
  const excludeSet = $derived.by(() => {
    if (!field || field.dynamic) return new Set<string>()
    const neg = negationOf(field.key as MultiField)
    return neg ? new Set((filters.filters[neg] ?? []) as string[]) : new Set<string>()
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
    dateField = null
    valueQuery = ''
  }
  function pickField(f: Axis) {
    field = f
    dateField = null
    valueQuery = ''
  }
  function pickDate(k: RangeField) {
    field = null
    dateField = k
    valueQuery = ''
  }
  function rangeBound(axis: RangeField, bound: 'from' | 'to'): string {
    return filters.filters[`${axis}_${bound}`] ?? ''
  }
  function setRangeBound(axis: RangeField, bound: 'from' | 'to', value: string) {
    const from = bound === 'from' ? value || null : rangeBound(axis, 'from') || null
    const to = bound === 'to' ? value || null : rangeBound(axis, 'to') || null
    filters.setRange(axis, from, to)
  }
  function toggle(axis: Axis, value: string, e?: MouseEvent) {
    if (axis.dynamic) {
      filters.toggleFieldValue(axis.key, value)
      return
    }
    const key = axis.key as MultiField
    // Alt-click is the exclude shortcut; a plain click on an excluded value
    // clears the exclusion (back to neutral) instead of double-listing it.
    if (e?.altKey) {
      toggleExclude(axis, value)
      return
    }
    const neg = negationOf(key)
    if (neg && excludeSet.has(value)) {
      filters.toggleValue(neg, value)
      return
    }
    filters.toggleValue(key, value)
  }
  function toggleExclude(axis: Axis, value: string) {
    if (axis.dynamic) return
    const key = axis.key as MultiField
    const neg = negationOf(key)
    if (!neg) return
    // Moving to the exclude side leaves the include side first — one value
    // never sits on both lists (exclude would win and the chip pair lies).
    if (includeSet.has(value)) filters.toggleValue(key, value)
    filters.toggleValue(neg, value)
  }
  function closeAll() {
    open = false
    field = null
    dateField = null
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
    closeAll()
  }

</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="flex flex-wrap items-center gap-1.5"
  onkeydown={onEsc}
  use:onEscape={onEsc}
  use:onOutsideClick={{ handler: closeAll, enabled: open }}
>
  <!-- Active chips -->
  {#each filters.activeChips as chip (chip.field + (chip.value ?? ''))}
    <button
      type="button"
      data-testid="filter-chip"
      data-filter-field={chip.field}
      data-filter-value={chip.value ?? chip.field}
      class="group inline-flex h-control-sm items-center gap-1 rounded-md border border-accent/60 bg-accent-subtle/40 px-2.5 text-micro text-accent-text transition-colors hover:border-accent/75 hover:text-text-primary"
      onclick={() => {
        if (chip.kind === 'multi')
          filters.removeValue(chip.field as MultiField | NegationField, chip.value!)
        else if (chip.kind === 'field') filters.removeFieldValue(chip.field, chip.value!)
        else if (chip.kind === 'flag') filters.toggleFlag(chip.field as 'reopened' | 'unassigned' | 'stale')
        else if (chip.kind === 'keys') filters.clearKeys()
        else filters.setRange(chip.field as RangeField, null, null)
      }}
      title={t('filter.remove')}
      class:order-last={chip.kind === 'range'}
    >
      <span class="truncate {chip.kind === 'range' ? 'max-w-[220px]' : 'max-w-[180px]'}"
        >{#if chip.negParts}{chip.negParts[0]}<span class="font-semibold">{chip.negParts[1]}</span
          >{chip.negParts[2]}{:else}{chip.label}{/if}</span
      >
      <Icon name="x" size={12} class="text-text-muted transition-colors group-hover:text-status-reopen" />
    </button>
  {/each}

  <!-- Add filter -->
  <div class="relative">
    <button
      type="button"
      data-testid="filter-add"
      class="inline-flex h-control-sm items-center gap-1 rounded-md border border-dashed border-border-strong px-2.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => (open ? closeAll() : openMenu())}
    >
      {t('filter.add')}
    </button>

    {#if open}
      <div
        class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-[70vh] w-64 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
      >
        {#if !field && !dateField}
          <!-- Step 1: field pick + date axes + flags -->
          <div class="px-2 py-1 text-micro font-medium text-text-muted">{t('filter.properties')}</div>
          {#each axes as f (f.key)}
            <!-- No per-axis capability caption here (GDK-771): every visible
                 axis excludes now, so the old "No exclude" note was pure
                 noise — the ⊘ affordance lives on the value rows. -->
            <button
              type="button"
              data-testid={`filter-axis-${f.key}`}
              class="flex min-h-control-sm w-full items-center justify-between rounded px-2 py-1 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              onclick={() => pickField(f)}
            >
              <span>{f.label}</span>
              <Icon name="chevron-right" size={13} class="text-text-muted" />
            </button>
          {/each}
          {#each DATE_AXES as d (d.key)}
            <button
              type="button"
              data-testid={`filter-date-axis-${d.key}`}
              class="flex min-h-control-sm w-full items-center justify-between rounded px-2 py-1 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              onclick={() => pickDate(d.key)}
            >
              <span>{d.label}</span>
              <Icon name="chevron-right" size={13} class="text-text-muted" />
            </button>
          {/each}
          <div class="my-1 border-t border-border-subtle"></div>
          <div class="px-2 py-1 text-micro font-medium text-text-muted">{t('filter.quick')}</div>
          {#each [{ k: 'reopened' as const, l: t('filter.flagReopened') }, { k: 'unassigned' as const, l: t('filter.flagUnassigned') }, { k: 'stale' as const, l: t('filter.flagStale') }] as flag (flag.k)}
            <button
              type="button"
              class="flex min-h-control-sm w-full items-center justify-between rounded px-2 py-1 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              onclick={() => filters.toggleFlag(flag.k as 'reopened' | 'unassigned' | 'stale')}
            >
              <span>{flag.l}</span>
              {#if filters.filters[flag.k as 'reopened' | 'unassigned' | 'stale']}
                <Icon name="check" size={13} class="text-accent-text" />
              {/if}
            </button>
          {/each}
        {:else if dateField}
          <div class="flex items-center gap-1 px-1 pb-1">
            <button
              type="button"
              class="flex h-control-sm w-control-sm items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
              onclick={() => (dateField = null)}
              aria-label={t('onboarding.back')}
            >
              <Icon name="chevron-left" size={14} />
            </button>
            <span class="text-micro text-text-secondary">{DATE_AXES.find((d) => d.key === dateField)?.label}</span>
          </div>
          <label class="flex flex-col gap-1 px-2 py-1">
            <span class="text-micro text-text-secondary">{t('filter.dateFrom')}</span>
            <input
              type="date"
              data-testid="filter-date-from"
              value={rangeBound(dateField, 'from')}
              oninput={(e) => setRangeBound(dateField!, 'from', (e.currentTarget as HTMLInputElement).value)}
              class="h-control rounded-md border border-border-strong bg-bg-base px-2.5 text-body text-text-primary outline-none focus:border-accent"
            />
          </label>
          <label class="flex flex-col gap-1 px-2 py-1">
            <span class="text-micro text-text-secondary">{t('filter.dateTo')}</span>
            <input
              type="date"
              data-testid="filter-date-to"
              value={rangeBound(dateField, 'to')}
              oninput={(e) => setRangeBound(dateField!, 'to', (e.currentTarget as HTMLInputElement).value)}
              class="h-control rounded-md border border-border-strong bg-bg-base px-2.5 text-body text-text-primary outline-none focus:border-accent"
            />
          </label>
        {:else}
          <!-- Step 2: value pick -->
          <div class="flex items-center gap-1 px-1 pb-1">
            <button
              type="button"
              class="flex h-control-sm w-control-sm items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
              onclick={() => (field = null)}
              aria-label={t('onboarding.back')}
            >
              <Icon name="chevron-left" size={14} />
            </button>
            <input
              type="text"
              bind:value={valueQuery}
              placeholder={t('filter.searchField', { field: field!.label })}
              class="h-control-sm min-w-0 flex-1 rounded bg-bg-base px-2 text-body text-text-primary placeholder:text-text-muted focus:outline-none"
            />
          </div>
          <div class="max-h-64 overflow-y-auto">
            {#if values.length === 0}
              <div class="px-2 py-3 text-center text-micro text-text-muted">{t('common.noValues')}</div>
            {/if}
            {#each values as v (v.value)}
              {@const excluded = excludeSet.has(v.value)}
              <!-- Tri-state row (GDK-771): click = include, ⊘ (or Alt-click)
                   = exclude, click again = clear. The ⊘ stays visible on an
                   excluded row and appears on hover elsewhere. -->
              <div class="group/vrow flex h-control-sm w-full items-center gap-2 rounded px-2 hover:bg-bg-hover">
                <button
                  type="button"
                  class="flex min-w-0 flex-1 items-center gap-2 text-left text-body text-text-secondary hover:text-text-primary"
                  data-testid="filter-value-row"
                  data-filter-value={v.value}
                  data-state={excluded ? 'excluded' : includeSet.has(v.value) ? 'included' : 'off'}
                  onclick={(e) => toggle(field!, v.value, e)}
                >
                  <span
                    class="flex h-3.5 w-3.5 flex-none items-center justify-center rounded border {excluded
                      ? 'border-status-reopen/70 text-status-reopen'
                      : includeSet.has(v.value)
                        ? 'border-accent bg-accent text-white'
                        : 'border-border-strong'}"
                  >
                    {#if excluded}<Icon name="x" size={10} />{:else if includeSet.has(v.value)}<Icon
                        name="check"
                        size={10}
                      />{/if}
                  </span>
                  <span class="min-w-0 flex-1 truncate {excluded ? 'text-text-muted line-through' : ''}"
                    >{v.label}</span
                  >
                  <span class="flex-none text-micro text-text-muted">{v.count}</span>
                </button>
                {#if negatable}
                  <button
                    type="button"
                    data-testid="filter-value-exclude"
                    class="flex h-4 w-4 flex-none items-center justify-center rounded text-micro transition-colors {excluded
                      ? 'text-status-reopen'
                      : 'text-text-muted opacity-0 hover:text-status-reopen group-hover/vrow:opacity-100'}"
                    title={t('filter.excludeValue', { value: v.label })}
                    aria-label={t('filter.excludeValue', { value: v.label })}
                    aria-pressed={excluded}
                    onclick={() => toggleExclude(field!, v.value)}
                  >
                    <Icon name="ban" size={12} />
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  {#if filters.hasUserChips}
      <button
        type="button"
        data-testid="filter-clear"
        class="inline-flex h-control-sm items-center rounded-md px-2 text-micro text-text-muted transition-colors hover:text-status-reopen"
        onclick={() => filters.clearUserFilters()}
      >
        {t('filter.clear')}
      </button>
  {/if}
</div>
