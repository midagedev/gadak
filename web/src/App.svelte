<script lang="ts">
  /*
   * App shell: 3-column layout skeleton + boot-state branching.
   *  [explore] wiring: sidebar=SidebarNav, main=ListView, right panel open=selection.
   *  Also owns selected issue ↔ URL (?issue=KEY) two-way sync (contract §2 selection).
   */
  import { onMount, untrack } from 'svelte'
  import { issues } from './stores/issues.svelte'
  import { views } from './stores/views.svelte'
  import { selection } from './stores/selection.svelte'
  import { pages } from './stores/pages.svelte'
  import { filters } from './stores/filters.svelte'
  import { me } from './stores/me.svelte'
  import { write } from './stores/write.svelte'
  import { bulk } from './stores/bulk.svelte'
  import { triage } from './stores/triage.svelte'
  import { router, setParams } from './lib/router.svelte'
  import { feature, isHostedDemo } from './lib/config'

  /** Where the demo banner sends people who want the real thing. */
  const REPO_URL = 'https://github.com/midagedev/scry'
  import {
    emptyConfig,
    parseConfig,
    VIEW_PARAM_KEYS,
    type ViewConfig,
  } from './lib/view-config'
  import { STORAGE_KEYS } from './lib/storage'
  import Sidebar from './components/shell/Sidebar.svelte'
  import MainColumn from './components/shell/MainColumn.svelte'
  import RightPanel from './components/shell/RightPanel.svelte'
  import LoadingShell from './components/shell/LoadingShell.svelte'
  import AuthGate from './components/shell/AuthGate.svelte'
  import SidebarNav from './components/sidebar/SidebarNav.svelte'
  import ListView from './components/list/ListView.svelte'
  import DetailPanel from './components/detail/DetailPanel.svelte'
  import DocumentPanel from './components/detail/DocumentPanel.svelte'
  import PersonalFeed from './components/personal/PersonalFeed.svelte'
  import DocsView from './components/docs/DocsView.svelte'
  import SpaceDocsView from './components/docs/SpaceDocsView.svelte'
  import NewIssueDialog from './components/write/NewIssueDialog.svelte'
  import QuickComment from './components/write/QuickComment.svelte'
  import JiraKeySettings from './components/write/JiraKeySettings.svelte'
  import SettingsDialog from './components/settings/SettingsDialog.svelte'
  import CommandPalette from './components/palette/CommandPalette.svelte'
  import ShortcutsDialog from './components/shell/ShortcutsDialog.svelte'
  import ToastHost from './components/write/ToastHost.svelte'
  import MediaViewer from './components/detail/MediaViewer.svelte'
  import { mediaViewer } from './stores/media-viewer.svelte'
  import { t } from './lib/i18n'

  const LAST_VIEW_KEY = STORAGE_KEYS.lastView

  /** Server settings dialog (sidebar gear). Shell-local — no need for a store. */
  let serverSettingsOpen = $state(false)
  /** Command palette (⌘K). Only opened from here, so shell-local too. */
  let paletteOpen = $state(false)
  /** Shortcut cheat sheet (?). */
  let shortcutsOpen = $state(false)
  /**
   * Delayed skeleton. IndexedDB cache hits usually finish within ~100ms, so skip
   * the skeleton then (avoids flash). index.html's inline boot shell already
   * fills the background in that gap.
   */
  let showSkeleton = $state(false)

  // Restore deep-linked issue (share / dashboard / push) before first render.
  // Otherwise the selection→URL effect can clear `issue` while selection is empty.

  const initialIssueKey = router.params.get('issue')
  if (initialIssueKey) selection.select(initialIssueKey)
  let syncedIssueKey = initialIssueKey

  onMount(() => {
    void issues.init()
    void me.init()
    void pages.init() // Sidebar DOCS section; hides itself when the mirror has none
    void write.loadWriteMeta() // Prefetch write meta (parallel with issues.init)
    views.init()

    const skeletonTimer = setTimeout(() => (showSkeleton = true), 120)
    return () => clearTimeout(skeletonTimer)
  })

  function retry() {
    void issues.refresh()
  }

  // ── Record recent issue on select ──
  //  untrack is required: recordRecent reads+writes me.recent → infinite loop if tracked.
  $effect(() => {
    const key = selection.selectedKey
    if (key) untrack(() => me.recordRecent(key))
  })

  // Mark feed events read whenever an issue is opened (list / feed / push).
  $effect(() => {
    const key = selection.selectedKey
    const identified = me.identified
    if (key && identified) untrack(() => void me.markIssueRead(key))
  })

  // ── Identity ↔ credential load/reset (sidebar ⚙︎ + write gate) ──
  $effect(() => {
    if (me.identified) {
      void write.loadCredential()
    } else if (me.authChecked) {
      write.resetCredential()
    }
  })

  // ── Global shortcuts ──
  //  ⌘K/Ctrl+K = command palette (even while a field is focused).
  //  List triage: j/k cursor, ↵ open, x select, s status, a assignee, c comment,
  //    Esc drops the selection before it closes anything.
  //  Detail open: s status / a assignee / c comment / x close.
  //  c with neither a cursor nor an open detail = new issue.
  //  One handler on purpose: the list used to run its own window listener, and
  //  two listeners racing is how Esc closed the detail *and* the selection.
  //  Keep ShortcutsDialog in sync — document only keys that have handlers.
  function onGlobalKey(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      paletteOpen = !paletteOpen
      return
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return
    const el = e.target as HTMLElement | null
    if (el) {
      const tag = el.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable) return
    }
    // Other modal layers own their keys; cheat sheet can still toggle with ?.
    if (
      write.settingsOpen ||
      write.newIssueOpen ||
      serverSettingsOpen ||
      paletteOpen ||
      triage.commentKey
    )
      return
    if (shortcutsOpen) {
      if (e.key === '?') {
        e.preventDefault()
        shortcutsOpen = false
      }
      return
    }

    const key = e.key
    const detailOpenNow = selection.selectedKey !== null
    const cursorKey = triage.listActive ? triage.cursorKey : null

    if (key === '?') {
      e.preventDefault()
      shortcutsOpen = true
      return
    }

    // ── List cursor ──
    if (triage.listActive && (key === 'j' || key === 'k')) {
      e.preventDefault()
      triage.move(key === 'j' ? 1 : -1)
      return
    }
    if (key === 'Enter' && cursorKey) {
      e.preventDefault()
      selection.select(cursorKey)
      return
    }

    // ── Esc: give back the selection first, then close panels ──
    if (key === 'Escape') {
      // An open popover is BulkBar's to close (it also hears Esc from inside its
      // own search box, which this handler never sees).
      if (triage.menu) return
      if (bulk.active) {
        e.preventDefault()
        bulk.clear()
        return
      }
      if (detailOpenNow) {
        e.preventDefault()
        selection.clear()
      }
      return
    }

    // ── x: pick the cursor row for a batch; falls back to closing panels ──
    if (key === 'x') {
      // A document keeps x first: it is the panel on screen, and s/a/c have no
      // meaning on a read-only page, so x is the only key that closes it.
      if (pages.selectedKey) {
        e.preventDefault()
        pages.clear()
        return
      }
      if (cursorKey) {
        e.preventDefault()
        bulk.toggle(cursorKey)
        return
      }
      if (detailOpenNow) {
        e.preventDefault()
        selection.clear()
      }
      return
    }

    // ── s / a: the selection wins, then the open detail, then the cursor row ──
    if (key === 's' || key === 'a') {
      const menu = key === 's' ? 'status' : 'assignee'
      if (bulk.active || (!detailOpenNow && cursorKey)) {
        e.preventDefault()
        triage.requestMenu(menu)
        return
      }
      if (detailOpenNow) {
        e.preventDefault()
        const testid = key === 's' ? 'status-transition' : 'assignee-picker'
        document.querySelector<HTMLButtonElement>(`[data-testid="${testid}"]`)?.click()
      }
      return
    }

    if (key === 'c') {
      e.preventDefault()
      if (detailOpenNow) {
        document.querySelector<HTMLTextAreaElement>('[data-testid="comment-composer"]')?.focus()
      } else if (cursorKey) {
        triage.openComment(cursorKey)
      } else {
        write.openNewIssue()
      }
    }
  }

  // ── Smart default: once. Never override URL view params. ──
  //  Priority: URL > last-used view (localStorage) > own group preset.
  //  Wait until members + auth check finish so group matching can work.
  let startupDone = false
  $effect(() => {
    if (startupDone) return
    if (!issues.ready || !me.authChecked) return
    startupDone = true
    applyStartupView()
  })

  // ── Persist last-used view (after smart default, on every view change) ──
  $effect(() => {
    const vk = filters.viewKey
    if (!startupDone) return
    try {
      localStorage.setItem(LAST_VIEW_KEY, vk)
    } catch {
      /* noop */
    }
  })

  function applyStartupView() {
    // Respect URL view params (shared link / refresh).
    if (VIEW_PARAM_KEYS.some((k) => router.params.get(k))) return

    // 1) Restore last-used view
    let last: string | null = null
    try {
      last = localStorage.getItem(LAST_VIEW_KEY)
    } catch {
      last = null
    }
    if (last) {
      filters.applyConfig(parseConfig(new URLSearchParams(last)))
      return
    }

    // 2) Group preset when taxonomy is on and identity has a group
    if (feature('teamGroups') && me.group) {
      const c: ViewConfig = emptyConfig()
      c.filters.team_group = [me.group]
      c.filters.status_category = ['new', 'inprogress']
      filters.applyConfig(c)
      return
    }

    // No identity / no group → all open issues
    const c: ViewConfig = emptyConfig()
    c.filters.status_category = ['new', 'inprogress']
    filters.applyConfig(c)
  }

  // ── Selected issue ↔ URL two-way sync ──
  // Use last-synced value to tell whether URL nav or user selection moved first.
  // Separate effects per direction let back/deeplink let the old selection
  // overwrite the new URL.
  $effect(() => {
    const urlKey = router.params.get('issue')
    const key = selection.selectedKey
    if (urlKey !== syncedIssueKey) {
      syncedIssueKey = urlKey
      if (urlKey) selection.select(urlKey)
      else selection.clear()
      return
    }
    if (key !== syncedIssueKey) {
      syncedIssueKey = key
      setParams({ issue: key }, true)
    }
  })

  // One right panel, two kinds of content. pages.select() clears the issue on the
  // way in; this closes the document on the way back so they can never stack.
  $effect(() => {
    if (selection.selectedKey) untrack(() => pages.clear())
  })

  const detailOpen = $derived(selection.selectedKey !== null)
  const docOpen = $derived(pages.selectedKey !== null)
  const panelOpen = $derived(detailOpen || docOpen)
