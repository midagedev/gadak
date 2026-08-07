<script lang="ts">
  /*
   * Issues that share text with this page ([detail]) — the mirror image of
   * RelatedDocs, and deliberately the same shape: one list, newest first, with
   * a clause rather than a section for the direction.
   *
   * The server sends keys, not rows, and only keys the mirror holds. The row is
   * built from the pool, which is why it can carry a status dot without a
   * request. A key the pool somehow lacks still gets a row — a link that says
   * only "NMB-204" is worth more than a link silently dropped — it just has
   * nothing to put in the two columns beside it.
   *
   * Row grammar is the epic's child row: dot, key, summary, in that order and
   * at those widths, so the two places a page of issues appears inside a panel
   * read as one thing.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { CATEGORY_META, categoryOf } from '../../lib/format'

  let {
    refKeys = [],
    backlinkKeys = [],
  }: { refKeys?: string[]; backlinkKeys?: string[] } = $props()

  type Row = { key: string; issue: IssueLite | undefined; backlinkOnly: boolean }

  const rows = $derived.by(() => {
    const seen = new Set<string>()
    const out: Row[] = []
    // Forward first, so a key in both directions is never marked backlink-only.
    for (const key of refKeys) {
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ key, issue: issues.get(key), backlinkOnly: false })
    }
    for (const key of backlinkKeys) {
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ key, issue: issues.get(key), backlinkOnly: true })
    }
    // Newest first, same rule as the documents section. A key the pool lacks has
    // no timestamp to sort on and falls to the end rather than to the top.
    out.sort((a, b) => (b.issue?.updated_at ?? '').localeCompare(a.issue?.updated_at ?? ''))
    return out
  })

  /** Same rule as RelatedDocs, and for the same reason: the direction clause is
   *  a distinction, so it is drawn only where there is something to distinguish
   *  it from. The demo mirror's page lists are uniformly forward, the mirror
   *  image of its issue lists. */
  const mixed = $derived(rows.some((r) => r.backlinkOnly) && rows.some((r) => !r.backlinkOnly))
</script>

<ul class="flex flex-col gap-1" data-testid="related-issues">
  {#each rows as row (row.key)}
    <li>
      <button
        type="button"
        data-testid="related-issue-row"
        data-issue-key={row.key}
        title={row.issue?.summary ?? row.key}
        onclick={() => selection.select(row.key)}
        class="group flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-bg-hover"
      >
        <!-- The dot's box is kept even when the pool cannot fill it, so the key
             column starts at the same x on every row. -->
        <span class="h-1.5 w-1.5 flex-none rounded-full" aria-hidden={!row.issue}>
          {#if row.issue}
            <span
              class="block h-full w-full rounded-full"
              style:background={CATEGORY_META[categoryOf(row.issue)].color}
              title={row.issue.status}
            ></span>
          {/if}
        </span>
        <span class="w-[76px] flex-none truncate font-mono text-micro font-medium text-accent-text">
          {row.key}
        </span>
        <span
          class="min-w-0 flex-1 truncate text-[12px] text-text-secondary group-hover:text-text-primary"
        >
          {row.issue?.summary ?? ''}
        </span>
        <!-- Mixed lists label both directions: an unmarked row beside a marked
             one reads as "the other kind", not "no distinction" (vision verdict
             2026-08-07 — Linked issues labels inward and outward alike). -->
        {#if mixed}
          <span
            class="flex-none text-micro text-text-secondary"
            data-testid={row.backlinkOnly ? 'related-issue-backlink' : 'related-issue-forward'}
          >
            {row.backlinkOnly ? t('doc.issueMentions') : t('doc.issueMentioned')}
          </span>
        {/if}
      </button>
    </li>
  {/each}
</ul>
