<script lang="ts">
  /*
   * Bottom sheet for status transitions (ux-report Q3) — the container the
   * status chip opens. Rows are the transitions[] entries: the to_status
   * display name with a to_category dot; category names never appear as
   * text. The POST and the optimistic chip swap live in Detail.svelte;
   * this component only picks and closes (Esc, backdrop tap, swipe down
   * on the handle). The parent locks its scroll region while the sheet is
   * mounted — the sheet locks nothing itself.
   */
  import { categoryInk, type Transition } from '../lib/api'
  import { t } from '../lib/i18n'

  let {
    transitions,
    loading = false,
    errorText = null,
    onpick,
    onclose,
  }: {
    transitions: Transition[]
    loading?: boolean
    errorText?: string | null
    onpick: (tr: Transition) => void
    onclose: () => void
  } = $props()

  // Swipe-down dismiss, on the handle row only — a drag on the list is a
  // scroll. dy follows the finger; past the threshold it is a close.
  const SWIPE_CLOSE_PX = 80
  let dragY = $state(0)
  let dragStart: number | null = null

  function onTouchStart(e: TouchEvent): void {
    dragStart = e.touches[0].clientY
  }

  function onTouchMove(e: TouchEvent): void {
    if (dragStart === null) return
    dragY = Math.max(0, e.touches[0].clientY - dragStart)
  }

  function onTouchEnd(): void {
    const close = dragY > SWIPE_CLOSE_PX
    dragStart = null
    dragY = 0
    if (close) onclose()
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div class="m-sheet-root" data-testid="transition-sheet">
  <button
    class="m-sheet-backdrop"
    type="button"
    aria-label={t('detail.transition.close')}
    onclick={onclose}
    data-testid="transition-backdrop"
  ></button>
  <div
    class="m-sheet"
    role="dialog"
    aria-modal="true"
    aria-label={t('detail.transition.title')}
    style:transform={`translateY(${dragY}px)`}
  >
    <button
      class="m-sheet-handle"
      type="button"
      aria-label={t('detail.transition.close')}
      onclick={onclose}
      ontouchstart={onTouchStart}
      ontouchmove={onTouchMove}
      ontouchend={onTouchEnd}
    >
      <span class="m-sheet-grip" aria-hidden="true"></span>
      <span class="m-sheet-title">{t('detail.transition.title')}</span>
    </button>
    {#if loading}
      <p class="m-sheet-state" data-testid="transition-loading">{t('detail.transition.loading')}</p>
    {:else if errorText}
      <p class="m-sheet-state m-sheet-error" role="alert" data-testid="transition-error">{errorText}</p>
    {:else if transitions.length === 0}
      <p class="m-sheet-state" data-testid="transition-empty">{t('detail.transition.empty')}</p>
    {:else}
      <ul class="m-sheet-list" role="listbox" data-testid="transition-options">
        {#each transitions as tr (tr.id)}
          <li>
            <button class="m-sheet-option" type="button" role="option" aria-selected="false" onclick={() => onpick(tr)}>
              <span class="m-sheet-dot" style:background={categoryInk(tr.to_category)} aria-hidden="true"></span>
              <span class="m-sheet-label">{tr.to_status}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  /* Interactive heights sit on the iOS 44pt floor, not the 42px web row
     token (ux-report Q1/Q7); min-height everywhere so Dynamic Type grows
     rows instead of clipping them. Colors are tokens only. */
  .m-sheet-root {
    position: fixed;
    inset: 0;
    z-index: 50;
  }

  .m-sheet-backdrop {
    position: absolute;
    inset: 0;
    width: 100%;
    padding: 0;
    border: 0;
    background: color-mix(in srgb, var(--color-bg-base) 55%, transparent);
    cursor: pointer;
  }

  .m-sheet {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    display: flex;
    flex-direction: column;
    max-height: 70dvh;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
    border-radius: 12px 12px 0 0;
    padding-bottom: env(safe-area-inset-bottom);
  }

  .m-sheet-handle {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.375rem;
    padding: 0.5rem 1rem 0.375rem;
    touch-action: none;
  }

  .m-sheet-grip {
    width: 2.25rem;
    height: 0.25rem;
    border-radius: 9999px;
    background: var(--color-border-strong);
  }

  .m-sheet-title {
    margin: 0;
    font-size: var(--text-body);
    font-weight: 600;
    color: var(--color-text-secondary);
  }

  .m-sheet-list {
    list-style: none;
    margin: 0;
    padding: 0 0.5rem 0.5rem;
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .m-sheet-option {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    width: 100%;
    min-height: 44px;
    padding: 0 0.75rem;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--color-text-primary);
    font-family: var(--font-sans);
    font-size: var(--text-body);
    text-align: left;
    cursor: pointer;
  }

  .m-sheet-option:active {
    background: var(--color-bg-hover);
  }

  .m-sheet-dot {
    flex: none;
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 9999px;
  }

  .m-sheet-label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .m-sheet-state {
    margin: 0;
    padding: 0.75rem 1rem 1rem;
    color: var(--color-text-muted);
    font-size: var(--text-body);
  }

  .m-sheet-error {
    color: var(--color-status-reopen);
  }
</style>
