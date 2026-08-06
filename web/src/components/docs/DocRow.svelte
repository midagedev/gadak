<script lang="ts">
  /*
   * One document row — the same shape in every document list (the three tabs and
   * a space's own list), so they read as one component rather than three.
   *
   * The row is one sentence (UX_PRINCIPLES §6): title, then who-did-what-when-
   * where. The space is a suffix, never a group header, so recency order
   * survives. A clause with nothing to say is dropped rather than shown empty.
   *
   * Height and divider are the issue row's, not a second density: 42px, one
   * hairline below. Two lines fit because the meta line is 11px.
   *
   * The lists that answer "what changed" also carry one line of the body, which
   * is the question a title alone often cannot settle. It is opt-in per list:
   * a screen for getting somewhere (the space tree, your own return path) is
   * not a place to browse content.
   */
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import type { PageLite } from '../../lib/types'
  import { pages } from '../../stores/pages.svelte'

  let {
    page,
    /** Clauses the surrounding screen already states are dropped rather than
     *  repeated on every row — the space on a space's own list, the author
     *  under an author heading. */
    showSpace = true,
    showAuthor = true,
    /** One line of the body, for lists read to find something worth opening.
     *  Off by default so a navigation surface stays a navigation surface. */
    showExcerpt = false,
  }: {
    page: PageLite
    showSpace?: boolean
    showAuthor?: boolean
    showExcerpt?: boolean
  } = $props()

  const selected = $derived(pages.selectedKey === page.key)
  // Edited since this browser last opened it. Never set for a page never opened.
  const unread = $derived(pages.unread.has(page.key))
  const author = $derived(showAuthor ? (page.author ?? '') : '')
  const when = $derived(relativeTime(page.updated_at))
  const space = $derived(pages.spaceLabel(page.space_key))
  // A page with an empty body gets no line at all — an empty row is worse than
  // a shorter one, and older servers send no excerpt field.
  const excerpt = $derived(showExcerpt ? (page.excerpt ?? '').trim() : '')
</script>

<button
  type="button"
  class="group flex w-full cursor-pointer select-none flex-col justify-center gap-0.5 border-b border-border-subtle/70 px-4 text-left transition-colors {excerpt
    ? 'h-row-excerpt'
    : 'h-row'} {selected ? 'bg-bg-active' : 'hover:bg-bg-hover'}"
  data-testid="doc-row"
  data-doc-key={page.key}
  data-unread={unread ? 'true' : undefined}
  title={page.title}
  onclick={() => pages.select(page.key)}
>
  <!-- Same tier as an issue row's summary — this list is a peer collection,
       not a secondary one (vision verdict 2026-08-06). -->
  <span class="w-full truncate text-body font-medium text-text-primary">
    {page.title}
  </span>
  <!-- Unread is the dot alone; painting the whole meta line accent put the
       meta above the title in the row's hierarchy (vision verdict). -->
  <span class="flex w-full min-w-0 items-center gap-1 text-micro text-text-muted">
    {#if unread}
      <!-- The mark the local mirror makes cheap: edited since your last visit. -->
      <span class="h-1.5 w-1.5 flex-none rounded-full bg-accent" aria-hidden="true"></span>
      <span class="sr-only">{t('docs.unread')}</span>
    {/if}
    {#if author}
      <span class="max-w-[40%] flex-none truncate">{author}</span>
      <span aria-hidden="true">·</span>
    {/if}
    <span class="flex-none" title={absTime(page.updated_at)}>{when}</span>
    {#if showSpace}
      <span aria-hidden="true">·</span>
      <span class="min-w-0 truncate" title={page.space_key}>{t('docs.metaIn', { space })}</span>
    {/if}
  </span>
  {#if excerpt}
    <!-- Exactly one line, clipped: the row answers "is this the one", and a
         second line would trade the list's density for a worse summary. -->
    <span class="w-full truncate text-micro leading-[15px] text-text-muted" data-testid="doc-excerpt">
      {excerpt}
    </span>
  {/if}
</button>
