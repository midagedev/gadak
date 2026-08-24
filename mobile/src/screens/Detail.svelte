<script module lang="ts">
  /*
   * Detail screen logic — exported pure so the contract tests can pin it
   * without a DOM (mobile tests run in a node environment; there is no
   * component harness). These are the screen's only decision rules; the
   * markup below consumes them and decides nothing itself.
   */
  import type { MessageKey } from '../lib/i18n'
  import type { QueueRow, Transition } from '../lib/api'

  /** What the header chip renders: the display name + the category axis. */
  export interface HeaderStatus {
    status: string
    status_category: string
  }

  export type TransitionState =
    | { phase: 'idle' }
    | { phase: 'pending'; optimistic: HeaderStatus; snapshot: HeaderStatus | null }
    | { phase: 'confirmed'; row: QueueRow }
    | { phase: 'failed'; snapshot: HeaderStatus | null }

  export type TransitionEvent =
    | { type: 'pick'; transition: Transition }
    | { type: 'ack'; row: QueueRow }
    | { type: 'fail' }

  /**
   * The header's status owner is the LIST ROW, not the detail payload:
   * GET {key}/detail/ carries no status_category/priority_rank for the
   * issue itself (measured, flow-report Q3 ②) — the web reads its
   * IssueLite pool the same way. `base` is therefore the only status
   * input on the idle path, by design.
   */
  export function displayHeader(base: QueueRow | null, st: TransitionState): HeaderStatus | null {
    if (st.phase === 'pending') return st.optimistic
    if (st.phase === 'confirmed') {
      return { status: st.row.status, status_category: st.row.status_category }
    }
    if (st.phase === 'failed') return st.snapshot
    if (base === null) return null
    return { status: base.status, status_category: base.status_category }
  }

  /** The true "before" of the current attempt — a chained pick keeps it. */
  function snapshotOf(base: QueueRow | null, st: TransitionState): HeaderStatus | null {
    return st.phase === 'pending' ? st.snapshot : displayHeader(base, st)
  }

  /**
   * One transition attempt as a pure step machine — the optimistic and
   * rollback branches the tests pin. pick swaps the chip immediately
   * (the sheet closes, no spinner lock: ux-report Q3); ack lets the
   * server's row win; fail rolls the chip back to the attempt's true
   * before and raises the banner.
   */
  export function transitionStep(
    base: QueueRow | null,
    st: TransitionState,
    event: TransitionEvent,
  ): TransitionState {
    switch (event.type) {
      case 'pick':
        return {
          phase: 'pending',
          optimistic: {
            status: event.transition.to_status,
            status_category: event.transition.to_category,
          },
          snapshot: snapshotOf(base, st),
        }
      case 'ack':
        return { phase: 'confirmed', row: event.row }
      case 'fail':
        return { phase: 'failed', snapshot: snapshotOf(base, st) }
    }
  }

  /**
   * ApiError.code → the sentence that names the next move (api.ts: the
   * code is the branch axis for the UI; the token never appears in any
   * of these). Unknown codes and transport failures share the offline
   * sentence — both mean "try again when reachable".
   */
  export function errorKey(code: string | undefined): MessageKey {
    if (code === 'forbidden_host') return 'detail.error.forbiddenHost'
    if (code === 'pairing_rejected') return 'detail.error.pairingRejected'
    if (code === 'credential_required') return 'detail.error.credentialRequired'
    return 'detail.error.offline'
  }
</script>

