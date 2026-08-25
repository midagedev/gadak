<script lang="ts">
  import { fly } from 'svelte/transition'
  import { initLocale } from './lib/i18n'
  import { app, boot, startClock } from './lib/store.svelte'
  import PairGate from './screens/PairGate.svelte'
  import Issues from './screens/Issues.svelte'
  import Search from './screens/Search.svelte'
  import PairingTab from './screens/PairingTab.svelte'
  import Detail from './screens/Detail.svelte'
  import PageDetail from './screens/PageDetail.svelte'
  import TabBar from './ui/TabBar.svelte'

  // Vocabulary has one owner (DESIGN.md §3.6): pick the locale once, before
  // the first render, so every t() below reads the same catalog table.
  initLocale()

  // Navigation shell (DESIGN.md §2): three always-mounted tabs (their query
  // and scroll state survive switches), one Detail push layer above them.
  $effect(() => {
    void boot()
    return startClock()
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
  <div class="tabs">
    <div class="pane" class:off={app.tab !== 'issues'}><Issues /></div>
    <div class="pane" class:off={app.tab !== 'search'}><Search /></div>
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
