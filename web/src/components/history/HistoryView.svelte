<script lang="ts">
  /*
   * History — first-class main-column view of what this profile looked at
   * and searched for. Visual language follows DocsView: header, segmented
   * kind filter, local narrow, grouped rows. No new tokens.
   */
  import { untrack } from 'svelte'
  import Icon from '../ui/Icon.svelte'
  import { t, formatNumber, relativeSeenLabel, absTime } from '../../lib/i18n'
  import {
    aggregateHistory,
    dateGroup,
    issueKeysOf,
    type DateGroup,
    type HistoryKindFilter,
    type TimelineEntry,
  } from '../../lib/history'
  import { emptyConfig } from '../../lib/view-config'
  import { rowMetrics } from '../../lib/row-metrics'
  import { history } from '../../stores/history.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { applyServerSearchOutcome } from '../../lib/server-search'
  import { showIssueList } from '../../lib/show-issue-list'
  import { filters } from '../../stores/filters.svelte'
  import { me } from '../../stores/me.svelte'
  import EmptyState from '../list/EmptyState.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import VirtualRows from '../ui/VirtualRows.svelte'

  const TABS: { key: HistoryKindFilter; label: string }[] = [
    { key: '', label: t('history.tabAll') },
    { key: 'issue', label: t('history.tabIssues') },
    { key: 'page', label: t('history.tabDocs') },
    { key: 'search', label: t('history.tabSearches') },
  ]

  const GROUP_LABEL: Record<DateGroup, string> = {
    today: t('history.groupToday'),
    yesterday: t('history.groupYesterday'),
    week: t('history.groupThisWeek'),
    older: t('history.groupOlder'),
  }

  $effect(() => {
    if (!pages.historyView) return
    untrack(() => void history.load())
  })

  const filterText = $derived(history.filterText)
  const raw = $derived(filterText.trim())
  const needle = $derived(raw.toLowerCase())
  const filtering = $derived(needle !== '')

  function textMatch(hay: string): boolean {
    if (!needle) return true
    return hay.toLowerCase().includes(needle)
  }

  function titleOf(entry: TimelineEntry): string {
    if (entry.type === 'search') return entry.query
    if (entry.kind === 'page') {
      const page = pages.byKey.get(entry.key)
      return page?.title ?? entry.key
    }
    return issues.pool.get(entry.key)?.summary ?? entry.key
  }

  function subtitleOf(entry: TimelineEntry): string {
    if (entry.type === 'search') {
      return [entry.openedKey ?? '', String(entry.resultCount ?? '')].join(' ')
    }
    if (entry.kind === 'page') {
      const page = pages.byKey.get(entry.key)
      return page ? pages.spaceLabel(page.space_key) : ''
    }
    return entry.key
  }

  function keep(entry: TimelineEntry): boolean {
    if (!filtering) return true
    return textMatch(titleOf(entry)) || textMatch(subtitleOf(entry)) || textMatch(entry.type === 'visit' ? entry.key : entry.query)
  }

  const entries = $derived(aggregateHistory(history.items).filter(keep))
  const listKeys = $derived(issueKeysOf(entries))

  type Row =
    | { kind: 'group'; group: DateGroup }
    | { kind: 'entry'; entry: TimelineEntry }
    | { kind: 'more' }

  const rows = $derived.by<Row[]>(() => {
    const out: Row[] = []
    let prev: DateGroup | null = null
    for (const entry of entries) {
      const g = dateGroup(entry.at)
      if (g !== prev) {
        out.push({ kind: 'group', group: g })
        prev = g
      }
      out.push({ kind: 'entry', entry })
    }
    if (history.nextCursor && !filtering) out.push({ kind: 'more' })
    return out
  })

  function rowHeight(item: Row): number {
    const m = rowMetrics()
    if (item.kind === 'group' || item.kind === 'more') return m.row
    if (item.entry.type === 'visit' && item.entry.kind === 'issue') return m.row
    return m.rowExcerpt
  }

  function rowKey(item: Row, index: number): string {
    if (item.kind === 'group') return `g:${item.group}:${index}`
    if (item.kind === 'more') return 'more'
    if (item.entry.type === 'search') return `s:${item.entry.id}`
    return `v:${item.entry.kind}:${item.entry.key}`
  }

  function openEntry(entry: TimelineEntry): void {
    if (entry.type === 'search') {
      me.closeFeed()
      pages.closeDocs()
      filters.setQuery(entry.query)
      void filters.runServerSearch().then(applyServerSearchOutcome)
      return
    }
    if (entry.kind === 'page') pages.select(entry.key)
    else selection.select(entry.key)
  }

  /**
   * Hand the visited issues to the list *in visit order*. `emptyConfig()`
   * carries `defaultDisplay()`, whose grouping is `status_category`, so the
   * grouping is named here rather than left to the default — otherwise the
   * order this pane exists to show is shredded into status buckets, and
   * `configToParams` (whose contextual default for a keys view is already
   * `none`) writes the regrouping into the URL as `g=status_category`.
   * Order itself needs no sort: `filters.effectiveSort` promotes a keys-only
   * view to the `keys` sort, which is the given order.
   */
  function openAsList(): void {
    if (!listKeys.length) return
    const c = emptyConfig()
    c.filters.keys = listKeys
    c.display.group_by = 'none'
    showIssueList(c)
  }

  function onFilterKey(e: KeyboardEvent): void {
    if (e.key !== 'Escape') return
    e.preventDefault()
    if (filterText) history.filterText = ''
    else (e.target as HTMLElement).blur()
  }
