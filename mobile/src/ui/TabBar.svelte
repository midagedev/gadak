<script lang="ts">
  import { app, switchTab, type Tab } from '../lib/store.svelte'

  const tabs: { id: Tab; label: string }[] = [
    { id: 'queue', label: 'Queue' },
    { id: 'search', label: 'Search' },
    { id: 'pairing', label: 'Pairing' },
  ]
</script>

<nav class="safe-bottom" aria-label="Tabs">
  {#each tabs as t (t.id)}
    <button
      class="tab"
      class:active={app.tab === t.id}
      aria-current={app.tab === t.id ? 'page' : undefined}
      onclick={() => switchTab(t.id)}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        {#if t.id === 'queue'}
          <path d="M4 6h16M4 12h16M4 18h10" />
        {:else if t.id === 'search'}
          <circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
        {:else}
          <path d="M10 13.5a5 5 0 0 0 7.1 0l2.4-2.4a5 5 0 0 0-7.1-7.1l-1.3 1.3" />
          <path d="M14 10.5a5 5 0 0 0-7.1 0l-2.4 2.4a5 5 0 0 0 7.1 7.1l1.3-1.3" />
        {/if}
      </svg>
      <span>{t.label}</span>
      {#if t.id === 'pairing' && app.offline}
        <span class="dot" aria-label="Offline"></span>
      {/if}
    </button>
  {/each}
</nav>

<style>
  nav {
    flex: none;
    display: flex;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
  }
  .tab {
    position: relative;
    flex: 1 1 0;
    min-height: 52px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 3px;
    color: var(--color-text-muted);
  }
  .tab.active {
    color: var(--color-accent-text);
  }
  svg {
    width: 23px;
    height: 23px;
  }
  span {
    font-size: var(--text-micro);
    line-height: 1;
  }
  .dot {
    position: absolute;
    top: 8px;
    right: calc(50% - 18px);
    width: 7px;
    height: 7px;
    border-radius: 9999px;
    background: var(--color-status-stale);
  }
</style>
