<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Sheet from '../ui/Sheet.svelte'
  import AdfBody from '../ui/AdfBody.svelte'
  import { app, closeIssue, openIssue, sync } from '../lib/store.svelte'
  import { overlayComments, pendingComment, relTime, spineToken } from '../lib/domain'
  import { request, errorMessage, ApiError } from '../lib/api'
  import {
    setDescription,
    setAssignee,
    setPriority,
    setSummary,
    getPriorities,
    searchUsers,
  } from '../lib/writes'
  import { keyboardInset } from '../lib/keyboard'
  import { t } from '../lib/i18n'
  import type {
    DetailComment,
    DetailResponse,
    IssueLite,
    PriorityDoc,
    TransitionDoc,
    UserDoc,
  } from '../lib/types'

  let { issueKey }: { issueKey: string } = $props()

  /** The last issue a write on this screen brought back (GDK-1497 A2). */
  let written = $state<IssueLite | null>(null)

  // The header paints the freshest lite it has: the mirror row, overlaid by
  // the last write's own answer. A sync lands a newer row and wins on
  // updated_at; until it does, the write response holds the header.
  const lite = $derived.by<IssueLite | undefined>(() => {
    const row = app.issues.find((i) => i.issue_key === issueKey)
    if (row && written && (written.updated_at ?? '') > (row.updated_at ?? '')) return written
    return row ?? written ?? undefined
  })

  let detail = $state<DetailResponse | null>(null)
  let detailError = $state<string | null>(null)

  let sheetOpen = $state(false)
  let transitions = $state<TransitionDoc[] | null>(null)
  let transitionError = $state<string | null>(null)
  let applying = $state<string | null>(null)
  /** Serve-level: GET/POST origin writes 409 credential_required. Sticky for this screen. */
  let writesOff = $state(false)
  let failedId = $state<string | null>(null)

  /* ── A2 header writes: assignee, priority, summary, description ── */
  let assigneeOpen = $state(false)
  let priorityOpen = $state(false)
  /** In-flight row id across both pick sheets (null = idle). */
  let applyingId = $state<string | null>(null)
  let rowError = $state<string | null>(null)
  let failedRow = $state<string | null>(null)
  let userQuery = $state('')
  let users = $state<UserDoc[]>([])
  let searching = $state(false)
  /** Search debounce handle + staleness guard: results land only for the
   *  newest keystroke, and the previous request is aborted, not just ignored. */
  let searchTimer: ReturnType<typeof setTimeout> | null = null
  let searchAbort: AbortController | null = null
  let searchSeq = 0
  /** Per-key catalog, kept for this screen's life once it loads. */
  let priorities = $state<PriorityDoc[] | null>(null)
  let prioritiesError = $state<string | null>(null)
  let prioritiesLoading = $state(false)
  let summaryEditing = $state(false)
  let summaryDraft = $state('')
  let summaryError = $state<string | null>(null)
  let summarySaving = $state(false)
  let descOpen = $state(false)
  let descDraft = $state('')
  let descSaving = $state(false)
  let descError = $state<string | null>(null)
  /** 409 format_loss answered: the next save re-PUTs with force. */
  let descForceAsk = $state(false)

  let comment = $state('')
  let sending = $state(false)
  let sendError = $state<string | null>(null)
  /** RAM-only overlay (DESIGN.md §5). Never written to the snapshot cache. */
  let pending = $state<DetailComment | null>(null)

  const thread = $derived(overlayComments(detail?.comments ?? [], pending))
  /** Accent fill only when this control can send (GDK-934). Empty or writes-off recedes. */
  const sendArmed = $derived(!writesOff && (comment.trim() !== '' || sending))

  /** Assignee sheet rows beyond the clearing one: me, the current assignee,
   *  then what the search brought — deduped by account id. */
  const assigneeCandidates = $derived.by<
    Array<{ id: string; accountId: string | null; label: string; sub: string | null }>
  >(() => {
    const out: Array<{ id: string; accountId: string | null; label: string; sub: string | null }> = []
    const seen = new Set<string>()
    const me = app.me
    const meId = me?.account_id ?? null
    if (me && meId) {
      seen.add(meId)
      out.push({
        id: 'me',
        accountId: meId,
        label: me.name || me.email || t('common.me'),
        sub: t('common.me'),
      })
    }
    const currentId = lite?.assignee_id ?? null
    if (currentId && !seen.has(currentId)) {
      seen.add(currentId)
      out.push({
        id: 'current',
        accountId: currentId,
        label: lite?.assignee ?? currentId,
        sub: null,
      })
    }
    for (const u of users) {
      if (seen.has(u.account_id)) continue
      seen.add(u.account_id)
      out.push({
        id: 'user-' + u.account_id,
        accountId: u.account_id,
        label: u.display_name,
        sub: u.email,
      })
    }
    return out
  })

  function isCredentialRequired(err: unknown): boolean {
    return err instanceof ApiError && err.code === 'credential_required'
  }

  // The A2 write states are NOT reset here: GDK-692 bans $state assigns in
  // effect bodies, and the app already owns this — App.svelte remounts the
  // whole screen per key ({#key}), so every control above starts clean on
  // a new issue. Only the search plumbing (plain lets, not $state) needs
  // an explicit teardown, and that rides this effect's cleanup.
  $effect(() => {
    const key = issueKey
    detail = null
    detailError = null
    sheetOpen = false
    comment = ''
    sendError = null
    pending = null
    transitions = null
    applying = null
    failedId = null
    if (!writesOff) transitionError = null
    void (async () => {
      try {
        const res = await request<DetailResponse>(`issues/${key}/detail/`)
        if (key === issueKey) detail = res.body
      } catch (err) {
        if (key !== issueKey) return
        detailError =
          err instanceof ApiError && err.code === 'not_found'
            ? t('detail.notFound')
            : errorMessage(err)
      }
    })()
    return () => {
      searchSeq++
      if (searchTimer) clearTimeout(searchTimer)
      searchTimer = null
      searchAbort?.abort()
      searchAbort = null
    }
  })

  // One owner (GDK-906): header chip is data (DESIGN.md §4); this is the control.
  async function openTransitions() {
    if (writesOff) return
    if (transitions !== null) {
      sheetOpen = true
      return
    }
    transitionError = null
    failedId = null
    try {
      const res = await request<{ transitions: TransitionDoc[] }>(`issues/${issueKey}/transitions/`)
      transitions = res.body?.transitions ?? []
      if (writesOff) return
      sheetOpen = true
    } catch (err) {
      transitionError = errorMessage(err)
      if (isCredentialRequired(err)) {
        writesOff = true
        return
      }
      sheetOpen = true
    }
  }

  async function applyTransition(doc: TransitionDoc) {
    if (applying || writesOff) return
    applying = doc.id
    transitionError = null
    failedId = null
    try {
      await request(`issues/${issueKey}/transition/`, {
        method: 'POST',
        body: { transition_id: doc.id },
      })
      sheetOpen = false
      transitions = null
      void sync()
    } catch (err) {
      transitionError = errorMessage(err)
      failedId = doc.id
      if (isCredentialRequired(err)) {
        writesOff = true
        sheetOpen = false
      }
    } finally {
      applying = null
    }
  }

  async function send() {
    const text = comment.trim()
    if (writesOff || text === '' || sending) return
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
      if (isCredentialRequired(err)) {
        writesOff = true
        transitionError = sendError
      }
    } finally {
      sending = false
    }
  }

  /* ── A2 write controls. Every one of them goes through the same 409 road
   *  the transition sheet built: sticky writesOff, one sentence on the
   *  status row, every control recedes. No control ever throws onward. */

  function refuseWrite(err: unknown): boolean {
    if (!isCredentialRequired(err)) return false
    writesOff = true
    transitionError = errorMessage(err)
    assigneeOpen = false
    priorityOpen = false
    summaryEditing = false
    descOpen = false
    return true
  }

  function openAssignee() {
    if (writesOff) return
    rowError = null
    failedRow = null
    assigneeOpen = true
  }

  function onUserQuery(next: string) {
    userQuery = next
    if (searchTimer) clearTimeout(searchTimer)
    if (next.trim().length < 2) {
      searchSeq++
      searchAbort?.abort()
      searchAbort = null
      users = []
      searching = false
      return
    }
    const seq = ++searchSeq
    searching = true
    searchTimer = setTimeout(() => void runUserSearch(next, seq), 250)
  }

  async function runUserSearch(q: string, seq: number) {
    searchAbort?.abort()
    const ctl = new AbortController()
    searchAbort = ctl
    try {
      const res = await searchUsers(issueKey, q.trim(), { signal: ctl.signal })
      if (seq !== searchSeq) return
      users = res.users
    } catch (err) {
      if (seq !== searchSeq || ctl.signal.aborted) return
      // The fixed rows stay usable; only the search band reports.
      users = []
      rowError = errorMessage(err)
    } finally {
      if (seq === searchSeq) searching = false
    }
  }

  async function pickAssignee(accountId: string | null) {
    if (writesOff || applyingId) return
    applyingId = accountId ?? 'unassigned'
    rowError = null
    failedRow = null
    try {
      const res = await setAssignee(issueKey, accountId)
      written = res.issue
      assigneeOpen = false
      void sync()
    } catch (err) {
      if (refuseWrite(err)) return
      rowError = errorMessage(err)
      failedRow = applyingId
    } finally {
      applyingId = null
    }
  }

  function openPriority() {
    if (writesOff) return
    priorityOpen = true
    if (priorities !== null || prioritiesLoading) return
    prioritiesError = null
    prioritiesLoading = true
    void (async () => {
      try {
        const res = await getPriorities(issueKey)
        priorities = res.priorities
      } catch (err) {
        if (refuseWrite(err)) return
        prioritiesError = errorMessage(err)
      } finally {
        prioritiesLoading = false
      }
    })()
  }

  async function pickPriority(priorityId: string | null) {
    if (writesOff || applyingId) return
    applyingId = priorityId ?? 'none'
    rowError = null
    failedRow = null
    try {
      const res = await setPriority(issueKey, priorityId)
      written = res.issue
      priorityOpen = false
      void sync()
    } catch (err) {
      if (refuseWrite(err)) return
      rowError = errorMessage(err)
      failedRow = applyingId
    } finally {
      applyingId = null
    }
  }

  function editSummary() {
    if (writesOff || !lite) return
    summaryDraft = lite.summary
    summaryError = null
    summaryEditing = true
  }

  async function saveSummary() {
    const text = summaryDraft.trim()
    if (writesOff || summarySaving || text === '') return
    summarySaving = true
    summaryError = null
    try {
      const res = await setSummary(issueKey, text)
      written = res.issue
      summaryEditing = false
      void sync()
    } catch (err) {
      if (refuseWrite(err)) return
      summaryError = errorMessage(err)
    } finally {
      summarySaving = false
    }
  }

  function editDescription() {
    if (writesOff) return
    // description_md is the write format; old serves predate it and the
    // flattened text is the best draft they can offer.
    descDraft = detail?.description_md ?? detail?.description_text ?? ''
    descError = null
    descForceAsk = false
    descOpen = true
  }

  async function saveDescription(force: boolean) {
    if (writesOff || descSaving) return
    descSaving = true
    descError = null
    try {
      await setDescription(issueKey, descDraft, { force })
      descOpen = false
      descForceAsk = false
      // Refetch so AdfBody re-renders the stored body, then sync the rows.
      const res = await request<DetailResponse>(`issues/${issueKey}/detail/`)
      detail = res.body
      void sync()
    } catch (err) {
      if (refuseWrite(err)) return
      if (err instanceof ApiError && err.code === 'format_loss') {
        descForceAsk = true
      } else if (err instanceof ApiError && err.code === 'placeholder' && err.serverMessage) {
        descError = t('write.placeholderRefused', { message: err.serverMessage })
      } else {
        descError = errorMessage(err)
      }
    } finally {
      descSaving = false
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
        </div>
        {#if summaryEditing}
          <div class="summary-edit">
            <input
              bind:value={summaryDraft}
              placeholder={t('write.issueTitle')}
              aria-label={t('write.editTitle')}
              enterkeyhint="done"
              onkeydown={(e) => {
                if (e.key === 'Enter') void saveSummary()
              }}
            />
            <button class="save" disabled={summarySaving || summaryDraft.trim() === ''} onclick={() => void saveSummary()}>
              {t('common.save')}
            </button>
            <button class="ghost" onclick={() => (summaryEditing = false)}>{t('common.cancel')}</button>
            {#if summaryDraft.trim() === ''}
              <p class="field-err">{t('write.titleRequired')}</p>
            {:else if summaryError}
              <p class="field-err">{summaryError}</p>
            {/if}
          </div>
        {:else}
          <div class="subject">
            <h1 class="type-subject">{lite.summary}</h1>
            <button class="edit" onclick={editSummary} disabled={writesOff} aria-label={t('write.editTitle')}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
              </svg>
            </button>
          </div>
        {/if}
        <p class="meta">
          {lite.issue_type}
          <span aria-hidden="true">·</span>
          <button class="m-btn" onclick={openPriority} disabled={writesOff}>
            {lite.priority ?? t('write.changePriority')}
            <svg class="m-chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="m6 9 6 6 6-6" />
            </svg>
          </button>
          <span aria-hidden="true">·</span>
          <button class="m-btn" onclick={openAssignee} disabled={writesOff}>
            {lite.assignee ?? t('common.unassigned')}
            <svg class="m-chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="m6 9 6 6 6-6" />
            </svg>
          </button>
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
        <h3>{t('detail.comments')} <span class="h-n">{thread.length}</span></h3>
        {#if thread.length === 0}
          <p class="none">No comments yet — yours starts the thread.</p>
        {/if}
        {#each thread as c (c.comment_id)}
          <div class="comment">
            <p class="c-head">
              <span class="c-author">{c.author ?? t('detail.unknownAuthor')}</span>
              <span class="c-when">{relTime(c.created_at, app.now)}</span>
            </p>
            <AdfBody doc={c.raw_body} fallback={c.body} {issueKey} attachments={detail.attachments} />
          </div>
        {/each}

        <div class="h-row">
          <h3>{t('detail.description')}</h3>
          <button class="edit" onclick={editDescription} disabled={writesOff} aria-label={t('write.editDescription')}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M12 20h9" />
              <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
            </svg>
          </button>
        </div>
        {#if detail.description_adf || detail.description_text}
          <AdfBody
            doc={detail.description_adf}
            fallback={detail.description_text}
            {issueKey}
            attachments={detail.attachments}
          />
        {:else}
          <p class="none">{t('detail.noDescription')}</p>
        {/if}

        {#if detail.linked_issues.length > 0}
          <h3>{t('detail.linked')}</h3>
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
        {#if lite}
          <button class="status" disabled={writesOff} onclick={openTransitions}>
            <span class="status-line">
              <span class="dot dot-{spineToken(lite)}" aria-hidden="true"></span>
              <span>{lite.status}</span>
              {#if !writesOff}
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="m6 9 6 6 6-6" />
                </svg>
              {/if}
            </span>
            {#if writesOff && transitionError}
              <span class="status-err">{transitionError}</span>
            {/if}
          </button>
        {/if}
        <div class="composer safe-bottom" class:off={writesOff}>
          <input
            bind:value={comment}
            disabled={writesOff}
            placeholder="Comment…"
            enterkeyhint="send"
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
            {sending ? 'Sending…' : 'Send'}
          </button>
          {#if sendError && !writesOff}
            <p class="send-error">{sendError}</p>
          {/if}
        </div>
      </div>
    {/snippet}
  </Screen>

  {#if sheetOpen}
    <Sheet title="Move status" onclose={() => (sheetOpen = false)}>
      <div class="t-list">
        {#if !transitions && !transitionError}
          <p class="none">Asking the server…</p>
        {:else if transitions}
          {#each transitions as tr (tr.id)}
            {@const blocked = (tr.fields?.length ?? 0) > 0}
            <button class="t-row" disabled={blocked || applying !== null} onclick={() => void applyTransition(tr)}>
              <span class="dot dot-{tr.to_category}" aria-hidden="true"></span>
              <span class="t-text">
                <span class="t-name">{applying === tr.id ? 'Applying…' : tr.name}</span>
                <span class="t-to">→ {tr.to_status}{blocked ? ' · needs fields — use desktop' : ''}</span>
                {#if failedId === tr.id && transitionError}
                  <span class="t-err">{transitionError}</span>
                {/if}
              </span>
            </button>
          {/each}
          {#if transitions.length === 0}
            <p class="none">No transitions available from this status.</p>
          {/if}
        {:else if transitionError}
          <p class="error">{transitionError}</p>
        {/if}
      </div>
    </Sheet>
  {/if}

  {#if assigneeOpen}
    <Sheet title={t('write.pickAssignee')} onclose={() => (assigneeOpen = false)}>
      <div class="pick-list">
        <button class="t-row" disabled={applyingId !== null} onclick={() => void pickAssignee(null)}>
          <span class="t-text">
            <span class="t-name">{t('common.unassigned')}</span>
            {#if failedRow === 'unassigned' && rowError}
              <span class="t-err">{rowError}</span>
            {/if}
          </span>
        </button>
        {#each assigneeCandidates as c (c.id)}
          <button
            class="t-row"
            class:current={lite?.assignee_id === c.accountId}
            disabled={applyingId !== null}
            onclick={() => void pickAssignee(c.accountId)}
          >
            <span class="t-text">
              <span class="t-name">{c.label}</span>
              {#if c.sub}
                <span class="t-to">{c.sub}</span>
              {/if}
              {#if failedRow === c.id && rowError}
                <span class="t-err">{rowError}</span>
              {/if}
            </span>
          </button>
        {/each}
        <div class="search">
          <input
            value={userQuery}
            placeholder={t('write.searchNameEmail')}
            inputmode="search"
            oninput={(e) => onUserQuery(e.currentTarget.value)}
          />
          {#if searching}
            <p class="none">Searching…</p>
          {:else if userQuery.trim().length >= 2 && users.length === 0}
            <p class="none">{t('write.userNotFound')}</p>
          {/if}
        </div>
      </div>
    </Sheet>
  {/if}

  {#if priorityOpen}
    <Sheet title={t('write.changePriority')} onclose={() => (priorityOpen = false)}>
      <div class="pick-list">
        {#if prioritiesLoading}
          <p class="none">Asking the server…</p>
        {:else if prioritiesError}
          <p class="error">{prioritiesError}</p>
        {:else if priorities && lite}
          <button class="t-row" disabled={applyingId !== null} onclick={() => void pickPriority(null)}>
            <span class="t-text">
              <span class="t-name">{t('common.none')}</span>
              {#if failedRow === 'none' && rowError}
                <span class="t-err">{rowError}</span>
              {/if}
            </span>
          </button>
          {#each priorities as p (p.id)}
            {@const current = lite.priority_id === p.id}
            <button
              class="t-row"
              class:current
              disabled={applyingId !== null}
              onclick={() => void pickPriority(p.id)}
            >
              {#if current}
                <svg class="tick" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M20 6 9 17l-5-5" />
                </svg>
              {/if}
              <span class="t-text">
                <span class="t-name">{p.name}</span>
                {#if failedRow === p.id && rowError}
                  <span class="t-err">{rowError}</span>
                {/if}
              </span>
            </button>
          {/each}
          {#if priorities.length === 0}
            <p class="none">{t('write.noPriorities')}</p>
          {/if}
        {/if}
      </div>
    </Sheet>
  {/if}

  {#if descOpen}
    <Sheet title={t('write.editDescription')} tall onclose={() => (descOpen = false)}>
      <div class="desc-edit">
        <textarea
          bind:value={descDraft}
          placeholder={t('write.descriptionPlain')}
          aria-label={t('write.editDescription')}
        ></textarea>
        {#if descForceAsk}
          <p class="error">{t('write.descriptionForceAsk')}</p>
          <div class="desc-actions">
            <button class="save" disabled={descSaving} onclick={() => void saveDescription(true)}>
              {t('write.descriptionReplace')}
            </button>
            <button class="ghost" onclick={() => (descForceAsk = false)}>{t('common.cancel')}</button>
          </div>
        {:else}
          {#if descError}
            <p class="error">{descError}</p>
          {/if}
          <div class="desc-actions">
            <button class="save" disabled={descSaving} onclick={() => void saveDescription(false)}>
              {t('common.save')}
            </button>
            <button class="ghost" onclick={() => (descOpen = false)}>{t('common.cancel')}</button>
          </div>
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
    flex-direction: column;
    align-items: stretch;
    padding: 0 16px;
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-micro);
    font-weight: 600;
    color: var(--color-text-primary);
    text-align: left;
  }
  .status:disabled {
    opacity: 0.45;
  }
  .status-line {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 6px;
  }
  .status-line svg {
    width: 13px;
    height: 13px;
    margin-left: auto;
    color: var(--color-text-muted);
  }
  .status-err {
    padding: 0 0 8px;
    font-size: var(--text-micro);
    font-weight: 400;
    color: var(--color-status-reopen);
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
  /* The subject line is a control surface now (GDK-1497 A2): the h1 keeps
     its grammar, an edit glyph rides its end, and the meta fragments below
     open pickers. Every control recedes the way the status row does. */
  .subject {
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }
  .subject h1 {
    flex: 1 1 auto;
  }
  .edit {
    flex: none;
    width: 44px;
    margin-top: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-text-muted);
  }
  .edit svg {
    width: 18px;
    height: 18px;
  }
  .edit:disabled {
    opacity: 0.45;
  }
  .summary-edit {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 10px 0 6px;
  }
  .summary-edit input {
    flex: 1 1 160px;
    min-width: 0;
    min-height: var(--spacing-control);
    padding: 0 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-size: var(--text-body);
  }
  .field-err {
    flex: 1 0 100%;
    margin: 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  .save {
    flex: none;
    min-height: var(--spacing-control);
    padding: 0 16px;
    border-radius: 6px;
    font-weight: 600;
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
  }
  .save:disabled {
    opacity: 0.45;
  }
  .ghost {
    flex: none;
    min-height: var(--spacing-control);
    padding: 0 12px;
    border-radius: 6px;
    color: var(--color-accent-text);
  }
  .meta {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0 4px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .m-btn {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 0 2px;
    max-width: 100%;
    color: var(--color-text-secondary);
    font-size: var(--text-micro);
  }
  .m-btn:disabled {
    opacity: 0.45;
  }
  .m-chev {
    flex: none;
    width: 12px;
    height: 12px;
    color: var(--color-text-muted);
  }
  .h-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .h-row h3 {
    flex: 1 1 auto;
  }
  .h-row .edit {
    width: 44px;
    margin-top: 16px;
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
  .send.armed {
    background: var(--color-accent);
    color: var(--color-bg-base);
  }
  .send-error {
    flex: 1 0 100%;
    margin: 0;
    padding: 0;
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
  .t-err {
    font-size: var(--text-micro);
    font-weight: 400;
    color: var(--color-status-reopen);
  }
  .t-row.current .t-name {
    color: var(--color-accent-text);
  }
  .tick {
    flex: none;
    width: 14px;
    height: 14px;
    color: var(--color-accent-text);
  }
  .pick-list {
    overflow-y: auto;
    padding: 4px 8px 8px;
    display: flex;
    flex-direction: column;
  }
  .search {
    border-top: 1px solid var(--color-border-subtle);
    padding: 8px;
    margin-top: 4px;
  }
  .search input {
    width: 100%;
    min-height: var(--spacing-control);
    padding: 0 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
  }
  .search .none {
    padding: 6px 2px 0;
  }
  .desc-edit {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
    padding: 4px 16px 16px;
  }
  .desc-edit textarea {
    flex: 1 1 auto;
    min-height: 180px;
    padding: 10px 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font: inherit;
    resize: none;
  }
  .desc-actions {
    display: flex;
    gap: 8px;
    padding-top: 8px;
  }
</style>
