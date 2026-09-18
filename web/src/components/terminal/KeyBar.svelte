<script lang="ts">
  import { keyboardInset } from '../../lib/keyboard'
  import { type BarKey, type StickySlots } from '../../lib/terminal/keys'

  // Soft-key row for the terminal pane (GDK-1995). A phone browser has no
  // Esc, Ctrl or arrows, so this strip carries them — the same contract the
  // phone's strip keeps (mobile/src/ui/KeyBar.svelte): the key table, the
  // encoder and the band are shared (web/src/lib/terminal/keys.ts,
  // web/src/lib/keyboard.ts); only the styling is the web's own. The three
  // behavioural verdicts its comments record are settled and kept here:
  // Ctrl/Alt show idle / armed / locked separated by FILL first and a shape
  // second (GDK-951 — colour alone, then stroke weight alone, both failed a
  // review cycle); the panic exit is labelled "No Mods", not "Clear" (`clear`
  // is already the command, and Ctrl-L); and the press handler keeps focus
  // where it was so the software keyboard does not dismiss.
  let {
    mods,
    onkey,
  }: {
    mods: StickySlots
    onkey: (key: BarKey) => void
  } = $props()

  // The panic exit (GDK-953). glasskeys' StickyModifiers.clear(): "Any UI
  // that offers lock must also offer this" — armed had no single-gesture
  // way back to idle. Persistent, disabled while every slot is idle: it is
  // visible before it is needed, and the strip's layout does not jump under
  // a thumb mid-gesture.
  const anyActive = $derived(mods.ctrl !== 'idle' || mods.alt !== 'idle')

  const KEYS: { key: BarKey; label: string }[] = [
    { key: 'esc', label: 'Esc' },
    { key: 'tab', label: 'Tab' },
    { key: 'ctrl', label: 'Ctrl' },
    { key: 'alt', label: 'Alt' },
    // "No Mods", not "Clear": in a terminal key strip `clear` is already
    // taken — it is the command, and Ctrl-L — so a control labelled Clear
    // reads as "wipe the screen" beside Esc/Tab/Ctrl/Alt, which all send or
    // arm something (look verdict, 2026-08-27). Naming the target instead of
    // the action also matches the disabled rule: when no modifier is held,
    // "No Mods" is already true, so there is nothing to press. `Reset` was
    // rejected for the same reason as Clear — `reset` is a terminal command too.
    { key: 'clear', label: 'No Mods' },
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
    // Keep focus where it was so the software keyboard does not dismiss.
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
      disabled={item.key === 'clear' && !anyActive}
      onpointerdown={(e) => press(e, item.key)}
    >
      {item.label}
    </button>
  {/each}
</div>

<style>
  /*
   * GDK-1995 visibility decision: `(pointer: coarse) and (hover: none)` — a
   * touch-only device. Not a width break, because a phone in landscape past
   * 900px still has no Esc key; and not bare `(pointer: coarse)`, because a
   * touch laptop has a physical keyboard and a fine pointer beside the touch
   * one, so a permanent soft-key strip there is clutter. `hover: none` is
   * what separates the two. This query lives here and nowhere else.
   */
  .bar {
    display: none;
    flex-wrap: wrap;
    gap: 0;
    background: var(--color-bg-panel);
    border-top: 1px solid var(--color-border-subtle);
  }
  @media (pointer: coarse) and (hover: none) {
    .bar {
      display: flex;
    }
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
  /*
    GDK-951 — armed and locked are told apart by FILL, not by stroke weight.
    They used to share a ground (--color-accent-subtle) and an ink
    (--color-accent-text) and differ only in a 1px inset ring against a 2px
    bottom rule, which at the distance a phone is held, under a thumb, is
    not a state at all: "the next letter is Ctrl-something" and "every
    letter is Ctrl-something until I say stop" looked the same.

    armed keeps the tint with the accent ring on it. locked inverts: the
    accent thread becomes the ground and the glyph becomes the page.
    --color-accent-text / --color-bg-base is the theme-safe pair for that —
    the first is the accent value that always contrasts the ground, the
    second always is the ground — so the inversion holds in light, dark,
    ink and ember (computed ≥8:1 in all four) with no new colour.
  */
  .key.armed {
    color: var(--color-accent-text);
    background: var(--color-accent-subtle);
    box-shadow: inset 0 0 0 1px var(--color-accent);
  }
  .key.locked {
    color: var(--color-bg-base);
    background: var(--color-accent-text);
    font-weight: 600;
    /* The fill is inset by the bar's own colour, so locked reads as a solid
       pill sitting in the cell rather than a cell that changed colour — a
       shape as well as a fill, which is what input-machines.test.ts has
       required of both states since GDK-953. */
    box-shadow: inset 0 0 0 2px var(--color-bg-panel);
  }
  .key:disabled {
    /* Sibling idiom (.act:disabled, .status:disabled): visibly off, not
       merely inert — an idle-strip "No Mods" that looks tappable is a lie. */
    opacity: 0.45;
  }
</style>
