<script lang="ts">
  import type { Snippet } from 'svelte'

  // An empty screen is direction, not mood (frontend-design): say what this
  // state is and offer the one useful next step.
  let {
    title,
    body,
    children,
  }: { title: string; body?: string; children?: Snippet } = $props()
</script>

<div class="empty">
  <p class="title">{title}</p>
  {#if body}
    <p class="body">{body}</p>
  {/if}
  {#if children}
    {@render children()}
  {/if}
</div>

<style>
  .empty {
    padding: 56px 32px;
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
  .title {
    margin: 0;
    color: var(--color-text-secondary);
    font-weight: 600;
  }
  .body {
    margin: 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    max-width: 260px;
  }
</style>
