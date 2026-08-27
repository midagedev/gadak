<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Row from '../ui/Row.svelte'
  import DocRow from '../ui/DocRow.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import GlanceStrip from '../ui/GlanceStrip.svelte'
  import Skeleton from '../ui/Skeleton.svelte'
  import ScopeSheet from '../ui/ScopeSheet.svelte'
  import { t } from '../lib/i18n'
  import { app, issuesBootKind, setScope, showOfflineBanner, sync, switchTab } from '../lib/store.svelte'
  import {
    buildList,
    buildScopes,
    hasIdentity,
    relTime,
    resolveScope,
    scopeCount,
    scopePages,
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
