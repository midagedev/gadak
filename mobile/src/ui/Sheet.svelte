<script lang="ts">
  import type { Snippet } from 'svelte'
  import { fly, fade } from 'svelte/transition'

  // Bottom sheet: scrim + rising panel, thumb territory. Owns its
  // safe-bottom inset (one of the sanctioned owners, DESIGN.md §4.1).
  let {
    title,
    onclose,
    children,
  }: { title: string; onclose: () => void; children: Snippet } = $props()
</script>

<div class="scrim" transition:fade={{ duration: 150 }} onclick={onclose} aria-hidden="true"></div>
<div
  class="sheet safe-bottom"
  role="dialog"
  aria-modal="true"
  aria-label={title}
  transition:fly={{ y: 320, duration: 240 }}
>
  <div class="grab" aria-hidden="true"></div>
  <div class="head">
    <h2>{title}</h2>
    <button class="cancel" onclick={onclose}>Cancel</button>
  </div>
  {@render children()}
</div>

<style>
  .scrim {
    position: absolute;
    inset: 0;
    background: var(--color-scrim);
    z-index: 30;
  }
  .sheet {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 31;
    background: var(--color-bg-panel);
    border-radius: 12px 12px 0 0;
    box-shadow: var(--shadow-overlay);
    max-height: 70%;
    display: flex;
    flex-direction: column;
  }
  .grab {
    flex: none;
    width: 36px;
    height: 4px;
    border-radius: 9999px;
    background: var(--color-border-strong);
    margin: 8px auto 0;
  }
  .head {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 16px 4px;
  }
  h2 {
    margin: 0;
    font-size: var(--text-title);
    line-height: var(--text-title--line-height);
    font-family: var(--font-display);
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .cancel {
    min-height: var(--spacing-control-sm);
    padding: 0 8px;
    color: var(--color-accent-text);
    font-size: var(--text-body);
  }
</style>
