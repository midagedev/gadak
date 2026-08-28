<script lang="ts">
  import { fly } from 'svelte/transition'
  import { initLocale } from './lib/i18n'
  import { t } from './lib/i18n'
  import { systemBack } from './lib/back'
  import { app, boot, closeIssue, exitDemo, startClock } from './lib/store.svelte'
  import PairGate from './screens/PairGate.svelte'
  import Issues from './screens/Issues.svelte'
  import Search from './screens/Search.svelte'
  import PairingTab from './screens/PairingTab.svelte'
  import Shell from './screens/Shell.svelte'
  import Detail from './screens/Detail.svelte'
  import PageDetail from './screens/PageDetail.svelte'
  import TabBar from './ui/TabBar.svelte'

  // Vocabulary has one owner (DESIGN.md §3.6): pick the locale once, before
  // the first render, so every t() below reads the same catalog table.
  initLocale()

  // Navigation shell (DESIGN.md §2): Issues/Search/Pairing always-mounted
  // (query and scroll survive switches). The Shell tab mounts only once a
  // terminal pairing is stored (DESIGN.md §10) and boots a PTY on first
  // activation, not at app boot. One Detail push layer above them.
  $effect(() => {
    void boot()
    return startClock()
  })

  // One owner for system back (DESIGN.md §2). Sheets register themselves;
  // this bind is the only history listener in the app.
  $effect(() => {
    return systemBack.bind(
      window.history,
      window,
      () => app.detail !== null,
      closeIssue,
    )
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
  <div class="tabs">
    <div class="pane" class:off={app.tab !== 'issues'}><Issues /></div>
    <div class="pane" class:off={app.tab !== 'search'}><Search /></div>
    {#if app.terminal}
      <div class="pane" class:off={app.tab !== 'shell'}><Shell /></div>
    {/if}
    <div class="pane" class:off={app.tab !== 'pairing'}><PairingTab /></div>
  </div>
  <TabBar />
  {#if app.detail}
    <div
      class="detail-layer"
      transition:fly={{ x: reduceMotion ? 0 : 80, duration: reduceMotion ? 0 : 200, opacity: 0.4 }}
    >
      {#key `${app.detail.kind}:${app.detail.key}`}
        {#if app.detail.kind === 'issue'}
          <Detail issueKey={app.detail.key} />
        {:else}
          <PageDetail pageKey={app.detail.key} />
        {/if}
      {/key}
    </div>
  {/if}
{/if}

<style>
  .boot {
    flex: 1 1 auto;
    background: var(--color-bg-base);
  }
  .tabs {
    position: relative;
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
  }
  .demo-strip {
    flex: none;
  }
  .demo-strip + .tabs :global(.safe-top) {
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
