<script lang="ts">
  /*
   * App shell: 3-column layout skeleton + boot-state branching.
   *  [explore] wiring: sidebar=SidebarNav, main=ListView, right panel open=selection.
   *  Also owns selected issue ↔ URL (?issue=KEY) two-way sync (contract §2 selection).
   */
  import { onMount, untrack } from 'svelte'
  import { issues } from './stores/issues.svelte'
  import { reachability } from './lib/reachability.svelte'
  import { views } from './stores/views.svelte'
  import { selection } from './stores/selection.svelte'
  import { pages } from './stores/pages.svelte'
  import { person } from './stores/person.svelte'
  import { panel } from './stores/panel.svelte'
  import { filters } from './stores/filters.svelte'
  import { me, type FeedFocus } from './stores/me.svelte'
  import { write } from './stores/write.svelte'
  import { bulk } from './stores/bulk.svelte'
  import { triage } from './stores/triage.svelte'
  import { dashboards } from './stores/dashboards.svelte'
  import { router } from './lib/router.svelte'
  import { bindParam, bindParams } from './lib/url-sync.svelte'
  import { createGlobalKeyHandler } from './lib/keymap.svelte'
  import { applyStartupView, readLastViewKey } from './lib/startup-view'
  import { feature, hasServerVerb, isHostedDemo } from './lib/config'
  import { takeUIFocus } from './lib/api'
  import { showIssueList } from './lib/show-issue-list'
  import { adoptRunningSync } from './lib/sync-now'
  import { installDesktopLinkOpener, openIssueOrigin, openOriginUrl } from './lib/desktop-links'
  import { browse, installBrowseSessions } from './lib/browse.svelte'
  import { createSkeletonGrace } from './lib/skeleton-grace.svelte'

  /** Where the demo banner sends people who want the real thing. */
  const REPO_URL = 'https://github.com/midagedev/gadak'
  import { parseView, VIEW_PARAM_KEYS } from './lib/view-config'
  import { builtinViews } from './lib/builtin-views'
  import { STORAGE_KEYS } from './lib/storage'
  import { hydrateThemeFromServer } from './lib/theme'
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
  import DashboardView from './components/dashboard/DashboardView.svelte'
  import SpaceDocsView from './components/docs/SpaceDocsView.svelte'
  import HistoryView from './components/history/HistoryView.svelte'
  import NewIssueDialog from './components/write/NewIssueDialog.svelte'
  import QuickComment from './components/write/QuickComment.svelte'
  import JiraKeySettings from './components/write/JiraKeySettings.svelte'
  import SettingsDialog, { isSettingsTab, type Tab } from './components/settings/SettingsDialog.svelte'
  import CommandPalette from './components/palette/CommandPalette.svelte'
  import ShortcutsDialog from './components/shell/ShortcutsDialog.svelte'
  import HostedLinks from './components/shell/HostedLinks.svelte'
  import ToastHost from './components/write/ToastHost.svelte'
  import MediaViewer from './components/detail/MediaViewer.svelte'
  import BrowseHost from './components/browse/BrowseHost.svelte'
  import { mediaViewer } from './stores/media-viewer.svelte'
  import { t } from './lib/i18n'
  import { bindPaletteOpener, bindShortcutsOpener } from './lib/unified-search'
  import {
    applyOverlayChrome,
    isOverlayModal,
    layoutTokenStyle,
    readViewportRegime,
    subscribeViewportRegime,
    type ViewportRegime,
  } from './lib/viewport-regime'

  const LAST_VIEW_KEY = STORAGE_KEYS.lastView

  /** Server settings dialog (sidebar gear). Shell-local — no need for a store.
   *  The tab is lifted out of the dialog so the `settings=` place binding
   *  (lib/url-state) can read and set it; closing resets it, which is what
   *  the dialog's unmount used to do for free — every open starts on `sync`. */
  let serverSettingsOpen = $state(false)
  let serverSettingsTab = $state<Tab>('sync')

  /** Every close path — Esc, backdrop, the URL losing `settings=` — resets the
   *  tab with it. */
  function closeServerSettings(): void {
    serverSettingsOpen = false
    serverSettingsTab = 'sync'
  }
  /** Command palette (⌘K). Only opened from here, so shell-local too. */
  let paletteOpen = $state(false)
  /** Shortcut cheat sheet (?). */
  let shortcutsOpen = $state(false)
  /**
   * Delayed skeleton via the shared grace owner. IndexedDB cache hits usually
   * finish inside the window, so skip the skeleton then (avoids flash).
   * index.html's inline boot shell already fills the background in that gap.
   * Auth / network errors paint immediately — they are not a load still in
   * flight.
   */
  const bootSkeleton = createSkeletonGrace(
    () => !issues.ready && issues.error !== 'auth' && issues.error !== 'network',
  )

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

  /*
   * The three surfaces that stayed memory-only while `issue` and `doc` grew
   * links: the third right-panel kind, the personal feed, the settings
   * dialog. One pattern for all three — presence means open, the value says
   * which — and the restore-before-bind rule above applies to them for the
   * same reason: a binding's first pass would otherwise read the un-opened
   * state as "the URL dropped the param" and erase it.
   */
  const initialPerson = router.params.get('person')
  if (initialPerson) person.select(initialPerson)

  // Agent dashboards (GDK-782) — same restore-before-bind rule: the binding's
  // first pass would otherwise read the un-opened store as "no dash param" and
  // erase it before the first render.
  const initialDash = router.params.get('dash')
  if (initialDash) dashboards.open(initialDash)

  /** The feed's focus slices (FeedFocus), for validating an incoming `feed=`
   *  value. PersonalFeed keeps its own labelled copy for its tabs; only this
   *  binding needs the bare closed list. */
  const FEED_FOCUSES: readonly FeedFocus[] = ['all', 'assignee', 'reporter', 'mention']
  const isFeedFocus = (v: string): v is FeedFocus => (FEED_FOCUSES as readonly string[]).includes(v)

  // The feed and the settings dialog are registered only where the surface
  // exists. On a deployment that cannot answer, an arriving param is left
  // alone in the URL — neither honored nor erased (a feed-less deployment has
  // no feed screen; the hosted snapshot has no settings server to edit, the
  // same rule the render guard on the dialog already keeps).
  if (feature('feed')) {
    const focus = router.params.get('feed')
    if (focus !== null) me.openFeed(isFeedFocus(focus) ? focus : 'all')
  }

  if (hasServerVerb('settings')) {
    const settingsTab = router.params.get('settings')
    if (settingsTab !== null) {
      serverSettingsOpen = true
      serverSettingsTab = isSettingsTab(settingsTab) ? settingsTab : 'sync'
    }
  }

  onMount(() => {
    const unbindPalette = bindPaletteOpener(() => {
      paletteOpen = true
    })
    const unbindShortcuts = bindShortcutsOpener(() => {
      shortcutsOpen = true
    })
    void hydrateThemeFromServer()
    void issues.init()
    void me.init()
    void pages.init() // Sidebar DOCS section; hides itself when the mirror has none
    dashboards.init() // Sidebar dashboards section; empty list hides it
    void adoptRunningSync() // A sync the server started (settings write, CLI) is still ours to show
    void write.loadWriteMeta() // Prefetch write meta (parallel with issues.init)
    views.init()

    // Desktop only: external links + in-app browse session tracking.
    const uninstallLinks = installDesktopLinkOpener()
    const uninstallBrowse = installBrowseSessions()
    const unsubRegime = subscribeViewportRegime((r) => {
      viewportRegime = r
    })
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
        // A dash= focus opens the dashboard and stops there: parseView on a
        // dash-only query would hand the list a default config nobody chose,
        // and `dashboards open` has no opinion about the issue list at all.
        const focusParams = new URLSearchParams(q)
        const dashId = focusParams.get('dash')
        if (dashId) {
          me.closeFeed()
          dashboards.open(dashId)
          const nextDash = q ? `#/?${q}` : '#/'
          if (location.hash !== nextDash) location.hash = nextDash
          return
        }
        // Column latches first (showIssueList → applyConfig replaceState).
        // Then the CLI's literal hash, so ks=A,B stays unescaped. Do not
        // replaceState+hashchange here: that re-parses location.hash before
        // applyConfig's router write and can drop the focus view.
        const focused = parseView(new URLSearchParams(q))
        showIssueList(focused.config)
        filters.notifyKeysCapped(focused.keys)
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
      focusTimer = setInterval(() => {
        void applyFocus()
        // Live authoring updates (GDK-793): the same 500ms tick that serves
        // `views open` also watches the dashboards change counter — only
        // while one is open, so a closed dashboard costs nothing.
        void dashboards.checkVersion()
      }, 500)
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
      unbindShortcuts()
      stopFocusPoll()
      document.removeEventListener('visibilitychange', onVis)
      uninstallLinks()
      uninstallBrowse()
      unsubRegime()
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
  //  One <svelte:window> listener on purpose: the list used to run its own,
  //  and two listeners racing is how Esc closed the detail *and* the selection.
  //  Resolution lives in lib/keymap.svelte.ts.
  const onGlobalKey = createGlobalKeyHandler({
    get paletteOpen() {
      return paletteOpen
    },
    set paletteOpen(v) {
      paletteOpen = v
    },
    get shortcutsOpen() {
      return shortcutsOpen
    },
    set shortcutsOpen(v) {
      shortcutsOpen = v
    },
    get mediaViewerOpen() {
      return mediaViewer.attachment !== null
    },
    get serverSettingsOpen() {
      return serverSettingsOpen
    },
    set serverSettingsOpen(v) {
      if (v) serverSettingsOpen = true
      else closeServerSettings()
    },
    write,
    triage,
    selection,
    pages,
    person,
    bulk,
    browse,
    me,
    feature,
    openOrigin(target) {
      if (target === 'page') {
        const key = pages.selectedKey
        if (!key) return
        const row = pages.lite(key) ?? pages.searchHits.find((p) => p.key === key)
        openOriginUrl(row?.url)
        return
      }
      const issueKey = (triage.listActive ? triage.cursorKey : null) ?? selection.selectedKey
      if (issueKey) openIssueOrigin(issueKey)
    },
  })

  // Inspectable next to cacheScope / uiFocusPoll: was the list ready for keys?
  $effect(() => {
    document.documentElement.dataset.keysReady = triage.keysReady ? 'true' : 'false'
    document.documentElement.dataset.cursorKey = triage.cursorKey ?? ''
  })

  // ── Smart default: once. Never override URL view params. ──
  //  Priority: URL > last-used view (localStorage) > own group preset.
  //  Hosted demo only: Epic breakdown instead of all-open (see applyStartupView).
  //  Wait until members + auth check finish so group matching can work.
  let startupDone = false
  $effect(() => {
    if (startupDone) return
    if (!issues.ready || !me.authChecked) return
    startupDone = true
    const startupInput = {
      urlHasViewParam: VIEW_PARAM_KEYS.some((k) => router.params.get(k)),
      // Dual-gate so gadak serve / the desktop app keep the existing default
      // even if config.json is wrong: VITE_HOSTED_DEMO is compile-time;
      // isHostedDemo() is the runtime config.json flag.
      hostedDemo: import.meta.env.VITE_HOSTED_DEMO === '1' && isHostedDemo(),
      epicBreakdown: builtinViews().find((v) => v.id === 'epic-breakdown')?.config,
      lastViewKey: readLastViewKey(LAST_VIEW_KEY),
      teamGroupEnabled: feature('teamGroups'),
      group: me.group,
    }
    applyStartupView(startupInput, (c) => {
      filters.applyConfig(c)
      filters.setViewOrigin(c.filters)
    })
    filters.latchOriginFromBuiltins()
    // End of boot view writes. Future boot sources must run *before* this
    // line — after it, the list becomes a keyboard target and a later hash
    // write is a user view change (resets the cursor).
    triage.noteStartupViewApplied()
    // lastViewKey apply writes the capped ks; recover given from the stored string.
    if (
      !startupInput.urlHasViewParam &&
      !(startupInput.hostedDemo && startupInput.epicBreakdown) &&
      startupInput.lastViewKey
    ) {
      filters.notifyKeysCapped(parseView(new URLSearchParams(startupInput.lastViewKey)).keys)
    }
  })

  // GDK-35: one toast per distinct truncated ks. viewKey (not render) is the
  // signal — typing q= re-runs this but notifyKeysCapped de-dupes the same list.
  $effect(() => {
    void filters.viewKey
    filters.notifyKeysCapped(filters.keysNormalization)
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

  // Engine-internal clears (applyConfig / clearAll / emptied q) drop page hits
  // without the filter engine importing the pages store.
  $effect(() => {
    if (!filters.serverMatchQuery) untrack(() => pages.clearSearchHits())
  })

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
   * GDK-201: docked | overlay is owned by viewport-regime.ts. Scrim, inert,
   * dialog role, and focus trap all derive from overlayModal — CSS must not
   * independently decide to cover the list.
   */
  let viewportRegime = $state<ViewportRegime>(readViewportRegime())
  const overlayModal = $derived(isOverlayModal(viewportRegime, panelOpen))
  let layoutEl = $state<HTMLElement | null>(null)

  $effect(() => {
    const el = layoutEl
    const modal = overlayModal
    if (!el) return
    return applyOverlayChrome(el, modal)
  })

  function closeOpenPanel(): void {
    const t = panel.target
    if (t) panel.close(t.kind)
  }

  // The third right-panel kind, in the shape of the two above it: the value is
  // the identity (account id or email — stores/person carries it opaquely),
  // absence is the closed panel.
  bindParam({
    param: 'person',
    read: () => person.selectedEmail,
    write: (identity) => (identity ? person.select(identity) : person.clear()),
  })

  // The feed as a main-column screen. An unrecognized focus falls back to
  // 'all' — openFeed's own default — rather than being rejected: the link can
  // still name the place when this build no longer knows the slice, and a
  // param that opens nothing reads as a broken link.
  if (feature('feed')) {
    bindParam({
      param: 'feed',
      read: () => (me.feedOpen ? me.feedFocus : null),
      write: (v) => (v === null ? me.closeFeed() : me.openFeed(isFeedFocus(v) ? v : 'all')),
    })
  }

  // The settings dialog. An unknown tab lands on 'sync', not a blank dialog —
  // the tab list will grow, and a link from before a rename must keep opening
  // something real. Registered only where the settings verb exists, so the
  // hosted snapshot neither opens nor rewrites a `settings=` param.
  if (hasServerVerb('settings')) {
    bindParam({
      param: 'settings',
      read: () => (serverSettingsOpen ? serverSettingsTab : null),
      write: (v) => {
        if (v === null) {
          closeServerSettings()
        } else {
          serverSettingsOpen = true
          serverSettingsTab = isSettingsTab(v) ? v : 'sync'
        }
      },
    })
  }

  // The open dashboard (GDK-782): value is the id, absence is the list. A
  // `dash=` link must work wherever the sidebar row would — same restore-
  // before-bind treatment as `doc`/`person` above (the initial open runs
  // before this binding first reads).
  bindParam({
    param: 'dash',
    read: () => dashboards.openId,
    write: (id) => (id ? dashboards.open(id) : dashboards.close()),
  })

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
   * those flags already lives here, in one place; `browse.setSurface` is the
   * only writer, and `resolveBrowseStack` is the only decision.
   */
  const browseEnabled = browse.enabled

  $effect(() => {
    if (!browseEnabled) return
    browse.setSurface({
      dialogOpen:
        write.settingsOpen ||
        write.newIssueOpen ||
        serverSettingsOpen ||
        paletteOpen ||
        shortcutsOpen ||
        triage.commentKey !== null ||
        mediaViewer.attachment !== null,
      toastVisible: write.toasts.length > 0,
    })
  })

  // Inspectable next to keysReady: which layer of the browse stack is up.
  $effect(() => {
    const s = browse.stack
    document.documentElement.dataset.browseNative = s.nativeVisible ? 'on' : 'off'
    document.documentElement.dataset.browseYield = s.chromeYields ? 'on' : 'off'
    document.documentElement.dataset.browseToastReserve = s.reserveToast ? 'on' : 'off'
  })

  // Boot has no persistent host during the grace (index.html already fills
  // the gap). Same inspectable-dataset idiom as uiFocusPoll / browseNative.
  $effect(() => {
    const a = bootSkeleton.attr
    if (a === undefined) delete document.documentElement.dataset.skeleton
    else document.documentElement.dataset.skeleton = a
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
        class="rounded-md border border-border-strong px-3 py-1.5 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('common.retry')}
      </button>
    </div>
  {:else if bootSkeleton.visible}
    <LoadingShell />
  {/if}
{:else}
  <div class="issue-shell">
    {#if isHostedDemo()}
      <!-- First thing on the page on purpose. Without it this reads as a real
           Jira client someone left signed in, which is how a visitor ends up
           looking for the credential box. HostedLinks is a flex item of this
           banner (GDK-766): absolute right-3 top-0 sat on the CTA at 800px.
           Copy takes leftover space and wraps; the links keep their width. -->
      <div
        class="flex flex-none flex-wrap items-center gap-x-2 gap-y-0.5 border-b border-accent-strong/40 bg-accent-strong/10 px-3 py-1.5 text-body text-text-secondary"
        role="status"
        data-testid="demo-banner"
      >
        <span class="flex min-w-0 flex-1 flex-wrap items-center justify-center gap-x-2 gap-y-0.5">
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
        </span>
        <HostedLinks />
      </div>
    {/if}
    {#if reachability.offline}
      <div
        class="flex flex-none items-center justify-center gap-2 border-b border-status-stale/40 bg-status-stale/10 px-3 py-1.5 text-body text-status-stale"
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
      data-viewport-regime={viewportRegime}
      style={layoutTokenStyle()}
      bind:this={layoutEl}
    >
      <Sidebar>
        {#snippet children()}
          <SidebarNav onOpenSettings={() => (serverSettingsOpen = true)} />
        {/snippet}
      </Sidebar>

      <MainColumn>
        {#snippet children()}
          <!-- The feed wins on purpose: opening it is an explicit request for
               this column, so it never has to close the docs view first. A
               dashboard sits between the feed and the docs screens: it is a
               full-column surface like they are, but one the feed must give
               up for (applyFocus closes it), because `dashboards open` asks
               for this column by name. -->
          {#if me.feedOpen && feature('feed')}
            <PersonalFeed />
          {:else if dashboards.openId !== null}
            <DashboardView />
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

      <!-- Overlay-regime scrim (GDK-201): only when overlayModal is true.
           Click closes; pointer-events are live. Docked has nothing to cover. -->
      <div
        class="issue-scrim"
        class:is-open={overlayModal}
        data-testid="issue-scrim"
        aria-hidden="true"
        onclick={overlayModal ? closeOpenPanel : undefined}
      ></div>

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

<!-- Second line of defense behind the absent entry point: even a keyboard
     shortcut or deep link cannot mount the server settings dialog where no
     server exists to edit (it 404s on load and renders an error screen). -->
{#if serverSettingsOpen && hasServerVerb('settings')}
  <SettingsDialog onclose={closeServerSettings} bind:tab={serverSettingsTab} />
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
