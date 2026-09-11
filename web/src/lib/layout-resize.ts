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
 */

import { writeThroughLook } from './settings-write'
import type { GadakSettings, UITokens } from './api'
import {
  clampLayoutPx,
  effectiveLayout,
  pinLayoutAxis,
  setLayoutOverride,
  unpinLayoutAxis,
  type DraggableLayoutAxis,
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
export function currentWidth(axis: DraggableLayoutAxis, layout: HTMLElement | null): number {
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
export function widthForPointer(
  axis: DraggableLayoutAxis,
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
  axis: DraggableLayoutAxis,
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

/** What one arrow press means, or null when the key is not ours. */
export function widthForKey(
  axis: DraggableLayoutAxis,
  from: number,
  key: string,
  fine: boolean,
): number | null {
  const step = fine ? RESIZE_STEP_FINE_PX : RESIZE_STEP_PX
  if (key === 'ArrowLeft') return clampLayoutPx(axis, from - step)
  if (key === 'ArrowRight') return clampLayoutPx(axis, from + step)
  return null
}

interface DragHandle {
  /** Stop listening and drop the pin; safe to call twice. */
  cancel(): void
}

/** What the handle is told on start, on every frame, and on release. */
export interface DragState {
  dragging: boolean
  width: number
}

/**
 * Begin a pointer drag of `axis`. Paints every frame, saves once on release.
 * `onChange` lets the caller mark the drag visually and keep its announced
 * value current without this file knowing about the DOM it lives in.
 */
export function startLayoutDrag(
  axis: DraggableLayoutAxis,
  event: PointerEvent,
  layout: HTMLElement,
  onChange: (state: DragState) => void,
): DragHandle {
  event.preventDefault()
  pinLayoutAxis(axis)
  let last = widthForPointer(axis, event.clientX, layout)
  setLayoutOverride(axis, last)
  onChange({ dragging: true, width: last })

  const move = (ev: PointerEvent): void => {
    last = widthForPointer(axis, ev.clientX, layout)
    setLayoutOverride(axis, last)
    onChange({ dragging: true, width: last })
  }
  let done = false
  const finish = (persist: boolean): void => {
    if (done) return
    done = true
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', cancel)
    unpinLayoutAxis(axis)
    onChange({ dragging: false, width: last })
    // The one PUT. It is here and nowhere in `move`.
    if (persist) void persistLayoutWidth(axis, last)
  }
  const up = (): void => finish(true)
  // A cancelled pointer (the browser took the gesture, the tab lost focus
  // mid-drag) is not a decision to save: the column returns to whatever the
  // document says on the next config apply.
  const cancel = (): void => finish(false)
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', cancel)
  return { cancel: () => finish(false) }
}

// Per axis, not one shared timer: arrowing the sidebar and then the list
// before the first burst settles must not drop the sidebar's save.
const keyTimers = new Map<DraggableLayoutAxis, ReturnType<typeof setTimeout>>()

/**
 * Handle one arrow press on a focused handle. Returns true when the key was
 * ours (the caller should preventDefault). The save is debounced so a held
 * arrow is still one PUT.
 */
export function handleResizeKey(
  axis: DraggableLayoutAxis,
  key: string,
  fine: boolean,
  layout: HTMLElement | null,
): boolean {
  const next = widthForKey(axis, currentWidth(axis, layout), key, fine)
  if (next === null) return false
  setLayoutOverride(axis, next)
  const pending = keyTimers.get(axis)
  if (pending) clearTimeout(pending)
  keyTimers.set(
    axis,
    setTimeout(() => {
      keyTimers.delete(axis)
      void persistLayoutWidth(axis, next)
    }, KEY_COMMIT_DELAY_MS),
  )
  return true
}

/** Double-click: back to the shipped width, by deleting the key. */
export function resetLayoutWidth(axis: DraggableLayoutAxis): void {
  const pending = keyTimers.get(axis)
  if (pending) {
    clearTimeout(pending)
    keyTimers.delete(axis)
  }
  setLayoutOverride(axis, undefined)
  void persistLayoutWidth(axis, null)
}
