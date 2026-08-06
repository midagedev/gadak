<script lang="ts">
  /*
   * Sidebar nav ([explore]). Sections: built-in views / personal (localStorage) /
   * team shared (api). Top: issue totals + last sync. Personalization (Wave 3)
   * sits above built-ins. View click = filters.applyConfig; active when it matches.
   */
  import { t, formatNumber, relativeTime, formatTimeOfDay } from '../../lib/i18n'
  import { filterIssues, filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { views } from '../../stores/views.svelte'
  import { me } from '../../stores/me.svelte'
  import { pages, type PageNode } from '../../stores/pages.svelte'
  import { write } from '../../stores/write.svelte'
  import { runSyncNow } from '../../lib/sync-now'
  import { getSyncRuns, getWorkspaces, type SyncRun, type WorkspaceInfo } from '../../lib/api'
  import { isHostedDemo, workspaceName } from '../../lib/config'
  import { builtinViews } from '../../lib/builtin-views'
  import { configToParams, type ViewConfig } from '../../lib/view-config'
  import MyIssuesNav from '../personal/MyIssuesNav.svelte'
  import FavoritesNav from '../personal/FavoritesNav.svelte'

  /** Open server settings dialog — App.svelte mounts the dialog itself. */
  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  const builtins = builtinViews()

  /** Apply view = close personal feed if open (back to list) then apply filters. */
  function applyView(config: ViewConfig) {
    me.closeFeed()
    pages.closeRecent()
    filters.applyConfig(config)
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
  const activeTeam = $derived(
    activeId(views.team.map((v) => ({ id: v.id, config: v.config as unknown as ViewConfig }))),
  )
  const builtinCounts = $derived.by(() => {
    const counts = new Map<string, number>()
    for (const view of builtins) {
      counts.set(view.id, filterIssues(issues.allIssues, view.config.filters).length)
    }
    return counts
  })

  const lastSyncLabel = $derived(issues.lastSync ? formatTimeOfDay(issues.lastSync) : '')

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
  const syncLabel = $derived(
    issues.syncHealth?.overall === 'failed'
      ? t('sidebar.syncFailTitle')
      : issues.syncHealth?.overall === 'warning'
        ? t('sidebar.syncDelayedTitle')
        : lastSyncLabel
          ? t('sidebar.syncLabel', { when: lastSyncLabel })
          : t('sidebar.syncChecking'),
  )
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

  /* ── Docs (mirrored wiki pages), a tree per space ── */
  //  Collapsed by default: a space holds dozens of pages, and the nav is for
  //  views first. Expanded state is per-tab — no need to persist it.
  let openSpaces = $state(new Set<string>())
  /** Expanded page nodes, by key. */
  let openDocs = $state(new Set<string>())

  function toggleSpace(space: string) {
    const next = new Set(openSpaces)
    if (!next.delete(space)) {
      next.add(space)
      // A space normally has one root page, so opening to a single collapsed
      // row reads as a broken toggle. Roots open with the space; deeper levels
      // stay closed.
      const docs = new Set(openDocs)
      for (const r of pages.treeBySpace.find((g) => g.space === space)?.roots ?? []) {
        docs.add(r.page.key)
      }
      openDocs = docs
    }
    openSpaces = next
  }

  function toggleDoc(key: string) {
    const next = new Set(openDocs)
    if (!next.delete(key)) next.add(key)
    openDocs = next
  }

  /** Open a page; a parent also expands, so one click never looks inert. */
  function openDoc(node: PageNode) {
    pages.select(node.page.key)
    if (node.children.length && !openDocs.has(node.page.key)) toggleDoc(node.page.key)
  }

  /** "Recently updated" takes over the main column, so the feed must give it up. */
  function toggleRecentDocs() {
    me.closeFeed()
    pages.toggleRecent()
  }
</script>

<svelte:document onclick={onDocClick} />

<!-- One page row in the DOCS tree, recursing into its children. Indent is a
     fixed step per depth so a leaf lines up under its parent's title. -->
{#snippet docNode(node: PageNode)}
  {@const expanded = openDocs.has(node.page.key)}
  {@const selected = pages.selectedKey === node.page.key}
  <div
    class="group flex min-h-7 items-center rounded-md pr-3 text-[12px] transition-colors {selected
      ? 'bg-bg-active'
      : 'hover:bg-bg-hover'}"
    style="padding-left: {16 + node.depth * 12}px"
    data-testid="doc-tree-node"
  >
    {#if node.children.length}
      <button
        type="button"
        class="flex h-4 w-4 flex-none items-center justify-center rounded text-text-muted transition-colors hover:text-text-primary"
        aria-expanded={expanded}
        aria-label={t('sidebar.docsToggleNode', { title: node.page.title })}
        data-testid="doc-tree-toggle"
        onclick={() => toggleDoc(node.page.key)}
      >
        <svg
          width="10"
          height="10"
          viewBox="0 0 10 10"
          fill="none"
          aria-hidden="true"
          class="transition-transform duration-150"
          style={expanded ? 'transform: rotate(90deg)' : ''}
        >
          <path d="M3.5 2l3 3-3 3" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    {:else}
      <!-- Keeps leaf titles on the same left edge as their siblings' -->
      <span class="h-4 w-4 flex-none" aria-hidden="true"></span>
    {/if}
    <button
      type="button"
      class="min-w-0 flex-1 truncate py-1 pl-1 text-left {selected
        ? 'text-text-primary'
        : 'text-text-secondary group-hover:text-text-primary'}"
      title={node.page.title}
      onclick={() => openDoc(node)}
    >
      {node.page.title}
    </button>
  </div>
  {#if expanded}
    {#each node.children as child (child.page.key)}
      {@render docNode(child)}
    {/each}
  {/if}
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
      class="flex w-full items-center justify-center gap-1.5 rounded-md bg-accent px-3 py-2 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-bg-elevated disabled:text-text-muted disabled:hover:bg-bg-elevated"
      title={isHostedDemo() ? t('app.demoWriteDisabled') : t('sidebar.newIssueTitle')}
    >
      <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
        <path d="M6 2v8M2 6h8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
      </svg>
      {t('sidebar.newIssue')}
    </button>
  </div>

  <!-- Update notice: server found a newer published release (daily check). -->
  {#if issues.latestVersion}
    <div class="flex-none px-3 pb-1">
      <a
        href={issues.releaseUrl || 'https://github.com/midagedev/scry/releases'}
        target="_blank"
        rel="noreferrer"
        class="block rounded-md border border-accent/30 bg-accent-subtle/30 px-2.5 py-1.5 text-[11px] text-accent-text transition-colors hover:bg-accent-subtle/50"
        data-testid="update-notice"
      >
        {t('sidebar.updateAvailable', { version: issues.latestVersion })}
      </a>
    </div>
  {/if}

  <!-- Totals / sync — badge click = history popover (with Sync now inside) -->
  <div class="flex-none px-3 pb-2 pt-1 text-[11px] text-text-muted">
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
          class="anim-enter absolute left-0 top-full z-40 mt-1 w-72 rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-xl shadow-black/40"
          data-testid="sync-history-popover"
        >
          <div class="px-2 py-1 text-[11px] font-medium text-text-muted">
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
                      <span class="flex-none text-[11px] text-text-muted" title={run.finished_at}>
                        {relativeTime(run.finished_at, 'long')}
                      </span>
                    </div>
                    {#if run.error}
                      <div class="break-words text-[11px] text-status-reopen">{run.error}</div>
                    {:else}
                      <div class="text-[11px] text-text-muted">
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
              class="w-full rounded px-2 py-1 text-left text-[12px] text-accent-text transition-colors hover:bg-bg-hover"
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
      <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
        {t('sidebar.builtinViews')}
      </div>
      {#each builtins as v (v.id)}
        <button
          type="button"
          class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {activeBuiltin ===
          v.id
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
          title={v.hint}
          onclick={() => applyView(v.config)}
        >
          <span class="flex-none text-[13px]">{v.icon}</span>
          <span class="min-w-0 flex-1 truncate">{v.name}</span>
          <span class="flex-none font-mono text-[11px] tabular-nums text-text-muted">
            {formatNumber(builtinCounts.get(v.id) ?? 0)}
          </span>
        </button>
      {/each}
    </div>

    <!-- Personal views -->
    {#if views.personal.length}
      <div class="mb-3">
        <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
          {t('sidebar.myViews')}
        </div>
        {#each views.personal as v (v.id)}
          <div
            class="group flex min-h-7 items-center gap-2 rounded-md px-3 py-1.5 text-[13px] transition-colors {activePersonal ===
            v.id
              ? 'bg-bg-active'
              : 'hover:bg-bg-hover'}"
          >
            <button
              type="button"
              class="min-w-0 flex-1 truncate text-left {activePersonal === v.id
                ? 'text-text-primary'
                : 'text-text-secondary group-hover:text-text-primary'}"
              onclick={() => applyView(v.config)}
            >
              {v.name}
            </button>
            <button
              type="button"
              class="flex-none text-text-muted opacity-0 transition-opacity hover:text-status-reopen group-hover:opacity-100"
              title={t('common.delete')}
              onclick={() => views.removePersonal(v.id)}
            >
              ✕
            </button>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Team shared views -->
    {#if views.team.length}
      <div class="mb-3">
        <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
          {t('sidebar.teamViews')}
        </div>
        {#each views.team as v (v.id)}
          <div
            class="group flex min-h-7 items-center gap-2 rounded-md px-3 py-1.5 text-[13px] transition-colors {activeTeam ===
            v.id
              ? 'bg-bg-active'
              : 'hover:bg-bg-hover'}"
          >
            <button
              type="button"
              class="min-w-0 flex-1 truncate text-left {activeTeam === v.id
                ? 'text-text-primary'
                : 'text-text-secondary group-hover:text-text-primary'}"
              onclick={() => applyView(v.config as unknown as ViewConfig)}
              title={v.owner_name ? t('sidebar.viewOwner', { name: v.owner_name }) : undefined}
            >
              {v.name}
              {#if v.owner_name}<span class="ml-1 text-[11px] text-text-muted">· {v.owner_name}</span>{/if}
            </button>
            {#if me.email && v.owner_email === me.email}
              <button
                type="button"
                class="flex-none text-text-muted opacity-0 transition-opacity hover:text-status-reopen group-hover:opacity-100"
                title={t('common.delete')}
                onclick={() =>
                  views.removeTeam(v.id).catch(() => alert(t('sidebar.viewDeleteFail')))}
              >
                ✕
              </button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    <!-- Docs: mirrored wiki pages by space. Hidden entirely when the mirror has
         none (no wiki configured / older server → empty index). -->
    {#if pages.bySpace.length}
      <div class="mb-3" data-testid="docs-section">
        <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
          {t('sidebar.docs')}
        </div>
        <!-- Cross-space entry point, above the per-space trees: what changed in
             the wiki lately is a question no single space answers. -->
        <button
          type="button"
          class="flex min-h-7 w-full items-center gap-1.5 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {pages.recentView
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
          aria-pressed={pages.recentView}
          title={t('sidebar.docsRecentTitle')}
          data-testid="docs-recent"
          onclick={toggleRecentDocs}
        >
          <svg
            width="10"
            height="10"
            viewBox="0 0 12 12"
            fill="none"
            aria-hidden="true"
            class="flex-none text-text-muted"
          >
            <circle cx="6" cy="6" r="4.6" stroke="currentColor" stroke-width="1.2" />
            <path d="M6 3.4V6l1.8 1.1" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" />
          </svg>
          <span class="min-w-0 flex-1 truncate">{t('sidebar.docsRecent')}</span>
        </button>
        {#each pages.treeBySpace as group (group.space)}
          {@const expanded = openSpaces.has(group.space)}
          <button
            type="button"
            class="flex min-h-7 w-full items-center gap-1.5 rounded-md px-3 py-1.5 text-left text-[13px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
            aria-expanded={expanded}
            title={t('sidebar.docsSpaceTitle', { space: group.space, n: group.count })}
            data-testid="docs-space"
            onclick={() => toggleSpace(group.space)}
          >
            <svg
              width="10"
              height="10"
              viewBox="0 0 10 10"
              fill="none"
              aria-hidden="true"
              class="flex-none text-text-muted transition-transform duration-150"
              style={expanded ? 'transform: rotate(90deg)' : ''}
            >
              <path d="M3.5 2l3 3-3 3" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
            <!-- The name is what people call the space; the key stays reachable
                 in the tooltip, and is the label itself until a name is mirrored. -->
            <span class="min-w-0 flex-1 truncate text-[12px] {group.name ? '' : 'font-mono'}">
              {group.name || group.space}
            </span>
            <span class="flex-none font-mono text-[11px] tabular-nums text-text-muted">
              {formatNumber(group.count)}
            </span>
          </button>
          {#if expanded}
            {#each group.roots as node (node.page.key)}
              {@render docNode(node)}
            {/each}
          {/if}
        {/each}
      </div>
    {/if}

    <!-- Workspaces: other profile mirrors this serve can mount. Hidden unless
         the server actually has more than one (older servers / demo → 404 → []). -->
    {#if workspaceList.length > 1}
      <div class="mb-3">
        <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
          {t('sidebar.workspaces')}
        </div>
        {#each workspaceList as w (w.name)}
          <a
            href={workspaceHref(w)}
            data-testid="workspace-link"
            class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {currentWorkspace ===
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
              <span class="max-w-[45%] flex-none truncate text-[10px] text-text-muted">
                {workspaceHost(w)}
              </span>
            {/if}
          </a>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Settings / identity area (sidebar footer) -->
  <div class="flex-none border-t border-border-subtle px-3 py-2">
    <button
      type="button"
      class="mb-1 flex w-full items-center gap-1.5 rounded-md px-1 py-1 text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={onOpenSettings}
      title={t('sidebar.serverSettings')}
    >
      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M8 10.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z" stroke="currentColor" stroke-width="1.2" />
        <path
          d="M8 1.5l.7 1.6 1.7-.5.3 1.8 1.8.3-.5 1.7 1.6.7-1.6.7.5 1.7-1.8.3-.3 1.8-1.7-.5L8 14.5l-.7-1.6-1.7.5-.3-1.8-1.8-.3.5-1.7L1.9 8l1.6-.7-.5-1.7 1.8-.3.3-1.8 1.7.5L8 1.5z"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linejoin="round"
          opacity="0.5"
        />
      </svg>
      {t('sidebar.settings')}
    </button>
    {#if me.identified}
      <div class="flex items-center gap-2 text-[12px]">
        <span class="min-w-0 flex-1 truncate text-text-secondary" title={me.email ?? undefined}>
          {me.name ?? me.email}
        </span>
        <button
          type="button"
          class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary {write.configured
            ? ''
            : 'text-status-stale'}"
          onclick={() => write.openSettings()}
          title={write.configured ? t('sidebar.jiraCreds') : t('sidebar.jiraCredsMissing')}
          aria-label={t('sidebar.jiraCreds')}
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path
              d="M8 10.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z"
              stroke="currentColor"
              stroke-width="1.2"
            />
            <path
              d="M8 1.5l.7 1.6 1.7-.5.3 1.8 1.8.3-.5 1.7 1.6.7-1.6.7.5 1.7-1.8.3-.3 1.8-1.7-.5L8 14.5l-.7-1.6-1.7.5-.3-1.8-1.8-.3.5-1.7L1.9 8l1.6-.7-.5-1.7 1.8-.3.3-1.8 1.7.5L8 1.5z"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linejoin="round"
              opacity="0.5"
            />
          </svg>
        </button>
      </div>
    {:else if isHostedDemo()}
      <!-- No credential button on the hosted demo: the dialog behind it asks for
           a real Atlassian token, and nothing on a static snapshot could use one. -->
      <p class="px-3 py-1.5 text-center text-[11px] text-text-muted">
        {t('app.demoNoCredentials')}
      </p>
    {:else if me.authChecked}
      <button
        type="button"
        class="flex w-full items-center justify-center gap-1.5 rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={() => write.openSettings()}
      >
        {t('common.setCredentials')}
      </button>
    {/if}
  </div>
</div>