</script>

<svelte:window onkeydown={onGlobalKey} />

{#if !issues.ready}
  {#if issues.error === 'auth'}
    <AuthGate onRetry={retry} />
  {:else if issues.error === 'network'}
    <div class="flex h-screen flex-col items-center justify-center gap-4 bg-bg-base px-6 text-center">
      <p class="max-w-sm text-[13px] text-text-secondary">
        {t('app.loadFailed')}
      </p>
      <button
        onclick={retry}
        class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('common.retry')}
      </button>
    </div>
  {:else if showSkeleton}
    <LoadingShell />
  {/if}
{:else}
  <div class="issue-shell">
    {#if isHostedDemo()}
      <!-- First thing on the page on purpose. Without it this reads as a real
           Jira client someone left signed in, which is how a visitor ends up
           looking for the credential box. -->
      <div
        class="flex flex-none flex-wrap items-center justify-center gap-x-2 gap-y-0.5 border-b border-accent-strong/40 bg-accent-strong/10 px-3 py-1.5 text-[12px] text-text-secondary"
        role="status"
        data-testid="demo-banner"
      >
        <span class="font-semibold text-accent-text">{t('app.demoBadge')}</span>
        <span>{t('app.demoBanner')}</span>
        {#if write.demoEdits.size}
          <!-- Writes are kept locally so they can be tried; the running count is
               what keeps "kept" from reading as "saved". -->
          <span class="rounded-full bg-bg-elevated px-2 py-0.5 text-text-primary">
            {t('app.demoEditCount', { n: write.demoEdits.size })}
          </span>
        {/if}
        <a
          href={REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          class="text-accent-text hover:underline">{t('app.demoBannerLink')}</a
        >
      </div>
    {/if}
    {#if issues.offline}
      <div
        class="flex flex-none items-center justify-center gap-2 border-b border-status-stale/40 bg-status-stale/10 px-3 py-1.5 text-[12px] text-status-stale"
        role="status"
        data-testid="offline-banner"
      >
        {t('app.offlineBanner')}
      </div>
    {/if}
    <div
      class="issue-layout"
      class:detail-open={panelOpen}
      data-testid="issue-layout"
      data-detail-open={panelOpen}
    >
      <Sidebar>
        {#snippet children()}
          <SidebarNav onOpenSettings={() => (serverSettingsOpen = true)} />
        {/snippet}
      </Sidebar>

      <MainColumn>
        {#snippet children()}
          <!-- The feed wins on purpose: opening it is an explicit request for
               this column, so it never has to close the docs view first. -->
          {#if me.feedOpen && feature('feed')}
            <PersonalFeed />
          {:else if pages.spaceView !== null}
            <SpaceDocsView space={pages.spaceView} />
          {:else if pages.docsView}
            <DocsView />
          {:else}
            <ListView onOpenSettings={() => (serverSettingsOpen = true)} />
          {/if}
        {/snippet}
      </MainColumn>

      <RightPanel open={panelOpen}>
        {#snippet children()}
          <DetailPanel />
          <DocumentPanel />
        {/snippet}
      </RightPanel>
    </div>
  </div>
{/if}

{#if write.settingsOpen}
  <JiraKeySettings />
{/if}

{#if write.newIssueOpen}
  <NewIssueDialog />
{/if}

{#if triage.commentKey}
  <QuickComment issueKey={triage.commentKey} onclose={() => triage.closeComment()} />
{/if}

{#if serverSettingsOpen}
  <SettingsDialog onclose={() => (serverSettingsOpen = false)} />
{/if}

{#if shortcutsOpen}
  <ShortcutsDialog onclose={() => (shortcutsOpen = false)} />
{/if}

{#if paletteOpen}
  <CommandPalette
    onclose={() => (paletteOpen = false)}
    onOpenSettings={() => (serverSettingsOpen = true)}
  />
{/if}

<!-- Toast host (bottom-right) — always mounted -->
<ToastHost />

{#if mediaViewer.attachment}
  <MediaViewer attachment={mediaViewer.attachment} onClose={() => mediaViewer.close()} />
{/if}
