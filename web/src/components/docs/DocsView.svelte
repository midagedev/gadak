<script lang="ts">
  /*
   * Documents — the main-column view for finding a page (UX_PRINCIPLES §6).
   *
   * Three parallel tabs, never merged: Viewed is your own return path and wins
   * the default; Updated is everyone else's activity; By author answers "who
   * wrote this". The tree is not here at all — hierarchy is for orienting once
   * a document is open (breadcrumbs) and for a space that is asked for by name.
   *
   * It is the main column rather than an overlay so the reading surface stays
   * open beside it: clicking a row opens the document in the right panel and the
   * list keeps its place, which is how someone skims several edits in one pass.
   * Everything is client-side over the page index — no request of its own.
   */
  import { onMount } from 'svelte'
  import ColumnHeader from '../ui/ColumnHeader.svelte'
  import Icon from '../ui/Icon.svelte'
  import { t, formatNumber } from '../../lib/i18n'
  import { pages, type DocsTab } from '../../stores/pages.svelte'
  import { docsEmpty } from '../../stores/docs-empty.svelte'
  import { pageMatches } from '../../lib/doc-search'
  import { docsListEmptyKind } from '../../lib/docs-empty'
  import type { PageLite } from '../../lib/types'
  import EmptyState from '../list/EmptyState.svelte'
  import DocsFilter from './DocsFilter.svelte'
  import DocRow from './DocRow.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import VirtualRows from '../ui/VirtualRows.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { onRowMetricsInvalidated, rowMetrics } from '../../lib/row-metrics'

  docsEmpty.bind()

  // In flight ≠ empty: while the index request is unanswered the settled
  // empty copy must not stand in for it (GDK-1054 finding 12). Same grace
  // owner as every other read path — a load that lands inside the window
  // paints nothing at all.
  const skeleton = createSkeletonGrace(() => pages.loading && !pages.loaded)

  const TABS: { key: DocsTab; label: string }[] = [
    { key: 'viewed', label: t('docs.tabViewed') },
    { key: 'updated', label: t('docs.tabUpdated') },
    { key: 'author', label: t('docs.tabAuthor') },
  ]

  const tab = $derived(pages.docsTab)

  /*
   * Narrowing, not searching: the filter keeps each tab's own order (recency,
   * author groups) and only removes rows, so the list someone is reading never
   * reshuffles under them. Title, space and author are the three things the row
   * itself shows — filtering by anything it does not display would make hits
   * look arbitrary. Every keystroke is a pass over the loaded index; nothing
   * here is allowed to reach the network.
   */
  let filterText = $state('')
  const raw = $derived(filterText.trim())
  const needle = $derived(raw.toLowerCase())
  /** The label a chip click put on the screen, and the way back out of it. */
  const label = $derived(pages.docsLabel)
  const filtering = $derived(needle !== '' || label !== null)

  /* Two narrowings, AND-ed: the text says what, the label says which kind. */
  const keep = $derived(
    (page: PageLite) =>
      (label === null || (page.labels ?? []).includes(label)) &&
      pageMatches(page, needle, pages.spaceLabel(page.space_key), { author: true }),
  )
  const narrow = $derived((list: PageLite[]) => (filtering ? list.filter(keep) : list))

  const viewed = $derived(narrow(pages.recentlyViewed))
  const updated = $derived(narrow(pages.recentlyUpdated))
  const authors = $derived.by(() => {
    if (!filtering) return pages.byAuthor
    // A group whose every page is filtered out goes with them — an author
    // heading over nothing claims a match that is not there.
    return pages.byAuthor
      .map((group) => ({ author: group.author, pages: group.pages.filter(keep) }))
      .filter((group) => group.pages.length > 0)
  })

  /** The tab's own total, before the filter — the denominator of "3 / 47". */
  const total = $derived(
    tab === 'viewed' ? pages.recentlyViewed.length : pages.index.length,
  )
  const count = $derived(
    tab === 'viewed'
      ? viewed.length
      : tab === 'updated'
        ? updated.length
        : authors.reduce((n, group) => n + group.pages.length, 0),
  )

  /*
   * All three tabs are one flat row list so a single window serves them. The
   * author tab's group labels ride in that list rather than wrapping it: a
   * per-group container would put every group's rows in the DOM again, which is
   * the thing being removed.
   */
  type Row =
    | { kind: 'header'; author: string; count: number }
    | {
        kind: 'doc'
        page: PageLite
        showAuthor: boolean
        showExcerpt: boolean
        showLabels: boolean
      }

  const rows = $derived.by<Row[]>(() => {
    if (tab === 'viewed') {
      // No excerpt, no labels: you have read these already, so the title is the
      // reminder and the row is a bookmark rather than a query to follow.
      return viewed.map((page) => ({
        kind: 'doc' as const,
        page,
        showAuthor: true,
        showExcerpt: false,
        showLabels: false,
      }))
    }
    if (tab === 'updated') {
      return updated.map((page) => ({
        kind: 'doc' as const,
        page,
        showAuthor: true,
        showExcerpt: true,
        showLabels: true,
      }))
    }
    const out: Row[] = []
    for (const group of authors) {
      out.push({ kind: 'header', author: group.author, count: group.pages.length })
      for (const page of group.pages) {
        out.push({ kind: 'doc', page, showAuthor: false, showExcerpt: true, showLabels: true })
      }
    }
    return out
  })

  const emptyKind = $derived(
    docsListEmptyKind({
      empty: rows.length === 0,
      filtering,
      hasNeedle: needle !== '',
      tab,
    }),
  )

  // Token-sourced heights, re-read when a user dims override lands
  // (applyUserTokens → invalidateRowMetrics fires the subscription below) —
  // the issue list's pattern: the snapshot is a signal, so the height prop's
  // reads inside VirtualRows' deriveds recompute the window at the new
  // geometry instead of waiting for a remount (GDK-850).
  let metrics = $state(rowMetrics())

  // Mirrors DocRow's own rule: the taller row exists only when there is a body
  // line to put in it, so a page with an empty excerpt stays 42px.
  function rowHeight(item: Row): number {
    if (item.kind === 'header') return metrics.row
    return item.showExcerpt && (item.page.excerpt ?? '').trim() ? metrics.rowExcerpt : metrics.row
  }

  function rowKey(item: Row): string {
    return item.kind === 'header' ? `h:${item.author}` : `d:${item.page.key}`
  }

  let list = $state<ReturnType<typeof VirtualRows> | null>(null)

  // Someone arriving from a person lands on that person's group rather than at
  // the top of a recency-ordered list. One shot: the store clears the request
  // here so a later tab visit opens where it always did.
  //
  // An index, not scrollIntoView — the row being scrolled to is outside the
  // window, so there is no element to scroll into view.
  $effect(() => {
    const author = pages.focusAuthor
    if (!author || tab !== 'author' || !list) return
    const index = rows.findIndex((r) => r.kind === 'header' && r.author === author)
    if (index >= 0) list.scrollToIndex(index)
    pages.focusAuthor = null
  })

  // Re-snapshot the token-sourced heights on invalidation; tear the
  // subscription down with the view so a later token change cannot write to
  // a destroyed component's state.
  onMount(() => {
    const offMetrics = onRowMetricsInvalidated(() => {
      metrics = rowMetrics()
    })
    return () => {
      offMetrics()
    }
  })
