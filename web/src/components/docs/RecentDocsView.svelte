<script lang="ts">
  /*
   * "Recently updated" documents — a main-column view, not an overlay.
   *
   * The sidebar tree answers "where does this page live"; this answers "what
   * moved in the wiki lately", which no single space can. It is the main column
   * rather than a palette overlay so the reading surface stays open beside it:
   * clicking a row opens the document in the right panel and the list keeps its
   * place, which is how someone skims several edits in one pass.
   *
   * Everything here is client-side over the page index the sidebar already
   * loaded — no request of its own.
   */
  import { t, relativeTime, absTime, formatNumber } from '../../lib/i18n'
  import { pages } from '../../stores/pages.svelte'

  const docs = $derived(pages.recentlyUpdated)
</script>

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="recent-docs-view">
  <header
    class="flex flex-none items-center gap-2 border-b border-border-subtle px-4 py-3"
  >
    <div class="min-w-0 flex-1">
      <h2 class="text-[13px] font-semibold text-text-primary">{t('docs.recentTitle')}</h2>
      <p class="text-[11px] text-text-muted">
        {t('docs.recentCount', { n: formatNumber(docs.length) })}
      </p>
    </div>
    <button
      type="button"
      class="flex-none rounded-md border border-border-strong px-2.5 py-1 text-[11px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => pages.closeRecent()}
      data-testid="recent-docs-close"
    >
      {t('docs.backToIssues')}
    </button>
  </header>

  <div class="min-h-0 flex-1 overflow-y-auto">
    {#if docs.length === 0}
      <p class="px-4 py-12 text-center text-[12px] text-text-muted">{t('docs.recentEmpty')}</p>
    {:else}
      <ul>
        {#each docs as page (page.key)}
          {@const selected = pages.selectedKey === page.key}
          <li>
            <button
              type="button"
              class="flex w-full items-center gap-3 border-b border-border-subtle/60 px-4 py-2 text-left transition-colors {selected
                ? 'bg-bg-active'
                : 'hover:bg-bg-hover'}"
              data-testid="recent-doc-row"
              onclick={() => pages.select(page.key)}
            >
              <span
                class="min-w-0 flex-1 truncate text-[13px] {selected
                  ? 'text-text-primary'
                  : 'text-text-secondary'}"
                title={page.title}
              >
                {page.title}
              </span>
              <span
                class="max-w-[28%] flex-none truncate text-[11px] text-text-muted"
                title={page.space_key}
              >
                {pages.spaceLabel(page.space_key)}
              </span>
              <span
                class="w-20 flex-none text-right text-[11px] tabular-nums text-text-muted"
                title={absTime(page.updated_at)}
              >
                {relativeTime(page.updated_at)}
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>
