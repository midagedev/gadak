/*
 * GDK-759: dragging the sidebar and the list column to the width you want.
 *
 * The behaviour half. Geometry — what the width IS, and everything derived
 * from it — stays in viewport-regime.ts; this file only decides what a
 * pointer, an arrow key and a double-click mean, and when to save.
 *
 * The design decision this file exists to honour: the drag writes the SAME
 * tokens the CLI writes (`ui.tokens.layout.sidebar` / `.list`), not a second
 * store. Three consequences follow, and all three are load-bearing:
 *
 *   - the catalog range IS the drag limit. Out-of-range is a warning the
 *     server carries rather than a refusal, so nothing below the handle
 *     would stop it; the clamp is where that decision is made, from the
 *     catalog's own numbers (viewport-regime.ts LAYOUT_DRAG_CLAMP).
 *   - the save has ONE owner. `gadak config set ui.tokens.layout.sidebar`
 *     and a drag land in the same key of the same document through the same
 *     serialized write-through (settings-write.ts), so they cannot disagree
 *     about the current width the way a browser-local store would.
 *   - other open tabs pick the change up for free, through the configVersion
 *     poll that already re-applies user tokens.
 *
 * Saving cadence: the local CSS variable moves on every pointermove frame,
 * and the document is PUT exactly ONCE, on pointerup. A per-frame save is
 * the failure this shape is built against — it would be ~60 read-modify-write
 * round trips per drag through a serialized queue, each one rewriting
 * config.json on disk. e2e/layout-resize.spec.ts counts the requests.
 *
 * "Unset" is a real value, not a missing one. Double-click DELETES the key
 * rather than writing the shipped px, because only an absent token lets
 * app.css's per-track var() fallbacks resolve — the list is 1360px capped
 * with nothing open, 1fr docked, a clamp under the browse pane, and one
 * pinned number would flatten all three (GDK-769).
 *
 * GDK-1815: the same gestures now drive the terminal dock's top edge, which
 * is NOT a token — its height is browser-local (lib/terminal/pane.svelte.ts).
 * So the grammar above got split from the store underneath it. `ResizeGrip`
 * is the seam: a grip says what its value is, what a pointer means, how to
 * paint it and how to save it, and the two drivers here (startGripDrag,
 * handleGripKey) own the *cadence* — paint per frame or per press, save once
 * on release or once a key burst settles, save NOTHING on pointercancel.
 * Cadence was the half the dock got wrong (one write per pointermove frame),
 * and it is fixed by moving the drag here; the store did not move, because
 * where the dock's height lives is a product decision about the shared
 * document, not a consistency fix.
 */

import { writeThroughLook } from './settings-write'
import type { GadakSettings, UITokens } from './api'
import {
  clampLayoutPx,
  effectiveLayout,
  pinLayoutAxis,
  setLayoutOverride,
  unpinLayoutAxis,
  LAYOUT_DRAG_CLAMP,
  RESIZE_GRIP_ORIENTATION,
  type DraggableLayoutAxis,
  type GripOrientation,
  type LayoutTokenAxis,
} from './viewport-regime'

/**
 * Arrow-key step. 8px is the app's spacing rhythm (--spacing-* are multiples
 * of it), and it is the smallest step that reads as movement at these column
 * widths — 1px per press is indistinguishable from a stuck key against a
 * 272px rail, while the whole sidebar range is 112px, so 8 crosses it in 14
 * presses rather than 112. Shift drops to 1px for the last pixel.
 */
export const RESIZE_STEP_PX = 8
export const RESIZE_STEP_FINE_PX = 1

/**
 * A burst of arrow presses is one intent, so it is one PUT: the save waits
 * this long after the last key. 400ms is above a fast key-repeat interval
 * (~30ms) and below the point where a person has moved on.
 */
export const KEY_COMMIT_DELAY_MS = 400

/** The width `axis` is painting right now, as the drag's starting point. */
function currentWidth(axis: LayoutTokenAxis, layout: HTMLElement | null): number {
  const eff = effectiveLayout()
  if (axis === 'sidebar') return eff.sidebar
  if (eff.list !== undefined) return eff.list
  // The list has no number when it is unset — its width is whatever the
  // active track resolved the var() fallback to, which only the box knows.
  // Measuring is therefore not a shortcut here, it is the only honest read,
  // and it is what makes the first drag continue from the visible seam
  // instead of jumping to a catalog default.
  const measured = layout?.querySelector<HTMLElement>('[data-testid="terminal-split"]')
  return measured ? measured.getBoundingClientRect().width : eff.listMin
}

