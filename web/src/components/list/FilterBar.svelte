<script lang="ts">
  /*
   * Filter bar ([explore]). Removable chips + "Add filter" dropdown (2-step:
   * field→value) + flag toggles. With active filters, shows "Save as view" /
   * "Reset". Every value toggle writes the filters store (URL) → local refilter.
   */
  import { filters, type FacetValue } from '../../stores/filters.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { emitJql } from '../../lib/api'
  import { isHostedDemo, hasServerVerb } from '../../lib/config'
  import { copyText } from '../../lib/copy-text'
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
  // Exclude mode (GDK-438): picks land in the axis's negation list instead of
  // its include list. Only offered on negatable axes (jira_project / source_project).
  let excludeMode = $state(false)
  let saveOpen = $state(false)
  let saveName = $state('')

  /** Whether the open axis has a negation twin (drives the exclude toggle). */
  const negatable = $derived(!!field && !field.dynamic && !!negationOf(field.key as MultiField))

  const activeSet = $derived.by(() => {
    // Selected values for the open field (for checkmarks) — in exclude mode,
    // checkmarks reflect the negation list, so a value already excluded shows
    // as checked and clicking it un-excludes.
    if (!field) return new Set<string>()
    if (field.dynamic) return new Set(filters.filters.fields[field.key] ?? [])
    const neg = excludeMode ? negationOf(field.key as MultiField) : null
    return new Set(filters.filters[neg ?? (field.key as MultiField)] as string[])
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
    excludeMode = false
  }
  function pickField(f: Axis) {
    field = f
    dateField = null
    valueQuery = ''
    excludeMode = false
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
  function toggle(axis: Axis, value: string) {
    if (axis.dynamic) {
      filters.toggleFieldValue(axis.key, value)
      return
    }
    // Exclude mode moves the pick into the axis's negation list (GDK-438).
    const neg = excludeMode ? negationOf(axis.key as MultiField) : null
    filters.toggleValue(neg ?? (axis.key as MultiField), value)
  }
  function closeAll() {
    open = false
    field = null
    dateField = null
    saveOpen = false
  }

  // Spend Esc so one keystroke cannot also clear the detail panel.
  // preventDefault is what DetailPanel declines; stopPropagation is what the
  // shell keymap needs — it does not read defaultPrevented, and its
  // svelte:window listener is registered first. The delegated onkeydown
  // below reaches the event while it still walks the focused trigger.
  function onEsc(e: KeyboardEvent) {
    if (e.key !== 'Escape' || (!open && !saveOpen)) return
    e.preventDefault()
    e.stopPropagation()
    closeAll()
  }

  async function copyJql() {
    if (isHostedDemo()) {
      write.toast(t('filter.jqlNotAvailable'), 'info')
      return
    }
    try {
      const cfg = filters.currentConfig()
      const res = await emitJql(cfg.filters, cfg.display, me.email)
      if (!res.jql) {
        write.toast(t('filter.jqlEmpty'), 'info')
        return
      }
      if (!(await copyText(res.jql))) {
        write.toast(t('clipboard.copyFailed'), 'error')
        return
      }
      if (res.omitted?.length) {
        write.toast(t('filter.jqlCopiedPartial', { omitted: res.omitted.join(', ') }), 'info')
      } else {
        write.toast(t('filter.jqlCopied'), 'success')
      }
    } catch {
      write.toast(t('filter.jqlNotAvailable'), 'error')
    }
  }

  // GDK-437: with a server behind this bundle the default save is the server
  // (Enter included) — that is what makes a view follow the user across
  // devices. The hosted demo has no server to write to, so it keeps the
  // browser-local default and hides the server choice entirely.
  const saveToServer = hasServerVerb('settings')
  const defaultScope: 'personal' | 'team' = saveToServer ? 'team' : 'personal'

  async function doSave(scope: 'personal' | 'team') {
    const name = saveName.trim()
    if (!name) return
    const config = filters.currentConfig()
    if (scope === 'team' && saveToServer) {
      try {
        await views.addTeam(name, config)
      } catch (e) {
        // Never lose the view quietly: keep it in this browser and say so.
        // The save did land (locally), so the dialog closes like a success —
        // staying open would invite a second, duplicate personal save.
        views.addPersonal(name, config)
        write.toast(t('filter.saveServerFailed'), 'error')
        console.warn('[filterbar] 서버 뷰 저장 실패, 브라우저 저장으로 폴백', e)
        saveName = ''
        saveOpen = false
        return
      }
    } else {
      views.addPersonal(name, config)
    }
    saveName = ''
    saveOpen = false
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="flex flex-wrap items-center gap-1.5"
  onkeydown={onEsc}
  use:onEscape={onEsc}
  use:onOutsideClick={{ handler: closeAll, enabled: open || saveOpen }}
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
            {@const canExclude = !f.dynamic && !!negationOf(f.key as MultiField)}
            <button
              type="button"
              data-testid={`filter-axis-${f.key}`}
              class="flex min-h-control-sm w-full items-center justify-between rounded px-2 py-1 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              onclick={() => pickField(f)}
              title={canExclude ? t('filter.excludeModeHelp') : t('filter.includeOnlyHelp')}
            >
              <span>{f.label}</span>
              <span class="flex items-center gap-1">
                <span class="text-micro text-text-muted"
                  >{canExclude ? t('filter.excludeMode') : t('filter.includeOnly')}</span
                >
                <Icon name="chevron-right" size={13} class="text-text-muted" />
              </span>
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
            {#if negatable}
              <button
                type="button"
                data-testid="filter-exclude-mode"
                class="flex h-control-sm flex-none items-center gap-1 rounded px-1.5 text-micro transition-colors {excludeMode
                  ? 'bg-accent-subtle/40 text-accent-text'
                  : 'text-text-muted hover:bg-bg-hover hover:text-text-primary'}"
                onclick={() => (excludeMode = !excludeMode)}
                title={t('filter.excludeModeHelp')}
              >
                {#if excludeMode}
                  <Icon name="check" size={12} />
                {/if}
                {t('filter.excludeMode')}
              </button>
            {:else}
              <span
                class="flex-none px-1.5 text-micro text-text-muted"
                data-testid="filter-include-only"
                title={t('filter.includeOnlyHelp')}>{t('filter.includeOnly')}</span
              >
            {/if}
          </div>
          <div class="max-h-64 overflow-y-auto">
            {#if values.length === 0}
              <div class="px-2 py-3 text-center text-micro text-text-muted">{t('common.noValues')}</div>
            {/if}
            {#each values as v (v.value)}
              <button
                type="button"
                class="flex h-control-sm w-full items-center gap-2 rounded px-2 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
                onclick={() => toggle(field!, v.value)}
              >
                <span
                  class="flex h-3.5 w-3.5 flex-none items-center justify-center rounded border {activeSet.has(
                    v.value,
                  )
                    ? 'border-accent bg-accent text-white'
                    : 'border-border-strong'}"
                >
                  {#if activeSet.has(v.value)}<Icon name="check" size={10} />{/if}
                </span>
                <span class="min-w-0 flex-1 truncate">{v.label}</span>
                <span class="flex-none text-micro text-text-muted">{v.count}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  {#if filters.hasFilters}
    <button
      type="button"
      class="inline-flex h-control-sm items-center rounded-md px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => void copyJql()}
      data-testid="filter-copy-jql"
      title={t('filter.copyJqlHelp')}
    >
      {t('filter.copyJql')}
    </button>
    <div class="relative">
      <button
        type="button"
        class="inline-flex h-control-sm items-center rounded-md px-2 text-micro text-accent-text transition-colors hover:bg-accent-subtle/40"
        onclick={() => (saveOpen = !saveOpen)}
      >
        {t('filter.saveAsView')}
      </button>
      {#if saveOpen}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 w-80 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-overlay"
        >
          <input
            type="text"
            bind:value={saveName}
            placeholder={t('filter.viewName')}
            class="mb-2 h-control-sm w-full rounded bg-bg-base px-2 text-body text-text-primary placeholder:text-text-muted focus:outline-none"
            onkeydown={(e) => e.key === 'Enter' && doSave(defaultScope)}
          />
          {#if saveToServer}
            <!-- Server first (GDK-437): one hint line per button says where
                 each save lands. Full renaming of the scopes is round C. -->
            <div class="mb-1 flex gap-1.5 text-micro text-text-muted">
              <span class="flex-1">{t('filter.saveServerHint')}</span>
              <span class="flex-1">{t('filter.saveLocalHint')}</span>
            </div>
            <div class="flex gap-1.5">
              <button
                type="button"
                class="h-control-sm flex-1 rounded bg-accent px-2 text-body font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-40"
                disabled={!saveName.trim()}
                onclick={() => doSave('team')}
              >
                {t('filter.saveTeam')}
              </button>
              <button
                type="button"
                class="h-control-sm flex-1 rounded border border-border-strong px-2 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover disabled:opacity-40"
                disabled={!saveName.trim()}
                onclick={() => doSave('personal')}
              >
                {t('filter.savePersonal')}
              </button>
            </div>
          {:else}
            <!-- Hosted demo: every non-GET is a 501 here, so the server save is
                 not offered at all and the save stays browser-local. -->
            <button
              type="button"
              class="h-control-sm w-full rounded bg-accent px-2 text-body font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-40"
              disabled={!saveName.trim()}
              onclick={() => doSave('personal')}
            >
              {t('filter.savePersonal')}
            </button>
            <div class="mt-1.5 text-micro text-text-muted">{t('filter.saveDemoLocal')}</div>
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
  {/if}
</div>
