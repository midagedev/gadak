<!--
  GDK-759 / GDK-1815: the one grab strip in the shell. Every seam a person
  can drag is this component — the sidebar's right edge, the issue list's
  right edge, and the terminal dock's top edge.

  It was two components' worth of shape and one component's worth of
  behaviour. The header this file used to carry said the dock's grip
  (TerminalPane, GDK-1194) "is the sibling this copies — same button-as-grip
  shape, same hover recipe", and that was exactly true and exactly the
  problem: the shape was copied, the keyboard was not. The dock was the only
  drag affordance in the app with no keyboard door. Rather than write the
  keyboard a second time, the seam moved: what differs between the three
  grips is what the value IS and where it is saved, and that is a
  `ResizeGrip` (lib/layout-resize.ts) the caller passes in. The dock's grip
  is browser-local (lib/terminal/pane.svelte.ts) and the two columns are
  `ui.tokens.layout` tokens, and this file does not know or care.

  Deliberately not a new visual language: transparent until you are near it,
  then the same border-subtle rule the rest of the shell draws its seams
  with, and the app's standard focus ring when it is tabbed to.

  9px across, because the seam itself is 1px and a 1px target is not
  something a hand can hit. On a column the 9px is 4px of overhang past the
  seam (a `right` offset on a positioned element, so the grid track it sits
  in is not widened by it); on the dock it sits fully INSIDE the pane,
  because the pane is `overflow-hidden` and an overhang there would be
  clipped to 5px — a grip that claims 9 and renders 5 is worse than one that
  is honest about where it is.

  Keyboard is not an extra: the handle is a real button in the tab order,
  arrows resize (8px, Shift for 1px — see lib/layout-resize.ts for why), and
  the same double-click that resets with a mouse is the Backspace/Delete key
  here. Tab alone is a long trip, though — the column grips sit behind the
  whole sidebar and the whole issue list — so the palette's resize rows
  (GDK-1796, GDK-1815) are the short door: they focus this element and let
  its own key handler work. lib/palette-coverage.test.ts is what keeps a
  draggable axis from shipping without one.

  It reports its numbers through the slider ARIA role so the value is
  announced rather than inferred from a moving edge. `aria-orientation` names
  the direction the VALUE moves, which is the direction of the arrow keys —
  the opposite of the seam you see, so a vertically-drawn column seam is
  `horizontal` and the horizontally-drawn dock seam is `vertical`. That
  mapping has one owner, RESIZE_GRIP_ORIENTATION in lib/viewport-regime.ts,
  and it is the same registry the coverage gate quantifies over.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import type { MessageKey } from '../../lib/i18n/catalog'
  import { handleGripKey, layoutGrip, startGripDrag, type ResizeGrip } from '../../lib/layout-resize'
  import {
    RESIZE_GRIP_ORIENTATION,
    type DraggableLayoutAxis,
    type LayoutTokenAxis,
  } from '../../lib/viewport-regime'
  import { RESIZE_GRIP_TESTID } from '../../lib/commands'
  import { asKeyTarget } from '../../lib/key-targets'

  let {
    axis,
    grip: providedGrip,
    ondragging,
  }: {
    axis: DraggableLayoutAxis
    /**
     * The behaviour behind this seam. The two layout tracks get the
     * ui.tokens.layout grip by default, so their call sites stay
     * `<LayoutResizeHandle axis="sidebar" />`; the dock passes its own.
     */
    grip?: ResizeGrip
    /** Told when a pointer drag starts and ends — the dock suppresses text
     *  selection in the pane underneath for the duration. */
    ondragging?: (dragging: boolean) => void
  } = $props()

  let dragging = $state(false)
  let el: HTMLButtonElement | null = $state(null)

  /** The .issue-layout box — the origin every column width is measured from. */
  function layoutBox(): HTMLElement | null {
    return el?.closest<HTMLElement>('[data-testid="issue-layout"]') ?? null
  }

  /**
   * The default grip, for the axes whose store this file may know about.
   * The dock's height is not one of them — where it is saved belongs to
   * lib/terminal/pane.svelte.ts — so that call site passes its grip in, and
   * forgetting to is a programming error rather than a silent cast.
   */
  function defaultGrip(a: DraggableLayoutAxis): ResizeGrip {
    if (a === 'terminal') {
      throw new Error('LayoutResizeHandle: the terminal axis must be given a grip prop')
    }
    const token: LayoutTokenAxis = a
    return layoutGrip(token, layoutBox)
  }

  const grip = $derived(providedGrip ?? defaultGrip(axis))

  // The announced value and its two ends. Seeded once the element exists
  // (the list's width is measured, so it needs a mounted box), then kept
  // current by the drag callback and the key handler — the seam is a moving
  // target a screen reader cannot see, so the number has to be said out
  // loud. The ends move too: the dock's ceiling is a fraction of the window.
  let valueNow = $state(0)
  let valueMin = $state(0)
  let valueMax = $state(0)

  function announce(value?: number): void {
    valueNow = Math.round(value ?? grip.current())
    valueMin = Math.round(grip.min())
    valueMax = Math.round(grip.max())
  }

  // onMount, not $effect: this is a one-shot seed from the mounted box, and
  // an $effect that assigns $state is the GDK-692 shape the repo bans.
  onMount(() => announce())

  const LABEL_KEY: Record<DraggableLayoutAxis, MessageKey> = {
    sidebar: 'layout.resizeSidebar',
    list: 'layout.resizeList',
    terminal: 'terminal.resize',
  }
  const label = $derived(t(LABEL_KEY[axis]))

  function onPointerDown(e: PointerEvent): void {
    // Primary button only; a right-click on a seam is a context menu.
    if (e.button !== 0) return
    startGripDrag(grip, e, (s) => {
      dragging = s.dragging
      ondragging?.(s.dragging)
      announce(s.value)
    })
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (handleGripKey(grip, e.key, e.shiftKey)) {
      e.preventDefault()
      announce()
    }
  }
</script>

<button
  bind:this={el}
  type="button"
  class="layout-resize-handle"
  class:is-dragging={dragging}
  role="slider"
  tabindex="0"
  aria-label={label}
  aria-orientation={RESIZE_GRIP_ORIENTATION[axis]}
  aria-valuemin={valueMin}
  aria-valuemax={valueMax}
  aria-valuenow={valueNow}
  data-no-press
  data-testid={RESIZE_GRIP_TESTID[axis]}
  data-axis={axis}
  data-orientation={RESIZE_GRIP_ORIENTATION[axis]}
  use:asKeyTarget={RESIZE_GRIP_TESTID[axis]}
  onpointerdown={onPointerDown}
  ondblclick={() => {
    grip.reset()
    announce()
  }}
  onfocus={() => announce()}
  onkeydown={onKeyDown}
></button>