</script>

<section
  class="flex h-full min-h-0 flex-col bg-bg-base"
  data-testid="docs-view"
  data-skeleton={skeleton.attr}
>
  <ColumnHeader
    title={t('docs.title')}
    count={filtering ? `${formatNumber(count)} / ${formatNumber(total)}` : formatNumber(count)}
    countTestid="docs-count"
    closeTestid="docs-close"
    onClose={() => pages.closeDocs()}
  >
    {#if label}
      <!--
        The narrowing a row's chip put on the screen, stated where the count is,
        and removable in the same place. A filter that only shows in the rows it
        removed is a filter someone forgets is on. It reads as a chip, not as a
        fourth piece of the segmented control beside it: pill corner, wider seam
        (`mr-2` on top of the header's gap-2), x a tier below the label
        (vision verdict 2026-08-07).
      -->
      <button
        type="button"
        class="group mr-2 flex h-control-sm flex-none items-center gap-1 rounded-full bg-bg-elevated pl-2.5 pr-1.5 text-micro text-text-primary hover:bg-bg-active"
        data-testid="docs-label-chip"
        data-label={label}
        title={t('docs.labelClear', { label: label })}
        aria-label={t('docs.labelClear', { label: label })}
        onclick={() => pages.setDocsLabel(null)}
      >
        <span class="max-w-[140px] truncate">{label}</span>
        <Icon name="x" size={11} class="text-text-muted group-hover:text-text-primary" />
      </button>
    {/if}

    <!-- p-1 so the wrapper stands 32px like the filter input beside it: two
         controls on one header row at two heights read as two size classes
         (vision verdict 2026-08-07). -->
    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-1">
      {#each TABS as entry (entry.key)}
        <button
          type="button"
          class="flex h-control-sm items-center rounded px-2 text-micro font-medium {tab === entry.key
            ? 'bg-bg-active text-text-primary'
            : 'text-text-muted hover:text-text-secondary'}"
          aria-pressed={tab === entry.key}
          data-testid="docs-tab"
          data-tab={entry.key}
          onclick={() => pages.selectTab(entry.key)}
        >
          {entry.label}
        </button>
      {/each}
    </div>

    {#snippet trailing()}
      <div class="w-full min-w-0 max-w-[300px]"><DocsFilter bind:value={filterText} /></div>
    {/snippet}
  </ColumnHeader>

  {#if pages.loadFailed}
    <!-- Failure is not emptiness (GDK-1054): the request answered with an
         error, so the error line replaces whatever the empty copy would say
         about a mirror that may hold documents. Same shape as DetailPanel's
         network branch. -->
    <div class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
      <p class="text-body text-text-secondary" data-testid="docs-load-error">
        {t('docs.loadFailed')}
      </p>
      <button
        type="button"
        onclick={() => void pages.reload()}
        class="rounded-md border border-border-strong px-3 py-1.5 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('common.retry')}
      </button>
    </div>
  {:else if !pages.loaded}
    {#if skeleton.visible}
      <div class="min-h-0 flex-1"><LoadingState label={t('common.loading')} /></div>
    {/if}
  {:else if emptyKind}
    <div
      class="min-h-0 flex-1 overflow-y-auto"
      data-docs-empty-state={emptyKind === 'recent' ? docsEmpty.state : undefined}
    >
      {#if emptyKind === 'filter-text' || emptyKind === 'filter-label'}
        <!-- Nothing here matched, but the mirror is bigger than this tab —
             Enter is the way to the rest of it. -->
        <EmptyState
          icon="search-x"
          title={t('docs.filterEmpty')}
          hint={emptyKind === 'filter-text'
            ? t('docs.filterEmptyHint')
            : t('docs.filterEmptyLabelHint', { label: label ?? '' })}
        />
      {:else if emptyKind === 'viewed'}
        <EmptyState icon="clock" title={t('docs.viewedEmpty')} hint={t('docs.viewedEmptyHint')} />
      {:else}
        <EmptyState
          icon="file"
          title={t('docs.recentEmpty')}
          hint={docsEmpty.copy.hintKey ? t(docsEmpty.copy.hintKey) : ''}
        />
      {/if}
    </div>
  {:else}
    <VirtualRows
      bind:this={list}
      {rows}
      height={rowHeight}
      key={rowKey}
      testid="docs-scroll"
    >
      {#snippet row(item)}
        {#if item.kind === 'header'}
          <!-- Group label line, same rhythm as the issue list's group headers. -->
          <div
            class="flex h-row items-end gap-2 px-4 pb-1.5"
            data-testid="docs-author-group"
            data-author={item.author}
          >
            <span
              class="truncate text-micro font-medium uppercase tracking-wide text-text-muted"
            >
              {item.author || t('docs.authorUnknown')}
            </span>
            <span class="flex-none text-micro tabular-nums text-text-muted">
              {formatNumber(item.count)}
            </span>
            <span class="h-px flex-1 self-center bg-border-subtle"></span>
          </div>
        {:else}
          <!-- Someone else's edit: the title alone rarely says whether it is
               worth opening, so those rows carry one line of the body. Viewed
               does not — you have already read those. -->
          <DocRow
            page={item.page}
            showAuthor={item.showAuthor}
            showExcerpt={item.showExcerpt}
            showLabels={item.showLabels}
            q={raw}
          />
        {/if}
      {/snippet}
    </VirtualRows>
  {/if}
</section>
