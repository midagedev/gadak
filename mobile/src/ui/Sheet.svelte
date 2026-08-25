<script lang="ts">
  import type { Snippet } from 'svelte'
  import { fly, fade } from 'svelte/transition'
  import { t } from '../lib/i18n'
  import { systemBack } from '../lib/back'

  // Bottom sheet: scrim + rising panel, thumb territory. The bottom inset
  // is a property of where the sheet sits (app.css: .detail-layer .sheet),
  // not a class the caller remembers to pass.
  let {
    title,
    onclose,
    children,
  }: { title: string; onclose: () => void; children: Snippet } = $props()

  $effect(() => {
    return systemBack.registerSheet(onclose)
  })
</script>

<button
  type="button"
  class="scrim"
  transition:fade={{ duration: 150 }}
  onclick={onclose}
  aria-label={t('common.cancel')}
></button>
<div
  class="sheet"
  role="dialog"
  aria-modal="true"
  aria-label={title}
  transition:fly={{ y: 320, duration: 240 }}
>
  <div class="grab" aria-hidden="true"></div>
  <div class="head">
    <h2>{title}</h2>
    <button class="cancel" onclick={onclose}>{t('common.cancel')}</button>
  </div>
  {@render children()}
</div>

<style>
  .scrim {
    position: absolute;
    inset: 0;
    display: block;
    width: 100%;
    background: var(--color-scrim);
    z-index: 30;
    border-radius: 0;
    appearance: none;
    -webkit-appearance: none;
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
    padding: 0 8px;
    color: var(--color-accent-text);
    font-size: var(--text-body);
  }
</style>
