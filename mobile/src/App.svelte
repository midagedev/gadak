<script lang="ts">
  import { fly } from 'svelte/transition'
  import { initLocale } from './lib/i18n'
  import { t } from './lib/i18n'
  import { browserHistory, systemBack } from './lib/back'
  import { app, boot, closeTop, exitDemo, goToList, hasBackTarget, openIssue, openPage, startClock } from './lib/store.svelte'
  import { bindOsDeepLinks, createDeepLinkRouter, exposeForTests } from './lib/deeplink-entry'
  import { bindKeyboardBand } from './lib/keyboard'
  import PairGate from './screens/PairGate.svelte'
  import Issues from './screens/Issues.svelte'
  import Settings from './screens/Settings.svelte'
  import Shell from './screens/Shell.svelte'
  import Sprints from './screens/Sprints.svelte'
  import Detail from './screens/Detail.svelte'
  import PageDetail from './screens/PageDetail.svelte'
  import ToastHost from './ui/ToastHost.svelte'

  /*
   * The detail the layer is painting, which outlives `app.detail` by the
   * length of the fly-out (GDK-1997).
   *
   * A prop in Svelte 5 is a getter, so `issueKey={app.detail.key}` is not
   * read once at render — it is read whenever the child reads `issueKey`.
   * The layer keeps its DOM for the 200ms outro after the detail closes,
   * and `screens/Detail.svelte` re-reads the prop in the continuation of
   * its own fetch (`if (key === issueKey)`, the guard that drops a response
   * for an issue the screen has moved off). Close the detail while that
   * request is still in flight and the continuation reads `.key` on null:
   * `Cannot read properties of null (reading 'key')`, an uncaught error on
   * the page. It is a race, so it surfaced as one CI failure in twenty-five
   * green runs on an unchanged tree, and reproduces every time with the
   * detail response delayed 700ms.
   *
   * Guarding the continuations is the wrong layer — there are a dozen of
   * them across two screens and the next one added would not know. This is
   * the one place the child's view of "which issue" is decided, so it is
   * the place that has to keep answering after the store has moved on: the
   * last non-null detail, held here, handed down as a plain value.
   */
  let lastDetail: { kind: 'issue' | 'page'; key: string } | null = null
  function shownDetail(): { kind: 'issue' | 'page'; key: string } {
    if (app.detail) lastDetail = { kind: app.detail.kind, key: app.detail.key }
    // Non-null by construction: every caller is inside `{#if app.detail}`,
    // so the first evaluation always happens with a detail open.
    return lastDetail as { kind: 'issue' | 'page'; key: string }
  }

  // Vocabulary has one owner (DESIGN.md §3.6): pick the locale once, before
  // the first render, so every t() below reads the same catalog table.
  initLocale()

  // Navigation shell (DESIGN.md §2, GDK-902): one column, one owner. The
  // list is always mounted (its scroll survives every excursion); the Shell
  // mounts on the latch that says it has been the owner at least once, so a
  // phone with a stored terminal pairing does not boot a PTY it was never
  // asked for — and stays mounted afterwards, hidden, so the session
  // survives a switch back. Two push layers above them, never both: the
  // Settings screen and the Detail.
  $effect(() => {
    void boot()
    return startClock()
  })

  // One owner for system back (DESIGN.md §2). Sheets register themselves;
  // this bind is the only history listener in the app. Since GDK-1970 the
  // seam is browserHistory() — window.history plus the hash read it cannot
  // make — and the bind also knows how to read the open detail and how to
  // reopen one, so the detail can be a real history entry on the hosted
  // page instead of store-only state a browser swipe walks past.
  $effect(() => {
    // The order is the entry/exit table's: detail, then a push layer, then
    // the palette. Both live in the store so the order is one statement and
    // a unit can read it (GDK-902).
    return systemBack.bind(
      browserHistory(),
      window,
      hasBackTarget,
      closeTop,
      () => app.detail,
      (kind, key) => (kind === 'issue' ? openIssue(key) : openPage(key)),
    )
  })

  // The keyboard band's one owner (GDK-1971): #app is the mount frame
  // (index.html's <div id="app">, the fixed layout viewport of DESIGN.md
  // §4.2), and this is the only place the band is written. Everything the
  // software keyboard covers reads it back as the --keyboard-inset CSS
  // variable — the scroll containers pad themselves out of the band, the
  // sheet panels lose it from their max-height — while the composers that
  // must ride ABOVE the keys keep the keyboardInset action (lib/keyboard.ts
  // owns both, one formula).
  $effect(() => {
    return bindKeyboardBand(document.getElementById('app')!)
  })

  // The detail ↔ history frame sync (GDK-1970): the store stays the owner
  // of what is open; this effect is the only thing that translates a store
  // change into a history change. Declared after the bind effect so the
  // seam exists before the first sync runs.
  $effect(() => {
    systemBack.syncDetail(app.detail)
  })

  // gadak:// deep links (GDK-873). The decision is entirely in lib/deeplink
  // — this is the connection to the OS on one side and the store on the
  // other. A link that lands on the list is the only navigation the scheme
  // can ask for; the scheme carries no verb.
  const deepLinks = createDeepLinkRouter({
    openIssue: (key) => {
      // The detail must land on the list with nothing over it: a link that
      // arrived while the palette or Settings was open would otherwise put
      // the issue on top of a surface the back gesture closes first.
      goToList()
      openIssue(key)
    },
    // A detail screen over a booting or unpaired app has nothing behind it,
    // so a cold-launch link waits here rather than pushing onto nothing.
    ready: () => app.phase === 'paired',
    // onRefused is deliberately unwired: a refusal a user should read needs
    // catalog keys in all three locales, which this round does not author
    // (see the report's string list). The refusal classes already exist and
    // are asserted in deeplink.test.ts, so wiring a toast later is one line.
  })

  $effect(() => {
    exposeForTests(deepLinks)
    let teardown: (() => void) | null = null
    void bindOsDeepLinks(deepLinks).then((off) => {
      teardown = off
    })
    return () => teardown?.()
  })

  // Release a link that arrived while the app was still booting, the moment
  // it can actually be shown. Reads app.phase, so it re-runs on the change.
  $effect(() => {
    if (app.phase === 'paired') deepLinks.flush()
  })

  // Safe-area policy (measured 2026-08-25 on the dev shell): the shell's
  // WKScrollView auto-inset anchors the 778pt layout viewport at y=0 on
  // some launches and y≈59 on others, while env() steadily reports
  // top 59 / bottom 34. The app always honors env() — in the shifted
  // dev-shell mood that wastes the inset once (cosmetic, dev only), but a
  // touch target can never land under the status bar or the home
  // indicator in either mood or in a full-bleed packaged build. The
  // structural fix is native (contentInsetAdjustmentBehavior = .never in
  // the shell) and is reported, not worked around here.

  const reduceMotion =
    typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches
