<!--
  GDK-759: the grab strip on a column's right seam.

  Deliberately not a new visual language: transparent until you are near it,
  then the same border-subtle rule the rest of the shell draws its seams
  with, and the app's standard focus ring when it is tabbed to. The terminal
  dock's grip (TerminalPane, GDK-1194) is the sibling this copies — same
  button-as-grip shape, same hover recipe, rotated to a vertical seam.

  9px wide with 4px of it hanging past the seam: the seam itself is 1px, and
  a 1px target is not something a hand can hit. The overhang is a `right`
  offset on a positioned element, so the grid track it sits in is not widened
  by it.

  Keyboard is not an extra: the handle is a real button in the tab order,
  arrows resize (8px, Shift for 1px — see lib/layout-resize.ts for why), and
  the same double-click that resets with a mouse is the Backspace/Delete key
  here. Tab alone is a long trip, though — this sits behind the whole sidebar
  and the whole issue list — so the palette's resize rows (GDK-1796) are the
  short door: they focus this element and let its own key handler work.
  It reports its numbers through the slider ARIA role so the value is
  announced rather than inferred from a moving column. The seam is drawn
  vertically but aria-orientation is horizontal: on a slider that attribute
  names the direction the VALUE moves, which is the direction of the arrow
  keys this handles.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import {
    currentWidth,
    handleResizeKey,
    resetLayoutWidth,
    startLayoutDrag,
  } from '../../lib/layout-resize'
  import { LAYOUT_DRAG_CLAMP, type DraggableLayoutAxis } from '../../lib/viewport-regime'
  import { RESIZE_GRIP_TESTID } from '../../lib/commands'
  import { asKeyTarget } from '../../lib/key-targets'

  let { axis }: { axis: DraggableLayoutAxis } = $props()

  let dragging = $state(false)
  let el: HTMLButtonElement | null = $state(null)
  // The announced width. Seeded once the element exists (the list's width is
  // measured, so it needs a mounted box), then kept current by the drag
  // callback and the key handler — the column is a moving target a screen
  // reader cannot see, so the number has to be said out loud.
  let valueNow = $state(0)
  // onMount, not $effect: this is a one-shot seed from the mounted box, and
  // an $effect that assigns $state is the GDK-692 shape the repo bans.
  onMount(() => {
    valueNow = Math.round(currentWidth(axis, layoutEl()))
  })

  const label = $derived(axis === 'sidebar' ? t('layout.resizeSidebar') : t('layout.resizeList'))
  const clamp = $derived(LAYOUT_DRAG_CLAMP[axis])

  /** The .issue-layout box — the origin every width is measured from. */
  function layoutEl(): HTMLElement | null {
    return el?.closest<HTMLElement>('[data-testid="issue-layout"]') ?? null
  }

  function onPointerDown(e: PointerEvent): void {
    // Primary button only; a right-click on a seam is a context menu.
    if (e.button !== 0) return
    const layout = layoutEl()
    if (!layout) return
    startLayoutDrag(axis, e, layout, (s) => {
      dragging = s.dragging
      valueNow = Math.round(s.width)
    })
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (e.key === 'Backspace' || e.key === 'Delete') {
      e.preventDefault()
      resetLayoutWidth(axis)
      valueNow = Math.round(currentWidth(axis, layoutEl()))
      return
    }
    if (handleResizeKey(axis, e.key, e.shiftKey, layoutEl())) {
      e.preventDefault()
      valueNow = Math.round(currentWidth(axis, layoutEl()))
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
  aria-orientation="horizontal"
  aria-valuemin={clamp.min}
  aria-valuemax={clamp.max}
  aria-valuenow={valueNow}
  data-no-press
  data-testid={RESIZE_GRIP_TESTID[axis]}
  data-axis={axis}
  use:asKeyTarget={RESIZE_GRIP_TESTID[axis]}
  onpointerdown={onPointerDown}
  ondblclick={() => resetLayoutWidth(axis)}
  onkeydown={onKeyDown}
></button>
