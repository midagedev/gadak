<script lang="ts">
  /*
   * Document panel ([detail]) — one mirrored wiki page.
   *
   * Same shell as DetailPanel: no props, subscribes to the pages store, pinned
   * header over a scrolling body, Esc closes. Only the parts a page actually has
   * are here — no fields/history/links/QA/PR/deploy, and no composer: the mirror
   * is read-only for pages.
   *
   * Latency hide follows DetailPanel too: the header renders from the in-memory
   * index row while the body is still in flight.
   */
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import { pages } from '../../stores/pages.svelte'
  import { createResource } from '../../lib/resource.svelte'
  import { onEscape } from '../../lib/dom-actions'
  import AdfContent from './AdfContent.svelte'
  import RelatedIssues from './RelatedIssues.svelte'
  import Section from './Section.svelte'
  import Icon from '../ui/Icon.svelte'

  const key = $derived(pages.selectedKey)
  // Index row for the instant header (a search hit may not be in the index yet).
  const lite = $derived.by(() => {
    if (!key) return undefined
    return pages.lite(key) ?? pages.searchHits.find((p) => p.key === key)
  })

  const resource = createResource(
    () => pages.selectedKey,
    (k) => pages.detail(k),
  )
  const detail = $derived(resource.data)
  const errorKind = $derived(resource.errorKind)

  // Keep `load` name for the markup retry handler (variable rename only).
  function load(_k: string): void {
    resource.reload()
  }

  // Never show the previous page's body mid-switch.
  const detailForKey = $derived(detail && key === detail.key ? detail : null)
  const head = $derived(detailForKey ?? lite)

  // Breadcrumb trail, from the client-side index — no extra request, and it
  // fills in the moment the index loads. Empty for a root page.
  const trail = $derived(key ? pages.ancestors(key) : [])

  // Both directions of the text-derived issue references, counted once — the
  // section renders them as one list (see RelatedIssues).
  const relatedIssueCount = $derived.by(() => {
    const keys = new Set<string>()
    for (const k of detailForKey?.ref_issue_keys ?? []) keys.add(k)
    for (const k of detailForKey?.backlink_issue_keys ?? []) keys.add(k)
    return keys.size
  })
</script>

