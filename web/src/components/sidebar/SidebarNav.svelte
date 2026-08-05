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
  import { write } from '../../stores/write.svelte'
  import { runSyncNow } from '../../lib/sync-now'
  import { isHostedDemo } from '../../lib/config'
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
</script>

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

  <!-- Totals / sync — badge click = Sync now (retry path on fail/stale) -->
  <div class="flex-none px-3 pb-2 pt-1 text-[11px] text-text-muted">
    {t('sidebar.issueCount', { n: formatNumber(issues.pool.size) })}
    <span class="ml-1">·</span>
    <button
      type="button"
      class="ml-1 inline-flex items-center gap-1 rounded px-0.5 {syncColor} transition-colors hover:bg-bg-hover hover:text-text-primary"
      title={[syncTitle || syncLabel, t('sidebar.syncNowTitle')].filter(Boolean).join('\n')}
      aria-label={t('sidebar.syncNow')}
      data-testid="sidebar-sync-now"
      onclick={() => void runSyncNow('incremental')}
    >
      <span class="h-1.5 w-1.5 flex-none rounded-full {syncDot}" aria-hidden="true"></span>
      {syncLabel}
    </button>
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
