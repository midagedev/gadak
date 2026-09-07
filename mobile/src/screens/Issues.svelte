<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Row from '../ui/Row.svelte'
  import DocRow from '../ui/DocRow.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import GlanceStrip from '../ui/GlanceStrip.svelte'
  import Skeleton from '../ui/Skeleton.svelte'
  import ScopeSheet from '../ui/ScopeSheet.svelte'
  import Sheet from '../ui/Sheet.svelte'
  import { t } from '../lib/i18n'
  import { ApiError, errorMessage } from '../lib/api'
  import { createIssue, getCreateMeta } from '../lib/writes'
  import type { CreateMetaProject } from '../lib/types'
  import {
    app,
    dismissSessionStrip,
    issuesBootKind,
    openIssue,
    setScope,
    showOfflineBanner,
    sync,
    switchTab,
  } from '../lib/store.svelte'
  import {
    buildList,
    buildScopes,
    hasIdentity,
    relTime,
    resolveScope,
    scopeCount,
    scopePages,
    sessionLine,
    SCOPE_ALL_OPEN,
    SCOPE_DOCS_UPDATED,
    SCOPE_ME,
    type Scope,
  } from '../lib/domain'

  // The desktop has no name for its list screen: its main column is titled by
  // the current view's name. The phone adopts that — the tab is the object
  // (Issues), the heading is the current scope, and the heading is the
  // control that changes it (DESIGN.md §2, GDK-885).
  let pickerOpen = $state(false)

  /* ── Create sheet (GDK-1497 A2). This screen owns its own writes-off
   *  state — Detail's is per-screen by design, and no global store. The
   *  sheet stays readable when writes are off; only its action recedes. */
  let createOpen = $state(false)
  let createSummary = $state('')
  let createDesc = $state('')
  let createProject = $state('')
  let projects = $state<CreateMetaProject[]>([])
  let metaLoaded = $state(false)
  let createWritesOff = $state(false)
  let writesOffSentence = $state<string | null>(null)
  let createError = $state<string | null>(null)
  let creating = $state(false)

  const scopes = $derived(buildScopes(app.views, app.sources, app.me, app.pages))
  const scope = $derived<Scope>(
    resolveScope(scopes, app.scopeId) ?? {
      id: SCOPE_ALL_OPEN,
      section: 'builtin',
      kind: 'issues',
      name: t('view.allOpen.name'),
      filters: null,
      unsupported: [],
    },
  )
  const isDocs = $derived(scope.kind === 'pages')
  const docRows = $derived(isDocs ? scopePages(app.pages, scope) : [])
  const view = $derived(isDocs
    ? { sections: [], total: docRows.length, scopeId: scope.id, fellBack: false }
    : buildList(app.issues, app.me, scope))
  // The heading must never wear a name the list is not showing: when the
  // fallback fires it says All open, and the note below says why.
  const heading = $derived(view.fellBack ? t('view.allOpen.name') : scope.name)

  // GDK-886: counts are one pass per row, taken when the sheet opens — never
  // on the list's scroll path.
  let counts = $state(new Map<string, number | null>())
  function openPicker(): void {
    counts = new Map(scopes.map((s) => [s.id, scopeCount(app.issues, app.me, s, app.pages)]))
    pickerOpen = true
  }
  function pick(id: string): void {
    setScope(id)
    pickerOpen = false
  }

  /** Projects the sheet may file under — subtask-only projects cannot take
   *  a top-level create, and the phone never asks for an issue type (the
   *  server resolves the default). */
  const creatableProjects = $derived(
    projects.filter((p) => (p.issue_types ?? []).some((ty) => !ty.subtask)),
  )

  function openCreate(): void {
    createOpen = true
    createError = null
    if (metaLoaded || createWritesOff) return
    void (async () => {
      try {
        const res = await getCreateMeta()
        projects = res.projects
        const list = creatableProjects
        if (list.length > 1) createProject = list[0].key
        metaLoaded = true
      } catch (err) {
        if (err instanceof ApiError && err.code === 'credential_required') {
          // The same sentence the Detail composer shows; the sheet stays
          // readable — the person can still read what they meant to file.
          createWritesOff = true
          writesOffSentence = errorMessage(err)
          return
        }
        // A serve whose catalog cannot be read still creates: with one
        // project (the common case) there is nothing to ask.
        metaLoaded = true
      }
    })()
  }

  async function createTheIssue(): Promise<void> {
    const summary = createSummary.trim()
    if (summary === '' || creating || createWritesOff) return
    creating = true
    createError = null
    try {
      const res = await createIssue({
        summary,
        ...(createDesc.trim() !== '' ? { description_text: createDesc } : {}),
        ...(createProject !== '' ? { project_key: createProject } : {}),
      })
      createOpen = false
      createSummary = ''
      createDesc = ''
      createProject = creatableProjects[0]?.key ?? ''
      void sync()
      openIssue(res.issue.issue_key)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'credential_required') {
        createWritesOff = true
        writesOffSentence = errorMessage(err)
        return
      }
      createError = errorMessage(err)
    } finally {
      creating = false
    }
  }

  /*
   * The session strip (GDK-1495 ①): one quiet line saying what changed since
   * the previous session — the first thing above the list, before the glance
   * strip, because it is the reading the return itself is for. Everything it
   * decides lives in the store (the latch and its snapshot) and in the
   * desktop's own rules; this is the line and the tap.
   *
   * Absence is the design: no boundary, no changes, or dismissed → nothing
   * renders and there is no empty state. The tap dismisses. The desk's strip
   * also *arranges* — it turns the changed keys into a view — but the phone
   * has no keys view to turn them into yet, and a control that pretends to
   * one would be the lie the picker's disabled rows exist to avoid.
   *
   * It may run to two lines on a 402px phone rather than truncate its own
   * second half; see the .session rule below.
   */
  const sessionText = $derived(
    app.session.delta && !app.session.dismissed && app.session.boundary
      ? sessionLine(app.session.delta, relTime(app.session.boundary, app.now), app.me)
      : '',
  )

  const syncLabel = $derived(
    app.syncing ? 'syncing' : app.lastSyncAt ? relTime(app.lastSyncAt.toISOString(), app.now) : '—',
  )
  const bootKind = $derived(
    issuesBootKind({
      loaded: app.loaded,
      offline: app.offline,
      issueCount: app.issues.length,
      pageCount: app.pages.length,
      lastSyncAt: app.lastSyncAt,
    }),
  )
  const offlineBanner = $derived(
    showOfflineBanner({
      offline: app.offline,
      issueCount: app.issues.length,
      pageCount: app.pages.length,
      lastSyncAt: app.lastSyncAt,
    }),
  )
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <h1>
        <button class="scope" onclick={openPicker} aria-haspopup="dialog" aria-expanded={pickerOpen}>
          <span class="name type-subject">{heading}</span>
          <span class="count">·{view.total}</span>
          <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </button>
      </h1>
      <span class="spacer"></span>
      <button
        class="new"
        onclick={openCreate}
        aria-label={t('write.newIssue')}
        aria-haspopup="dialog"
        aria-expanded={createOpen}
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M5 12h14" />
          <path d="M12 5v14" />
        </svg>
      </button>
      <button class="fresh" onclick={() => void sync()} aria-label={t('sidebar.syncNow')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class:spin={app.syncing} aria-hidden="true">
          <path d="M21 12a9 9 0 1 1-2.6-6.3" /><path d="M21 3v6h-6" />
        </svg>
        <span>{syncLabel}</span>
      </button>
    </div>
    {#if offlineBanner}
      <p class="offline">{t('app.offlineBanner')}</p>
    {:else if view.fellBack && hasIdentity(app.me) && scope.id === SCOPE_ME}
      <p class="note">Nothing open is assigned to you.</p>
    {:else if view.fellBack}
      <p class="note">This serve has no identity to filter by.</p>
    {/if}
  {/snippet}

  {#if sessionText}
    <button class="session" data-testid="session-strip" onclick={dismissSessionStrip}>
      {sessionText}
    </button>
  {/if}

  <!-- GDK-871: the glance strip — first band under the heading, above every
       plate, scope-independent (the feed is a person's, not a scope's). It
       gates itself on unread counts and renders nothing otherwise. -->
  <GlanceStrip />

  {#if bootKind === 'skeleton'}
    <Skeleton />
  {:else if bootKind === 'failed'}
    <EmptyState title={t('list.renderFailedTitle')}>
      <button class="link" onclick={() => void sync()}>{t('list.renderFailedRetry')}</button>
    </EmptyState>
  {:else if isDocs}
    {#if docRows.length === 0}
      <EmptyState title={t('docs.recentEmpty')} />
    {:else}
      {#each docRows as page (page.key)}
        <DocRow
          {page}
          showSpace={!scope.spaceKey}
          showExcerpt={scope.id === SCOPE_DOCS_UPDATED}
        />
      {/each}
      <div class="foot" aria-hidden="true"></div>
    {/if}
  {:else if view.total === 0}
    <EmptyState
      title={app.issues.length === 0 ? t('list.emptyTitle') : t('list.noMatchTitle')}
      body={app.issues.length === 0 ? t('list.emptyHint') : t('list.noMatchHint')}
    >
      <button class="link" onclick={() => switchTab('search')}>Search everything</button>
    </EmptyState>
  {:else}
    {#each view.sections as section (section.rank)}
      <div class="section">
        <span class="label">{section.label}</span>
        <span class="n">{section.issues.length}</span>
      </div>
      {#each section.issues as issue (issue.issue_key)}
        <Row {issue} showAssignee={view.scopeId !== SCOPE_ME} />
      {/each}
    {/each}
    <div class="foot" aria-hidden="true"></div>
  {/if}
</Screen>

{#if pickerOpen}
  <ScopeSheet
    {scopes}
    {counts}
    current={scope.id}
    onpick={pick}
    onclose={() => (pickerOpen = false)}
  />
{/if}

{#if createOpen}
  <!-- The project question appears only when the serve offers more than
       one project — one-project serves (and the fixture) file into the
       default without asking, and the type is always the server's default. -->
  <Sheet title={t('write.newIssue')} onclose={() => (createOpen = false)}>
    <div class="create">
      {#if writesOffSentence}
        <p class="off-note">{writesOffSentence}</p>
      {/if}
      <label class="lbl" for="create-summary">{t('write.issueTitle')}</label>
      <input
        id="create-summary"
        bind:value={createSummary}
        placeholder={t('write.issueTitle')}
        enterkeyhint="next"
        disabled={createWritesOff}
      />
      <label class="lbl" for="create-desc">{t('detail.description')}</label>
      <textarea
        id="create-desc"
        bind:value={createDesc}
        placeholder={t('write.descriptionPlain')}
        disabled={createWritesOff}
      ></textarea>
      {#if metaLoaded && creatableProjects.length > 1}
        <label class="lbl" for="create-project">{t('common.project')}</label>
        <select id="create-project" bind:value={createProject} disabled={createWritesOff}>
          {#each creatableProjects as p (p.key)}
            <option value={p.key}>{p.key} · {p.name}</option>
          {/each}
        </select>
      {/if}
      <button
        class="go"
        disabled={creating || createSummary.trim() === '' || createWritesOff}
        onclick={() => void createTheIssue()}
      >
        {creating ? 'Creating…' : t('write.newIssue')}
      </button>
      {#if createError}
        <p class="err">{createError}</p>
      {/if}
    </div>
  </Sheet>
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    gap: 6px;
    /* The heading is a 44pt control now, so the padding that used to carry
       the header's height moved into the button itself (§3.3 still needs
       12+ rows below it). */
    padding: 4px 0;
    min-width: 0;
  }
  h1 {
    margin: 0;
    min-width: 0;
  }
  .scope {
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 0;
    min-width: 0;
    max-width: 100%;
    color: var(--color-text-primary);
  }
  .name {
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .count {
    flex: none;
    font-family: var(--font-mono);
    font-size: var(--text-title);
    color: var(--color-text-muted);
  }
  .chev {
    flex: none;
    align-self: center;
    width: 16px;
    height: 16px;
    color: var(--color-text-muted);
  }
  .spacer {
    flex: 1 1 auto;
  }
  .fresh {
    align-self: center;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 0 4px;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
    font-variant-numeric: tabular-nums;
  }
  .fresh svg {
    width: 14px;
    height: 14px;
  }
  /* The create action (GDK-1497 A2): a 44pt square beside the sync state,
     drawn heavier than .fresh because it acts on the tracker, not the
     mirror. */
  .new {
    flex: none;
    align-self: center;
    width: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-text-primary);
  }
  .new svg {
    width: 20px;
    height: 20px;
  }

  .create {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 4px 16px 16px;
  }
  .lbl {
    margin: 6px 0 0;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .create input,
  .create textarea,
  .create select {
    min-height: var(--spacing-control);
    padding: 6px 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font: inherit;
  }
  .create textarea {
    min-height: 96px;
  }
  .create input:disabled,
  .create textarea:disabled,
  .create select:disabled {
    opacity: 0.45;
  }
  .go {
    min-height: var(--spacing-control);
    margin-top: 8px;
    padding: 0 16px;
    border-radius: 6px;
    font-weight: 600;
    background: var(--color-accent);
    color: var(--color-bg-base);
  }
  .go:disabled {
    opacity: 0.45;
  }
  .err {
    margin: 6px 0 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  .off-note {
    margin: 2px 0 0;
    font-size: var(--text-micro);
    color: var(--color-status-stale);
  }
  .fresh svg.spin {
    animation: spin 1.2s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .offline,
  .note {
    margin: 0 0 8px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .offline {
    color: var(--color-status-stale);
  }
  .section {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 10px 16px 4px;
    background: var(--color-bg-base);
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .label {
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .n {
    font-family: var(--font-mono);
  }
  .link {
    color: var(--color-accent-text);
    font-size: var(--text-body);
    min-height: var(--spacing-control);
    padding: 0 16px;
  }
  /* Two lines at most, one whenever it fits (vision FIX 2026-09-07). The
     desk's one-line rule was written for a panel three times this wide; on
     402px it cut "1 of them assigned to you" off at the ellipsis, losing the
     most specific fact in the sentence. A block above the list still reads
     as chrome — so the clamp is 2, not none. */
  .session {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    width: 100%;
    padding: 6px 16px;
    text-align: left;
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    overflow: hidden;
  }
  .session:active {
    background: var(--color-bg-hover);
  }
  .foot {
    height: 24px;
  }
</style>