{#if key}
  <!-- Esc to close (mirrors DetailPanel). -->
  <div
    class="flex h-full flex-col text-text-primary"
    data-testid="doc-panel"
    use:onEscape={() => pages.clear()}
  >
    <!-- Header — outside the scroll (see DetailPanel). -->
    <div class="relative z-10 flex-none bg-bg-panel">
      <header class="border-b border-border-strong/70 px-5 pt-4 pb-4">
        <div class="mb-2 flex items-start justify-between gap-2">
          <!-- Type badge only. The space is the breadcrumb's first segment,
               which renders under exactly the same condition. -->
          <span
            class="flex-none rounded bg-bg-active px-1.5 py-0.5 text-micro font-medium uppercase tracking-wide text-text-muted"
          >
            {t('doc.badge')}
          </span>
          <button
            type="button"
            onclick={() => pages.clear()}
            class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
            aria-label={t('common.close')}
            title={t('common.closeEsc')}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
          </button>
        </div>

        <!-- Where the page sits in its space. One line: ancestors give up width
             first, the open page keeps at most half. -->
        {#if head}
          <!-- Same rule as the sidebar: the space's name when the mirror has
               one, its key (monospace, and the tooltip either way) until then. -->
          {@const spaceLabel = pages.spaceLabel(head.space_key)}
          <nav
            class="mb-2 flex items-center gap-1 overflow-hidden whitespace-nowrap text-micro text-text-muted"
            aria-label={t('doc.breadcrumb')}
            data-testid="doc-breadcrumb"
          >
            <span
              class="flex-none {spaceLabel === head.space_key ? 'font-mono' : ''}"
              title={head.space_key}
            >
              {spaceLabel}
            </span>
            {#each trail as a (a.key)}
              <Icon name="chevron-right" size={12} class="text-text-muted" />
              <button
                type="button"
                class="min-w-0 shrink truncate text-text-secondary transition-colors hover:text-text-primary hover:underline"
                title={a.title}
                data-testid="doc-breadcrumb-ancestor"
                onclick={() => pages.select(a.key)}
              >
                {a.title}
              </button>
            {/each}
            <Icon name="chevron-right" size={12} class="text-text-muted" />
            <span class="max-w-[60%] flex-none truncate text-text-secondary">{head.title}</span>
          </nav>
        {/if}

        <!-- Same tier as an issue's title: both are "the subject of this panel". -->
        <h2 class="mb-2 text-heading font-semibold text-text-primary" data-testid="doc-title">
          {head?.title ?? ''}
        </h2>

        <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-micro text-text-muted">
          {#if head?.author}
            <span class="min-w-0 truncate text-text-secondary">{head.author}</span>
            <span aria-hidden="true">·</span>
          {/if}
          {#if head?.updated_at}
            <span title={absTime(head.updated_at)}>{relativeTime(head.updated_at, 'long')}</span>
            <span aria-hidden="true">·</span>
          {/if}
          {#if head}
            <span class="font-mono tabular-nums">{t('doc.version', { n: head.version })}</span>
          {/if}
          {#if head?.url}
            <a
              href={head.url}
              target="_blank"
              rel="noopener noreferrer"
              class="ml-auto flex-none text-accent-text hover:underline"
              data-testid="doc-source-link"
              title={t('doc.openSource')}
            >
              {t('doc.openSource')} ↗
            </a>
          {/if}
        </div>
      </header>
    </div>

    <!-- Body — the panel's own scroller. -->
    <div class="min-h-0 flex-1 overflow-y-auto" data-testid="doc-scroll">
      {#if errorKind}
        <div class="flex flex-col items-center gap-3 px-5 py-16 text-center">
          <p class="text-body text-text-secondary">
            {errorKind === 'notfound' ? t('doc.notFound') : t('doc.loadFailed')}
          </p>
          {#if errorKind === 'network'}
            <button
              type="button"
              onclick={() => key && void load(key)}
              class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
            >
              {t('common.retry')}
            </button>
          {/if}
        </div>
      {:else if !detailForKey}
        <!-- Skeleton (body loading) -->
        <div class="flex flex-col gap-2 px-5 py-4" aria-hidden="true">
          <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
          <div class="mt-4 h-3 w-1/2 animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
        </div>
      {:else}
        <div class="anim-enter divide-y divide-border-subtle">
          <Section title={t('doc.content')}>
            <div class="text-body text-text-secondary">
              <AdfContent node={detailForKey.body_adf} emptyLabel={t('doc.noContent')} />
            </div>
          </Section>

          <!-- The work this page is about, above the conversation about the
               page. Both come from the body's own text, so they belong with it
               rather than at the end of the panel. -->
          {#if relatedIssueCount > 0}
            <Section title={t('doc.issues')} count={relatedIssueCount}>
              <RelatedIssues
                refKeys={detailForKey.ref_issue_keys}
                backlinkKeys={detailForKey.backlink_issue_keys}
              />
            </Section>
          {/if}

          <Section title={t('doc.comments')} count={detailForKey.comments.length}>
            {#if detailForKey.comments.length === 0}
              <p class="text-[12px] italic text-text-muted">{t('doc.noComments')}</p>
            {:else}
              <ol class="flex flex-col gap-4">
                {#each detailForKey.comments as c, i (`${c.created_at}-${i}`)}
                  <li data-testid="doc-comment">
                    <div class="mb-1 flex items-center gap-2">
                      <span class="text-[12px] font-medium text-text-primary">
                        {c.author ?? t('detail.unknownAuthor')}
                      </span>
                      <span class="text-micro text-text-muted" title={absTime(c.created_at)}>
                        {relativeTime(c.created_at, 'long')}
                      </span>
                    </div>
                    <div class="text-body leading-relaxed text-text-secondary">
                      <AdfContent
                        node={c.body_adf}
                        fallback={c.body_text}
                        emptyLabel={t('detail.emptyComment')}
                      />
                    </div>
                  </li>
                {/each}
              </ol>
            {/if}
          </Section>
        </div>
      {/if}
    </div>
  </div>
{/if}
