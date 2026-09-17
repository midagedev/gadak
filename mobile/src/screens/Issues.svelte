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
    allOpenOrder,
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
  import { fitHeading } from '../lib/fit-heading'

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
      order: allOpenOrder(),
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
  /*
   * The door pays its toll before it opens (GDK-886/GDK-902): the counts
   * the owner list shows and the scroll position Cancel restores.
   *
   * Two doors, and they land differently (GDK-1990, re-judging GDK-1985's
   * one): the heading opens the owner list with the keyboard down, the
   * header's magnifier opens the same body with the field focused. Both
   * toggle (GDK-1984) — a second tap closes what the first opened, so
   * aria-expanded is true in more than name. The heading wears a chevron
   * again, because a magnifier in two places would be one glyph meaning
   * two things; what it did not have in GDK-1974 is the rule under it
   * (GDK-1989), which is why the chevron went unseen then.
   */
  function preparePalette(focus: boolean): void {
    counts = new Map(scopes.map((s) => [s.id, scopeCount(app.issues, app.me, s, app.pages)]))
    savedScroll = scroller?.scrollTop ?? 0
    openPalette(focus)
  }
  /** The heading's tap: the owner list with the keyboard down — or, already
   *  open, the close that reopens the list (GDK-1984). */
  function showPalette(): void {
    if (app.palette) {
      closePalette()
      return
    }
    preparePalette(false)
  }
  /** The header magnifier's tap: the same body, ready to type. */
  function showSearch(): void {
    if (app.palette) {
      closePalette()
      return
    }
    preparePalette(true)
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
    <div class="head" use:fitHeading>
      <h1>
        <button class="scope" onclick={showPalette} aria-expanded={app.palette}>
          <span class="name type-subject">{heading}</span>
          <span class="count">·{view.total}</span>
          <svg class="glass" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </button>
      </h1>
      <div class="actions">
      <!-- GDK-1990: search has a control of its own, in the slot the refresh
           glyph gave up. A fourth 44px control would have cost the heading
           46px, and that measured as every ja view name dropping a size step
           and three of five losing their count. Manual sync keeps its door
           in Settings (`sync.now` there), the app syncs on its own, and the
           list's own render-failure retry is untouched. -->
      <button class="search" onclick={showSearch} aria-expanded={app.palette} aria-label={t('palette.entryLabel')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
          <circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
        </svg>
      </button>
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
      </div>
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
    align-items: center;
    gap: 6px;
    /* The heading is a 44pt control now, so the padding that used to carry
       the header's height moved into the button itself (§3.3 still needs
       12+ rows below it). */
    padding: 4px 0;
    min-width: 0;
  }
  /* GDK-1989: the heading takes the room the row has left, so a long ko/ja
     name steps down later than it used to — the 73px a `.spacer` element
     held was space the name could not reach, because the door was sized by
     its content and nothing let it grow. */
  h1 {
    margin: 0;
    flex: 1 1 auto;
    min-width: 0;
  }
  /* The door draws a surface (GDK-1989, the third pass on GDK-1974/1985).
     Two rounds put a mark ON the door — a chevron, then the magnifier — and
     neither made the door look like one: a 163x44 button with a transparent
     background, no border and no shadow, whose only signal was a 17px glyph
     in the same muted ink as the passive count beside it. A rule under the
     text is the one surface that says "control" without saying "field":
     the heading stays a heading (DESIGN.md §2, GDK-885), and `max-content`
     keeps the rule exactly as wide as the name, so it never reads as the
     header's own divider.

     The rule is `--color-border-strong`, not `--color-border-subtle`, and
     that is the whole point of it (vision verdict, 2026-09-18): at subtle
     the rule was pixel-identical to the row separators stacked below it —
     same rgb(213,201,178), 1.43:1 on the header ground — so it read as a
     divider that stopped early rather than as a control. Strong measures
     1.97:1 in light and 2.21:1 in dark. The ceiling is 3:1: past that it
     starts reading as a text input's underline, which is the failure the
     `max-content` width and the heading's own weight exist to avoid. */
  .scope {
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 0;
    width: max-content;
    min-width: 0;
    max-width: 100%;
    color: var(--color-text-primary);
    border-bottom: 1px solid var(--color-border-strong);
  }
  .scope[aria-expanded='true'] {
    border-bottom-color: var(--color-text-primary);
  }
  .name {
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* The name keeps its words and gives up size first (GDK-1974): a language
     whose view name would not fit whole steps down one size at a time, the
     step chosen after layout by lib/fit-heading (`data-fit` on .head). Only
     past the last step does the ellipsis above become the answer. */
  .head:global([data-fit='1']) .name {
    /* midpoint between --text-heading and --text-title; a step, not a new type size (GDK-1974) */
    font-size: 22px;
    line-height: 1.25;
  }
  .head:global([data-fit='2']) .name,
  .head:global([data-fit='3']) .name {
    font-size: var(--text-title);
    line-height: var(--text-title--line-height);
  }
  /* The last step: 19px still could not hold the words, so the count is
     what gives (the palette rows carry every count; GDK-1974, 2026-09-17). */
  .head:global([data-fit='3']) .count {
    display: none;
  }
  .count {
    flex: none;
    font-family: var(--font-mono);
    /* GDK-1974 (2026-09-17): one size down — the name pays for a third
       header control otherwise. */
    font-size: var(--text-body);
    color: var(--color-text-muted);
  }
  /* The heading's glyph is the magnifier the palette's own field wears
     (ui/Palette.svelte) — one door carrying the mark of what it opens, so
     the screen says search without a second 44pt control (GDK-1985,
     superseding GDK-1974's chevron-plus-header-magnifier pair). */
  .glass {
    flex: none;
    align-self: center;
    width: 19px;
    height: 19px;
    /* GDK-1989: the heading's ink, not the count's. Muted put the door's
       only mark at the same weight as the number beside it and as the two
       least important controls in the row. */
    color: var(--color-text-primary);
  }
  /* GDK-1989: the three actions are one set — one glyph size, held close,
     and separated from the door by more than they are from each other.
     GDK-1990 made the boxes equal too, by swapping the one narrow control
     out rather than by widening it: a fourth box, or a wider third, is what
     costs the heading a fit step.

     The set closes to a flush row and its margin halves (GDK-1990), and
     that is not taste — it is the 11px the new 44pt box would otherwise
     have taken from the heading. Measured at 402 across the seeded view
     names: at gap 2 / margin 12 the heading's slot is 216 and `Unassigned
     new` falls to the ladder's last step and loses its count; at gap 0 /
     margin 6 it is 226 and every name lands exactly where it landed before
     this control existed. Three 44pt boxes flush against each other still
     read as three — the glyphs are 19px centred in them, so there is 25px
     of air between neighbours — and nothing can be mis-tapped into, because
     there is no dead space between them to miss into. */
  .actions {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0;
    margin-left: 6px;
  }
  /* The search door (GDK-1990), in the slot the refresh glyph gave up. Same
     44pt square as its two neighbours: with the narrow box gone the set is
     three equal ones, so the even glyph rhythm the vision round had to
     hand-correct (a 6px margin before a 27px box) is the geometry's own
     answer now. Muted like the gear — it opens something, it does not act
     on the tracker. The last-sync time the refresh control used to carry
     lives in Settings (`sync.settledOk` there), beside the manual sync. */
  .search {
    flex: none;
    align-self: center;
    width: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-text-muted);
  }
  .search svg {
    width: 19px;
    height: 19px;
  }
  /* The create action (GDK-1497 A2): a 44pt square in the set, drawn
     heavier than its neighbours because it acts on the tracker, not the
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
    width: 19px;
    height: 19px;
  }
  /* Same 44pt square as the create action, drawn in the muted weight the
     search door wears: it opens a screen, it does not act on the tracker. */
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
