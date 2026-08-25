<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Sheet from '../ui/Sheet.svelte'
  import { app, closeIssue, openIssue, sync } from '../lib/store.svelte'
  import { overlayComments, pendingComment, relTime, spineToken } from '../lib/domain'
  import { request, errorMessage } from '../lib/api'
  import { keyboardInset } from '../lib/keyboard'
  import type { DetailComment, DetailResponse, IssueLite, TransitionDoc } from '../lib/types'

  let { issueKey }: { issueKey: string } = $props()

  const lite = $derived<IssueLite | undefined>(app.issues.find((i) => i.issue_key === issueKey))

  let detail = $state<DetailResponse | null>(null)
  let detailError = $state<string | null>(null)

  let sheetOpen = $state(false)
  let transitions = $state<TransitionDoc[] | null>(null)
  let transitionError = $state<string | null>(null)
  let applying = $state<string | null>(null)

  let comment = $state('')
  let sending = $state(false)
  let sendError = $state<string | null>(null)
  /** RAM-only overlay (DESIGN.md §5). Never written to the snapshot cache. */
  let pending = $state<DetailComment | null>(null)

  const thread = $derived(overlayComments(detail?.comments ?? [], pending))

  $effect(() => {
    const key = issueKey
    detail = null
    detailError = null
    sheetOpen = false
    comment = ''
    sendError = null
    pending = null
    void (async () => {
      try {
        const res = await request<DetailResponse>(`issues/${key}/detail/`)
        if (key === issueKey) detail = res.body
      } catch (err) {
        if (key === issueKey) detailError = errorMessage(err)
      }
    })()
  })

  async function openTransitions() {
    sheetOpen = true
    transitions = null
    transitionError = null
    try {
      const res = await request<{ transitions: TransitionDoc[] }>(`issues/${issueKey}/transitions/`)
      transitions = res.body?.transitions ?? []
    } catch (err) {
      transitionError = errorMessage(err)
    }
  }

  async function applyTransition(t: TransitionDoc) {
    if (applying) return
    applying = t.id
    transitionError = null
    try {
      await request(`issues/${issueKey}/transition/`, {
        method: 'POST',
        body: { transition_id: t.id },
      })
      sheetOpen = false
      void sync()
    } catch (err) {
      transitionError = errorMessage(err)
    } finally {
      applying = null
    }
  }

  async function send() {
    const text = comment.trim()
    if (text === '' || sending) return
    sending = true
    sendError = null
    const overlay = pendingComment(text, app.me, new Date())
    pending = overlay
    comment = ''
    try {
      await request(`issues/${issueKey}/comment/`, { method: 'POST', body: { text } })
      pending = null
      const res = await request<DetailResponse>(`issues/${issueKey}/detail/`)
      detail = res.body
      void sync()
    } catch (err) {
      pending = null
      if (comment.trim() === '') comment = text
      sendError = errorMessage(err)
    } finally {
      sending = false
    }
  }
</script>

