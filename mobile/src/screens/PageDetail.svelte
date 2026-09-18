<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import { untrack } from 'svelte'
  import AdfBody from '../ui/AdfBody.svelte'
  import DeskRow from '../ui/DeskRow.svelte'
  import { app, closeIssue, openIssue } from '../lib/store.svelte'
  import { relTime, spaceLabel } from '../lib/domain'
  import { request, errorMessage, ApiError } from '../lib/api'
  import { keyboardInset } from '../../../web/src/lib/keyboard'
  import { clearDraft, loadDraft, saveDraft } from '../lib/drafts'
  import { t } from '../lib/i18n'
  import type { PageComment, PageDetail as PageDetailDoc, PageLite } from '../lib/types'

  // Page detail (GDK-887). Same push layer as issue Detail. A page is read
  // for its body, so comments follow the body (unlike issues).
  //
  // One write control, and only one (GDK-1873). Editing a page stays on the
  // desk: the server refuses a plain-text replace with 409 format_loss, and
  // a phone that offers an edit it cannot finish is the data loss this
  // round exists to avoid. A comment is one line to say and the whole
  // composer already exists next door — this screen borrows Detail's slab,
  // its class names and its draft discipline, and adds nothing else.

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
  const hasBody = $derived(!!(detail?.body_adf || (detail?.body_text ?? '').trim()))
  const refs = $derived(detail?.ref_issue_keys ?? [])

  /* ── The comment composer (GDK-1873) ── */

  // What you were typing here last time. Read once at mount: App.svelte
  // remounts this screen per key ({#key}), so the initializer is the
  // per-page reset and no $effect has to assign it (GDK-692).
  const commentDraft = loadDraft('page-comment', untrack(() => pageKey))
  let comment = $state(commentDraft ?? '')
  /** One muted line under the composer, dismissed by the first keystroke. */
  let commentRestored = $state(commentDraft !== null)
  let sending = $state(false)
  let sendError = $state<string | null>(null)
  /** RAM-only overlay (DESIGN.md §5). Never written to the snapshot cache. */
  let pending = $state<PageComment | null>(null)
  /**
   * Writability, the same verdict with the same two roads Detail uses
   * (GDK-952): the store's probe of GET credential/, or this screen's own
   * 409 latch for a credential that disappeared mid-session. The screen
   * never assigns the verdict — a refusal here only latches.
   */
  let refused = $state(false)
  const writesOff = $derived(app.writes === 'off' || refused)
  const sendArmed = $derived(!writesOff && (comment.trim() !== '' || sending))

  // The overlay rides under the mirror's comments until the re-fetch brings
  // the real row back. Pages have no comment ids (types.ts), so there is
  // nothing to de-duplicate against — the overlay is dropped by hand on
  // both roads out of send().
  const comments = $derived.by<PageComment[]>(() => {
    const rows = detail?.comments ?? []
    return pending ? [...rows, pending] : rows
  })

  /*
   * The draft debounce. Plain lets, not $state: nothing renders them, and
   * the effect's teardown must read them after the last render. `draftOwed`
   * is what the timer still owes storage — flushed on teardown (a back-tap
   * inside the debounce window must not lose a word) and before the POST,
   * which may fail.
   */
  let draftTimer: ReturnType<typeof setTimeout> | null = null
  let draftOwed: string | null = null

  function queueDraft(key: string, text: string): void {
    draftOwed = text
    if (draftTimer) clearTimeout(draftTimer)
    draftTimer = setTimeout(() => {
      draftTimer = null
      draftOwed = null
      saveDraft('page-comment', key, text)
    }, 250)
  }

  /** Writes what the debounce still owes, now. Storage is synchronous, so
   *  this is safe from an effect teardown and from a send's first line. */
  function flushDraft(key: string): void {
    const text = draftOwed
    if (draftTimer) clearTimeout(draftTimer)
    draftTimer = null
    draftOwed = null
    if (text !== null) saveDraft('page-comment', key, text)
  }

  function onCommentInput(next: string): void {
    commentRestored = false
    queueDraft(pageKey, next)
  }

  async function send(): Promise<void> {
    const text = comment.trim()
    if (writesOff || text === '' || sending) return
    // The draft goes to storage before the POST, not after: a refused send
    // must leave it there, and the box is emptied four lines below.
    flushDraft(pageKey)
    saveDraft('page-comment', pageKey, text)
    sending = true
    sendError = null
    pending = {
      author: app.me?.name || app.me?.email || null,
      created_at: new Date().toISOString(),
      body_adf: null,
      body_text: text,
    }
    comment = ''
    commentRestored = false
    const key = pageKey
    try {
      await request(`issues/pages/${encodeURIComponent(key)}/comment/`, {
        method: 'POST',
        body: { text },
      })
      clearDraft('page-comment', key) // only a landed comment forgets its draft
      pending = null
      // The write answers with the refreshed page, but this screen reads the
      // page the same way it read it on arrival — one shape, one parser.
      const res = await request<PageDetailDoc>(`issues/pages/${encodeURIComponent(key)}/`)
      if (key === pageKey) detail = res.body
    } catch (err) {
      pending = null
      if (comment.trim() === '') comment = text
      sendError = errorMessage(err)
      if (err instanceof ApiError && err.code === 'credential_required') refused = true
    } finally {
      sending = false
    }
  }

  // Only `detail` and `detailError` are reset here — the rest of this
  // screen's state starts clean because App.svelte remounts it per key
  // ({#key}), and GDK-692 bans $state assigns in effect bodies that a
  // remount already owns (web/src/lib/effect-assigns-state.test.ts).
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
    /*
     * The debounce's owed text, written now. An app switch is not a
     * teardown — iOS freezes the webview with the screen still mounted, so
     * a 250 ms debt would die there. pagehide is the one event this webview
     * is guaranteed before that (and before a reload); visibilitychange
     * catches the background that never unloads. Both are synchronous and
     * localStorage is synchronous, so the write completes inside the
     * handler. Same reasoning, same pair, as Detail.svelte's.
     */
    const flush = () => flushDraft(key)
    const onHidden = () => {
      if (document.visibilityState === 'hidden') flush()
    }
    window.addEventListener('pagehide', flush)
    document.addEventListener('visibilitychange', onHidden)
    return () => {
      window.removeEventListener('pagehide', flush)
      document.removeEventListener('visibilitychange', onHidden)
      flush()
    }
  })
