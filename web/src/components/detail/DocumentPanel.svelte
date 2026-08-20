<script lang="ts">
  /*
   * Document panel ([detail]) — one mirrored wiki page.
   *
   * Same shell as DetailPanel: no props, subscribes to the pages store, pinned
   * header over a scrolling body, Esc closes. Only the parts a page actually has
   * are here — no fields/history/links/QA/PR/deploy. The one write surface is
   * the page comment composer (GDK-381), gated like issue comments; the body
   * itself stays read-only in this panel (no page editor yet).
   *
   * Latency hide follows DetailPanel too: the header renders from the in-memory
   * index row while the body is still in flight.
   */
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import { commentOnPage } from '../../lib/api'
  import { pages } from '../../stores/pages.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { isHostedDemo } from '../../lib/config'
  import { createResource } from '../../lib/resource.svelte'
  import { onEscape } from '../../lib/dom-actions'
  import type { PageDetail } from '../../lib/types'
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
    // watch detailNonce so a desktop browse resync re-reads after cache drop
    { watch: () => pages.detailNonce },
  )
  const detail = $derived(resource.data)
  const errorKind = $derived(resource.errorKind)

  // Keep `load` name for the markup retry handler (variable rename only).
  function load(_k: string): void {
    resource.reload()
  }

  // Write-through overlay: the POST comment/ body is the origin's page, so
  // the thread can render it without waiting on a GET (and in e2e, without
  // a second stub). Cleared when the open page changes.
  let postedDetail = $state<PageDetail | null>(null)
  $effect(() => {
    void key
    postedDetail = null
  })

  // Never show the previous page's body mid-switch.
  const detailForKey = $derived.by(() => {
    if (postedDetail && key === postedDetail.key) return postedDetail
    return detail && key === detail.key ? detail : null
  })
  const head = $derived(detailForKey ?? lite)

  // Breadcrumb trail, from the client-side index — no extra request, and it
  // fills in the moment the index loads. Empty for a root page.
  const trail = $derived(key ? pages.ancestors(key) : [])

  // Same rule as the sidebar: the space's name when the mirror has one, its key
  // (monospace, and the tooltip either way) until then.
  const spaceLabel = $derived(head ? pages.spaceLabel(head.space_key) : '')

  /*
   * The homepage is the root of every document in the space; the space label
   * already fills that slot in the breadcrumb. Drop the first ancestor only
   * when its key equals the space's homepage_id. Mirrors that have not learned
   * homepage_id yet (before the next sync, or demo.db) omit nothing.
   */
  const displayTrail = $derived.by(() => {
    if (!head) return trail
    const hp = pages.spaceHomepage(head.space_key)
    if (hp && trail[0]?.key === hp) return trail.slice(1)
    return trail
  })

  // Both directions of the text-derived issue references, counted once — the
  // section renders them as one list (see RelatedIssues).
  // Page comment composer (GDK-381) — plain text through the origin.
  let draft = $state('')
  let posting = $state(false)
  let postError = $state('')
  async function postPageComment(): Promise<void> {
    const text = draft.trim()
    if (!key || !text || posting) return
    if (!(await write.ensureWritable())) return
    posting = true
    postError = ''
    try {
      // Same hosted-demo guard as issue comments: no POST, keep the edit local.
      if (isHostedDemo()) {
        const first = write.demoEdits.size === 0
        write.demoEdits.add(key)
        if (first) write.toast(t('app.demoWriteNotice'), 'info')
        draft = ''
        return
      }
      const res = await commentOnPage(key, text)
      draft = ''
      if (res.page) {
        postedDetail = res.page
        pages.invalidateDetail(key)
      }
    } catch (e) {
      postError = e instanceof Error ? e.message : String(e)
    } finally {
      posting = false
    }
  }

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
            {#each displayTrail as a (a.key)}
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
        <h2 class="type-subject mb-2 text-heading text-text-primary" data-testid="doc-title">
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
      {#if errorKind && !detailForKey}
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

            <form
              class="mt-3 flex flex-col gap-2"
              onsubmit={(e) => {
                e.preventDefault()
                void postPageComment()
              }}
            >
              <textarea
                bind:value={draft}
                rows="2"
                placeholder={me.identified || isHostedDemo()
                  ? t('doc.commentPlaceholder')
                  : t('doc.commentNeedCredentials')}
                data-testid="doc-comment-composer"
                class="w-full resize-y rounded-md border border-border-subtle bg-bg-elevated px-2.5 py-1.5 text-body text-text-primary placeholder:text-text-muted focus:border-border-strong focus:outline-none"
                onkeydown={(e) => {
                  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                    e.preventDefault()
                    void postPageComment()
                  }
                }}
              ></textarea>
              {#if postError}
                <p class="text-[12px] text-danger-text" data-testid="doc-comment-error">{postError}</p>
              {/if}
              <div class="flex justify-end">
                <button
                  type="submit"
                  disabled={posting || !draft.trim()}
                  data-testid="doc-comment-submit"
                  class="inline-flex h-control items-center rounded-md bg-accent px-3 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-40"
                >
                  {posting ? t('write.commentPosting') : t('write.commentButton')}
                </button>
              </div>
            </form>
          </Section>
        </div>
      {/if}
    </div>
  </div>
{/if}