<div class="push">
  <Screen>
    {#snippet header()}
      <div class="bar">
        <button class="back" onclick={closeIssue} aria-label="Back">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M15 18l-6-6 6-6" />
          </svg>
          <span>Back</span>
        </button>
        <span class="bar-key">{issueKey}</span>
        <span class="bar-pad" aria-hidden="true"></span>
      </div>
    {/snippet}

    {#if lite}
      <article class="spined spine-{spineToken(lite)}">
        <div class="chips">
          <span class="chip">
            <span class="dot dot-{spineToken(lite)}" aria-hidden="true"></span>
            <span>{lite.status}</span>
          </span>
          {#if lite.priority}
            <span class="chip">{lite.priority}</span>
          {/if}
          {#if lite.assignee}
            <span class="chip">{lite.assignee}</span>
          {/if}
        </div>
        <h1 class="type-subject">{lite.summary}</h1>
        <p class="meta">
          {lite.issue_type}
          <span aria-hidden="true">·</span>
          updated {relTime(lite.updated_at, app.now)}
          {#if lite.reporter}
            <span aria-hidden="true">·</span>
            by {lite.reporter}
          {/if}
        </p>
      </article>
    {/if}

    <section class="body">
      {#if detailError}
        <p class="error">{detailError}</p>
      {:else if !detail}
        <div class="ghost" aria-hidden="true">
          <span class="g w1"></span><span class="g w2"></span><span class="g w3"></span>
        </div>
      {:else}
        <h3>Comments <span class="h-n">{thread.length}</span></h3>
        {#if thread.length === 0}
          <p class="none">No comments yet — yours starts the thread.</p>
        {/if}
        {#each thread as c (c.comment_id)}
          <div class="comment">
            <p class="c-head">
              <span class="c-author">{c.author ?? 'Unknown'}</span>
              <span class="c-when">{relTime(c.created_at, app.now)}</span>
            </p>
            <p class="c-body">{c.body}</p>
          </div>
        {/each}

        <h3>Description</h3>
        {#if detail.description_text}
          <p class="desc">{detail.description_text}</p>
        {:else}
          <p class="none">No description.</p>
        {/if}

        {#if detail.linked_issues.length > 0}
          <h3>Linked</h3>
          {#each detail.linked_issues as l (l.key + l.direction + l.type)}
            <button class="linked" onclick={() => openIssue(l.key)}>
              <span class="l-key">{l.key}</span>
              <span class="l-type">{l.type}</span>
              <span class="l-sum">{l.summary ?? ''}</span>
            </button>
          {/each}
        {/if}
      {/if}
      <div class="tail" aria-hidden="true"></div>
    </section>

    {#snippet footer()}
      <div class="composer-slab" use:keyboardInset>
        {#if sendError}
          <p class="send-error">{sendError}</p>
        {/if}
        {#if lite}
          <button class="status" onclick={openTransitions}>
            <span class="dot dot-{spineToken(lite)}" aria-hidden="true"></span>
            <span>{lite.status}</span>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="m6 9 6 6 6-6" />
            </svg>
          </button>
        {/if}
        <div class="composer safe-bottom">
          <input
            bind:value={comment}
            placeholder="Comment…"
            enterkeyhint="send"
            onkeydown={(e) => {
              if (e.key === 'Enter') void send()
            }}
          />
          <button class="send" disabled={comment.trim() === '' || sending} onclick={() => void send()}>
            {sending ? 'Sending…' : 'Send'}
          </button>
        </div>
      </div>
    {/snippet}
  </Screen>

  {#if sheetOpen}
    <Sheet title="Move status" onclose={() => (sheetOpen = false)}>
      <div class="t-list">
        {#if transitionError}
          <p class="error">{transitionError}</p>
        {/if}
        {#if !transitions && !transitionError}
          <p class="none">Asking the server…</p>
        {:else if transitions}
          {#each transitions as t (t.id)}
            {@const blocked = (t.fields?.length ?? 0) > 0}
            <button class="t-row" disabled={blocked || applying !== null} onclick={() => void applyTransition(t)}>
              <span class="dot dot-{t.to_category}" aria-hidden="true"></span>
              <span class="t-text">
                <span class="t-name">{applying === t.id ? 'Applying…' : t.name}</span>
                <span class="t-to">→ {t.to_status}{blocked ? ' · needs fields — use desktop' : ''}</span>
              </span>
            </button>
          {/each}
          {#if transitions.length === 0}
            <p class="none">No transitions available from this status.</p>
          {/if}
        {/if}
      </div>
    </Sheet>
  {/if}
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
  .bar-key {
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .bar-pad {
    flex: 1 1 0;
  }

  .spined {
    border-left: 3px solid transparent;
    padding: 4px 16px 12px 13px;
  }
  .spine-new {
    border-left-color: var(--color-spine-new);
  }
  .spine-inprogress {
    border-left-color: var(--color-status-inprogress);
  }
  .spine-done {
    border-left-color: var(--color-status-done);
  }
  .spine-reopen {
    border-left-color: var(--color-status-reopen);
  }

  .chips {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .status {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 6px;
    padding: 0 16px;
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-micro);
    font-weight: 600;
    color: var(--color-text-primary);
    text-align: left;
  }
  .status svg {
    width: 13px;
    height: 13px;
    margin-left: auto;
    color: var(--color-text-muted);
  }
  .chip {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: var(--spacing-control-sm);
    padding: 0 10px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 9999px;
    flex: none;
  }
  .dot-new {
    background: var(--color-status-new);
  }
  .dot-inprogress {
    background: var(--color-status-inprogress);
  }
  .dot-done {
    background: var(--color-status-done);
  }
  .dot-reopen {
    background: var(--color-status-reopen);
  }

  h1 {
    margin: 10px 0 6px;
    font-size: var(--text-title);
    line-height: var(--text-title--line-height);
    overflow-wrap: anywhere;
  }
  .meta {
    margin: 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }

  .body {
    padding: 0 16px;
  }
  .desc {
    margin: 4px 0 0;
    color: var(--color-text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
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

  .linked {
    display: flex;
    width: 100%;
    align-items: baseline;
    gap: 8px;
    padding: 6px 0;
    text-align: left;
    border-bottom: 1px solid var(--color-border-subtle);
    min-width: 0;
  }
  .l-key {
    flex: none;
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-accent-text);
  }
  .l-type {
    flex: none;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .l-sum {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
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

  .composer-slab {
    flex: none;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
  }
  .composer {
    display: flex;
    gap: 8px;
    padding: 8px 16px;
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
  .composer input::placeholder {
    color: var(--color-text-muted);
  }
  .send {
    flex: none;
    min-height: var(--spacing-control);
    padding: 0 16px;
    border-radius: 6px;
    background: var(--color-accent);
    color: var(--color-bg-base);
    font-weight: 600;
  }
  .send:disabled {
    opacity: 0.45;
  }
  .send-error {
    margin: 0;
    padding: 6px 16px 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }

  .t-list {
    overflow-y: auto;
    padding: 4px 8px 8px;
  }
  .t-row {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 10px;
    min-height: var(--spacing-control);
    padding: 6px 8px;
    border-radius: 6px;
    text-align: left;
  }
  .t-row:active:not(:disabled) {
    background: var(--color-bg-hover);
  }
  .t-row:disabled {
    opacity: 0.5;
  }
  .t-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .t-name {
    color: var(--color-text-primary);
    font-weight: 600;
  }
  .t-to {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
</style>
