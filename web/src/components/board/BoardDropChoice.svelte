<script lang="ts">
  /*
   * The ambiguous drop (GDK-1176): two or more transitions reach the column
   * the card was dropped on, and the write needs a name. This menu is that
   * question asked at the drop point — pick one and it is write.transition,
   * Escape or a click elsewhere and nothing was written at all.
   */
  import { t } from '../../lib/i18n'
  import { boardDrag } from '../../lib/board-drag.svelte'
  import { effectiveCategory } from '../../lib/view-config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'

  let { choice }: { choice: NonNullable<typeof boardDrag.choice> } = $props()

  let listEl = $state<HTMLDivElement | null>(null)

  // Focus the first option so Enter answers and Escape declines, keyboard-only.
  $effect(() => {
    queueMicrotask(() => listEl?.querySelector('button')?.focus())
  })

  function catDot(key: string): string {
    const c = effectiveCategory(key)
    return c === 'done' ? 'bg-status-done' : c === 'new' ? 'bg-status-new' : 'bg-status-inprogress'
  }
</script>

<div
  bind:this={listEl}
  use:onEscape={(e) => {
    e.preventDefault()
    boardDrag.dismissChoice()
  }}
  use:onOutsideClick={{ handler: () => boardDrag.dismissChoice() }}
  data-testid="board-drop-choice"
  role="menu"
  aria-label={t('board.dropChoose')}
  class="anim-enter fixed z-40 w-52 rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
  style:left="{choice.x}px"
  style:top="{choice.y}px"
>
  <div class="px-3 py-1 text-micro text-text-muted">{t('board.dropChoose')}</div>
  {#each choice.candidates as tr (tr.id)}
    <button
      type="button"
      role="menuitem"
      onclick={() => boardDrag.choose(tr)}
      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-body text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:text-text-primary focus:outline-none"
    >
      <span class="h-1.5 w-1.5 flex-none rounded-full {catDot(tr.to_category)}"></span>
      <span class="min-w-0 flex-1 truncate">{tr.name}</span>
    </button>
  {/each}
</div>
