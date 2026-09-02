<script lang="ts">
  /*
   * One space's documents — reached from the sidebar's Spaces disclosure.
   *
   * The flat list is the default because that is how a page is found
   * (UX_PRINCIPLES §6); the tree is a toggle on the same screen, for the times
   * the question really is "what lives under what". The sidebar carries neither
   * — it must not grow with content volume.
   */
  import { onMount } from 'svelte'
  import ColumnHeader from '../ui/ColumnHeader.svelte'
  import Icon from '../ui/Icon.svelte'
  import Marks from '../ui/Marks.svelte'
  import { t, formatNumber } from '../../lib/i18n'
  import { highlightSegments, mergeAdjacentHits } from '../../lib/format'
  import { pages, type PageNode } from '../../stores/pages.svelte'
  import { docsEmpty } from '../../stores/docs-empty.svelte'
  import { pageMatches } from '../../lib/doc-search'
  import EmptyState from '../list/EmptyState.svelte'
  import DocsFilter from './DocsFilter.svelte'
  import DocRow from './DocRow.svelte'
  import VirtualRows from '../ui/VirtualRows.svelte'
  import { onRowMetricsInvalidated, rowMetrics } from '../../lib/row-metrics'

  let { space }: { space: string } = $props()

  docsEmpty.bind()

  const all = $derived(pages.inSpace(space))
  const tree = $derived(pages.treeBySpace.find((group) => group.space === space))
  const label = $derived(pages.spaceLabel(space))

  /* Same narrowing rule as the tabbed view — title, space and author, locally,
   * on every keystroke, AND-ed with the label a row's chip put on the screen. */
  let filterText = $state('')
  const raw = $derived(filterText.trim())
  const needle = $derived(raw.toLowerCase())
  const labelFilter = $derived(pages.docsLabel)
  const filtering = $derived(needle !== '' || labelFilter !== null)
  const matched = $derived.by(() => {
    if (!filtering) return null
    const keys = new Set<string>()
    for (const page of all) {
      if (labelFilter !== null && !(page.labels ?? []).includes(labelFilter)) continue
      if (pageMatches(page, needle, label, { author: true })) keys.add(page.key)
    }
    return keys
  })

  const docs = $derived(matched ? all.filter((page) => matched.has(page.key)) : all)

  /** Match marks for a tree row's title — the flat list gets the same from
   *  DocRow, and a hit with nothing marked on it reads as an arbitrary row.
   *  Merged the same way too, so a phrase is one mark here and there. */
  const titleSegs = $derived((title: string) =>
    mergeAdjacentHits(highlightSegments(title, raw)),
  )

  /** Tree mode lives in the store because the URL restores it (`dview=tree`)
   *  and this component is remounted per space. */
  const treeMode = $derived(pages.spaceTree)

  /**
   * Expansion as overrides, not an absolute set (GDK-817).
   *
   * The default is structural — roots open, deeper levels shut — and a
   * person's clicks are recorded as deltas against it. An absolute set had
   * to be reconciled with every new index (`pages.reload` swaps the array
   * after each sync), and the reconciler, an effect re-adding "missing"
   * roots, is what re-opened every root that had been collapsed. Against
   * the defaults a reload needs no reconciliation: roots are open by
   * construction, a collapsed root is an override that still says so, and a
   * brand-new root from the sync arrives open — the same welcome the first
   * render gave its siblings. Keys from another space are inert: nothing
   * default mentions them and toggles are only written while this space is
   * up. The `{#key}` below makes each arrival (space, or tree mode back on)
   * start from the defaults again.
   */
  let overrides = $state(new Map<string, boolean>())

  /** What the tree draws as expanded: the defaults, plus this visit's toggles. */
  const expandedKeys = $derived.by(() => {
    const open = new Set<string>()
    for (const root of tree?.roots ?? []) open.add(root.page.key)
    for (const [key, expanded] of overrides) {
      if (expanded) open.add(key)
      else open.delete(key)
    }
    return open
  })

  function toggleDoc(key: string) {
    overrides = new Map(overrides).set(key, !expandedKeys.has(key))
  }

  /** Open a page; a parent also expands, so one click never looks inert. */
  function openDoc(node: PageNode) {
    pages.select(node.page.key)
    if (node.children.length && !expandedKeys.has(node.page.key)) toggleDoc(node.page.key)
  }

  /*
   * The tree is flattened to the rows that are actually visible, so the window
   * can measure it. Recursion stays in the flattening, not in the markup: a
   * space with no hierarchy makes every page a root, and showTree() opens every
   * root, so "collapsed" is no protection against the size of the space.
   */
  /**
   * While the filter is on, the nodes worth drawing: every hit, plus the
   * ancestors that say where it lives. A hit shown without its path is a title
   * floating in a hierarchy screen, which is the one thing this mode is for.
   * Null when nothing is filtered.
   */
  const treeKeep = $derived.by<Set<string> | null>(() => {
    if (!matched) return null
    const keep = new Set<string>()
    const mark = (node: PageNode): boolean => {
      let any = matched.has(node.page.key)
      for (const child of node.children) if (mark(child)) any = true
      if (any) keep.add(node.page.key)
      return any
    }
    for (const root of tree?.roots ?? []) mark(root)
    return keep
  })

  const treeRows = $derived.by(() => {
    const out: PageNode[] = []
    const keep = treeKeep
    const walk = (nodes: PageNode[]) => {
      for (const node of nodes) {
        // Filtering opens the path to every hit. The chevron still records what
        // was clicked; that choice comes back the moment the filter is cleared.
        if (keep) {
          if (!keep.has(node.page.key)) continue
          out.push(node)
          walk(node.children)
          continue
        }
        out.push(node)
        if (expandedKeys.has(node.page.key)) walk(node.children)
      }
    }
    walk(tree?.roots ?? [])
    return out
  })

  // Token-sourced heights (row for the flat list, control for the tree),
  // re-read when a user dims override lands (applyUserTokens →
  // invalidateRowMetrics fires the subscription below) — the issue list's
  // pattern: the snapshot is a signal, so the height props' reads inside
  // VirtualRows' deriveds recompute the window at the new geometry instead
  // of waiting for a remount (GDK-850).
  let metrics = $state(rowMetrics())

  onMount(() => {
    const offMetrics = onRowMetricsInvalidated(() => {
      metrics = rowMetrics()
    })
    return () => {
      offMetrics()
    }
  })
