<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Row from '../ui/Row.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import Skeleton from '../ui/Skeleton.svelte'
  import { app, sync, switchTab } from '../lib/store.svelte'
  import { buildQueue, hasIdentity, relTime, type QueueScope } from '../lib/domain'

  let scope = $state<QueueScope>('mine')

  const view = $derived(buildQueue(app.issues, app.me, scope))
  const canScope = $derived(hasIdentity(app.me))
  const syncLabel = $derived(
    app.syncing ? 'syncing' : app.lastSyncAt ? relTime(app.lastSyncAt.toISOString(), app.now) : '—',
  )
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <h1 class="type-subject">Queue</h1>
      <span class="count">·{view.total}</span>
      <span class="spacer"></span>
      {#if canScope}
        <div class="scope" role="group" aria-label="Scope">
          <button class:on={scope === 'mine'} onclick={() => (scope = 'mine')}>Mine</button>
          <button class:on={scope === 'all'} onclick={() => (scope = 'all')}>All</button>
        </div>
      {/if}
      <button class="fresh" onclick={() => void sync()} aria-label="Sync now">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class:spin={app.syncing} aria-hidden="true">
          <path d="M21 12a9 9 0 1 1-2.6-6.3" /><path d="M21 3v6h-6" />
        </svg>
        <span>{syncLabel}</span>
      </button>
    </div>
    {#if app.offline}
      <p class="offline">Offline — showing the last synced snapshot.</p>
    {:else if view.fellBack && canScope}
      <p class="note">Nothing open is assigned to you — showing all open issues.</p>
    {:else if view.fellBack}
      <p class="note">All open issues — this serve has no identity to filter by.</p>
    {/if}
  {/snippet}

  {#if !app.loaded}
    <Skeleton />
  {:else if view.total === 0}
    <EmptyState title="The queue is clear" body="No open issues on this mirror. Enjoy it while it lasts.">
      <button class="link" onclick={() => switchTab('search')}>Search everything</button>
    </EmptyState>
  {:else}
    {#each view.sections as section (section.rank)}
      <div class="section">
        <span class="label">{section.label}</span>
        <span class="n">{section.issues.length}</span>
      </div>
      {#each section.issues as issue (issue.issue_key)}
        <Row {issue} showAssignee={view.scope === 'all'} />
      {/each}
    {/each}
    <div class="foot" aria-hidden="true"></div>
  {/if}
</Screen>

<style>
  .head {
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 12px 0 10px;
    min-width: 0;
  }
  h1 {
    margin: 0;
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
  }
  .count {
    font-family: var(--font-mono);
    font-size: var(--text-title);
    color: var(--color-text-muted);
  }
  .spacer {
    flex: 1 1 auto;
  }
  .scope {
    display: flex;
    align-self: center;
    background: var(--color-bg-panel);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    padding: 2px;
  }
  .scope button {
    min-height: 28px;
    padding: 0 10px;
    border-radius: 4px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .scope button.on {
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
    font-weight: 600;
  }
  .fresh {
    align-self: center;
    display: flex;
    align-items: center;
    gap: 4px;
    min-height: var(--spacing-control-sm);
    padding: 0 4px;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
    font-variant-numeric: tabular-nums;
  }
  .fresh svg {
    width: 14px;
    height: 14px;
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
  .foot {
    height: 24px;
  }
</style>
