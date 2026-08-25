<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import { app, closeIssue, openIssue } from '../lib/store.svelte'
  import { bodyParagraphs, relTime, spaceLabel } from '../lib/domain'
  import { request, ApiError } from '../lib/api'
  import { t } from '../lib/i18n'
  import type { PageDetail as PageDetailDoc, PageLite } from '../lib/types'

  // Page detail (GDK-887). Same push layer as issue Detail. A page is read
  // for its body, so comments follow the body (unlike issues). No write
  // controls: page writes are a desktop job and this screen must not
  // suggest them.

  let { pageKey }: { pageKey: string } = $props()

  const lite = $derived<PageLite | undefined>(app.pages.find((p) => p.key === pageKey))

  let detail = $state<PageDetailDoc | null>(null)
  let detailError = $state<string | null>(null)

  const title = $derived((detail?.title || lite?.title || '').trim())
  const meta = $derived.by(() => {
    const page = detail ?? lite
    if (!page) return [] as string[]
    const out: string[] = []
    const author = (page.author ?? '').trim()
    if (author) out.push(author)
    const when = relTime(page.updated_at, app.now)
    if (when) out.push(when)
    const space = spaceLabel(page)
    if (space) out.push(t('docs.metaIn', { space }))
    return out
  })
  const paragraphs = $derived(bodyParagraphs(detail?.body_text ?? ''))
  const comments = $derived(detail?.comments ?? [])
  const refs = $derived(detail?.ref_issue_keys ?? [])

  $effect(() => {
    const key = pageKey
    detail = null
    detailError = null
    void (async () => {
      try {
        const res = await request<PageDetailDoc>(`issues/pages/${encodeURIComponent(key)}/`)
        if (key === pageKey) detail = res.body
      } catch (err) {
        if (key !== pageKey) return
        detailError =
          err instanceof ApiError && err.code === 'not_found' ? t('doc.notFound') : t('doc.loadFailed')
      }
    })()
  })
</script>

<div class="push page-detail">
  <Screen>
    {#snippet header()}
      <div class="bar">
        <button class="back" onclick={closeIssue} aria-label="Back">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M15 18l-6-6 6-6" />
          </svg>
          <span>Back</span>
        </button>
        <span class="bar-pad" aria-hidden="true"></span>
      </div>
    {/snippet}

    <article>
      {#if title}
        <h1 class="type-subject">{title}</h1>
      {/if}
      {#if meta.length > 0}
        <p class="meta">{meta.join(' · ')}</p>
      {/if}
      {#if refs.length > 0}
        <div class="refs">
          {#each refs as k (k)}
            <button class="ref-key" onclick={() => openIssue(k)}>{k}</button>
          {/each}
        </div>
      {/if}
    </article>

    <section class="body">
      {#if detailError}
        <p class="error">{detailError}</p>
      {:else if !detail}
        <div class="ghost" aria-hidden="true">
          <span class="g w1"></span><span class="g w2"></span><span class="g w3"></span>
        </div>
      {:else}
        {#each paragraphs as p, i (i)}
          <p class="para">{p}</p>
        {/each}

        {#if comments.length > 0}
          <h3>{t('doc.comments')} <span class="h-n">{comments.length}</span></h3>
          {#each comments as c, i (`${c.created_at}-${i}`)}
            <div class="comment">
              <p class="c-head">
                <span class="c-author">{(c.author ?? '').trim() || t('detail.unknownAuthor')}</span>
                <span class="c-when">{relTime(c.created_at, app.now)}</span>
              </p>
              <p class="c-body">{c.body_text}</p>
            </div>
          {/each}
        {/if}
      {/if}
      <div class="tail" aria-hidden="true"></div>
    </section>
  </Screen>
</div>

<style>
  .push {
    position: absolute;
    inset: 0;
    z-index: 20;
    display: flex;
    flex-direction: column;
    background: var(--color-bg-base);
  }
  .bar {
    display: flex;
    align-items: center;
    min-height: 48px;
    gap: 8px;
  }
  .back {
    display: flex;
    align-items: center;
    gap: 2px;
    min-height: var(--spacing-control);
    padding-right: 12px;
    margin-left: -6px;
    color: var(--color-accent-text);
    flex: 1 1 0;
  }
  .back svg {
    width: 22px;
    height: 22px;
  }
  .bar-pad {
    flex: 1 1 0;
  }

  article {
    padding: 4px 16px 12px;
  }
  h1 {
    margin: 0 0 6px;
    font-size: var(--text-title);
    line-height: var(--text-title--line-height);
    overflow-wrap: anywhere;
  }
  .meta {
    margin: 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .refs {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 8px;
  }
  .ref-key {
    flex: none;
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-accent-text);
    padding: 0 4px;
  }

  .body {
    padding: 0 16px;
  }
  .para {
    margin: 0 0 12px;
    color: var(--color-text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .error {
    margin: 8px 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  h3 {
    margin: 24px 0 6px;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .body > h3:first-of-type {
    margin-top: 8px;
  }
  .h-n {
    font-family: var(--font-mono);
    font-weight: 400;
  }

  .comment {
    padding: 10px 0;
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .c-head {
    margin: 0 0 2px;
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }
  .c-author {
    font-size: var(--text-micro);
    font-weight: 600;
    color: var(--color-text-primary);
  }
  .c-when {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .c-body {
    margin: 0;
    color: var(--color-text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .tail {
    height: 16px;
  }

  .ghost {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-top: 8px;
  }
  .g {
    height: 12px;
    border-radius: 4px;
    background: var(--color-bg-elevated);
  }
  .w1 {
    width: 90%;
  }
  .w2 {
    width: 76%;
  }
  .w3 {
    width: 40%;
  }
</style>
