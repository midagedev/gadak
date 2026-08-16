<script lang="ts">
  /*
   * Sidebar nav ([explore]). Sections: built-in views / personal (localStorage) /
   * team shared (api). Top: issue totals + last sync. Personalization (Wave 3)
   * sits above built-ins. View click = filters.applyConfig; active when it matches.
   */
  import { t, formatNumber, relativeTime } from '../../lib/i18n'
  import { filterIssues, filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { views } from '../../stores/views.svelte'
  import { me } from '../../stores/me.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { showIssueList } from '../../lib/show-issue-list'
  import { write } from '../../stores/write.svelte'
  import { runSyncNow } from '../../lib/sync-now'
  import { getSyncRuns, getWorkspaces, type SyncRun, type WorkspaceInfo } from '../../lib/api'
  import {
    config,
    hasServerVerb,
    isHostedDemo,
    jiraFilterUrl,
    workspaceName,
  } from '../../lib/config'
  import { busyLabel, fetchingDocuments, mirrorLabel } from '../../lib/mirror-status'
  import { builtinViews } from '../../lib/builtin-views'
  import { configToParams, type ViewConfig } from '../../lib/view-config'
  import MyIssuesNav from '../personal/MyIssuesNav.svelte'
  import FavoritesNav from '../personal/FavoritesNav.svelte'
  import Icon from '../ui/Icon.svelte'

  /** Open server settings dialog — App.svelte mounts the dialog itself. */
  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  const builtins = builtinViews()

  /** Apply view = give ListView the column, then the filters. */
  function applyView(config: ViewConfig) {
    showIssueList(config)
  }

  function applySource(v: (typeof views.source)[number]) {
    applyView(v.config)
    if (v.unsupported?.length) {
      write.toast(t('filter.jqlPartial', { clauses: v.unsupported.join('; ') }), 'info')
    }
  }

  /** Order-independent canonical string for comparing configs. */
  function canon(config: ViewConfig): string {
    const p = configToParams(config)
    return Object.keys(p)
      .sort()
      .map((k) => `${k}=${p[k] ?? ''}`)
      .join('&')
  }

  const currentCanon = $derived(canon(filters.currentConfig()))

  function activeId(views: { id: string; config: ViewConfig }[]): string | null {
    for (const v of views) if (canon(v.config) === currentCanon) return v.id
    return null
  }
  const activeBuiltin = $derived(activeId(builtins))
  const activePersonal = $derived(activeId(views.personal))
  const activeTeam = $derived(activeId(views.team))
  const activeSource = $derived(activeId(views.source))
  const builtinCounts = $derived.by(() => {
    const counts = new Map<string, number>()
    for (const view of builtins) {
      counts.set(view.id, filterIssues(issues.allIssues, view.config.filters).length)
    }
    return counts
  })

  const STATUS_LABEL: Record<string, string> = {
    healthy: t('sidebar.syncOk'),
    running: t('sidebar.syncing'),
    paused: t('sidebar.syncOffHours'),
    idle: t('sidebar.syncWaiting'),
    stale: t('sidebar.syncDelayed'),
    failed: t('sidebar.syncFailed'),
    missing: t('sidebar.syncNoRecord'),
  }

  function relativeSync(value: string | null): string {
    return relativeTime(value, 'long')
  }

  const syncTitle = $derived.by(() =>
    issues.syncHealth?.sources
      .map((source) => {
        const time = relativeSync(source.synced_at)
        // source.message is server-locale text; '정상' is a healthy-message fallback.
        return `${source.label} · ${STATUS_LABEL[source.status] ?? source.status}${time ? ` · ${time}` : ''}${source.message && source.message !== '정상' && source.message.toLowerCase() !== 'ok' ? `\n${source.message}` : ''}`
      })
      .join('\n'),
  )
  // The mirror's one line, running or at rest. This row used to report "Synced
  // 18:12" from the browser↔server delta cursor while the chip reported the
  // mirror's own age — two different facts sharing a verb, and only the second
  // is what anyone means by synced.
  const syncLabel = $derived(mirrorLabel())
  const syncColor = $derived(
    issues.syncHealth?.overall === 'failed'
      ? 'text-status-reopen'
      : issues.syncHealth?.overall === 'warning'
        ? 'text-status-stale'
        : 'text-text-muted',
  )
  const syncDot = $derived(
    issues.syncHealth?.overall === 'failed'
      ? 'bg-status-reopen'
      : issues.syncHealth?.overall === 'warning'
        ? 'bg-status-stale'
        : 'bg-status-done',
  )

  /* ── Sync history popover (click on the sync timestamp) ── */
  let historyOpen = $state(false)
  let historyRuns = $state<SyncRun[]>([])
  let historyLoading = $state(false)
  let historyEl = $state<HTMLDivElement | null>(null)

  async function toggleHistory() {
    if (historyOpen) {
      historyOpen = false
      return
    }
    historyOpen = true
    historyLoading = true
    historyRuns = await getSyncRuns()
    historyLoading = false
  }

  function onDocClick(e: MouseEvent) {
    if (historyOpen && historyEl && !e.composedPath().includes(historyEl)) historyOpen = false
  }

  function runKindLabel(kind: string): string {
    const base = kind.startsWith('full') ? t('sidebar.runFull') : t('sidebar.runIncremental')
    return kind.includes('+reconcile') ? `${base} ${t('sidebar.runReconcile')}` : base
  }

  /* ── Workspaces (one process, several profile mirrors) ── */
  let workspaceList = $state<WorkspaceInfo[]>([])
  $effect(() => {
    void getWorkspaces().then((list) => (workspaceList = list))
  })
  /** The mirror this page is looking at: URL mount, or the serve primary. */
  const currentWorkspace = $derived(
    workspaceName() || workspaceList.find((w) => w.active)?.name || 'default',
  )
  function workspaceHref(w: WorkspaceInfo): string {
    return w.active ? '/' : `/w/${w.name}/`
  }
  function workspaceHost(w: WorkspaceInfo): string {
    if (!w.site) return ''
    try {
      return new URL(w.site).host
    } catch {
      return w.site
    }
  }

  /* ── Docs (mirrored wiki pages) ── */
  //  The nav carries two entries only — the document view, and a collapsed
  //  disclosure of the spaces. No tool lists every container by default, and a
  //  sidebar must not grow with content volume (UX_PRINCIPLES §6); the page
  //  tree lives on the space screen, where it is asked for.
  /*
   * The disclosure starts open when the app is already standing in a space, and
   * opens whenever it arrives in one from somewhere that is not this list — a
   * restored URL, a pasted link, the back button. Those never pass through the
   * row that would have opened it, and a highlight the nav is drawing inside a
   * collapsed section is a highlight nobody can see: the same address rendered
   * two different sidebars depending on how it was reached.
   *
   * Local state with an effect, not a derivation from `pages.spaceView`: this
   * stays something a person can collapse while still reading a space, which a
   * derivation would take away. Nothing new is stored — the store already knows
   * which space is open, and this only reads it.
   */
  let spacesOpen = $state(pages.spaceView !== null)
  $effect(() => {
    if (pages.spaceView !== null) spacesOpen = true
  })

  /*
   * An empty DOCS section has five causes, and telling someone to go connect a
   * source is right for exactly one of them. It was wrong for the user who had
   * just chosen a space and saved: the app kept asking for what it already had.
   *
   * unavailable — this deployment has no docs server at all (static snapshot).
   * off      — Confluence is not configured. The CTA belongs here and only here.
   * syncing  — a pull is running. Status, not an errand.
   * failed   — the last Confluence pass errored. Say so; the reason is in the title.
   * never    — configured, nothing fetched yet. The one state with an action here.
   * empty    — fetched fine, and the chosen spaces hold nothing. Blame the selection.
   */
  type DocsEmptyState = 'off' | 'syncing' | 'failed' | 'never' | 'empty' | 'unavailable'

  let confluenceRuns = $state<SyncRun[] | null>(null)
  const docsConfigured = $derived(config().confluenceEnabled)

  // Only ask when the answer changes what the section says: configured, holding
  // no pages, and nothing in flight. Re-runs after a pull because mirrorSyncing
  // is read here, which is what refreshes a failure into a success.
  $effect(() => {
    if (!docsConfigured || pages.bySpace.length > 0 || fetchingDocuments()) return
    void getSyncRuns('confluence').then((runs) => (confluenceRuns = runs))
  })

  const docsEmptyState = $derived.by((): DocsEmptyState => {
    // First question, before "is it configured": does this deployment have a
    // docs server to configure at all. A static snapshot is not "off" — there
    // is no Settings screen that could switch it on.
    if (!hasServerVerb('docs')) return 'unavailable'
    if (!docsConfigured) return 'off'
    // Only while the mirror is fetching *documents*. An issue pass is the sync
    // row's business, not this section's — that split is what made one mirror
    // look like two.
    if (fetchingDocuments()) return 'syncing'
    if (confluenceRuns === null) return 'never' // not asked yet — never claim failure
    if (confluenceRuns[0]?.error) return 'failed'
    if (confluenceRuns.length === 0) return 'never'
    return 'empty'
  })

  const docsEmptyText = $derived.by(() => {
    switch (docsEmptyState) {
      case 'unavailable':
        return { title: t('sidebar.docsUnavailable'), hint: '' }
      case 'syncing':
        // Literally the sync row's string, not a paraphrase of it: two places
        // rendering one sentence cannot end up telling different stories.
        return { title: busyLabel() ?? t('sidebar.docsSyncing'), hint: '' }
      case 'failed':
        return { title: t('sidebar.docsFetchFailed'), hint: t('sidebar.docsFetchFailedHint') }
      case 'never':
        return { title: t('sidebar.docsNotFetched'), hint: t('sidebar.docsNotFetchedHint') }
      case 'empty':
        return { title: t('sidebar.docsEmptySpaces'), hint: t('sidebar.docsEmptySpacesHint') }
      default:
        return { title: t('sidebar.docsNoneTitle'), hint: t('sidebar.docsNoneHint') }
    }
  })

  /** 'never' is the one state the user can act on without leaving the sidebar. */
  function onDocsEmptyClick() {
    if (docsEmptyState === 'unavailable') return
    if (docsEmptyState === 'syncing') return
    if (docsEmptyState === 'never') {
      void issues.pullMirror('full')
      return
    }
    onOpenSettings()
  }

  /** A docs surface takes over the main column, so the feed must give it up. */
  function openDocuments() {
    me.closeFeed()
    pages.toggleDocs()
  }

  function openSpace(space: string) {
    me.closeFeed()
    pages.openSpace(space)
  }
</script>

<svelte:document onclick={onDocClick} />

<!-- The label above a group of rows. Every group in the rail uses it, so the
     one thing that separates a section from a row stays one thing. -->
{#snippet sectionHeader(label: string)}
  <div class="px-3 py-1 text-micro font-medium uppercase tracking-wide text-text-muted">
    {label}
  </div>
{/snippet}

<!-- A saved view's row. A personal view and a team view are the same row: the
     team one only adds who shared it (in the label and the tooltip) and asks
     who is allowed to delete it, so both render through here rather than
     through two copies that drift apart.

     `{' '}` rather than a newline before the owner suffix — the space has to
     belong to the branch that draws the suffix, or a personal row would carry a
     trailing one it never had. -->
{#snippet viewRow(row: {
  name: string
  active: boolean
  apply: () => void
  /** Team views only: who shared it. Absent draws no suffix and no tooltip. */
  owner?: string | null
  /** Null when this viewer may not delete it — a team view that is not theirs. */
  remove: (() => void) | null
})}
  <div
    class="group flex h-control items-center gap-2 rounded-md px-3 text-body transition-colors {row.active
      ? 'bg-bg-active'
      : 'hover:bg-bg-hover'}"
  >
    <button
      type="button"
      class="min-w-0 flex-1 truncate text-left {row.active
        ? 'text-text-primary'
        : 'text-text-secondary group-hover:text-text-primary'}"
      onclick={row.apply}
      title={row.owner ? t('sidebar.viewOwner', { name: row.owner }) : undefined}
      >{row.name}{#if row.owner}{' '}<span class="ml-1 text-micro text-text-muted"
          >· {row.owner}</span
        >{/if}</button
    >
    {#if row.remove}
      <button
        type="button"
        class="flex flex-none items-center text-text-muted opacity-0 transition-opacity hover:text-status-reopen group-hover:opacity-100"
        title={t('common.delete')}
        onclick={row.remove}
      >
        <Icon name="x" size={13} />
      </button>
    {/if}
  </div>
{/snippet}

<div class="flex h-full flex-col">
  <!-- New issue (shortcut c). Disabled on the hosted demo, where the snapshot
       service worker answers every write with 501 — offering the button only to
       fail on submit wastes the visitor's time. -->
  <div class="flex-none px-3 pt-1 pb-2">
    <button
      type="button"
      disabled={isHostedDemo()}
      onclick={() => write.openNewIssue()}
      class="flex h-control w-full items-center justify-center gap-1.5 rounded-md bg-accent px-3 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-bg-elevated disabled:text-text-muted disabled:hover:bg-bg-elevated"
      title={isHostedDemo() ? t('app.demoWriteDisabled') : t('sidebar.newIssueTitle')}
    >
      <Icon name="plus" size={13} />
      {t('sidebar.newIssue')}
    </button>
  </div>

  <!-- Update notice: server found a newer published release (daily check). -->
  {#if issues.latestVersion}
    <div class="flex-none px-3 pb-1">
      <a
        href={issues.releaseUrl || 'https://github.com/midagedev/gadak/releases'}
        target="_blank"
        rel="noreferrer"
        class="block rounded-md border border-accent/30 bg-accent-subtle/30 px-2.5 py-1.5 text-micro text-accent-text transition-colors hover:bg-accent-subtle/50"
        data-testid="update-notice"
      >
        {t('sidebar.updateAvailable', { version: issues.latestVersion })}
      </a>
    </div>
  {/if}

  <!-- Totals / sync — badge click = history popover (with Sync now inside) -->
  <div class="flex-none px-3 pb-2 pt-1 text-micro text-text-muted">
    {t('sidebar.issueCount', { n: formatNumber(issues.pool.size) })}
    <span class="ml-1">·</span>
    <div class="relative inline-block" bind:this={historyEl}>
      <button
        type="button"
        class="ml-1 inline-flex items-center gap-1 rounded px-0.5 {syncColor} transition-colors hover:bg-bg-hover hover:text-text-primary"
        title={[syncTitle || syncLabel, t('sidebar.syncHistoryTitle')].filter(Boolean).join('\n')}
        aria-label={t('sidebar.syncHistory')}
        aria-expanded={historyOpen}
        data-testid="sidebar-sync-now"
        onclick={() => void toggleHistory()}
      >
        <span class="h-1.5 w-1.5 flex-none rounded-full {syncDot}" aria-hidden="true"></span>
        {syncLabel}
      </button>
      {#if historyOpen}
        <div
          class="anim-enter absolute left-0 top-full z-40 mt-1 w-72 rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
          data-testid="sync-history-popover"
        >
          <div class="px-2 py-1 text-micro font-medium text-text-muted">
            {t('sidebar.syncHistory')}
          </div>
          {#if historyLoading}
            <div class="px-2 py-2 text-[12px] text-text-muted">{t('common.searching')}</div>
          {:else if historyRuns.length === 0}
            <div class="px-2 py-2 text-[12px] text-text-muted">{t('sidebar.syncNoHistory')}</div>
          {:else}
            <div class="max-h-64 overflow-y-auto">
              {#each historyRuns as run (run.started_at + run.kind)}
                <div class="flex items-start gap-2 rounded px-2 py-1 text-[12px]">
                  <span
                    class="mt-1.5 h-1.5 w-1.5 flex-none rounded-full {run.error
                      ? 'bg-status-reopen'
                      : 'bg-status-done'}"
                    aria-hidden="true"
                  ></span>
                  <div class="min-w-0 flex-1">
                    <div class="flex items-baseline justify-between gap-2">
                      <span class="text-text-secondary">{runKindLabel(run.kind)}</span>
                      <span class="flex-none text-micro text-text-muted" title={run.finished_at}>
                        {relativeTime(run.finished_at, 'long')}
                      </span>
                    </div>
                    {#if run.error}
                      <div class="break-words text-micro text-status-reopen">{run.error}</div>
                    {:else}
                      <div class="text-micro text-text-muted">
                        {t('sidebar.runCounts', {
                          changed: formatNumber(run.changed),
                          deleted: formatNumber(run.deleted),
                        })}
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
          <div class="mt-1 border-t border-border-subtle px-1 pt-1">
            <button
              type="button"
              class="flex h-control-sm w-full items-center rounded px-2 text-left text-[12px] text-accent-text transition-colors hover:bg-bg-hover"
              onclick={() => {
                historyOpen = false
                void runSyncNow('incremental')
              }}
            >
              {t('sidebar.syncNow')}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto">
    <!-- Personalization (Wave 3): My Issues / recent — above built-ins -->
    <MyIssuesNav />
    <FavoritesNav />

    <!-- Built-in views -->
    <div class="mb-3">
      {@render sectionHeader(t('sidebar.builtinViews'))}
      {#each builtins as v (v.id)}
        <button
          type="button"
          class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {activeBuiltin ===
          v.id
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
          title={v.hint}
          onclick={() => applyView(v.config)}
        >
          <!-- The icon is orientation, not content: it stays a tier below the
               label it sits next to, and only rises with the row it marks. -->
          <Icon
            name={v.icon}
            size={15}
            class={activeBuiltin === v.id ? 'text-text-secondary' : 'text-text-muted'}
          />
          <span class="min-w-0 flex-1 truncate">{v.name}</span>
          <span class="flex-none font-mono text-micro tabular-nums text-text-muted">
            {formatNumber(builtinCounts.get(v.id) ?? 0)}
          </span>
        </button>
      {/each}
    </div>

    <!-- Jira saved filters (owned + starred). No delete — Jira is the record. -->
    {#if views.source.length}
      <div class="mb-3" data-testid="sidebar-jira-filters">
        {@render sectionHeader(t('sidebar.jiraFilters'))}
        {#each views.source as v (v.id)}
          {@const href = jiraFilterUrl(v.external_id ?? '', v.jql)}
          <div
            class="group flex h-control items-center gap-2 rounded-md px-3 text-body transition-colors {activeSource ===
            v.id
              ? 'bg-bg-active'
              : 'hover:bg-bg-hover'}"
          >
            <button
              type="button"
              class="min-w-0 flex-1 truncate text-left {activeSource === v.id
                ? 'text-text-primary'
                : 'text-text-secondary group-hover:text-text-primary'}"
              onclick={() => applySource(v)}
              title={v.jql || undefined}
              data-testid="sidebar-jira-filter"
              data-filter-id={v.id}
            >
              {#if v.favourite}<span class="mr-1 text-micro text-text-muted" aria-hidden="true">★</span>{/if}{v.name}
            </button>
            {#if href}
              <a
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                class="flex flex-none items-center text-text-muted opacity-0 transition-opacity hover:text-accent-text group-hover:opacity-100"
                title={t('sidebar.openFilterInJira')}
                aria-label={t('sidebar.openFilterInJira')}
                data-testid="sidebar-jira-filter-open"
                onclick={(e) => e.stopPropagation()}
              >
                <Icon name="arrow-up-right" size={13} />
              </a>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    <!-- Personal views -->
    {#if views.personal.length}
      <div class="mb-3">
        {@render sectionHeader(t('sidebar.myViews'))}
        {#each views.personal as v (v.id)}
          {@render viewRow({
            name: v.name,
            active: activePersonal === v.id,
            apply: () => applyView(v.config),
            remove: () => views.removePersonal(v.id),
          })}
        {/each}
      </div>
    {/if}

    <!-- Team shared views -->
    {#if views.team.length}
      <div class="mb-3">
        {@render sectionHeader(t('sidebar.teamViews'))}
        {#each views.team as v (v.id)}
          {@render viewRow({
            name: v.name,
            active: activeTeam === v.id,
            apply: () => applyView(v.config),
            owner: v.owner_name,
            remove:
              me.email && v.owner_email === me.email
                ? () => views.removeTeam(v.id).catch(() => alert(t('sidebar.viewDeleteFail')))
                : null,
          })}
        {/each}
      </div>
    {/if}

    <!--
      Docs: mirrored wiki pages. When the mirror has none this used to render
      nothing at all, so "you have not set this up" and "this app cannot do
      that" looked the same — and someone who had configured it on another
      profile read the switch as the feature disappearing. The row below says
      which of the two it is and leads to the screen that fixes it.
    -->
    {#if !pages.bySpace.length}
      <div class="mb-3" data-testid="docs-section-empty">
        {@render sectionHeader(t('sidebar.docs'))}
        <!-- Same gutter and leading glyph as the live rows below this header
             would have: without them the two lines aligned with DOCS itself and
             read as a caption about the section rather than as the one thing in
             it you can click. The glyph is the destination, not a document —
             a file icon would advertise a view that does not exist yet. Once
             the source is configured the glyph stops being a gear, because the
             errand is no longer "go to Settings". -->
        <button
          type="button"
          class="flex w-full items-start gap-2 rounded-md px-3 py-1.5 text-left transition-colors {docsEmptyState ===
          'syncing'
            ? 'cursor-progress'
            : docsEmptyState === 'unavailable'
              ? 'cursor-default'
              : 'hover:bg-bg-hover'}"
          data-testid="docs-empty-cta"
          data-state={docsEmptyState}
          title={docsEmptyState === 'failed' ? (confluenceRuns?.[0]?.error ?? '') : ''}
          onclick={onDocsEmptyClick}
        >
          <!-- Five causes, five silhouettes. The gear is reserved for the one
               state that is genuinely unconfigured: once the source is on, a
               gear would say "set this up" about something already set up. The
               unavailable snapshot shares search-x with 'empty' — nothing to
               find either way — but never the gear: there is nowhere to go. -->
          <Icon
            name={docsEmptyState === 'failed'
              ? 'warning'
              : docsEmptyState === 'syncing' || docsEmptyState === 'never'
                ? 'refresh'
                : docsEmptyState === 'empty' || docsEmptyState === 'unavailable'
                  ? 'search-x'
                  : 'settings'}
            size={15}
            class="mt-0.5 flex-none {docsEmptyState === 'failed'
              ? 'text-status-reopen'
              : 'text-text-muted'} {docsEmptyState === 'syncing'
              ? 'motion-safe:animate-spin'
              : ''}"
          />
          <span class="flex min-w-0 flex-col gap-0.5">
            <span class="text-body text-text-secondary">{docsEmptyText.title}</span>
            {#if docsEmptyText.hint}
              <span class="text-micro leading-relaxed text-text-muted">
                {docsEmptyText.hint}
              </span>
            {/if}
          </span>
        </button>
      </div>
    {/if}
    {#if pages.bySpace.length}
      <div class="mb-3" data-testid="docs-section">
        {@render sectionHeader(t('sidebar.docs'))}
        <!-- The way in: recency first, across every space. What changed lately,
             and what you had open, are questions no single space answers. -->
        <button
          type="button"
          class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {pages.docsView
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
          aria-pressed={pages.docsView}
          title={t('sidebar.docsAllTitle')}
          data-testid="docs-documents"
          onclick={openDocuments}
        >
          <Icon
            name="file"
            size={15}
            class={pages.docsView ? 'text-text-secondary' : 'text-text-muted'}
          />
          <span class="min-w-0 flex-1 truncate">{t('sidebar.docsAll')}</span>
        </button>
        <!-- Spaces: collapsed, and one level deep. Seeing every container at all
             times was the thing that read as too much. -->
        <button
          type="button"
          class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
          aria-expanded={spacesOpen}
          title={t('sidebar.docsSpacesTitle')}
          data-testid="docs-spaces"
          onclick={() => (spacesOpen = !spacesOpen)}
        >
          <span
            class="flex-none text-text-muted transition-transform duration-150"
            style={spacesOpen ? 'transform: rotate(90deg)' : ''}
          >
            <Icon name="chevron-right" size={15} />
          </span>
          <span class="min-w-0 flex-1 truncate">{t('sidebar.docsSpaces')}</span>
          <span class="flex-none font-mono text-micro tabular-nums text-text-muted">
            {formatNumber(pages.bySpace.length)}
          </span>
        </button>
        {#if spacesOpen}
          {#each pages.bySpace as group (group.space)}
            <button
              type="button"
              class="flex h-control w-full items-center gap-2 rounded-md pl-[35px] pr-3 text-left text-[12px] transition-colors {pages.spaceView ===
              group.space
                ? 'bg-bg-active text-text-primary'
                : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
              aria-pressed={pages.spaceView === group.space}
              title={t('sidebar.docsSpaceTitle', { space: group.space, n: group.pages.length })}
              data-testid="docs-space"
              onclick={() => openSpace(group.space)}
            >
              <!-- The name is what people call the space; the key stays reachable
                   in the tooltip, and is the label itself until a name is mirrored. -->
              <span class="min-w-0 flex-1 truncate {group.name ? '' : 'font-mono'}">
                {group.name || group.space}
              </span>
              <span class="flex-none font-mono text-micro tabular-nums text-text-muted">
                {formatNumber(group.pages.length)}
              </span>
            </button>
          {/each}
        {/if}
      </div>
    {/if}

    <!-- Workspaces: other profile mirrors this serve can mount. Hidden unless
         the server actually has more than one (older servers / demo → 404 → []). -->
    {#if workspaceList.length > 1}
      <div class="mb-3">
        {@render sectionHeader(t('sidebar.workspaces'))}
        {#each workspaceList as w (w.name)}
          <a
            href={workspaceHref(w)}
            data-testid="workspace-link"
            class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {currentWorkspace ===
            w.name
              ? 'bg-bg-active text-text-primary'
              : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
            title={w.error ? t('sidebar.workspaceUnreadable') : w.site}
          >
            <span
              class="h-1.5 w-1.5 flex-none rounded-full {currentWorkspace === w.name
                ? 'bg-status-done'
                : 'bg-border-strong'}"
              aria-hidden="true"
            ></span>
            <span class="min-w-0 flex-1 truncate">{w.name}</span>
            {#if workspaceHost(w)}
              <span class="max-w-[45%] flex-none truncate text-micro text-text-muted">
                {workspaceHost(w)}
              </span>
            {/if}
          </a>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Settings / identity area (sidebar footer). The server settings entry
       point is absent — not disabled — wherever there is no server to edit:
       on a static snapshot the dialog's own load (settings/ → 404) is an
       error screen, so the errand it offers does not exist. -->
  <div class="flex-none border-t border-border-subtle px-3 py-2">
    {#if hasServerVerb('settings')}
      <button
        type="button"
        class="mb-1 flex h-control-sm w-full items-center gap-1.5 rounded-md px-1 text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={onOpenSettings}
        title={t('sidebar.serverSettings')}
      >
        <Icon name="settings" size={14} />
        {t('sidebar.settings')}
      </button>
    {/if}
    {#if me.identified}
      <button
        type="button"
        class="flex h-control-sm w-full items-center gap-1.5 rounded-md px-1 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={() => write.openSettings()}
        title={write.configured ? t('sidebar.jiraCreds') : t('sidebar.jiraCredsMissing')}
        aria-label={t('sidebar.jiraCreds')}
      >
        <Icon name="user" size={14} class={write.configured ? '' : 'text-status-stale'} />
        <span class="min-w-0 flex-1 truncate text-left">{me.name ?? me.email}</span>
      </button>
    {:else if isHostedDemo()}
      <!-- No credential button on the hosted demo: the dialog behind it asks for
           a real Atlassian token, and nothing on a static snapshot could use one. -->
      <p class="px-3 py-1.5 text-center text-micro text-text-muted">
        {t('app.demoNoCredentials')}
      </p>
    {:else if me.authChecked}
      <button
        type="button"
        class="flex h-control w-full items-center justify-center gap-1.5 rounded-md border border-border-strong px-3 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={() => write.openSettings()}
      >
        {t('common.setCredentials')}
      </button>
    {/if}
  </div>
</div>
