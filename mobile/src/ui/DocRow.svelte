<script lang="ts">
  import type { PageLite } from '../lib/types'
  import { relTime, spaceLabel } from '../lib/domain'
  import { t } from '../lib/i18n'
  import { app, openPage } from '../lib/store.svelte'

  /*
   * One document row (DESIGN.md §3.4): the issue row's height and hairline,
   * no status mark. The row is one sentence (desktop DocRow): title, then
   * author · relative time · space. The space clause drops when the scope
   * is that space. Excerpt rides the same second line on the Updated plate
   * only, so density stays 56px.
   */
  let {
    page,
    showSpace = true,
    showExcerpt = false,
    snippet = '',
  }: {
    page: PageLite
    showSpace?: boolean
    showExcerpt?: boolean
    snippet?: string
  } = $props()

  const clauses = $derived.by(() => {
    const out: string[] = []
    const snip = snippet.trim()
    if (!snip) {
      const author = (page.author ?? '').trim()
      if (author) out.push(author)
      const when = relTime(page.updated_at, app.now)
      if (when) out.push(when)
    }
    if (showSpace) {
      const space = spaceLabel(page)
      if (space) out.push(t('docs.metaIn', { space }))
    }
    if (snip) out.push(snip)
    else if (showExcerpt) {
      const ex = (page.excerpt ?? '').trim()
      if (ex) out.push(ex)
    }
    return out
  })
</script>

<button class="row" data-testid="doc-row" data-doc-key={page.key} onclick={() => openPage(page.key)}>
  <span class="text">
    <span class="line1">
      <span class="title">{page.title}</span>
    </span>
    {#if clauses.length > 0}
      <span class="line2">{clauses.join(' · ')}</span>
    {/if}
  </span>
</button>

<style>
  .row {
    position: relative;
    display: flex;
    width: 100%;
    min-height: var(--spacing-row);
    align-items: center;
    text-align: left;
    padding: 8px 16px;
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .row:active {
    background: var(--color-bg-hover);
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1 1 auto;
  }
  .line1 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .title {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }
  .line2 {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
</style>
