<script lang="ts">
  import { keyboardInset } from '../lib/keyboard'
  import { type BarKey, type StickySlots } from '../lib/terminal/keys'

  // Strip above the keyboard (DESIGN.md §10.3). Every control is a 44pt
  // target (`--spacing-control`). Ctrl/Alt show idle / armed / locked as
  // three states: colour plus a shape (inset ring vs bottom rule), because
  // a colour-only state was a defect in this review cycle.
  let {
    mods,
    onkey,
  }: {
    mods: StickySlots
    onkey: (key: BarKey) => void
  } = $props()

  const KEYS: { key: BarKey; label: string }[] = [
    { key: 'esc', label: 'Esc' },
    { key: 'tab', label: 'Tab' },
    { key: 'ctrl', label: 'Ctrl' },
    { key: 'alt', label: 'Alt' },
    { key: 'up', label: '↑' },
    { key: 'down', label: '↓' },
    { key: 'left', label: '←' },
    { key: 'right', label: '→' },
    { key: 'home', label: 'Home' },
    { key: 'end', label: 'End' },
    { key: 'pipe', label: '|' },
    { key: 'slash', label: '/' },
    { key: 'dash', label: '-' },
    { key: 'tilde', label: '~' },
  ]

  function slotOf(key: BarKey): StickySlots['ctrl'] | undefined {
    if (key === 'ctrl') return mods.ctrl
    if (key === 'alt') return mods.alt
    return undefined
  }

  function press(e: PointerEvent, key: BarKey) {
    // Keep the IME field focused so the keyboard does not dismiss.
    e.preventDefault()
    onkey(key)
  }
</script>

<div class="bar" use:keyboardInset data-testid="key-bar">
  {#each KEYS as item (item.key)}
    {@const slot = slotOf(item.key)}
    <button
      type="button"
      class="key"
      class:armed={slot === 'armed'}
      class:locked={slot === 'locked'}
      aria-pressed={slot === undefined ? undefined : slot !== 'idle'}
      data-slot={slot}
      aria-label={item.label}
      onpointerdown={(e) => press(e, item.key)}
    >
      {item.label}
    </button>
  {/each}
</div>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    gap: 0;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
  }
  .key {
    flex: 0 0 auto;
    min-width: var(--spacing-control);
    min-height: var(--spacing-control);
    padding: 0 8px;
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }
  .key.armed {
    color: var(--color-accent-text);
    background: var(--color-accent-subtle);
    box-shadow: inset 0 0 0 1px var(--color-accent);
  }
  .key.locked {
    color: var(--color-accent-text);
    background: var(--color-accent-subtle);
    font-weight: 600;
    box-shadow: inset 0 -2px 0 var(--color-accent);
  }
</style>
