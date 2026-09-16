<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Row from '../ui/Row.svelte'
  import DocRow from '../ui/DocRow.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import GlanceStrip from '../ui/GlanceStrip.svelte'
  import Skeleton from '../ui/Skeleton.svelte'
  import Palette from '../ui/Palette.svelte'
  import CreateSheet from '../ui/CreateSheet.svelte'
  import SprintLine from '../ui/SprintLine.svelte'
  import { t } from '../lib/i18n'
  import {
    app,
    closePalette,
    dismissSessionStrip,
    issuesBootKind,
    openPalette,
    openSettings,
    setScope,
    showOfflineBanner,
    sync,
  } from '../lib/store.svelte'
  import { tick } from 'svelte'
  import {
    buildList,
    buildScopes,
    hasIdentity,
    relTime,
    resolveScope,
    scopeCount,
    scopePages,
    sessionLine,
    SCOPE_ACTIVE_SPRINT,
    SCOPE_ALL_OPEN,
    SCOPE_MY_WORK,
    type Scope,
  } from '../lib/domain'
  import { pickActiveSprint } from '../lib/sprint'

  // The desktop has no name for its list screen: its main column is titled by
  // the current view's name. The phone adopts that — the heading is the
  // current owner's name, and the heading is the control that changes it
  // (DESIGN.md §2, GDK-885/GDK-902).
  //
  // The open flag lives in the store, not here: a deep link has to close the
  // palette (App.svelte's router), system back has to close it in order
  // behind the detail and the settings layer, and a test has to be able to
  // read it. This screen owns only the scroll position it lends out.
  let scroller = $state<HTMLElement | null>(null)
  let savedScroll: number | null = null

  /*
   * "The list keeps its scroll position across a palette open-and-cancel"
   * (DESIGN.md §2). One effect closes both roads out — the Cancel control
   * and system back — because both end at app.palette going false; a
   * restore hung off the Cancel handler would leave back landing at the
   * top. Picking a scope clears the memory first, so that road scrolls to
   * the top instead, which is what a new owner wants.
   */
  $effect(() => {
    if (app.palette) return
    const target = savedScroll
    savedScroll = null
    const el = scroller
    if (!el || target === null) return
    void tick().then(() => {
      el.scrollTop = target
    })
  })

  /* ── Create sheet (GDK-1497 A2), a component of its own since GDK-1871:
   *  an epic's detail opens the same sheet to file a child, so this screen
   *  owns only the flag that says it is open. It is mounted inside the
   *  `{#if}` below, which is what makes each open a fresh restore of the
   *  drafts it holds (no $effect watching a prop — GDK-692). */
  let createOpen = $state(false)

  /*
   * The sprint line's subject (GDK-1867), and the scope row that goes with
   * it — one derivation, so the line and the picker row can never name two
   * different sprints. Null on a kanban workspace, between sprints, or on a
   * serve that has no `issues/sprints/` route: then neither exists.
   */
  const activeSprint = $derived(pickActiveSprint(app.sprints, app.issues))
  const scopes = $derived(buildScopes(app.views, app.sources, app.me, app.pages, activeSprint))
  const scope = $derived<Scope>(
    resolveScope(scopes, app.scopeId, app.me) ?? {
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

  // GDK-886: counts are one pass per row, taken when the palette opens —
  // never on the list's scroll path.
  let counts = $state(new Map<string, number | null>())
  function showPalette(): void {
    counts = new Map(scopes.map((s) => [s.id, scopeCount(app.issues, app.me, s, app.pages)]))
    savedScroll = scroller?.scrollTop ?? 0
    openPalette()
  }
  function pick(id: string): void {
    setScope(id)
    // A new owner draws from the top; the remembered position belonged to
    // the owner being left.
    savedScroll = null
    closePalette()
    if (scroller) scroller.scrollTop = 0
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

<Screen bind:scroller>
  {#snippet header()}
    <div class="head">
      <h1>
        <button class="scope" onclick={showPalette} aria-expanded={app.palette}>
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
        onclick={() => (createOpen = true)}
        aria-label={t('write.newIssue')}
        aria-haspopup="dialog"
        aria-expanded={createOpen}
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M5 12h14" />
          <path d="M12 5v14" />
        </svg>
      </button>
      <!-- Settings is a push layer, not an owner (DESIGN.md §2), and the
           gear is its only door. The offline dot moved here from the tab
           that used to carry it: the honest place for "is this thing still
           connected" is the control that opens the screen which answers.
           The label is the heading behind the door, not a second word for
           the same screen (§3.6). -->
      <button class="gear" onclick={openSettings} aria-label={t('settings.title')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6 1.65 1.65 0 0 0 10 3.09V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
        </svg>
        {#if app.offline}
          <span class="dot" aria-label={t('app.offline')}></span>
        {/if}
      </button>
      <button class="fresh" onclick={() => void sync()} aria-label={t('sync.now')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class:spin={app.syncing} aria-hidden="true">
          <path d="M21 12a9 9 0 1 1-2.6-6.3" /><path d="M21 3v6h-6" />
        </svg>
        <span>{syncLabel}</span>
      </button>
    </div>
    {#if offlineBanner}
      <p class="offline">{t('app.offlineBanner')}</p>
    {:else if view.fellBack && hasIdentity(app.me) && scope.id === SCOPE_MY_WORK}
      <p class="note">{t('list.nothingOpenAssigned')}</p>
    {:else if view.fellBack}
      <p class="note">{t('list.noIdentityFilter')}</p>
    {/if}
  {/snippet}

  {#if app.palette}
    <!-- The body IS the palette (DESIGN.md §2): switching owners is not a
         screen change but a change in what the list shows. The bands above
         the rows go with the rows — a glance strip over a search ranking
         describes a list that is not on screen. -->
    <Palette {scopes} {counts} current={scope.id} onpickScope={pick} />
  {:else}
  {#if sessionText}
    <button class="session" data-testid="session-strip" onclick={dismissSessionStrip}>
      {sessionText}
    </button>
  {/if}

  <!-- GDK-1867: the current sprint as one line, under the heading and above
       the queue it describes. Inside the scroller, like every band here —
       a band in the header would cost the list a row of density
       (mobile/e2e/viewport.spec.ts floors it at 9). Tapping it scopes the
       list to that sprint; absent when there is no active one. -->
  {#if !isDocs}
    <!-- Issue data: under a documents scope the line described a list that
         was not on screen (review 2026-09-14, capture 06). -->
    <SprintLine
      sprint={activeSprint}
      issues={app.issues}
      now={app.now}
      current={scope.id === SCOPE_ACTIVE_SPRINT}
      onpick={() => setScope(SCOPE_ACTIVE_SPRINT)}
    />
  {/if}

  <!-- GDK-871: the glance strip — the last band before the plates, and the
       only scope-independent one (the feed is a person's, not a scope's).
       It gates itself on unread counts and renders nothing otherwise. -->
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
        <DocRow {page} showSpace={!scope.spaceKey} />
      {/each}
      <div class="foot" aria-hidden="true"></div>
    {/if}
  {:else if view.total === 0}
    <EmptyState
      title={app.issues.length === 0 ? t('list.emptyTitle') : t('list.noMatchTitle')}
      body={app.issues.length === 0 ? t('list.emptyHint') : t('list.noMatchHint')}
    >
      <button class="link" onclick={showPalette}>{t('palette.entryLabel')}</button>
    </EmptyState>
  {:else}
    {#each view.sections as section (section.rank)}
      <div class="section">
        <span class="label">{section.label}</span>
        <span class="n">{section.issues.length}</span>
      </div>
      {#each section.issues as issue (issue.issue_key)}
        <Row {issue} showAssignee={view.scopeId !== SCOPE_MY_WORK} />
      {/each}
    {/each}
    <div class="foot" aria-hidden="true"></div>
  {/if}
  {/if}
</Screen>

{#if createOpen}
  <CreateSheet open={createOpen} onclose={() => (createOpen = false)} />
{/if}

<style>
  .head {
    display: flex;
    /* GDK-1936 (measured at 402px, all-open scope): the ja row's natural
       width is ~423px against 370px of line, and the shrinkable stamp paid
       for the overflow by folding its text while the name took the
       ellipsis (すべて…, scrollWidth 177 > 124). Wrapping the row moves only
       the overflowing locale's stamp to the header's second line — line 1
       without it is 357px, so the name keeps its full width; en (348px) and
       ko (352px) never reach the break and stay single-line as they were. */
    flex-wrap: wrap;
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
    /* GDK-1936: the stamp is never the element that folds. One breath
       (nowrap), no width traded for the heading's (flex:none — its old
       default shrink is what bent the text into two lines), and it keeps
       its right-edge stance on whichever line it lands. */
    flex: none;
    white-space: nowrap;
    margin-left: auto;
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
  /* Same 44pt square as the create action, drawn in the muted weight the
     sync state wears: it opens a screen, it does not act on the tracker. */
  .gear {
    position: relative;
    flex: none;
    align-self: center;
    width: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-text-muted);
  }
  .gear svg {
    width: 19px;
    height: 19px;
  }
  /* The offline dot, moved from the tab bar with its shape intact: a square
     with a slash, so the state is never color-only. */
  .dot {
    position: absolute;
    top: 6px;
    right: 4px;
    width: 7px;
    height: 7px;
    border-radius: 1px;
    background: var(--color-status-stale);
    box-shadow: 0 0 0 1px var(--color-text-primary);
  }
  .dot::after {
    content: '';
    position: absolute;
    left: -1px;
    right: -1px;
    top: 50%;
    height: 1.5px;
    margin-top: -0.75px;
    background: var(--color-text-primary);
    transform: rotate(-45deg);
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
