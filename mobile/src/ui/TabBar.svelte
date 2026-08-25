<script lang="ts">
  import { t } from '../lib/i18n'
  import { systemBack } from '../lib/back'
  import { app, switchTab, type Tab } from '../lib/store.svelte'

  // The tab is the object, not a metaphor: the desktop's word for what this
  // list holds is `doc.issues` (DESIGN.md §3.6). Search and Pairing have no
  // catalog equivalent yet and stay phone-only copy. The Shell tab is absent
  // until a terminal pairing is stored (DESIGN.md §10); its label is the
  // catalog's word for the same object (`sidebar.terminal`).
  const tabs = $derived(
    (
      [
        { id: 'issues' as Tab, label: t('doc.issues') },
        { id: 'search' as Tab, label: 'Search' },
        ...(app.terminal ? [{ id: 'shell' as Tab, label: t('sidebar.terminal') }] : []),
        { id: 'pairing' as Tab, label: 'Pairing' },
      ]
    ),
  )
</script>

<nav class="safe-bottom" aria-label="Tabs">
  {#each tabs as item (item.id)}
    <button
      class="tab"
      class:active={app.tab === item.id}
      aria-current={app.tab === item.id ? 'page' : undefined}
      onclick={() => {
        if (app.tab !== item.id) systemBack.dismissSheets()
        switchTab(item.id)
      }}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        {#if item.id === 'issues'}
          <path d="M4 6h16M4 12h16M4 18h10" />
        {:else if item.id === 'search'}
          <circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
        {:else if item.id === 'shell'}
          <path d="M4 8l5 4-5 4" />
          <path d="M12 16h8" />
        {:else}
          <path d="M10 13.5a5 5 0 0 0 7.1 0l2.4-2.4a5 5 0 0 0-7.1-7.1l-1.3 1.3" />
          <path d="M14 10.5a5 5 0 0 0-7.1 0l-2.4 2.4a5 5 0 0 0 7.1 7.1l1.3-1.3" />
        {/if}
      </svg>
      <span>{item.label}</span>
      {#if item.id === 'pairing' && app.offline}
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
    font-weight: 600;
  }
  .tab.active svg {
    stroke-width: 2.4;
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
    border-radius: 1px;
    background: var(--color-status-stale);
    box-shadow: 0 0 0 1px var(--color-text-primary);
  }
  .dot::after {
    content: '';
    position: absolute;
    left: -1px;
    right: -1px;
    top: 50%;
    height: 1.5px;
    margin-top: -0.75px;
    background: var(--color-text-primary);
    transform: rotate(-45deg);
  }
</style>
