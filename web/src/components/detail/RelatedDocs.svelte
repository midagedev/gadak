<script lang="ts">
  /*
   * Documents that share text with this issue ([detail]).
   *
   * These links are derived from prose, not drawn by hand: nobody in Jira has to
   * have remembered to attach the page. So the two directions are not two
   * sections — a reader asking "what is written about this" wants one list, and
   * which side did the naming is a detail of that list, not a heading of its
   * own.
   *
   * A page that names the issue *and* is named by it is a forward reference; the
   * suffix is only for the pages that would otherwise look like the issue's own
   * pointers when they are the page author's. Backlinks get a clause, not a
   * fold: the lists are short, and a collapsed section is a thing to click
   * before it is a thing to read.
   *
   * That clause is drawn only when the list actually holds both directions.
   * Measured on the demo mirror (2026-08-07), every issue's list is backlink-
   * only — pages name issue keys constantly, issue descriptions rarely name a
   * page — so the literal rule put the same six words on all eight rows of
   * NMB-104. A distinction repeated on every row distinguishes nothing; it
   * reads as a template that forgot to vary. The section header is "Documents",
   * which is true of a page in either direction, so a uniform list says the
   * direction by not raising it.
   *
   * When the list IS mixed, both directions get a label — an unmarked row next
   * to a marked one stops meaning "no distinction" and starts meaning "the
   * other kind", which only the person who wrote the rule can decode (vision
   * verdict 2026-08-07; Linked issues labels inward and outward alike). The
   * label sits right-aligned, apart from the meta clause, because it
   * classifies the relationship rather than describing the page.
   *
   * Row grammar is LinkedIssues': same padding, same hover, same two tiers. The
   * content is a document's, though — a page has no key worth showing, so the
   * title leads and the meta line says where and when.
   */
  import Icon from '../ui/Icon.svelte'
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import type { PageLite } from '../../lib/types'
  import { pages } from '../../stores/pages.svelte'

  let {
    refPages = [],
    backlinkPages = [],
  }: { refPages?: PageLite[]; backlinkPages?: PageLite[] } = $props()

  type Row = { page: PageLite; backlinkOnly: boolean }

  const rows = $derived.by(() => {
    const seen = new Set<string>()
    const out: Row[] = []
    // Forward first so a page in both directions is never marked backlink-only.
    for (const page of refPages) {
      if (seen.has(page.key)) continue
      seen.add(page.key)
      out.push({ page, backlinkOnly: false })
    }
    for (const page of backlinkPages) {
      if (seen.has(page.key)) continue
      seen.add(page.key)
      out.push({ page, backlinkOnly: true })
    }
    // Newest first: of several pages about one issue, the current one is the
    // one that was edited last.
    out.sort((a, b) => (b.page.updated_at ?? '').localeCompare(a.page.updated_at ?? ''))
    return out
  })

  /** Both directions present — only then does saying which one a row is add
   *  anything to the row beside it. */
  const mixed = $derived(rows.some((r) => r.backlinkOnly) && rows.some((r) => !r.backlinkOnly))
</script>

<ul class="flex flex-col gap-1" data-testid="related-docs">
  {#each rows as row (row.page.key)}
    <li>
      <button
        type="button"
        data-testid="related-doc-row"
        data-doc-key={row.page.key}
        title={row.page.title}
        onclick={() => pages.select(row.page.key)}
        class="group flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-bg-hover"
      >
        <!-- The gutter glyph is the row's claim to be clickable: Linked issues
             fills this column with its keys, and a row whose gutter is empty
             next to one that speaks reads as a static list (vision verdict
             2026-08-07). -->
        <Icon name="file" size={14} class="mt-0.5 flex-none text-text-muted" />
        <span class="flex min-w-0 flex-1 flex-col gap-0.5">
          <span class="w-full truncate text-[12px] text-text-secondary group-hover:text-text-primary">
            {row.page.title}
          </span>
          <span class="flex w-full min-w-0 items-center gap-1 text-micro text-text-muted">
            <span class="min-w-0 truncate" title={row.page.space_key}>
              {pages.spaceLabel(row.page.space_key)}
            </span>
            {#if row.page.updated_at}
              <span aria-hidden="true">·</span>
              <span class="flex-none" title={absTime(row.page.updated_at)}>
                {relativeTime(row.page.updated_at)}
              </span>
            {/if}
            {#if mixed}
              <span
                class="ml-auto flex-none pl-2 text-micro text-text-secondary"
                data-testid={row.backlinkOnly ? 'related-doc-backlink' : 'related-doc-forward'}
              >
                {row.backlinkOnly ? t('detail.docMentions') : t('detail.docMentioned')}
              </span>
            {/if}
          </span>
        </span>
      </button>
    </li>
  {/each}
</ul>
