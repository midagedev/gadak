<script lang="ts">
  /*
   * The shell — A-nav (GDK-800/801/802 마감). App owns the one nav value
   * (lib/nav.ts) and the two app-lifetime lifecycles the screens must not
   * each run: the feed poll (one per app, paired-only) and the
   * notification-tap binding (banner → Detail direct, ux-report Q5).
   * Screens stay isolated — they receive callbacks and never touch nav.
   *
   * Mount discipline: Queue is always mounted (hidden while covered) so
   * its pool, cache paint, and scroll survive tab switches — Search and
   * Detail read that pool through App's mirror. Search/More remount per
   * visit (Search refocuses its input, ux-report Q4). Detail sits in a
   * {#key} so each push gets clean state. The wordmark header and the
   * tab bar hide under Detail — it carries its own back header (Q7: back
   * is a gesture + one button, not a tab).
   */
  import Queue from './screens/Queue.svelte'
  import Search from './screens/Search.svelte'
  import More from './screens/More.svelte'
  import Pair from './screens/Pair.svelte'
  import Detail from './screens/Detail.svelte'
  import { onMount } from 'svelte'
  import { t } from './lib/i18n'
  import { readPairing, readToken, type Pairing } from './lib/settings'
  import type { ApiContext, QueueRow } from './lib/api'
  import type { QueueRowFull } from './lib/queue-rows'
  import {
    NAV_HOME,
    openDetailFromNotification,
    openPair,
    openTab,
    popDetail,
    pushDetail,
    rowFor,
    type NavState,
    type NavTab,
  } from './lib/nav'
  import { startFeedPolling, type FeedPollingHandle } from './lib/feed'
  import { bindNotificationTap, ensurePermission } from './lib/notify'

  let nav = $state<NavState>(NAV_HOME)

  // The Queue's pool, mirrored downward: Search rows and Detail's status
  // row both read it (the detail payload carries no status_category).
  let pool = $state<QueueRow[]>([])

  // Feed lifecycle (A-nav): one poller per app, alive only while paired.
  let poller: FeedPollingHandle | null = null
  let feedTick = $state(0)
  let permissionAsked = false

  let pairing = $state<Pairing | null>(readPairing())

  const onTabs = $derived(nav.view === 'tabs')
  // The tab bar's highlighted tab: the tab under whatever is pushed, so
  // Pair keeps its bail-out tab lit instead of silently moving it.
  const activeTab: NavTab = $derived(
    nav.view === 'tabs' ? nav.tab : nav.view === 'pair' ? nav.back : 'queue',
  )

  function endpointOf(): string {
    const p = readPairing()
    return p?.endpoint ?? (import.meta.env.DEV ? 'http://127.0.0.1:7899' : '')
  }

  async function startPolling(): Promise<void> {
    if (poller !== null) return
    const endpoint = endpointOf()
    if (endpoint === '') return
    const token = await readToken()
    // Unpaired (or re-paired elsewhere) while the token read was in
    // flight — starting now would poll the wrong home, or none.
    if (endpointOf() !== endpoint) return
    const ctx: ApiContext = { endpoint, token }
    poller = startFeedPolling(ctx, {
      events: {
        // Each successful poll nudges the Queue to revalidate its pool
        // (the poll already fetched the feed; the queue refetch keeps
        // the two lists honest with each other).
        onfeed: () => {
          feedTick++
        },
        // Permission is asked at the first promotion (ux-report Q5),
        // not at launch — an ask before anything to show is decline bait.
        onpromoted: (items) => {
          if (!permissionAsked && items.length > 0) {
            permissionAsked = true
            void ensurePermission()
          }
        },
      },
    })
  }

  function stopPolling(): void {
    poller?.stop()
    poller = null
  }

  onMount(() => {
    // Paired: the poll starts with the app. Unpaired DEV gets it too —
    // the vite proxy rides the dev serve, same as the screens.
    if (readPairing() !== null || import.meta.env.DEV) void startPolling()
    bindNotificationTap((key) => {
      nav = openDetailFromNotification(key)
    })
  })

  function onPaired(): void {
    pairing = readPairing()
    // A re-pair is a different home: restart the poll on the new origin.
    stopPolling()
    void startPolling()
    nav = openTab(nav, 'queue')
  }

  function onUnpaired(): void {
    pairing = null
    stopPolling()
  }

  /* ── left-edge swipe = Detail back (ux-report Q7) ──
     WKWebView has no interactivePopGestureRecognizer to inherit, so the
     shell owns the gesture: a rightward drag from the left edge pops the
     pushed Detail. Off at every tab root by construction — the listener
     only lives on the Detail page. Passive listeners: the vertical pan
     (the scroller's) is never blocked. */
  const EDGE_X = 24 // the edge zone a touch must start in
  const SWIPE_DX = 56 // minimum rightward travel
  const SWIPE_DY = 40 // max drift — a diagonal scroll must not pop

  let swipeStart: { x: number; y: number } | null = null

  function onEdgeTouchStart(e: TouchEvent): void {
    const tc = e.touches[0]
    swipeStart = tc.clientX <= EDGE_X ? { x: tc.clientX, y: tc.clientY } : null
  }

  function onEdgeTouchMove(e: TouchEvent): void {
    if (swipeStart === null) return
    const tc = e.touches[0]
    if (tc.clientX - swipeStart.x >= SWIPE_DX && Math.abs(tc.clientY - swipeStart.y) <= SWIPE_DY) {
      swipeStart = null // one pop per gesture
      nav = popDetail(nav)
    }
  }

  function onEdgeTouchEnd(): void {
    swipeStart = null
  }

  const edgeSwipe = (el: HTMLElement): { destroy: () => void } => {
    const opts = { passive: true } as AddEventListenerOptions
    el.addEventListener('touchstart', onEdgeTouchStart, opts)
    el.addEventListener('touchmove', onEdgeTouchMove, opts)
    el.addEventListener('touchend', onEdgeTouchEnd, opts)
    el.addEventListener('touchcancel', onEdgeTouchEnd, opts)
    return {
      destroy: () => {
        el.removeEventListener('touchstart', onEdgeTouchStart)
        el.removeEventListener('touchmove', onEdgeTouchMove)
        el.removeEventListener('touchend', onEdgeTouchEnd)
        el.removeEventListener('touchcancel', onEdgeTouchEnd)
      },
    }
  }
