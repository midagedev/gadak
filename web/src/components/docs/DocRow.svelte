<script lang="ts">
  /*
   * One document row — the same shape in every document list (the three tabs and
   * a space's own list), so they read as one component rather than three.
   *
   * The row is one sentence (UX_PRINCIPLES §6): title, then who-did-what-when-
   * where. The space is a suffix, never a group header, so recency order
   * survives. A clause with nothing to say is dropped rather than shown empty.
   * Labels are the last clause of that sentence, after the space, and only on
   * the lists where they are an axis someone would follow.
   *
   * Height and divider are the issue row's, not a second density: 42px, one
   * hairline below. Two lines fit because the meta line is 11px.
   *
   * The lists that answer "what changed" also carry one line of the body, which
   * is the question a title alone often cannot settle. It is opt-in per list:
   * a screen for getting somewhere (the space tree, your own return path) is
   * not a place to browse content.
   *
   * The root is a div with role=button, like the issue row, because the label
   * chips inside it are buttons of their own and a button cannot nest one. It
   * keeps tabindex=0: it was a real button until the chips arrived, and losing
   * keyboard reach was never the point of that change.
   */
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import { highlightSegments, mergeAdjacentHits } from '../../lib/format'
  import type { PageLite } from '../../lib/types'
  import { pages } from '../../stores/pages.svelte'
  import Marks from '../ui/Marks.svelte'

  /** Labels beyond this are a count — the row is a sentence, not a tag cloud. */
  const MAX_LABELS = 2

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
    /** Label chips, on the lists where a label is a way to keep looking. Off on
     *  a return path (Viewed), where the row is a bookmark and not a query. */
    showLabels = false,
    /** What the screen is currently narrowed by, for match highlighting. Empty
     *  while nothing is typed, which is also when no mark is drawn. */
    q = '',
  }: {
    page: PageLite
    showSpace?: boolean
    showAuthor?: boolean
    showExcerpt?: boolean
    showLabels?: boolean
    q?: string
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
  const labels = $derived(showLabels ? (page.labels ?? []) : [])
  const shownLabels = $derived(labels.slice(0, MAX_LABELS))
  const extraLabels = $derived(Math.max(0, labels.length - MAX_LABELS))

  /*
   * Why this row is here, marked in it. A filter that only removes rows leaves
   * the ones it kept unexplained, which reads as an arbitrary list — the same
   * gap the issue list closed with these marks. Chosung queries have no literal
   * slice to mark and simply come back whole; so do the rows matched by their
   * space or author, which is why those two clauses are marked as well.
   *
   * Every clause the row draws is marked, not only the ones the filter reads.
   * The excerpt and the labels are not in the haystack (doc-search.ts), but a
   * query the eye can find in a line the row is showing has to be marked there
   * too — an unmarked occurrence sitting under a marked one reads as the
   * highlighting having missed it, not as a statement about the haystack.
   */
  const segsOf = (text: string) => mergeAdjacentHits(highlightSegments(text, q))
  const titleSegs = $derived(segsOf(page.title))
  const authorSegs = $derived(segsOf(author))
  const excerptSegs = $derived(segsOf(excerpt))
  /** Only the space's own name is marked inside the "in {space}" clause: the
   *  filter matches the name, so marking the connecting word would claim a hit
   *  the list did not make. */
  const spaceSegs = $derived.by(() => {
    const text = t('docs.metaIn', { space })
    const at = q ? text.indexOf(space) : -1
    if (at < 0) return [{ text, hit: false }]
    const out = at > 0 ? [{ text: text.slice(0, at), hit: false }] : []
    out.push(...segsOf(space))
    const tail = text.slice(at + space.length)
    if (tail) out.push({ text: tail, hit: false })
    return out
  })

  function stop(fn: () => void) {
    return (e: MouseEvent) => {
      e.stopPropagation()
      fn()
    }
  }
</script>

<div
  class="group flex w-full cursor-pointer select-none flex-col justify-center gap-0.5 border-b border-border-subtle/70 px-4 text-left transition-colors {excerpt
    ? 'h-row-excerpt'
    : 'h-row'} {selected ? 'bg-bg-active' : 'hover:bg-bg-hover'}"
  role="button"
  tabindex="0"
  data-testid="doc-row"
  data-doc-key={page.key}
  data-unread={unread ? 'true' : undefined}
  title={page.title}
  onclick={() => pages.select(page.key)}
  onkeydown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      pages.select(page.key)
    }
  }}
>
  <!-- Same tier as an issue row's summary — this list is a peer collection,
       not a secondary one (vision verdict 2026-08-06). -->
  <span class="w-full truncate text-body font-normal text-text-primary"
    ><Marks segs={titleSegs} /></span
  >
  <!-- Unread is the dot alone; painting the whole meta line accent put the
       meta above the title in the row's hierarchy (vision verdict). -->
  <span class="flex w-full min-w-0 items-center gap-1 text-micro text-text-muted">
    {#if unread}
      <!-- The mark the local mirror makes cheap: edited since your last visit. -->
      <span class="h-1.5 w-1.5 flex-none rounded-full bg-accent" aria-hidden="true"></span>
      <span class="sr-only">{t('docs.unread')}</span>
    {/if}
    {#if author}
      <span class="max-w-[40%] flex-none truncate"><Marks segs={authorSegs} /></span>
      <span aria-hidden="true">·</span>
    {/if}
    <span class="flex-none" title={absTime(page.updated_at)}>{when}</span>
    {#if showSpace}
      <span aria-hidden="true">·</span>
      <span class="min-w-0 truncate" title={page.space_key}><Marks segs={spaceSegs} /></span>
    {/if}
    {#if shownLabels.length}
      <!--
        The last clause: what this page is about, and a way to ask for more of
        the same.

        It carries a background of its own from the start. Quiet-until-hovered
        left it indistinguishable from the muted clause on its left, so nothing
        on the row said it could be clicked and the whole axis went unused
        (vision verdict 2026-08-07). The fill is roughly half the lift the
        header's active chip has over its own surface — enough to read as a
        control, not enough to compete with the title above it — and bg-active
        rather than bg-elevated because a chip must not turn darker than the row
        it sits on once that row is hovered.

        `ml-1.5` on top of the line's own gap-1: the space clause and the labels
        are different kinds of thing, so the seam between them has to be wider
        than the seam between two labels.
      -->
      <span class="ml-1.5 flex flex-none items-center gap-1" data-testid="doc-labels">
        {#each shownLabels as label (label)}
          <button
            type="button"
            class="max-w-[110px] truncate rounded bg-bg-active/25 px-1.5 py-0.5 text-micro text-text-secondary transition-colors hover:bg-bg-active/60 hover:text-text-primary"
            data-testid="doc-label"
            data-label={label}
            title={t('docs.labelFilterTo', { label })}
            onclick={stop(() => pages.setDocsLabel(label))}
          ><Marks segs={segsOf(label)} /></button>
        {/each}
        {#if extraLabels}
          <span class="tabular-nums" title={labels.join(', ')}>+{extraLabels}</span>
        {/if}
      </span>
    {/if}
  </span>
  {#if excerpt}
    <!-- Exactly one line, clipped: the row answers "is this the one", and a
         second line would trade the list's density for a worse summary. -->
    <span class="w-full truncate text-micro leading-[15px] text-text-muted" data-testid="doc-excerpt"
      ><Marks segs={excerptSegs} /></span
    >
  {/if}
</div>