/**
 * The width `axis` should take for a pointer at `clientX`: the seam follows
 * the pointer (direct manipulation), rather than accumulating a delta that
 * drifts away from the hand once the clamp bites. `layout` is the
 * .issue-layout element — the sidebar starts at its left edge, and the list
 * starts where the sidebar ends.
 */
function widthForPointer(
  axis: LayoutTokenAxis,
  clientX: number,
  layout: HTMLElement,
): number {
  const left = layout.getBoundingClientRect().left
  const origin = axis === 'sidebar' ? left : left + effectiveLayout().sidebar
  return clampLayoutPx(axis, clientX - origin)
}

/**
 * PUT the document once with `axis` set to `px`, or with the key DELETED
 * when `px` is null. Read-modify-write through the shared settings queue, so
 * a resize landing next to a theme change cannot lose either one.
 */
export async function persistLayoutWidth(
  axis: LayoutTokenAxis,
  px: number | null,
): Promise<void> {
  await writeThroughLook(
    (current: GadakSettings): GadakSettings => {
      const ui = { ...(current.ui ?? {}) }
      const tokens: UITokens = { ...(ui.tokens ?? {}) }
      const layout = { ...(tokens.layout ?? {}) }
      if (px === null) delete layout[axis]
      else layout[axis] = `${px}px`
      // An empty map would be a stored "the user customised layout" that
      // holds nothing; drop it so a reset leaves the document exactly as it
      // was before the first drag.
      if (Object.keys(layout).length === 0) delete tokens.layout
      else tokens.layout = layout
      ui.tokens = tokens
      return { ...current, ui }
    },
    'layout.savedLocally',
    `layout.${axis} width`,
  )
}

/**
 * The signed px one arrow press means on a grip of this orientation, or null
 * when the key is not a resize key at all.
 *
 * Pure, and the only place the arrow → direction mapping is written. The
 * direction is the one `aria-orientation` names on a slider — the direction
 * the VALUE moves, not the direction the seam is drawn — so a horizontal
 * grip takes Left/Right and a vertical one takes Up/Down. Up is taller,
 * because the dock's grip is on its TOP edge and direct manipulation means
 * the seam goes where the hand goes (GDK-1194, and the pointer drag has
 * always agreed).
 */
export function stepForKey(
  key: string,
  fine: boolean,
  orientation: GripOrientation,
): number | null {
  const step = fine ? RESIZE_STEP_FINE_PX : RESIZE_STEP_PX
  if (orientation === 'horizontal') {
    if (key === 'ArrowLeft') return -step
    if (key === 'ArrowRight') return step
    return null
  }
  if (key === 'ArrowDown') return -step
  if (key === 'ArrowUp') return step
  return null
}

/** Keys that mean "put it back where it shipped" — the keyboard's double-click. */
function isResetKey(key: string): boolean {
  return key === 'Backspace' || key === 'Delete'
}

interface DragHandle {
  /** Stop listening and drop the pin; safe to call twice. */
  cancel(): void
}

/** What the handle is told on start, on every frame, and on release. */
export interface DragState {
  dragging: boolean
  value: number
}

/** Where a gesture began: the grip's value and the pointer, at pointerdown. */
export interface GripGestureStart {
  value: number
  clientX: number
  clientY: number
}

/**
 * One draggable seam, told apart from every other by what it measures and
 * where it saves — never by what a pointer or an arrow key means, which is
 * the same everywhere and lives in the two drivers below.
 *
 * The split exists because the terminal dock is not a `ui.tokens.layout`
 * token (GDK-1815). Before this, "the grip grammar" and "the layout token
 * store" were one body, so the dock could not have the grammar without also
 * moving into the config document — and that is a product decision nobody
 * had made. A grip is the smallest thing that lets the behaviour cross the
 * store boundary without dragging the store across with it.
 */
export interface ResizeGrip {
  /** Identity, and the key the per-grip commit timer is held under. */
  axis: DraggableLayoutAxis
  orientation: GripOrientation
  /** The announced bounds. Functions, not numbers: the dock's ceiling is a
   *  fraction of the window and moves when the window does. */
  min(): number
  max(): number
  /** The value in force right now — the drag's starting point, and what the
   *  slider announces after every paint (clamped, because the grip clamps). */
  current(): number
  /** What a pointer here means, or null when the gesture cannot be measured
   *  (the layout grip needs its .issue-layout box) and should be ignored. */
  fromPointer(event: PointerEvent, start: GripGestureStart): number | null
  /** Paint locally. Never saves — the driver decides when that happens. */
  paint(px: number): void
  /** Save. Called once per gesture, never per frame. */
  commit(px: number): void
  /** Back to the shipped value, locally and in the store. */
  reset(): void
  /** Held for the lifetime of a pointer gesture (the layout grip pins its
   *  axis so a config poll cannot yank the column out from under the hand). */
  hold?(held: boolean): void
  /** A cancelled gesture: put the paint back where the store says it is. */
  rollback?(): void
}