</script>

<div class="m-shell">
  {#if nav.view !== 'detail'}
    <header class="m-header">
      <span class="type-subject wordmark">gadak</span>
    </header>
  {/if}

  <!-- Queue: always mounted, hidden while another view covers it — the
       pool and its cache paint must survive every tab switch. -->
  <div class="m-tabpage" hidden={!(nav.view === 'tabs' && nav.tab === 'queue')}>
    <Queue
      onpair={() => (nav = openPair(nav))}
      onopen={(issueKey) => (nav = pushDetail(nav, issueKey))}
      onpool={(rows: QueueRowFull[]) => (pool = rows)}
      feedTick={feedTick}
    />
  </div>

  {#if nav.view === 'tabs' && nav.tab === 'search'}
    <div class="m-tabpage">
      <Search rows={pool} onopen={(issue_key) => (nav = pushDetail(nav, issue_key))} />
    </div>
  {/if}

  {#if nav.view === 'tabs' && nav.tab === 'more'}
    <div class="m-tabpage">
      <More pairing={pairing} onopenpair={() => (nav = openPair(nav))} />
    </div>
  {/if}

  {#if nav.view === 'pair'}
    <div class="m-tabpage">
      <Pair onpaired={onPaired} onunpaired={onUnpaired} />
    </div>
  {/if}

  {#if nav.view === 'detail'}
    <div class="m-tabpage" use:edgeSwipe>
      {#key nav.issueKey}
        <Detail
          issueKey={nav.issueKey}
          row={rowFor(pool, nav.issueKey)}
          onback={() => (nav = popDetail(nav))}
        />
      {/key}
    </div>
  {/if}

  {#if nav.view !== 'detail'}
    <nav class="m-tabbar">
      <button
        class="m-tab"
        class:m-tab-active={activeTab === 'queue'}
        type="button"
        onclick={() => (nav = openTab(nav, 'queue'))}
        aria-current={activeTab === 'queue' ? 'page' : undefined}
      >
        {t('nav.queue')}
      </button>
      <button
        class="m-tab"
        class:m-tab-active={activeTab === 'search'}
        type="button"
        onclick={() => (nav = openTab(nav, 'search'))}
        aria-current={activeTab === 'search' ? 'page' : undefined}
      >
        {t('nav.search')}
      </button>
      <button
        class="m-tab"
        class:m-tab-active={activeTab === 'more'}
        type="button"
        onclick={() => (nav = openTab(nav, 'more'))}
        aria-current={activeTab === 'more' ? 'page' : undefined}
      >
        {t('nav.more')}
      </button>
    </nav>
  {/if}
</div>

<style>
  /* Scoped — app.css is frozen this round. One wrapper shape for every
     page so .m-main's flex sizing survives being one level down. */
  .m-tabpage {
    display: flex;
    flex: 1 1 auto;
    flex-direction: column;
    min-height: 0;
  }

  /* display:flex overrides the hidden attribute — the cover rule needs
     its own override or the Queue would keep painting under tabs. */
  .m-tabpage[hidden] {
    display: none;
  }
</style>
