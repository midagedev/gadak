<script lang="ts">
  /*
   * The browser pane — everything you can see of an in-app page except the page.
   *
   * The page itself is a native WKWebView the app draws over this document, so
   * the one thing this component must get exactly right is the rectangle: the
   * viewport box of `viewport` below is POSTed to /desktop/browse/frame, and the
   * native view lands there. Which means the div is deliberately empty. What
   * sits behind it is the hint that fills the gap between "a tab exists" and
   * "the page has painted" — a moment the native side gives us no signal for.
   *
   * The chrome above it is the tab strip and a toolbar, in that order, because
   * the tabs are what you aim at and the toolbar is the dense band that rides
   * along: h-control for a tab, h-control-sm for a toolbar button, per the
   * two-step control grid in app.css.
   */
  import { browse, tabLabel } from '../../lib/browse.svelte'
  import { applyToastReservation } from '../../lib/browse-stack'
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { toastHostSlot } from '../write/ToastHost.svelte'

  let viewport = $state<HTMLDivElement | null>(null)

  const active = $derived(browse.activeTab)

  /** Host of the page being waited on, for the hint behind the native view. */
  const activeHost = $derived.by(() => {
    const url = active?.url
    if (!url) return ''
    try {
      return new URL(url).host
    } catch {
      return url
    }
  })

  function report(): void {
    const el = viewport
    if (!el) return
    const r = el.getBoundingClientRect()
    let next = { x: r.x, y: r.y, w: r.width, h: r.height }
    if (browse.stack.reserveToast) {
      const host = toastHostSlot.el
      if (host) {
        const t = host.getBoundingClientRect()
        next = applyToastReservation(next, { x: t.x, y: t.y, w: t.width, h: t.height })
      }
    }
    browse.reportFrame(next)
  }

  /*
   * Size changes come from the observer; position changes do not fire one at all
   * (the sidebar collapsing shifts this box without resizing it), so the poll
   * tick re-reports as well — see browse.reportFrame. Reporting the same
   * rectangle twice costs nothing: it is deduped before the request.
   */
  $effect(() => {
    const el = viewport
    if (!el) return
    const reserve = browse.stack.reserveToast
    report()
    const ro = new ResizeObserver(report)
    ro.observe(el)
    const host = reserve ? toastHostSlot.el : null
    if (host) ro.observe(host)
    window.addEventListener('resize', report)
    const tick = setInterval(report, 1000)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', report)
      clearInterval(tick)
    }
  })
</script>

<section
  class="browse-pane flex h-full min-w-0 flex-col border-l border-border-subtle bg-bg-panel"
  aria-label={t('browse.paneLabel')}
  data-testid="browse-pane"
  data-browse-yield={browse.stack.chromeYields ? 'true' : undefined}
>
  <!-- ── Tab strip ──
       h-12, which is the sidebar's first row: in the app that row is the title
       bar, so this is the band the window controls sit in and the one horizontal
       line the eye already has across the top of the screen. -->
  <div
    class="flex h-12 flex-none items-center gap-1 overflow-x-auto border-b border-border-subtle px-2"
    role="tablist"
    aria-label={t('browse.tabs')}
    data-testid="browse-tabs"
  >
    {#each browse.tabs as tab (tab.id)}
      {@const isActive = tab.id === browse.activeId}
      <div
        class="group flex h-control min-w-0 flex-none items-center gap-1.5 rounded-md pr-1 pl-2 transition-colors {isActive
          ? 'bg-bg-active text-text-primary'
          : 'text-text-secondary hover:bg-bg-hover'}"
        data-testid="browse-tab"
        data-tab-id={tab.id}
        data-active={isActive}
      >
        <button
          type="button"
          role="tab"
          aria-selected={isActive}
          onclick={() => browse.activate(tab.id)}
          class="flex min-w-0 items-center gap-1.5 text-body"
          title={tab.url}
        >
          <Icon
            name="globe"
            size={13}
            class={isActive ? 'text-accent-text' : 'text-text-muted'}
          />
          <span class="max-w-[220px] truncate">{tabLabel(tab)}</span>
        </button>
        <button
          type="button"
          onclick={() => browse.closeTab(tab.id)}
          aria-label={t('browse.closeTab')}
          title={t('browse.closeTab')}
          data-testid="browse-tab-close"
          class="flex h-5 w-5 flex-none items-center justify-center rounded text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-primary"
        >
          <Icon name="x" size={13} />
        </button>
      </div>
    {/each}
  </div>

  <!-- ── Toolbar ── -->
  <div
    class="flex h-10 flex-none items-center gap-3 border-b border-border-subtle px-2"
    data-testid="browse-toolbar"
  >
    <button
      type="button"
      onclick={() => browse.hidePane()}
      data-testid="browse-back"
      class="flex h-control-sm flex-none items-center gap-1 rounded-md border border-border-strong px-2 text-micro font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
    >
      <Icon name="arrow-left" size={13} />
      {t('browse.back')}
    </button>

    <!-- Read-only: the address is context, not a field. Nothing in the pane
         navigates by typing — links inside the page do that natively. -->
    <span
      class="min-w-0 flex-1 truncate font-mono text-micro text-text-muted"
      data-testid="browse-url"
      title={active?.url ?? ''}>{active?.url ?? ''}</span
    >

    <button
      type="button"
      onclick={() => browse.openActiveExternally()}
      aria-label={t('browse.openExternal')}
      title={t('browse.openExternal')}
      data-testid="browse-open-external"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
    >
      <Icon name="arrow-up-right" size={14} />
    </button>
  </div>

  <!-- ── The rectangle ──
       Empty on purpose: the native view is what fills it. The hint underneath
       is only ever seen in the gap before the page paints (and in a browser,
       where there is no native layer at all — which is what the E2E asserts). -->
  <div class="relative min-h-0 flex-1 bg-bg-base">
    <div
      class="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 text-body text-text-muted"
      aria-hidden="true"
    >
      <Icon name="globe" size={14} class="motion-safe:animate-pulse" />
      {t('browse.loading', { host: activeHost })}
    </div>
    <div class="absolute inset-0" bind:this={viewport} data-testid="browse-viewport"></div>
  </div>
</section>