</script>

<!-- One page row in the tree. The indent step is the toggle's own width, so a
     child's chevron sits exactly under where its parent's title begins — a step
     narrower than the toggle (it used to be 18px against a 24px control) leaves
     every level slightly out of true, which is what makes deep trees hard to
     read. This is the main column, not the sidebar: rows carry the body size and
     the list's row rhythm rather than the nav's compressed one. Children are not
     rendered from here — treeRows already holds them in visual order, which is
     what lets the list be windowed. -->
{#snippet docNode(node: PageNode)}
  {@const expanded = treeKeep
    ? node.children.some((child) => treeKeep.has(child.page.key))
    : expandedKeys.has(node.page.key)}
  {@const selected = pages.selectedKey === node.page.key}
  <!-- While filtering, a row is either an answer or the path to one. The path
       stays legible but recedes, so "where does this live" and "what did I ask
       for" are not the same weight (vision verdict 2026-08-07). -->
  {@const hit = !matched || matched.has(node.page.key)}
  <div
    class="group flex min-h-control items-center rounded-md pr-3 text-body transition-colors {selected
      ? 'bg-bg-active'
      : 'hover:bg-bg-hover'}"
    style="padding-left: calc(8px + {node.depth} * var(--spacing-control-sm))"
    data-testid="doc-tree-node"
    data-hit={hit ? 'true' : 'false'}
  >
    {#if node.children.length}
      <button
        type="button"
        class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        aria-expanded={expanded}
        aria-label={t('sidebar.docsToggleNode', { title: node.page.title })}
        data-testid="doc-tree-toggle"
        onclick={() => toggleDoc(node.page.key)}
      >
        <Icon
          name="chevron-right"
          size={14}
          class="transition-transform duration-150 {expanded ? 'rotate-90' : ''}"
        />
      </button>
    {:else}
      <!-- Keeps leaf titles on the same left edge as their siblings' -->
      <span class="h-control-sm w-control-sm flex-none" aria-hidden="true"></span>
    {/if}
    <button
      type="button"
      class="flex min-w-0 flex-1 items-center gap-2 py-1 text-left {selected
        ? 'text-text-primary'
        : hit
          ? 'text-text-secondary group-hover:text-text-primary'
          : 'text-text-muted group-hover:text-text-secondary'}"
      title={node.page.title}
      onclick={() => openDoc(node)}
    >
      <span class="min-w-0 truncate"
        ><Marks segs={titleSegs(node.page.title)} /></span
      >
      {#if node.children.length}
        <!-- What a collapsed branch is hiding. It rides with the title rather
             than in a right-hand column: pushed to the edge, the numbers line up
             into a structure of their own, and the count is an attribute of the
             row, not a second axis to read. -->
        <span
          class="flex-none text-micro tabular-nums text-text-muted"
          data-testid="doc-tree-count"
          title={t('docs.treeChildCount', { n: node.children.length })}
        >
          {formatNumber(node.children.length)}
        </span>
      {/if}
    </button>
  </div>
{/snippet}

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="space-docs-view" data-space={space}>
  <ColumnHeader
    title={label}
    titleAttr={space}
    count={filtering
      ? `${formatNumber(docs.length)} / ${formatNumber(all.length)}`
      : formatNumber(all.length)}
    countTestid="docs-count"
    closeTestid="docs-close"
    onClose={() => pages.closeDocs()}
  >
    {#if labelFilter}
      <!-- Same chip as the tabbed view: the narrowing is stated where the count
           is, and removed from the same place. -->
      <button
        type="button"
        class="group mr-2 flex h-control-sm flex-none items-center gap-1 rounded-full bg-bg-elevated pl-2.5 pr-1.5 text-micro text-text-primary hover:bg-bg-active"
        data-testid="docs-label-chip"
        data-label={labelFilter}
        title={t('docs.labelClear', { label: labelFilter })}
        aria-label={t('docs.labelClear', { label: labelFilter })}
        onclick={() => pages.setDocsLabel(null)}
      >
        <span class="max-w-[140px] truncate">{labelFilter}</span>
        <Icon name="x" size={11} class="text-text-muted group-hover:text-text-primary" />
      </button>
    {/if}

    <!-- p-1: same 32px wrapper as DocsView's tabs, for the same header-row
         height rule (vision verdict 2026-08-07). -->
    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-1">
      <button
        type="button"
        class="flex h-control-sm items-center rounded px-2 text-micro font-medium {treeMode
          ? 'text-text-muted hover:text-text-secondary'
          : 'bg-bg-active text-text-primary'}"
        aria-pressed={!treeMode}
        data-testid="space-list-toggle"
        onclick={() => (pages.spaceTree = false)}
      >
        {t('docs.viewList')}
      </button>
      <button
        type="button"
        class="flex h-control-sm items-center rounded px-2 text-micro font-medium {treeMode
          ? 'bg-bg-active text-text-primary'
          : 'text-text-muted hover:text-text-secondary'}"
        aria-pressed={treeMode}
        data-testid="space-tree-toggle"
        onclick={() => (pages.spaceTree = true)}
      >
        {t('docs.viewTree')}
      </button>
    </div>

    {#snippet trailing()}
      <div class="w-full min-w-0 max-w-[300px]"><DocsFilter bind:value={filterText} /></div>
    {/snippet}
  </ColumnHeader>

  {#if docs.length === 0}
    <div
      class="min-h-0 flex-1 overflow-y-auto"
      data-docs-empty-state={filtering ? undefined : docsEmpty.state}
    >
      {#if filtering}
        <EmptyState
          icon="search-x"
          title={t('docs.filterEmpty')}
          hint={needle
            ? t('docs.filterEmptyHint')
            : t('docs.filterEmptyLabelHint', { label: labelFilter ?? '' })}
        />
      {:else}
        <EmptyState
          icon="file"
          title={t('docs.recentEmpty')}
          hint={docsEmpty.copy.hintKey ? t(docsEmpty.copy.hintKey) : ''}
        />
      {/if}
    </div>
  {:else if treeMode}
    <!-- {#key} on the arrival, not on the data: entering a space or switching
         the tree back on starts from the defaults (roots open), while a sync
         that swaps the index underneath keeps every override — the split the
         GDK-817 fix is (a reload is not an arrival). -->
    {#key space + ':' + treeMode}
      <!-- Capped width: a hierarchy read left-to-right gains nothing from a
           1300px column, and unbounded rows lose the parent–child alignment.
           The cap rides on the windowed slice, where horizontal padding is safe;
           vertical padding there would scroll with the window instead of the
           list, so the header's rule is the top edge. -->
      <VirtualRows
        rows={treeRows}
        height={() => metrics.control}
        key={(node) => node.page.key}
        innerClass="max-w-[720px] px-3"
        testid="space-tree-scroll"
      >
        {#snippet row(node)}
          {@render docNode(node)}
        {/snippet}
      </VirtualRows>
    {/key}
  {:else}
    <!-- The space is the screen, so every row would repeat it — dropped. -->
    <VirtualRows
      rows={docs}
      height={() => metrics.row}
      key={(page) => page.key}
      testid="space-list-scroll"
    >
      {#snippet row(page)}
        <!-- Labels are on here and off in the tree: this is the list read to
             find something, and the tree is read to see where things sit. -->
        <DocRow {page} showSpace={false} showLabels q={raw} />
      {/snippet}
    </VirtualRows>
  {/if}
</section>