<script lang="ts">
  /*
   * Issue detail + comment composer + transition sheet (GDK-800 detail/
   * comments, GDK-801 transitions; ux-report Q2·Q3). Status/priority in
   * the header come from the list row the caller holds — the detail
   * payload does not carry them (flow-report Q3 ②). Writes all ride the
   * api.ts wrappers through the home's origin; the only device-local
   * state is the comment draft (lib/comment-draft.ts).
   */
  import { onMount } from 'svelte'
  import {
    ApiError,
    categoryInk,
    detail,
    postComment,
    postTransition,
    priorityInk,
    transitions,
    type ApiContext,
    type Detail as DetailData,
  } from '../lib/api'
  import { readPairing, readToken } from '../lib/settings'
  import { t } from '../lib/i18n'
  import { readDraft, saveDraft, sendWithDraft } from '../lib/comment-draft'
  import TransitionSheet from '../components/TransitionSheet.svelte'

  let {
    issueKey,
    row = null,
    onback,
  }: { issueKey: string; row?: QueueRow | null; onback: () => void } = $props()

  /** One comment as rendered — detail rows carry only account ids. */
  interface CommentVM {
    id: string
    author: string
    body: string
    at: string | null
  }

  /* ── detail load ── */
  let data: DetailData | null = $state(null)
  let loading = $state(true)
  // Banner lines: title (what failed) + action (the next move). Load
  // errors carry only the action; send/transition failures carry both.
  let bannerTitle: MessageKey | null = $state(null)
  let bannerAction: MessageKey | null = $state(null)

  /* ── status header (row-owned; see displayHeader) ── */
  let confirmedRow: QueueRow | null = $state(null) // the server row a write returned
  const baseRow = $derived(confirmedRow ?? row)
  let trState = $state<TransitionState>({ phase: 'idle' })
  const header = $derived(displayHeader(baseRow, trState))

  /* ── description fold (~12 lines, ux-report Q2) ── */
  const DESC_FOLD_LINES = 12
  const DESC_FOLD_CHARS = 600
  let descExpanded = $state(false)
  const description = $derived.by(() => {
    const d = data
    if (d === null || typeof d.description_text !== 'string') return ''
    return d.description_text
  })
  const descIsLong = $derived(
    description.split('\n').length > DESC_FOLD_LINES || description.length > DESC_FOLD_CHARS,
  )

  /* ── comments ── */
  let comments = $state<CommentVM[]>([])

  /* ── composer ── */
  let text = $state('')
  let hydrating = $state(true) // draft load on key change; suppresses save-on-input
  let sending = $state(false)
  let draftRestored = $state(false) // the "draft restored" microcopy, once
  let kbInset = $state(0)
  let ta: HTMLTextAreaElement | null = $state(null)

  /* ── transition sheet ── */
  let sheetOpen = $state(false)
  let sheetLoading = $state(false)
  let sheetError: unknown = $state(null)
  let sheetRows = $state<Transition[]>([])

  function errorKeyOf(err: unknown): MessageKey {
    return errorKey(err instanceof ApiError ? err.code : undefined)
  }

  /** Same endpoint rule as Queue.svelte (pairing meta; DEV rides the vite proxy). */
  function endpointOf(): string {
    const pairing = readPairing()
    return pairing?.endpoint ?? (import.meta.env.DEV ? 'http://127.0.0.1:7899' : '')
  }

  /** Detail comments have no display-name field — an abbreviated id is the label. */
  function abbreviate(id: string | null): string {
    if (id === null || id === '') return '—'
    return id.length > 8 ? `${id.slice(0, 8)}…` : id
  }

  function vmOf(c: DetailData['comments'][number]): CommentVM {
    return { id: c.comment_id, author: abbreviate(c.author_account_id), body: c.body, at: c.created_at }
  }

  function whenLabel(iso: string): string {
    const d = new Date(iso)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
  }

  async function load(): Promise<void> {
    loading = true
    const endpoint = endpointOf()
    if (endpoint === '') {
      // Unpaired: nothing to read. The sentence is the offline one — the
      // actionable move is the same (reach the home).
      bannerTitle = null
      bannerAction = 'detail.error.offline'
      loading = false
      return
    }
    try {
      const ctx: ApiContext = { endpoint, token: await readToken() }
      const d = await detail(ctx, issueKey)
      data = d
      comments = d.comments.map(vmOf)
      bannerTitle = null
      bannerAction = null
    } catch (err) {
      bannerTitle = null
      bannerAction = errorKeyOf(err)
    } finally {
      loading = false
    }
  }

  $effect(() => {
    void load()
  })

  // Draft lifecycle on the screen: hydrate once per key, then persist on
  // every input. `sending` suspends the save so the optimistic clear does
  // not delete the stored draft before the ack (comment-draft contract).
  $effect(() => {
    const key = issueKey
    hydrating = true
    descExpanded = false
    const stored = readDraft(endpointOf(), key)
    text = stored
    draftRestored = stored.trim() !== ''
    queueMicrotask(() => (hydrating = false))
  })

  $effect(() => {
    const key = issueKey
    const body = text
    if (hydrating || sending) return
    saveDraft(endpointOf(), key, body)
  })

  // Keyboard inset: the layout viewport keeps its height while the visual
  // one shrinks — the difference is the keyboard. Without this the
  // WKWebView leaves the composer under it (ux-report Q2 gotcha).
  onMount(() => {
    const vv = window.visualViewport
    if (!vv) return
    const update = (): void => {
      kbInset = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop))
    }
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    update()
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  })

  // One-line composer that grows with the body, capped ~4 rows; the next
  // step (a taller sheet) is a later round (ux-report Q2).
  const COMPOSER_MAX_PX = 96

  function autosize(): void {
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${Math.min(ta.scrollHeight, COMPOSER_MAX_PX)}px`
  }

  async function send(): Promise<void> {
    const body = text.trim()
    if (body === '' || sending) return
    const ctx: ApiContext = { endpoint: endpointOf(), token: await readToken() }
    const prev = text
    sending = true
    text = '' // optimistic clear — the stored draft survives until the ack
    const res = await sendWithDraft(ctx.endpoint, issueKey, body, (b) => postComment(ctx, issueKey, b))
    sending = false
    if (res.ok) {
      bannerTitle = null
      bannerAction = null
      draftRestored = false
      const w = res.value
      comments = [
        ...comments,
        { id: w.comment.comment_id, author: w.comment.author || '—', body: w.comment.body, at: w.comment.created_at },
      ]
      confirmedRow = w.issue // the fresh row the write returned
      queueMicrotask(autosize)
    } else {
      text = prev // restore the body — storage still holds the draft
      bannerTitle = 'detail.comment.failed'
      bannerAction = errorKeyOf(res.error)
      queueMicrotask(autosize)
    }
  }

  async function openSheet(): Promise<void> {
    sheetOpen = true
    sheetLoading = true
    sheetError = null
    try {
      const ctx: ApiContext = { endpoint: endpointOf(), token: await readToken() }
      sheetRows = (await transitions(ctx, issueKey)).transitions
    } catch (err) {
      sheetError = err
      sheetRows = []
    } finally {
      sheetLoading = false
    }
  }

  async function pickTransition(tr: Transition): Promise<void> {
    // Optimistic: the sheet closes and the chip swaps before the POST
    // waits; a failure rolls back and says so (ux-report Q3).
    sheetOpen = false
    trState = transitionStep(baseRow, trState, { type: 'pick', transition: tr })
    try {
      const ctx: ApiContext = { endpoint: endpointOf(), token: await readToken() }
      const res = await postTransition(ctx, issueKey, tr.id) // body: transition_id only
      confirmedRow = res.issue
      trState = transitionStep(baseRow, trState, { type: 'ack', row: res.issue })
      bannerTitle = null
      bannerAction = null
    } catch (err) {
      trState = transitionStep(baseRow, trState, { type: 'fail' })
      bannerTitle = 'detail.transition.failed'
      bannerAction = errorKeyOf(err)
    }
  }
</script>

<section
  class="m-main scroll-region"
  class:m-detail-locked={sheetOpen}
  data-testid="detail-screen"
  data-key={issueKey}
>
  <header class="m-detail-header">
    <button
      class="m-detail-back"
      type="button"
      onclick={onback}
      aria-label={t('detail.back')}
      data-testid="detail-back"
    >‹</button>
    <div class="m-detail-heading">
      <div class="m-detail-keyline">
        <span class="m-detail-key">{issueKey}</span>
        {#if header}
          <button
            class="m-detail-chip"
            type="button"
            onclick={() => void openSheet()}
            aria-haspopup="dialog"
            data-testid="detail-status"
            data-category={header.status_category}
          >
            <span class="m-detail-dot" style:background={categoryInk(header.status_category)} aria-hidden="true"></span>
            {header.status}
          </button>
        {/if}
        <span
          class="m-detail-prio"
          style:color={priorityInk(baseRow?.priority_rank ?? 0)}
          aria-label={baseRow?.priority ?? t('priority.none')}
          role="img"
        ></span>
      </div>
      {#if baseRow}
        <h1 class="m-detail-summary">{baseRow.summary}</h1>
      {/if}
    </div>
  </header>

  {#if bannerAction}
    <div class="m-banner" role="status" data-testid="detail-banner">
      {#if bannerTitle}<p>{t(bannerTitle)}</p>{/if}
      <p>{t(bannerAction)}</p>
    </div>
  {/if}

  {#if loading}
    <p class="m-empty" data-testid="detail-loading">{t('detail.loading')}</p>
  {:else if data}
    {#if description !== ''}
      <div
        class="m-detail-desc"
        class:m-detail-desc-clamped={!descExpanded}
        data-testid="detail-description"
      >{description}</div>
      {#if descIsLong}
        <button
          class="m-detail-more"
          type="button"
          onclick={() => (descExpanded = !descExpanded)}
          data-testid="detail-desc-toggle"
        >
          {descExpanded ? t('detail.desc.less') : t('detail.desc.more')}
        </button>
      {/if}
    {:else}
      <p class="m-detail-quiet" data-testid="detail-description">{t('detail.desc.empty')}</p>
    {/if}

    <section class="m-detail-comments">
      <h2 class="m-detail-comments-title">{t('detail.comments.title', { n: comments.length })}</h2>
      {#if comments.length === 0}
        <p class="m-detail-quiet">{t('detail.comments.empty')}</p>
      {:else}
        <ul data-testid="detail-comments">
          {#each comments as c (c.id)}
            <li class="m-detail-comment" data-testid="detail-comment">
              <div class="m-detail-comment-meta">
                <span class="m-detail-comment-author">{c.author}</span>
                {#if c.at}<span class="m-detail-comment-at">{whenLabel(c.at)}</span>{/if}
              </div>
              <p class="m-detail-comment-body">{c.body}</p>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}

  <div class="m-detail-composer-wrap" style:transform={`translateY(${ -kbInset }px)`}>
    {#if draftRestored}
      <p class="m-detail-draft-note" data-testid="draft-restored">{t('detail.comment.draftRestored')}</p>
    {/if}
    <form
      class="m-detail-composer"
      onsubmit={(e) => {
        e.preventDefault()
        void send()
      }}
      data-testid="comment-composer"
    >
      <textarea
        bind:this={ta}
        bind:value={text}
        rows="1"
        placeholder={t('detail.comment.placeholder')}
        oninput={() => {
          autosize()
          if (draftRestored) draftRestored = false
        }}
        data-testid="comment-input"
      ></textarea>
      <button
        class="m-detail-send"
        type="submit"
        disabled={text.trim() === '' || sending}
        data-testid="comment-send"
      >
        {t('detail.comment.send')}
      </button>
    </form>
  </div>
</section>

{#if sheetOpen}
  <TransitionSheet
    transitions={sheetRows}
    loading={sheetLoading}
    errorText={sheetError === null ? null : t(errorKeyOf(sheetError))}
    onpick={(tr) => void pickTransition(tr)}
    onclose={() => (sheetOpen = false)}
  />
{/if}

<style>
  /* Same rules as every mobile screen: tokens only, interactive heights
     on the iOS 44pt floor (never the 42px web row token), min-height so
     Dynamic Type grows instead of clips. */
  .m-detail-locked {
    overflow: hidden;
  }

  .m-detail-header {
    display: flex;
    align-items: flex-start;
    gap: 0.25rem;
    padding: 0.25rem 0.25rem 0.25rem 0;
  }

  .m-detail-back {
    flex: none;
    width: 44px;
    height: 44px;
    border: 0;
    background: transparent;
    color: var(--color-accent-text);
    font-size: var(--text-heading);
    line-height: 1;
    cursor: pointer;
  }

  .m-detail-heading {
    flex: 1 1 auto;
    min-width: 0;
    padding-top: 0.5rem;
  }

  .m-detail-keyline {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .m-detail-key {
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }

  .m-detail-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    min-height: 44px;
    padding: 0 0.75rem;
    border: 1px solid var(--color-border-subtle);
    border-radius: 9999px;
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    font-family: var(--font-sans);
    font-size: var(--text-micro);
    font-weight: 500;
    cursor: pointer;
  }

  .m-detail-dot {
    flex: none;
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 9999px;
  }

  .m-detail-prio {
    flex: none;
    width: 3px;
    height: 1.25rem;
    border-radius: 2px;
    background: currentColor;
  }

  .m-detail-summary {
    margin: 0.125rem 0 0;
    color: var(--color-text-primary);
    font-size: var(--text-heading);
    line-height: 1.25;
    overflow-wrap: break-word;
  }

  .m-detail-desc {
    padding: 0.5rem 0.25rem 0;
    color: var(--color-text-primary);
    font-size: var(--text-body);
    line-height: 1.45;
    white-space: pre-wrap;
    overflow-wrap: break-word;
  }

  .m-detail-desc-clamped {
    display: -webkit-box;
    -webkit-line-clamp: 12;
    line-clamp: 12;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .m-detail-more {
    min-height: 44px;
    padding: 0 0.25rem;
    border: 0;
    background: transparent;
    color: var(--color-accent-text);
    font-family: var(--font-sans);
    font-size: var(--text-body);
    cursor: pointer;
  }

  .m-detail-quiet {
    margin: 0.5rem 0.25rem;
    color: var(--color-text-muted);
    font-size: var(--text-body);
  }

  .m-detail-comments {
    margin: 0.75rem 0 1rem;
  }

  .m-detail-comments-title {
    margin: 0 0 0.25rem;
    padding: 0 0.25rem;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
  }

  .m-detail-comment {
    padding: 0.5rem 0.25rem;
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .m-detail-comment-meta {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .m-detail-comment-author {
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }

  .m-detail-comment-at {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }

  .m-detail-comment-body {
    margin: 0.125rem 0 0;
    color: var(--color-text-primary);
    font-size: var(--text-body);
    white-space: pre-wrap;
    overflow-wrap: break-word;
  }

  .m-detail-composer-wrap {
    position: sticky;
    bottom: 0;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
  }

  .m-detail-draft-note {
    margin: 0;
    padding: 0.25rem 0.75rem 0;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
  }

  .m-detail-composer {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
    padding: 0.5rem 0.25rem;
  }

  .m-detail-composer textarea {
    flex: 1 1 auto;
    min-width: 0;
    min-height: 44px;
    max-height: 96px;
    padding: 0.5rem 0.625rem;
    border: 1px solid var(--color-border-strong);
    border-radius: 8px;
    background: var(--color-bg-base);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
    font-size: var(--text-body);
    resize: none;
  }

  .m-detail-send {
    flex: none;
    min-height: 44px;
    padding: 0 1rem;
    border: 0;
    border-radius: 8px;
    background: var(--color-accent);
    color: var(--color-bg-base);
    font-family: var(--font-sans);
    font-size: var(--text-body);
    cursor: pointer;
  }

  .m-detail-send:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