</script>

<div class="push page-detail">
  <Screen>
    {#snippet header()}
      <div class="bar">
        <button class="back" onclick={closeIssue} aria-label={t('app.back')}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M15 18l-6-6 6-6" />
          </svg>
          <span>{t('app.back')}</span>
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

    <!-- GDK-1874: page edit stays on the desk, and now says so rather than
         being an absence. The reason has not changed since GDK-1873 — the
         server refuses a plain-text replace with 409 format_loss, so a phone
         that offered an edit could not finish it — but "there is no edit
         here" was something a person could only discover by looking for one.
         Above the body it describes, and above the comment composer, which
         is the write this screen does have. -->
    <div class="desk">
      <DeskRow label={t('doc.content')} testid="desk-row-page-edit" />
    </div>

    <section class="body">
      {#if detailError}
        <p class="error">{detailError}</p>
      {:else if !detail}
        <div class="ghost" aria-hidden="true">
          <span class="g w1"></span><span class="g w2"></span><span class="g w3"></span>
        </div>
      {:else}
        {#if hasBody}
          <AdfBody
            doc={detail.body_adf}
            fallback={detail.body_text}
            attachments={detail.attachments}
          />
        {:else}
          <p class="none">{t('doc.noContent')}</p>
        {/if}

        <!-- The heading and the count stay on an empty thread (GDK-1873):
             the composer below is the first write control this screen has
             ever had, and an empty body with a bare input under it says
             nothing about what the input does. -->
        <h3>{t('doc.comments')} <span class="h-n">{comments.length}</span></h3>
        {#if comments.length === 0}
          <p class="none">{t('detail.noComments')}</p>
        {:else}
          {#each comments as c, i (`${c.created_at}-${i}`)}
            <div class="comment">
              <p class="c-head">
                <span class="c-author">{(c.author ?? '').trim() || t('detail.unknownAuthor')}</span>
                <span class="c-when">{relTime(c.created_at, app.now)}</span>
              </p>
              <AdfBody
                doc={c.body_adf}
                fallback={c.body_text}
                attachments={detail.attachments}
              />
            </div>
          {/each}
        {/if}
      {/if}
      <div class="tail" aria-hidden="true"></div>
    </section>

    {#snippet footer()}
      <div class="composer-slab" use:keyboardInset>
        {#if writesOff}
          <!-- Detail carries this sentence on its status chip, which sits in
               the slab but outside .composer — so the 0.45 a refused
               composer wears never dims the words that explain it. This
               screen has no chip, so the line takes the chip's place, with
               the chip's padding and .status-err's type. -->
          <p class="slab-err">{t('app.errorNoCredential')}</p>
        {/if}
        <div class="composer safe-bottom" class:off={writesOff}>
          <input
            bind:value={comment}
            disabled={writesOff}
            placeholder={t('doc.commentPlaceholder')}
            enterkeyhint="send"
            oninput={(e) => onCommentInput(e.currentTarget.value)}
            onkeydown={(e) => {
              if (e.key === 'Enter') void send()
            }}
          />
          <button
            class="send"
            class:armed={sendArmed}
            disabled={writesOff || comment.trim() === '' || sending}
            onclick={() => void send()}
          >
            {sending ? t('write.commentPosting') : t('write.commentButton')}
          </button>
          {#if sendError && !writesOff}
            <p class="send-error">{sendError}</p>
          {:else if commentRestored && !writesOff}
            <p class="draft-note">{t('write.draftRestored')}</p>
          {/if}
        </div>
      </div>
    {/snippet}
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

  /* 8 here plus the row's own 8 is the 16px column the article above and
     the body below both sit on. */
  .desk {
    padding: 0 8px;
  }

  .body {
    padding: 0 16px;
  }
  .none {
    margin: 4px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
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
  .tail {
    height: 16px;
  }

  /* The composer slab, copied value for value from Detail.svelte's so the
     two screens are one control, not two that look alike. The armed fill is
     app.css's `button.send.armed` (GDK-1525) and is not restated here. */
  .composer-slab {
    flex: none;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
  }
  .slab-err {
    margin: 0;
    padding: 8px 16px 0;
    font-size: var(--text-micro);
    font-weight: 400;
    color: var(--color-status-reopen);
  }
  .composer {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 8px 16px;
  }
  .composer.off {
    opacity: 0.45;
  }
  .composer input {
    flex: 1 1 auto;
    min-width: 0;
    min-height: var(--spacing-control);
    padding: 0 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
  }
  .composer.off input:disabled {
    opacity: 1;
  }
  .composer input::placeholder {
    color: var(--color-text-muted);
  }
  .send {
    flex: none;
    min-height: var(--spacing-control);
    padding: 0 16px;
    border-radius: 6px;
    font-weight: 600;
    background: var(--color-bg-elevated);
    color: var(--color-text-muted);
  }
  .send-error {
    flex: 1 0 100%;
    margin: 0;
    padding: 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  .draft-note {
    flex: 1 0 100%;
    margin: 0;
    padding: 4px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
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