</script>

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="history-view">
  <header class="flex flex-none flex-wrap items-center gap-2 border-b border-border-subtle px-4 py-2">
    <h2 class="whitespace-nowrap text-body font-semibold text-text-primary">{t('history.title')}</h2>
    <span class="flex-none text-micro tabular-nums text-text-muted" data-testid="history-count">
      {formatNumber(entries.length)}
    </span>

    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-1">
      {#each TABS as entry (entry.key)}
        <button
          type="button"
          class="flex h-control-sm items-center rounded px-2 text-micro font-medium transition-colors {history.kind ===
          entry.key
            ? 'bg-bg-active text-text-primary'
            : 'text-text-muted hover:text-text-secondary'}"
          aria-pressed={history.kind === entry.key}
          data-testid="history-tab"
          data-tab={entry.key || 'all'}
          onclick={() => history.setKind(entry.key)}
        >
          {entry.label}
        </button>
      {/each}
    </div>

    {#if listKeys.length}
      <button
        type="button"
        class="flex h-control-sm flex-none items-center rounded-md px-2 text-micro font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        data-testid="history-open-as-list"
        onclick={openAsList}
      >
        {t('history.openAsList')}
      </button>
    {/if}

    <div class="ml-auto min-w-0 max-w-[300px] flex-1">
      <div
        class="flex h-control items-center gap-2 rounded-md border border-border-strong/70 bg-bg-elevated px-3 shadow-sm shadow-black/10 focus-within:border-accent/70"
      >
        <Icon name="search" size={14} class="text-text-muted" />
        <input
          value={filterText}
          oninput={(e) => (history.filterText = (e.currentTarget as HTMLInputElement).value)}
          onkeydown={onFilterKey}
          type="text"
          data-testid="history-filter-input"
          placeholder={t('history.filterPlaceholder')}
          aria-label={t('history.filterLabel')}
          class="min-w-0 flex-1 bg-transparent text-body text-text-primary placeholder:text-text-muted focus:outline-none"
          spellcheck="false"
          autocomplete="off"
        />
        {#if filterText}
          <button
            type="button"
            class="flex flex-none items-center text-text-muted hover:text-text-primary"
            onclick={() => (history.filterText = '')}
            title={t('history.filterClear')}
            aria-label={t('history.filterClear')}
          >
            <Icon name="x" size={13} />
          </button>
        {:else}
          <kbd class="flex-none rounded border border-border-subtle px-1 text-micro text-text-muted">/</kbd>
        {/if}
      </div>
    </div>
    <button
      type="button"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => pages.closeHistory()}
      title={t('docs.backToIssues')}
      aria-label={t('docs.backToIssues')}
      data-testid="history-close"
    >
      <Icon name="arrow-left" size={15} />
    </button>
  </header>

  {#if !history.loaded && history.loading}
    <div class="min-h-0 flex-1">
      <LoadingState label={t('common.loading')} />
    </div>
  {:else if rows.length === 0}
    <div class="min-h-0 flex-1 overflow-y-auto">
      {#if filtering}
        <EmptyState icon="search-x" title={t('history.filterEmpty')} />
      {:else}
        <EmptyState icon="clock" title={t('history.empty')} hint={t('history.emptyHint')} />
      {/if}
    </div>
  {:else}
    <VirtualRows {rows} height={rowHeight} key={rowKey} testid="history-scroll">
      {#snippet row(item)}
        {#if item.kind === 'group'}
          <div
            class="flex h-row items-end gap-2 px-4 pb-1.5"
            data-testid="history-group"
            data-group={item.group}
          >
            <span class="truncate text-micro font-medium uppercase tracking-wide text-text-muted">
              {GROUP_LABEL[item.group]}
            </span>
            <span class="h-px flex-1 self-center bg-border-subtle"></span>
          </div>
        {:else if item.kind === 'more'}
          <button
            type="button"
            class="flex h-row-excerpt w-full items-center justify-center text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
            data-testid="history-load-more"
            disabled={history.loading}
            onclick={() => void history.loadMore()}
          >
            {t('history.loadMore')}
          </button>
        {:else if item.entry.type === 'search'}
          {@const entry = item.entry}
          <button
            type="button"
            class="flex h-row-excerpt w-full flex-col justify-center gap-0.5 px-4 py-1.5 text-left transition-colors hover:bg-bg-hover"
            data-testid="history-row"
            data-type="search"
            data-query={entry.query}
            onclick={() => openEntry(entry)}
          >
            <span class="flex w-full min-w-0 items-center gap-2">
              <Icon name="search" size={13} class="flex-none text-text-muted" />
              <span class="min-w-0 flex-1 truncate text-body text-text-primary">{entry.query}</span>
              <span
                class="flex-none text-micro text-text-muted"
                title={absTime(entry.at)}
              >
                {relativeSeenLabel(entry.at)}
              </span>
            </span>
            <span class="flex min-w-0 items-center gap-2 text-micro text-text-muted">
              {#if entry.resultCount != null}
                <span>{t('history.searchResults', { n: formatNumber(entry.resultCount) })}</span>
              {/if}
              {#if entry.openedKey}
                <span data-testid="history-search-opened">
                  {t('history.searchOpened', { key: entry.openedKey })}
                </span>
              {/if}
            </span>
          </button>
        {:else}
          {@const entry = item.entry}
          {@const selected =
            entry.kind === 'page'
              ? pages.selectedKey === entry.key
              : selection.selectedKey === entry.key}
          <button
            type="button"
            class="flex w-full flex-col justify-center gap-0.5 px-4 py-1.5 text-left transition-colors {entry.kind ===
            'page'
              ? 'h-row-excerpt'
              : 'h-row'} {selected ? 'bg-bg-active' : 'hover:bg-bg-hover'}"
            data-testid="history-row"
            data-type="visit"
            data-kind={entry.kind}
            data-key={entry.key}
            onclick={() => openEntry(entry)}
          >
            <span class="flex w-full min-w-0 items-center gap-2">
              {#if entry.kind === 'page'}
                <span
                  class="flex-none rounded bg-bg-active px-1.5 py-0.5 text-micro font-medium uppercase tracking-wide text-text-muted"
                >
                  {t('doc.badge')}
                </span>
                <span class="min-w-0 flex-1 truncate text-body font-medium text-text-primary">
                  {titleOf(entry)}
                </span>
              {:else}
                <span class="w-[70px] flex-none truncate font-mono text-micro text-accent-text">
                  {entry.key}
                </span>
                <span class="min-w-0 flex-1 truncate text-body font-medium text-text-primary">
                  {titleOf(entry)}
                </span>
              {/if}
              {#if entry.count >= 2}
                <span
                  class="flex-none text-micro tabular-nums text-text-muted"
                  data-testid="history-visit-count"
                >
                  {t('history.visitCount', { n: entry.count })}
                </span>
              {/if}
              <span
                class="flex-none text-micro text-text-muted"
                title={absTime(entry.at)}
              >
                {relativeSeenLabel(entry.at)}
              </span>
            </span>
            {#if entry.kind === 'page'}
              <span class="truncate text-micro text-text-muted">{subtitleOf(entry)}</span>
            {/if}
          </button>
        {/if}
      {/snippet}
    </VirtualRows>
  {/if}
</section>
