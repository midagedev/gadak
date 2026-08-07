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
  import Icon from '../ui/Icon.svelte'
  import { t, formatNumber } from '../../lib/i18n'
  import { pages, type DocsTab } from '../../stores/pages.svelte'
  import type { PageLite } from '../../lib/types'
  import EmptyState from '../list/EmptyState.svelte'
  import DocRow from './DocRow.svelte'
  import VirtualRows from '../ui/VirtualRows.svelte'
  import { rowMetrics } from '../../lib/row-metrics'

  const TABS: { key: DocsTab; label: string }[] = [
    { key: 'viewed', label: t('docs.tabViewed') },
    { key: 'updated', label: t('docs.tabUpdated') },
    { key: 'author', label: t('docs.tabAuthor') },
  ]

  const tab = $derived(pages.docsTab)
  const viewed = $derived(pages.recentlyViewed)
  const updated = $derived(pages.recentlyUpdated)
  const authors = $derived(pages.byAuthor)
  const count = $derived(tab === 'viewed' ? viewed.length : updated.length)

  /*
   * All three tabs are one flat row list so a single window serves them. The
   * author tab's group labels ride in that list rather than wrapping it: a
   * per-group container would put every group's rows in the DOM again, which is
   * the thing being removed.
   */
  type Row =
    | { kind: 'header'; author: string; count: number }
    | { kind: 'doc'; page: PageLite; showAuthor: boolean; showExcerpt: boolean }

  const rows = $derived.by<Row[]>(() => {
    if (tab === 'viewed') {
      // No excerpt: you have read these already, so the title is the reminder.
      return viewed.map((page) => ({ kind: 'doc', page, showAuthor: true, showExcerpt: false }))
    }
    if (tab === 'updated') {
      return updated.map((page) => ({ kind: 'doc', page, showAuthor: true, showExcerpt: true }))
    }
    const out: Row[] = []
    for (const group of authors) {
      out.push({ kind: 'header', author: group.author, count: group.pages.length })
      for (const page of group.pages) {
        out.push({ kind: 'doc', page, showAuthor: false, showExcerpt: true })
      }
    }
    return out
  })

  // Mirrors DocRow's own rule: the taller row exists only when there is a body
  // line to put in it, so a page with an empty excerpt stays 42px.
  function rowHeight(item: Row): number {
    const m = rowMetrics()
    if (item.kind === 'header') return m.row
    return item.showExcerpt && (item.page.excerpt ?? '').trim() ? m.rowExcerpt : m.row
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
</script>

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="docs-view">
  <header class="flex flex-none flex-wrap items-center gap-2 border-b border-border-subtle px-4 py-2">
    <h2 class="whitespace-nowrap text-body font-semibold text-text-primary">{t('docs.title')}</h2>
    <span class="flex-none text-micro tabular-nums text-text-muted">{formatNumber(count)}</span>

    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-0.5">
      {#each TABS as entry (entry.key)}
        <button
          type="button"
          class="flex h-control-sm items-center rounded px-2 text-micro font-medium transition-colors {tab ===
          entry.key
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

    <div class="flex-1"></div>
    <button
      type="button"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => pages.closeDocs()}
      title={t('docs.backToIssues')}
      aria-label={t('docs.backToIssues')}
      data-testid="docs-close"
    >
      <Icon name="arrow-left" size={15} />
    </button>
  </header>

  {#if rows.length === 0}
    <div class="min-h-0 flex-1 overflow-y-auto">
      {#if tab === 'viewed'}
        <EmptyState icon="" title={t('docs.viewedEmpty')} hint={t('docs.viewedEmptyHint')} />
      {:else}
        <EmptyState icon="" title={t('docs.recentEmpty')} />
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
              class="truncate text-micro font-semibold uppercase tracking-wider text-text-muted"
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
          />
        {/if}
      {/snippet}
    </VirtualRows>
  {/if}
</section>