</script>

{#if app.phase === 'boot'}
  <!-- Sub-100ms blank in app colors; the first real screen paints from cache. -->
  <div class="boot"></div>
{:else if app.phase === 'unpaired'}
  <PairGate />
{:else}
  <!-- Demo banner (GDK-1051): the strip pays the top inset once, so the
       screens below it drop their own (rule below) — no double gap, no
       touch target under the status bar. -->
  {#if app.demo}
    <div class="safe-top demo-strip">
      <div class="demo-banner">
        <span class="demo-label">{t('app.demoMode')}</span>
        <button class="demo-exit" onclick={exitDemo}>{t('app.demoExit')}</button>
      </div>
    </div>
  {/if}
  <div class="column">
    <div class="pane" class:off={app.owner !== 'list'}><Issues /></div>
    {#if app.terminal && app.shellEntered}
      <div class="pane" class:off={app.owner !== 'shell'}><Shell /></div>
    {/if}
    <!-- The sprint list (GDK-1827), on the shell's own latch pattern: mounted
         only once the palette has sent the column there, hidden with `.off`
         afterwards — a workspace without sprints never pays for this pane. -->
    {#if app.sprintsEntered}
      <div class="pane" class:off={app.owner !== 'sprints'}><Sprints /></div>
    {/if}
  </div>
  {#if app.layer === 'settings'}
    <!-- Same layer class and z-order as the detail, and never both: the
         store refuses to open this one over a Detail, and a Detail cannot
         be opened from here. -->
    <div
      class="detail-layer settings-layer"
      transition:fly={{ x: reduceMotion ? 0 : 80, duration: reduceMotion ? 0 : 200, opacity: 0.4 }}
    >
      <Settings />
    </div>
  {/if}
  {#if app.detail}
    {@const d = shownDetail()}
    <div
      class="detail-layer"
      transition:fly={{ x: reduceMotion ? 0 : 80, duration: reduceMotion ? 0 : 200, opacity: 0.4 }}
    >
      {#key `${d.kind}:${d.key}`}
        {#if d.kind === 'issue'}
          <Detail issueKey={d.key} />
        {:else}
          <PageDetail pageKey={d.key} />
        {/if}
      {/key}
    </div>
  {/if}
  <!-- The app-level announcer (GDK-1504): one host for every transient
       verdict, above the detail layer so a copy from a description or a
       page body is visible wherever it happened. Empty in itself. -->
  <ToastHost />
{/if}

<style>
  .boot {
    flex: 1 1 auto;
    background: var(--color-bg-base);
  }
  .column {
    position: relative;
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
  }
  .demo-strip {
    flex: none;
  }
  .demo-strip + .column :global(.safe-top) {
    /* The strip above already paid the top safe-area inset — composing
       .safe-top again in the screen header would double the gap. */
    padding-top: 0;
  }
  .demo-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 0 8px 0 16px;
    background: var(--color-bg-panel);
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .demo-label {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .demo-exit {
    color: var(--color-accent-text);
    font-weight: 600;
  }
  .pane {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .pane.off {
    display: none;
  }
  .detail-layer {
    position: absolute;
    inset: 0;
    z-index: 20;
    display: flex;
    flex-direction: column;
  }
</style>
