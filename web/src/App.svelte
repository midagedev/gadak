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
  import { person } from './stores/person.svelte'
  import { panel } from './stores/panel.svelte'
  import { filters } from './stores/filters.svelte'
  import { me } from './stores/me.svelte'
  import { write } from './stores/write.svelte'
  import { bulk } from './stores/bulk.svelte'
  import { triage } from './stores/triage.svelte'
  import { router } from './lib/router.svelte'
  import { bindParam, bindParams } from './lib/url-sync.svelte'
  import { feature, isHostedDemo } from './lib/config'
  import { takeUIFocus } from './lib/api'
  import { showIssueList } from './lib/show-issue-list'
  import { adoptRunningSync } from './lib/sync-now'
  import { installDesktopLinkOpener } from './lib/desktop-links'
  import { browse, installBrowseSessions } from './lib/browse.svelte'

  /** Where the demo banner sends people who want the real thing. */
  const REPO_URL = 'https://github.com/midagedev/gadak'
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
  import PersonPanel from './components/detail/PersonPanel.svelte'
  import PersonalFeed from './components/personal/PersonalFeed.svelte'
  import DocsView from './components/docs/DocsView.svelte'
  import SpaceDocsView from './components/docs/SpaceDocsView.svelte'
  import HistoryView from './components/history/HistoryView.svelte'
  import NewIssueDialog from './components/write/NewIssueDialog.svelte'
  import QuickComment from './components/write/QuickComment.svelte'
  import JiraKeySettings from './components/write/JiraKeySettings.svelte'
  import SettingsDialog from './components/settings/SettingsDialog.svelte'
  import CommandPalette from './components/palette/CommandPalette.svelte'
  import ShortcutsDialog from './components/shell/ShortcutsDialog.svelte'
  import ToastHost from './components/write/ToastHost.svelte'
  import MediaViewer from './components/detail/MediaViewer.svelte'
  import BrowseHost from './components/browse/BrowseHost.svelte'
  import { mediaViewer } from './stores/media-viewer.svelte'
  import { t } from './lib/i18n'
  import { bindPaletteOpener } from './lib/unified-search'

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

  /*
   * The document screens, restored the same way.
   *
   * Everything a document view knows lived in memory, so a reload landed back
   * on the issue list and a link to a page could not be shared at all — while
   * the issue beside it had survived both since the first release. These four
   * params are selection state, never view state: `isViewParam` does not list
   * them, so the sidebar's active-view match and the saved-view serialization
   * read exactly what they read before.
   *
   * `dview` is written only for the tree, since the flat list is the default
   * and a URL should not carry a value that changes nothing. The tab is
   * deliberately not here either: it is a return path this browser remembers
   * (localStorage), not something a link should impose on the person opening it.
   */
  const initialDocKey = router.params.get('doc')
  if (initialDocKey) pages.select(initialDocKey)

  const initialSpace = router.params.get('space')
  if (initialSpace) {
    pages.openSpace(initialSpace)
    if (router.params.get('dview') === 'tree') pages.spaceTree = true
  } else if (router.params.get('docs') === '1') {
    pages.docsView = true
  } else if (router.params.get('hist') === '1') {
    pages.historyView = true
  } else if (initialDocKey) {
    /*
     * A link to a page with nothing behind it lands on the document screen, not
     * on the issue list with a page floating over it. What arrived was one
     * address, and everything it restores has to belong to the same place: a
     * panel over a list nobody asked for offers a Close that leads somewhere
     * the visitor has never been.
     *
     * Only here, on the way in from a URL. Opening a page from a search hit in
     * the issue list is the opposite request — the list is being worked
     * through, several hits at a time, and it stays put (see DocsView's header).
     * That flow writes `doc` alone; this one is what a `doc` alone means when
     * it is all the app was given.
     */
    pages.docsView = true
  }
  // The promotion above is a change the *store* made, and the bindings below
  // are what carry it out to the URL — they seed themselves from the URL for
  // exactly that reason (see lib/url-sync). It happens here, once, on the way
  // in: nothing after this turns a bare `?doc=` into a document screen.

  onMount(() => {
    const unbindPalette = bindPaletteOpener(() => {
      paletteOpen = true
    })
    void issues.init()
    void me.init()
    void pages.init() // Sidebar DOCS section; hides itself when the mirror has none
    void adoptRunningSync() // A sync the server started (settings write, CLI) is still ours to show
    void write.loadWriteMeta() // Prefetch write meta (parallel with issues.init)
    views.init()

    const skeletonTimer = setTimeout(() => (showSkeleton = true), 120)
    // Desktop only: external links + in-app browse session tracking.
    const uninstallLinks = installDesktopLinkOpener()
    const uninstallBrowse = installBrowseSessions()
    const applyFocus = async () => {
      if (isHostedDemo()) return
      try {
        const hash = await takeUIFocus()
        if (!hash) return
        const q = hash.startsWith('#/?')
          ? hash.slice(3)
          : hash.startsWith('?')
            ? hash.slice(1)
            : hash
        // Column latches first (showIssueList → applyConfig replaceState).
        // Then the CLI's literal hash, so ks=A,B stays unescaped. Do not
        // replaceState+hashchange here: that re-parses location.hash before
        // applyConfig's router write and can drop the focus view.
        showIssueList(parseConfig(new URLSearchParams(q)))
        const next = q ? `#/?${q}` : '#/'
        if (location.hash !== next) location.hash = next
      } catch {
        /* serve without the endpoint, or offline */
      }
    }
    let focusTimer: ReturnType<typeof setInterval> | null = null
    const markFocusPoll = (on: boolean) => {
      document.documentElement.dataset.uiFocusPoll = on ? 'on' : 'off'
    }
    const startFocusPoll = () => {
      if (focusTimer !== null) return
      focusTimer = setInterval(() => void applyFocus(), 500)
      markFocusPoll(true)
    }
    const stopFocusPoll = () => {
      if (focusTimer === null) return
      clearInterval(focusTimer)
      focusTimer = null
      markFocusPoll(false)
    }
    const onVis = () => {
      if (document.visibilityState === 'visible') {
        void applyFocus()
        startFocusPoll()
      } else {
        stopFocusPoll()
      }
    }
    if (document.visibilityState === 'visible') {
      void applyFocus()
      startFocusPoll()
    } else {
      markFocusPoll(false)
    }
    document.addEventListener('visibilitychange', onVis)
    return () => {
      unbindPalette()
      clearTimeout(skeletonTimer)
      stopFocusPoll()
      document.removeEventListener('visibilitychange', onVis)
      uninstallLinks()
      uninstallBrowse()
    }
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
  //  List triage: j/k cursor, ↵ open, x select, s status, a assignee, l labels,
  //    c comment, Esc drops the selection before it closes anything.
  //  Detail open: s status / a assignee / l labels / c comment / x close.
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

    // ── / : narrow whatever is in the main column ──
    //  One key, one meaning: go to the field that narrows this screen. Which
    //  field that is belongs to the screen, so this asks the main column rather
    //  than letting each list bind its own listener (the list used to, which is
    //  why `/` did nothing on a document screen).
    if (key === '/') {
      const testid = pages.historyView
        ? 'history-filter-input'
        : pages.open
          ? 'docs-filter-input'
          : 'search-input'
      // The feed has nothing to narrow — no field, no key.
      const field = me.feedOpen && feature('feed')
        ? null
        : document.querySelector<HTMLInputElement>(`[data-testid="${testid}"]`)
      if (field) {
        e.preventDefault()
        field.focus()
      }
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
      // Browsing: Esc is the way back to Gadak, same as the toolbar button. Only
      // reachable while the SPA has focus — with the page focused the key goes
      // to the native view and never arrives here.
      if (browse.paneOpen) {
        e.preventDefault()
        browse.hidePane()
        return
      }
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
      // A document keeps x first: it is the panel on screen, and s/a/l/c have no
      // meaning on a read-only page, so x is the only key that closes it. A
      // person reads the same way — nothing on that panel is triageable.
      if (pages.selectedKey) {
        e.preventDefault()
        pages.clear()
        return
      }
      if (person.selectedEmail) {
        e.preventDefault()
        person.clear()
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

    // ── s / a / l: the selection wins, then the open detail, then the cursor row ──
    if (key === 's' || key === 'a' || key === 'l') {
      const menu = key === 's' ? 'status' : key === 'a' ? 'assignee' : 'labels'
      if (bulk.active || (!detailOpenNow && cursorKey)) {
        e.preventDefault()
        triage.requestMenu(menu)
        return
      }
      if (detailOpenNow) {
        e.preventDefault()
        if (key === 'l') {
          const field = document.querySelector<HTMLInputElement>('[data-testid="label-editor-input"]')
          if (field) field.focus()
          else document.querySelector<HTMLButtonElement>('[data-testid="label-editor-add"]')?.click()
          return
        }
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

  /*
   * ── State ↔ URL ──
   *
   * Which state each param mirrors, and nothing about how the two are kept
   * level: that protocol — who moved first, and therefore who follows — lives
   * once in lib/url-sync, where it also owns the last-synced value it needs.
   */

  bindParam({
    param: 'issue',
    read: () => selection.selectedKey,
    write: (key) => (key ? selection.select(key) : selection.clear()),
  })

  bindParam({
    param: 'doc',
    read: () => pages.selectedKey,
    write: (key) => (key ? pages.select(key) : pages.clear()),
  })

  // Which document screen owns the main column. One binding rather than three:
  // a space and the tabbed view are exclusive and `dview` only means anything
  // inside a space, so a pass that saw them one at a time would be reading a
  // half-applied screen.
  bindParams({
    params: ['space', 'docs', 'dview', 'hist'],
    read: () => ({
      space: pages.spaceView,
      docs: pages.docsView ? '1' : null,
      dview: pages.spaceTree ? 'tree' : null,
      hist: pages.historyView ? '1' : null,
    }),
    write: ({ space, docs, dview, hist }) => {
      if (space) {
        pages.openSpace(space)
        pages.spaceTree = dview === 'tree'
      } else if (docs === '1') {
        pages.spaceView = null
        pages.spaceTree = false
        pages.historyView = false
        pages.docsView = true
      } else if (hist === '1') {
        pages.spaceView = null
        pages.spaceTree = false
        pages.docsView = false
        pages.historyView = true
      } else {
        pages.closeDocs()
      }
    },
  })

  // One right panel, three kinds of content — and one value holding which
  // (stores/panel). Opening any of them is what closes the other two, so there
  // is nothing here to keep them from stacking.
  const panelOpen = $derived(panel.target !== null)

  /*
   * ── The in-app browser pane (desktop app only) ──
   *
   * `browseEnabled` is read once: it is config, not state, and everything the
   * pane needs is behind it — no component, no listener, no /desktop request in
   * a `gadak serve` tab.
   *
   * The effect below is the whole reason the shell knows about browsing at all.
   * An Atlassian page renders in a native view the app draws *over* this
   * document, so every SPA surface that covers the screen — palette, dialog,
   * media viewer — would open underneath the page instead of over it. Each of
   * those flags already lives here, in one place, which is the only place the
   * question "is something covering the app right now" can be answered.
   */
  const browseEnabled = browse.enabled

  $effect(() => {
    if (!browseEnabled) return
    browse.setOverlayOpen(
      write.settingsOpen ||
        write.newIssueOpen ||
        serverSettingsOpen ||
        paletteOpen ||
        shortcutsOpen ||
        triage.commentKey !== null ||
        mediaViewer.attachment !== null,
    )
  })
</script>

<svelte:window onkeydown={onGlobalKey} />

{#if !issues.ready}
  {#if issues.error === 'auth'}
    <AuthGate onRetry={retry} />
  {:else if issues.error === 'network'}
    <div class="flex h-screen flex-col items-center justify-center gap-4 bg-bg-base px-6 text-center">
      <p class="max-w-sm text-body text-text-secondary">
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
      class:browse-open={browse.paneOpen}
      data-testid="issue-layout"
      data-detail-open={panelOpen}
      data-browse-open={browse.paneOpen}
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
          {:else if pages.historyView}
            <HistoryView />
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
          <PersonPanel />
        {/snippet}
      </RightPanel>

      <!-- Over the detail area: an original page is what you asked to see
           *instead of* Gadak's copy, and the copy is still there when the pane
           steps away. Nothing at all in a browser tab. -->
      {#if browseEnabled}
        <BrowseHost />
      {/if}
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
