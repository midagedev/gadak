<script lang="ts">
  /*
   * Sidebar nav ([explore]). Sections: built-in views / saved views (server, with
   * leftover localStorage rows until absorb) / Jira filters. Top: issue totals +
   * last sync. Personalization (Wave 3) sits above built-ins. View click =
   * filters.applyConfig; active when it matches.
   */
  import { onMount } from 'svelte'
  import { t, formatNumber, relativeTime } from '../../lib/i18n'
  import { filterIssues, filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { reachability } from '../../lib/reachability.svelte'
  import { views } from '../../stores/views.svelte'
  import { me } from '../../stores/me.svelte'
  import { onboarding } from '../../stores/onboarding.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { showIssueList } from '../../lib/show-issue-list'
  import { write } from '../../stores/write.svelte'
  import { runSyncNow } from '../../lib/sync-now'
  import { copyText } from '../../lib/copy-text'
  import { trapFocus } from '../../lib/focus-trap'
  import { upgradeCta } from '../../lib/upgrade-cta'
  import { getSyncRuns, getWorkspaces, type SyncRun, type WorkspaceInfo } from '../../lib/api'
  import {
    config,
    feature,
    hasServerVerb,
    isHostedDemo,
    jiraFilterUrl,
    workspaceName,
  } from '../../lib/config'
  import { isStandalone, STANDALONE_INIT_COMMAND } from '../../lib/workspace'
  import { mirrorLabel } from '../../lib/mirror-status'
  import { docsEmptyClickAction, docsEmptyGlyph } from '../../lib/docs-empty'
  import { docsEmpty } from '../../stores/docs-empty.svelte'
  import { dashboards } from '../../stores/dashboards.svelte'
  import { builtinViews } from '../../lib/builtin-views'
  import { configToParams, type ViewConfig } from '../../lib/view-config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import MyIssuesNav from '../personal/MyIssuesNav.svelte'
  import FavoritesNav from '../personal/FavoritesNav.svelte'
  import Icon from '../ui/Icon.svelte'
  import DialogShell from '../ui/DialogShell.svelte'
  import SidebarSection from './SidebarSection.svelte'
  import { sidebarSections, type SectionDrag, type SectionId } from '../../stores/sidebar-sections.svelte'

  sidebarSections.hydrate()
  docsEmpty.bind()

  /** Open server settings dialog — App.svelte mounts the dialog itself. */
  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  let draggingId = $state<SectionId | null>(null)
  let dropTargetId = $state<SectionId | null>(null)
  const drag: SectionDrag = {
    get draggingId() {
      return draggingId
    },
    get dropTargetId() {
      return dropTargetId
    },
    start(id) {
      draggingId = id
    },
    hover(id) {
      if (dropTargetId !== id) dropTargetId = id
    },
    clear() {
      draggingId = null
      dropTargetId = null
    },
  }

  let notesOpen = $state(false)
  let copiedCmd = $state(false)
  const cta = $derived(upgradeCta(config().os))
  const notesText = $derived(issues.releaseNotes.trim())

  $effect(() => {
    if (!notesText) notesOpen = false
  })

  function closeNotes() {
    notesOpen = false
    copiedCmd = false
  }

  function onNotesKeydown(e: KeyboardEvent) {
    if (!notesOpen || e.key !== 'Escape') return
    e.preventDefault()
    closeNotes()
  }

  async function copyUpgradeCmd(): Promise<void> {
    if (!cta.command) return
    if (await copyText(cta.command)) {
      copiedCmd = true
      setTimeout(() => {
        copiedCmd = false
      }, 1500)
    }
  }

  const builtins = builtinViews()

  /** Apply view = give ListView the column, then the filters. */
  function applyView(config: ViewConfig) {
    showIssueList(config, true)
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
  // View-row tint means "this is the list you are on". Docs, history, a
  // dashboard, and the feed (when that surface exists) take the column;
  // space is selection on top of the still-current view, so its place-row
  // lights without clearing this match. Feed is gated the same way
  // App.svelte renders it. A dashboard was missing once (GDK-815) — the
  // tinted row beside a dashboard screen claimed a list nobody was on.
  const mainColumnIsList = $derived(
    !(feature('feed') && me.feedOpen) &&
      !pages.historyView &&
      !pages.docsView &&
      dashboards.openId === null,
  )
  const activeBuiltin = $derived(mainColumnIsList ? activeId(builtins) : null)
  const activePersonal = $derived(mainColumnIsList ? activeId(views.personal) : null)
  const activeTeam = $derived(mainColumnIsList ? activeId(views.team) : null)
  const activeSource = $derived(mainColumnIsList ? activeId(views.source) : null)
  const builtinCounts = $derived.by(() => {
    const counts = new Map<string, number>()
    // GDK-153: skip the six-view refilter while this section is collapsed.
    if (sidebarSections.collapsedIds.includes('builtin')) return counts
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
  // GDK-460: this row is the sync-history entry, not a second freshness
  // sentence. The toolbar chip owns "Sync delayed · Nw ago" / the busy line.
  // Title still carries per-source detail; the visible label is the entry's
  // own name.
  const syncLabel = $derived(mirrorLabel())
  const syncDot = $derived(
    issues.syncHealth?.overall === 'failed'
      ? 'bg-status-reopen'
      : issues.syncHealth?.overall === 'warning'
        ? 'bg-status-stale'
        : 'bg-status-done',
  )

  /* ── Sync history popover (click on the Sync history entry) ── */
  let historyOpen = $state(false)
  let historyRuns = $state<SyncRun[]>([])
  let historyLastCheckedAt = $state<string | null>(null)
  let historyLoading = $state(false)

  // Spend Esc so one keystroke cannot also reach the shell keymap — same
  // negotiation as the list-header menus (DisplayMenu).
  function onHistoryEsc(e: KeyboardEvent) {
    if (e.key !== 'Escape' || !historyOpen) return
    e.preventDefault()
    e.stopPropagation()
    historyOpen = false
  }

  async function toggleHistory() {
    if (historyOpen) {
      historyOpen = false
      return
    }
    historyOpen = true
    historyLoading = true
    historyLastCheckedAt = null
    const doc = await getSyncRuns()
    historyRuns = doc.runs
    historyLastCheckedAt =
      typeof doc.last_checked_at === 'string' && doc.last_checked_at ? doc.last_checked_at : null
    historyLoading = false
  }

  const historyLastCheckedLine = $derived.by(() => {
    if (!historyLastCheckedAt) return ''
    const when = relativeTime(historyLastCheckedAt, 'long')
    if (!when) return ''
    return t('sidebar.syncLastChecked', { when })
  })

  let deleteArmedId = $state<string | null>(null)

  function onDocClick(e: MouseEvent) {
    // Delete-confirm disarm only. The popover's outside-close lives on the
    // onOutsideClick boundary below; this one stays a document click because
    // disarming an armed delete must fire on any click, popover or not.
    const onDelete = e
      .composedPath()
      .some((n) => n instanceof HTMLElement && n.dataset.testid === 'sidebar-view-delete')
    if (!onDelete) deleteArmedId = null
  }

  function runKindLabel(kind: string): string {
    const base = kind.startsWith('full') ? t('sidebar.runFull') : t('sidebar.runIncremental')
    return kind.includes('+reconcile') ? `${base} ${t('sidebar.runReconcile')}` : base
  }

  /* ── Workspaces (one process, several profile mirrors) ── */
  let workspaceList = $state<WorkspaceInfo[]>([])
  onMount(() => {
    void getWorkspaces().then((list) => (workspaceList = list))
  })
  /** The mirror this page is looking at: URL mount, or the serve primary. */
  const currentWorkspace = $derived(
    workspaceName() || workspaceList.find((w) => w.active)?.name || 'default',
  )
  const visibleIds = $derived.by((): SectionId[] => {
    const present = new Set<SectionId>(['builtin', 'docs'])
    if (views.source.length && !onboarding.needsOnboarding) present.add('jira')
    // One saved-views section (GDK-437). The persisted section id stays
    // `personal` so collapse/order for that slot survive; `team` remains in
    // SECTION_IDS for stored-order parse but is never present here.
    if (views.personal.length || views.team.length) present.add('personal')
    // Agent dashboards (GDK-782): server rows only — nothing to list without
    // a serve, and the section's whole lifecycle is CLI-authored anyway.
    if (dashboards.list.length) present.add('dashboards')
    if (workspaceList.length > 1) present.add('workspaces')
    return sidebarSections.order.filter((id) => present.has(id))
  })
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
  const standalone = isStandalone(config())
  let standaloneHowOpen = $state(false)

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
   * Empty-docs cause, copy, and the Confluence run fetch live in
   * stores/docs-empty.svelte.ts — this row only renders them. The table
   * itself is lib/docs-empty.ts.
   */
  const docsEmptyState = $derived(docsEmpty.state)
  const docsEmptyText = $derived(docsEmpty.text)
  const confluenceRuns = $derived(docsEmpty.confluenceRuns)

  /** 'never' and a failed index are the states the user can act on without
   *  leaving the sidebar — sync, or retry the index request (GDK-1067). */
  function onDocsEmptyClick() {
    const action = docsEmptyClickAction(docsEmptyState)
    if (action === 'none') return
    if (action === 'sync') {
      void issues.pullMirror('full')
      return
    }
    if (action === 'retry') {
      void pages.reload()
      return
    }
    onOpenSettings()
  }

  /** Each of these is one show onto the column union (GDK-821): taking the
   *  column releases whatever held it — the feed included — so none of them
   *  needs to close the others by hand anymore. */
  function openDocuments() {
    pages.toggleDocs()
  }

  function openSpace(space: string) {
    pages.openSpace(space)
  }

  function openDash(id: string) {
    dashboards.open(id)
  }
</script>

<svelte:document onclick={onDocClick} />

<!-- A saved view's row. Local leftover and server rows share this snippet so
     the list stays one kind. Delete permission still keys on owner_email
     (server rows) vs always-allowed (local leftovers); the email itself is
     not drawn. data-view-storage is how e2e reads where the row sat. -->
{#snippet viewRow(row: {
  id: string
  name: string
  active: boolean
  apply: () => void
  storage: 'local' | 'server'
  /** Null when this viewer may not delete it — a server view that is not theirs. */
  remove: (() => void) | null
})}
  {@const armed = deleteArmedId === row.id}
  <div
    class="group flex h-control items-center gap-2 rounded-md px-3 text-body transition-colors {row.active
      ? 'bg-bg-active'
      : 'hover:bg-bg-hover'}"
    data-testid="sidebar-view-row"
    data-view-id={row.id}
    data-view-storage={row.storage}
  >
    <button
      type="button"
      class="min-w-0 flex-1 truncate text-left {row.active
        ? 'text-text-primary'
        : 'text-text-secondary group-hover:text-text-primary'}"
      onclick={row.apply}
      >{row.name}</button
    >
    {#if row.remove}
      <button
        type="button"
        class="flex flex-none items-center text-text-muted opacity-0 transition-opacity hover:text-status-reopen group-hover:opacity-100 {armed
          ? 'opacity-100 text-status-reopen'
          : ''}"
        title={armed ? t('jiraSettings.deleteConfirm') : t('common.delete')}
        aria-label={armed ? t('jiraSettings.deleteConfirm') : t('common.delete')}
        data-testid="sidebar-view-delete"
        data-armed={armed ? 'true' : undefined}
        onclick={(e) => {
          e.stopPropagation()
          if (deleteArmedId !== row.id) {
            deleteArmedId = row.id
            return
          }
          deleteArmedId = null
          row.remove?.()
        }}
      >
        <Icon name="x" size={13} />
      </button>
    {/if}
  </div>
{/snippet}

