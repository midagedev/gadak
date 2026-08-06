<script lang="ts">
  /*
   * One space's documents — reached from the sidebar's Spaces disclosure.
   *
   * The flat list is the default because that is how a page is found
   * (UX_PRINCIPLES §6); the tree is a toggle on the same screen, for the times
   * the question really is "what lives under what". The sidebar carries neither
   * — it must not grow with content volume.
   */
  import { ArrowLeft } from '@lucide/svelte'
  import Icon from '../ui/Icon.svelte'
  import { t, formatNumber } from '../../lib/i18n'
  import { pages, type PageNode } from '../../stores/pages.svelte'
  import DocRow from './DocRow.svelte'

  let { space }: { space: string } = $props()

  const docs = $derived(pages.inSpace(space))
  const tree = $derived(pages.treeBySpace.find((group) => group.space === space))
  const label = $derived(pages.spaceLabel(space))

  let treeMode = $state(false)
  /** Expanded page nodes, by key. */
  let openDocs = $state(new Set<string>())

  function toggleDoc(key: string) {
    const next = new Set(openDocs)
    if (!next.delete(key)) next.add(key)
    openDocs = next
  }

  /** A space normally has one root page, so opening onto a single collapsed row
   *  reads as a broken toggle. Roots open with the tree; deeper levels stay shut. */
  function showTree() {
    treeMode = true
    const next = new Set(openDocs)
    for (const root of tree?.roots ?? []) next.add(root.page.key)
    openDocs = next
  }

  /** Open a page; a parent also expands, so one click never looks inert. */
  function openDoc(node: PageNode) {
    pages.select(node.page.key)
    if (node.children.length && !openDocs.has(node.page.key)) toggleDoc(node.page.key)
  }
</script>

<!-- One page row in the tree, recursing into its children. Indent is a fixed
     step per depth so a leaf lines up under its parent's title. This is the
     main column, not the sidebar: rows carry the body size and the list's row
     rhythm rather than the nav's compressed one. -->
{#snippet docNode(node: PageNode)}
  {@const expanded = openDocs.has(node.page.key)}
  {@const selected = pages.selectedKey === node.page.key}
  <div
    class="group flex min-h-control items-center rounded-md pr-3 text-body transition-colors {selected
      ? 'bg-bg-active'
      : 'hover:bg-bg-hover'}"
    style="padding-left: {8 + node.depth * 18}px"
    data-testid="doc-tree-node"
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
      class="min-w-0 flex-1 truncate py-1 pl-1 text-left {selected
        ? 'text-text-primary'
        : 'text-text-secondary group-hover:text-text-primary'}"
      title={node.page.title}
      onclick={() => openDoc(node)}
    >
      {node.page.title}
    </button>
  </div>
  {#if expanded}
    {#each node.children as child (child.page.key)}
      {@render docNode(child)}
    {/each}
  {/if}
{/snippet}

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="space-docs-view" data-space={space}>
  <header class="flex flex-none flex-wrap items-center gap-2 border-b border-border-subtle px-4 py-2">
    <h2 class="truncate text-[13px] font-semibold text-text-primary" title={space}>{label}</h2>
    <span class="flex-none text-[11px] tabular-nums text-text-muted">{formatNumber(docs.length)}</span>

    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-0.5">
      <button
        type="button"
        class="flex h-control-sm items-center rounded px-2 text-[11px] font-medium transition-colors {treeMode
          ? 'text-text-muted hover:text-text-secondary'
          : 'bg-bg-active text-text-primary'}"
        aria-pressed={!treeMode}
        data-testid="space-list-toggle"
        onclick={() => (treeMode = false)}
      >
        {t('docs.viewList')}
      </button>
      <button
        type="button"
        class="flex h-control-sm items-center rounded px-2 text-[11px] font-medium transition-colors {treeMode
          ? 'bg-bg-active text-text-primary'
          : 'text-text-muted hover:text-text-secondary'}"
        aria-pressed={treeMode}
        data-testid="space-tree-toggle"
        onclick={showTree}
      >
        {t('docs.viewTree')}
      </button>
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
      <ArrowLeft size={15} strokeWidth={1.8} />
    </button>
  </header>

  <div class="min-h-0 flex-1 overflow-y-auto">
    {#if docs.length === 0}
      <p class="px-4 py-12 text-center text-[12px] text-text-muted">{t('docs.recentEmpty')}</p>
    {:else if treeMode}
      <!-- Capped width: a hierarchy read left-to-right gains nothing from a
           1300px column, and unbounded rows lose the parent–child alignment. -->
      <div class="max-w-[720px] px-3 py-2">
        {#each tree?.roots ?? [] as node (node.page.key)}
          {@render docNode(node)}
        {/each}
      </div>
    {:else}
      <!-- The space is the screen, so every row would repeat it — dropped. -->
      {#each docs as page (page.key)}
        <DocRow {page} showSpace={false} />
      {/each}
    {/if}
  </div>
</section>
