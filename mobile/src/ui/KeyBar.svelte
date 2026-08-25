<script lang="ts">
  import { keyboardInset } from '../lib/keyboard'
  import { type BarKey, type StickyMods } from '../lib/terminal/keys'

  // Strip above the keyboard (DESIGN.md §10.3). Every control is a 44pt
  // target (`--spacing-control`); Ctrl/Alt look armed because a modifier
  // whose state you cannot see is worse than no modifier.
  let {
    mods,
    onkey,
  }: {
    mods: StickyMods
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

  function press(e: PointerEvent, key: BarKey) {
    // Keep the IME field focused so the keyboard does not dismiss.
    e.preventDefault()
    onkey(key)
  }
</script>

<div class="bar" use:keyboardInset data-testid="key-bar">
  {#each KEYS as item (item.key)}
    {@const armed =
      (item.key === 'ctrl' && mods.ctrl) || (item.key === 'alt' && mods.alt)}
    <button
      type="button"
      class="key"
      class:armed
      aria-pressed={item.key === 'ctrl' || item.key === 'alt' ? armed : undefined}
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
  }
</style>