<svelte:window onkeydown={onNotesKeydown} />

<div class="flex h-full flex-col">
  <!-- New issue (shortcut c). Disabled on the hosted demo, where the snapshot
       service worker answers every write with 501 — offering the button only to
       fail on submit wastes the visitor's time. Hidden during onboarding: the
       wizard is the write path (GDK-299 F6). -->
  {#if !onboarding.needsOnboarding}
  <div class="flex-none px-3 pt-1 pb-2">
    <button
      type="button"
      disabled={isHostedDemo()}
      onclick={() => write.openNewIssue()}
      class="flex h-control w-full items-center justify-center gap-1.5 rounded-md bg-accent px-3 text-body font-medium text-white transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-bg-elevated disabled:text-text-muted disabled:hover:bg-bg-elevated"
      title={isHostedDemo() ? t('app.demoWriteDisabled') : t('sidebar.newIssueTitle')}
    >
      <Icon name="plus" size={13} />
      {t('write.newIssue')}
    </button>
  </div>
  {/if}

  <!-- Update notice: server found a newer published release (daily check).
       Notes present → dialog (plain text). Empty body → same external link
       as before; do not open an empty dialog. -->
  {#if issues.latestVersion}
    <div class="flex-none px-3 pb-1">
      {#if notesText}
        <button
          type="button"
          class="block w-full rounded-md border border-accent/30 bg-accent-subtle/30 px-2.5 py-1.5 text-left text-micro text-accent-text transition-colors hover:bg-accent-subtle/50"
          data-testid="update-notice"
          onclick={() => (notesOpen = true)}
        >
          {t('sidebar.updateAvailable', { version: issues.latestVersion })}
        </button>
      {:else}
        <a
          href={issues.releaseUrl || 'https://github.com/midagedev/gadak/releases'}
          target="_blank"
          rel="noreferrer"
          class="block rounded-md border border-accent/30 bg-accent-subtle/30 px-2.5 py-1.5 text-micro text-accent-text transition-colors hover:bg-accent-subtle/50"
          data-testid="update-notice"
        >
          {t('sidebar.updateAvailable', { version: issues.latestVersion })}
        </a>
      {/if}
    </div>
  {/if}

  <!-- Totals / sync-history entry (GDK-460). Click opens the run popover
       (Sync now lives inside). Freshness wording is the toolbar chip's.
       Hidden during onboarding: pool size 0 / a sync sentence contradicts
       the wizard's own fetched count (GDK-299 F7). -->
  {#if !onboarding.needsOnboarding}
  <div class="flex-none px-3 pb-2 pt-1 text-micro text-text-muted">
    {t('sidebar.issueCount', { n: formatNumber(issues.pool.size) })}
    <span class="ml-1">·</span>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="relative inline-block"
      onkeydown={onHistoryEsc}
      use:onEscape={onHistoryEsc}
      use:onOutsideClick={{ handler: () => (historyOpen = false), enabled: historyOpen }}
    >
      <button
        type="button"
        class="ml-1 inline-flex items-center gap-1 rounded px-0.5 text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        title={[syncTitle || syncLabel, t('sidebar.syncHistoryTitle')].filter(Boolean).join('\n')}
        aria-label={t('sidebar.syncHistory')}
        aria-expanded={historyOpen}
        data-testid="sidebar-sync-now"
        data-state={issues.mirrorBusy ? 'syncing' : 'idle'}
        onclick={() => void toggleHistory()}
      >
        <span class="h-1.5 w-1.5 flex-none rounded-full {syncDot}" aria-hidden="true"></span>
        {t('sidebar.syncHistory')}
      </button>
      {#if historyOpen}
        <div
          class="anim-enter absolute left-0 top-full z-40 mt-1 w-72 rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
          data-testid="sync-history-popover"
        >
          <div class="px-2 py-1 text-micro font-medium text-text-muted">
            {t('sidebar.syncHistory')}
          </div>
          {#if historyLastCheckedLine}
            <div
              class="px-2 py-1 text-micro text-text-muted"
              data-testid="sync-history-last-checked"
            >
              {historyLastCheckedLine}
            </div>
          {/if}
          {#if reachability.offline}
            <div
              class="flex items-center justify-between gap-2 px-2 py-1.5"
              data-testid="sync-history-offline"
            >
              <p class="text-body text-status-stale">{t('sidebar.serverUnreachable')}</p>
              <button
                type="button"
                class="flex-none rounded-md border border-border-strong px-2 py-0.5 text-body text-text-secondary transition-colors hover:bg-bg-hover"
                data-testid="sync-history-retry"
                onclick={() => void issues.refresh()}
              >
                {t('common.retry')}
              </button>
            </div>
          {/if}
          {#if historyLoading}
            <div class="px-2 py-2 text-micro text-text-muted">{t('common.searching')}</div>
          {:else if historyRuns.length === 0}
            <div class="px-2 py-2 text-micro text-text-muted">{t('sidebar.syncNoHistory')}</div>
          {:else}
            <div class="max-h-64 overflow-y-auto">
              {#each historyRuns as run (run.started_at + run.kind)}
                <div class="flex items-start gap-2 rounded px-2 py-1 text-body">
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
              class="flex h-control-sm w-full items-center rounded px-2 text-left text-body text-accent-text transition-colors hover:bg-bg-hover"
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
  {/if}

  <div class="scroll-region min-h-0 flex-1" data-testid="sidebar-scroll">
    <!-- Personalization (Wave 3): My Issues / recent — above built-ins.
         Hidden during onboarding: the unauthenticated row is a second
         Set credentials (GDK-299 F6). -->
    {#if !onboarding.needsOnboarding}
      <MyIssuesNav />
      <FavoritesNav />
    {/if}

    <div role="list" data-testid="sidebar-sections">
      {#each visibleIds as id (id)}
        {#if id === 'builtin'}
          <SidebarSection id="builtin" label={t('sidebar.builtinViews')} {visibleIds} {drag}>
            {#each builtins as v (v.id)}
              <!-- aria-current rides the same condition as the paint: the class
                   is decoration, the attribute is the contract e2e and screen
                   readers read (GDK-613). -->
              <button
                type="button"
                class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {activeBuiltin ===
                v.id
                  ? 'bg-bg-active text-text-primary'
                  : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
                aria-current={activeBuiltin === v.id ? 'true' : undefined}
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
          </SidebarSection>
        {:else if id === 'jira'}
          <!-- Jira saved filters (owned + starred). No delete — Jira is the record. -->
          <SidebarSection
            id="jira"
            label={t('sidebar.jiraFilters')}
            testid="sidebar-jira-filters"
            {visibleIds}
            {drag}
          >
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
                  {#if v.favourite}<span class="mr-1 text-micro text-text-muted" aria-hidden="true">★</span>{/if}{v.name}{#if v.unsupported?.length}{' '}<span
                      class="ml-1 truncate text-micro text-text-muted"
                      title={t('filter.jqlPartial', { clauses: v.unsupported.join('; ') })}
                      data-testid="sidebar-view-partial"
                      >{t('filter.jqlPartial', { clauses: v.unsupported.join('; ') })}</span
                    >{/if}
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
          </SidebarSection>
        {:else if id === 'personal'}
          <SidebarSection id="personal" label={t('sidebar.myViews')} {visibleIds} {drag}>
            {#each views.personal as v (v.id)}
              {@render viewRow({
                id: v.id,
                name: v.name,
                active: activePersonal === v.id,
                apply: () => applyView(v.config),
                storage: 'local',
                remove: () => views.removePersonal(v.id),
              })}
            {/each}
            {#each views.team as v (v.id)}
              {@render viewRow({
                id: v.id,
                name: v.name,
                active: activeTeam === v.id,
                apply: () => applyView(v.config),
                storage: 'server',
                remove:
                  me.email && v.owner_email === me.email
                    ? () =>
                        views
                          .removeTeam(v.id)
                          .catch(() => write.toast(t('sidebar.viewDeleteFail'), 'error'))
                    : null,
              })}
            {/each}
          </SidebarSection>
        {:else if id === 'dashboards'}
          <!--
            Agent dashboards (GDK-782): saved like views, opened like the feed —
            the row hands the main column to a sandboxed iframe. No delete
            affordance: rows come and go through `gadak dashboards` verbs, and a
            half of that lifecycle (save) is not in this UI either — a delete
            button here would be the one editing verb on a read-only surface.
          -->
          <SidebarSection id="dashboards" label={t('sidebar.dashboards')} {visibleIds} {drag}>
            {#each dashboards.list as d (d.id)}
              <button
                type="button"
                class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {dashboards.openId ===
                d.id
                  ? 'bg-bg-active text-text-primary'
                  : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
                aria-current={dashboards.openId === d.id ? 'true' : undefined}
                title={d.name}
                data-testid="sidebar-dashboard-row"
                data-dashboard-id={d.id}
                onclick={() => openDash(d.id)}
              >
                <Icon
                  name="layout-dashboard"
                  size={15}
                  class={dashboards.openId === d.id ? 'text-text-secondary' : 'text-text-muted'}
                />
                <span class="min-w-0 flex-1 truncate">{d.name}</span>
              </button>
            {/each}
          </SidebarSection>
        {:else if id === 'docs'}
          <!--
            Docs: mirrored wiki pages. When the mirror has none this used to render
            nothing at all, so "you have not set this up" and "this app cannot do
            that" looked the same — and someone who had configured it on another
            profile read the switch as the feature disappearing. The row below says
            which of the two it is and leads to the screen that fixes it.
          -->
          <SidebarSection id="docs" label={t('sidebar.docs')} {visibleIds} {drag}>
            {#if !pages.bySpace.length}
              <div data-testid="docs-section-empty">
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
                  <!-- Seven causes, four silhouettes. Both failure causes share the
                       warning — they differ in copy and click, not in kind. The gear is
                       reserved for the one state that is genuinely unconfigured: once
                       the source is on, a gear would say "set this up" about something
                       already set up. The unavailable snapshot shares search-x with
                       'empty' — nothing to find either way — but never the gear: there
                       is nowhere to go. -->
                  <Icon
                    name={docsEmptyGlyph(docsEmptyState)}
                    size={15}
                    class="mt-0.5 flex-none {docsEmptyState === 'failed' || docsEmptyState === 'loadfailed'
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
            {:else}
              <div data-testid="docs-section">
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
                      class="flex h-control w-full items-center gap-2 rounded-md pl-[35px] pr-3 text-left text-body transition-colors {pages.spaceView ===
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
          </SidebarSection>
        {:else if id === 'workspaces'}
          <!-- Workspaces: other profile mirrors this serve can mount. Hidden unless
               the server actually has more than one (older servers / demo → 404 → []). -->
          <SidebarSection id="workspaces" label={t('sidebar.workspaces')} {visibleIds} {drag}>
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
                {#if currentWorkspace === w.name && standalone}
                  <span
                    class="inline-flex flex-none items-center rounded-full border border-border-subtle px-1.5 py-0.5 text-micro text-text-secondary"
                    data-testid="workspace-kind"
                    data-kind="standalone"
                    title={t('settings.workspaceStandaloneHint')}
                    aria-label={t('settings.workspaceStandaloneHint')}
                  >
                    {t('settings.workspaceStandalone')}
                  </span>
                {/if}
                {#if workspaceHost(w)}
                  <span class="max-w-[45%] flex-none truncate text-micro text-text-muted">
                    {workspaceHost(w)}
                  </span>
                {/if}
              </a>
            {/each}
          </SidebarSection>
        {/if}
      {/each}
    </div>
  </div>

  <!-- Settings / identity area (sidebar footer). The server settings entry
       point is absent — not disabled — wherever there is no server to edit:
       on a static snapshot the dialog's own load (settings/ → 404) is an
       error screen, so the errand it offers does not exist. -->
  <div class="flex-none border-t border-border-subtle px-3 py-2">
    {#if hasServerVerb('settings')}
      {#if !onboarding.needsOnboarding && !standalone}
      <!-- GDK-1122: the how-to-create affordance is for connected workspaces;
           offering it inside a standalone one advertises leaving the origin
           this workspace is bound to. -->
      <button
        type="button"
        class="mb-1 flex h-control-sm w-full items-center gap-1.5 rounded-md px-1 text-body text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        data-testid="standalone-create"
        aria-expanded={standaloneHowOpen}
        onclick={() => (standaloneHowOpen = !standaloneHowOpen)}
      >
        {t('settings.standaloneHow')}
      </button>
      {#if standaloneHowOpen}
        <div class="mb-1 px-1">
          <code class="break-all font-mono text-micro text-text-primary">{STANDALONE_INIT_COMMAND}</code>
          <div class="mt-0.5 text-micro text-text-muted">{t('settings.workspaceStandaloneHint')}</div>
          <div class="mt-0.5 text-micro text-text-muted">{t('settings.standaloneCommandHint')}</div>
        </div>
      {/if}
      {/if}
      <button
        type="button"
        class="mb-1 flex h-control-sm w-full items-center gap-1.5 rounded-md px-1 text-body text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={onOpenSettings}
        title={t('sidebar.serverSettings')}
      >
        <Icon name="settings" size={14} />
        {t('sidebar.settings')}
      </button>
    {/if}
    {#if !onboarding.needsOnboarding && me.identified}
      <button
        type="button"
        class="flex h-control-sm w-full items-center gap-1.5 rounded-md px-1 text-body text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={() => write.openSettings()}
        title={write.configured ? t('sidebar.jiraCreds') : t('sidebar.jiraCredsMissing')}
        aria-label={t('sidebar.jiraCreds')}
      >
        <Icon name="user" size={14} class={write.configured ? '' : 'text-status-stale'} />
        <span class="min-w-0 flex-1 truncate text-left">{me.name ?? me.email}</span>
      </button>
    {:else if !onboarding.needsOnboarding && isHostedDemo()}
      <!-- No credential button on the hosted demo: the dialog behind it asks for
           a real Atlassian token, and nothing on a static snapshot could use one. -->
      <p class="px-3 py-1.5 text-center text-micro text-text-muted">
        {t('app.demoNoCredentials')}
      </p>
    {:else if !onboarding.needsOnboarding && me.authChecked}
      <button
        type="button"
        class="flex h-control w-full items-center justify-center gap-1.5 rounded-md border border-border-strong px-3 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={() => write.openSettings()}
      >
        {t('common.setCredentials')}
      </button>
    {/if}
  </div>
</div>

{#if notesOpen && notesText}
  <DialogShell
    title={t('sidebar.updateAvailable', { version: issues.latestVersion })}
    ariaLabel={t('sidebar.updateAvailable', { version: issues.latestVersion })}
    data-testid="update-notes"
    onclose={closeNotes}
    trap={trapFocus}
    panelClass="anim-pop max-h-[80vh] max-w-lg"
    headerClass="flex flex-none flex-col border-b border-border-subtle px-4 py-3"
    footerClass="flex flex-none flex-wrap items-center gap-2 border-t border-border-subtle px-4 py-3"
  >
    <pre
      class="scroll-region min-h-0 flex-1 overflow-auto whitespace-pre-wrap px-4 py-3 font-mono text-micro text-text-primary"
    >{notesText}</pre>
    {#snippet footer()}
      {#if cta.command}
        <span class="font-mono text-micro text-text-primary">{cta.command}</span>
        <button
          type="button"
          class="inline-flex h-control-sm items-center rounded border border-border-strong px-1.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
          onclick={() => void copyUpgradeCmd()}
        >
          {copiedCmd ? t('settings.copied') : t('settings.copy')}
        </button>
      {/if}
      {#if issues.releaseUrl}
        <a
          href={issues.releaseUrl}
          target="_blank"
          rel="noreferrer"
          class="text-micro text-accent-text hover:underline"
        >
          {t('settings.updateReleaseNotes')}
        </a>
      {/if}
    {/snippet}
  </DialogShell>
{/if}