/** The grip for a layout track: measures the .issue-layout box, saves a token. */
export function layoutGrip(
  axis: LayoutTokenAxis,
  layoutEl: () => HTMLElement | null,
): ResizeGrip {
  return {
    axis,
    orientation: RESIZE_GRIP_ORIENTATION[axis],
    min: () => LAYOUT_DRAG_CLAMP[axis].min,
    max: () => LAYOUT_DRAG_CLAMP[axis].max,
    current: () => currentWidth(axis, layoutEl()),
    fromPointer: (event) => {
      const layout = layoutEl()
      return layout ? widthForPointer(axis, event.clientX, layout) : null
    },
    paint: (px) => setLayoutOverride(axis, px),
    commit: (px) => void persistLayoutWidth(axis, px),
    reset: () => resetLayoutWidth(axis),
    hold: (held) => (held ? pinLayoutAxis(axis) : unpinLayoutAxis(axis)),
  }
}

/**
 * Begin a pointer drag of `grip`. Paints every frame, saves once on release.
 * `onChange` lets the caller mark the drag visually and keep its announced
 * value current without this file knowing about the DOM it lives in.
 */
export function startGripDrag(
  grip: ResizeGrip,
  event: PointerEvent,
  onChange: (state: DragState) => void,
): DragHandle | null {
  const start: GripGestureStart = {
    value: grip.current(),
    clientX: event.clientX,
    clientY: event.clientY,
  }
  const first = grip.fromPointer(event, start)
  // Nothing to measure against — not a drag, and emphatically not a save.
  if (first === null) return null
  event.preventDefault()
  grip.hold?.(true)
  grip.paint(first)
  let last = grip.current()
  onChange({ dragging: true, value: last })

  const move = (ev: PointerEvent): void => {
    const next = grip.fromPointer(ev, start)
    if (next === null) return
    grip.paint(next)
    last = grip.current()
    onChange({ dragging: true, value: last })
  }
  let done = false
  const finish = (persist: boolean): void => {
    if (done) return
    done = true
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', cancel)
    grip.hold?.(false)
    if (!persist) grip.rollback?.()
    onChange({ dragging: false, value: grip.current() })
    // The one save. It is here and nowhere in `move`.
    if (persist) grip.commit(last)
  }
  const up = (): void => finish(true)
  // A cancelled pointer (the browser took the gesture, the tab lost focus
  // mid-drag) is not a decision to save: the seam returns to whatever the
  // store says.
  const cancel = (): void => finish(false)
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', cancel)
  return { cancel: () => finish(false) }
}

// Per axis, not one shared timer: arrowing the sidebar and then the list
// before the first burst settles must not drop the sidebar's save.
const keyTimers = new Map<DraggableLayoutAxis, ReturnType<typeof setTimeout>>()

function clearKeyTimer(axis: DraggableLayoutAxis): void {
  const pending = keyTimers.get(axis)
  if (pending) {
    clearTimeout(pending)
    keyTimers.delete(axis)
  }
}

/**
 * Handle one key press on a focused grip. Returns true when the key was ours
 * (the caller should preventDefault). The save is debounced so a held arrow
 * is still one write, and Backspace/Delete is the keyboard's double-click.
 */
export function handleGripKey(grip: ResizeGrip, key: string, fine: boolean): boolean {
  if (isResetKey(key)) {
    clearKeyTimer(grip.axis)
    grip.reset()
    return true
  }
  const delta = stepForKey(key, fine, grip.orientation)
  if (delta === null) return false
  grip.paint(grip.current() + delta)
  // Read back rather than trusting the arithmetic: the grip clamps, so this
  // is the number the person sees and the number that gets saved.
  const landed = grip.current()
  clearKeyTimer(grip.axis)
  keyTimers.set(
    grip.axis,
    setTimeout(() => {
      keyTimers.delete(grip.axis)
      grip.commit(landed)
    }, KEY_COMMIT_DELAY_MS),
  )
  return true
}

/** Double-click: back to the shipped width, by deleting the key. */
export function resetLayoutWidth(axis: LayoutTokenAxis): void {
  clearKeyTimer(axis)
  setLayoutOverride(axis, undefined)
  void persistLayoutWidth(axis, null)
}
