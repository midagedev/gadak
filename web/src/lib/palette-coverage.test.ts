/*
 * F1 — the palette-coverage gate (GDK-137).
 *
 * UX_PRINCIPLES §3 promises that every action the app can take is registered
 * in the palette and is auditable, and the audit was a habit: diff the two
 * lists by hand when a wave adds a screen. It measured nothing, and the gap
 * it let through was the plainest one — every column view had a row to enter
 * it and none had a row to leave it (`list`, the resting state, was not in
 * the palette at all). Adding that row closes today's gap; this file is what
 * stops tomorrow's.
 *
 * The rule it enforces: every destination the main column can show is
 * reachable from the palette. Both sides are read from their own owner —
 * COLUMN_KINDS (derived from the ColumnView union, compiler-checked in
 * lib/column-view.ts) and the `opens` field on the palette specs in
 * COMMANDS. There is deliberately no second hand-written list here: a list
 * a test keeps for itself is a list that drifts away from the app.
 */

import { describe, expect, test } from 'vitest'
import { COLUMN_KINDS, type ColumnKind } from './column-view'
import { COMMANDS } from './commands'
import { RESIZE_GRIP_ORIENTATION, type DraggableLayoutAxis } from './viewport-regime'

/**
 * Destinations that legitimately have no palette row. Every entry carries the
 * reason it is exempt — the type makes an undocumented exemption impossible
 * to add, which is the point of the allowlist existing at all.
 */
const NO_PALETTE_ROW: Partial<Record<ColumnKind, string>> = {
  // One space per document tree: a static row cannot name which one. The
  // palette reaches a space through its own document search rows (section
  // 'docs'), and "Open documents" (a:docs) is the door to the index.
  space: 'per-instance destination — opened by name through the palette document rows',
  // Same shape: a dashboard is opened by id. The palette lists dashboards as
  // view rows, not as one fixed action.
  dashboard: 'per-instance destination — opened by id from the palette view rows',
}

function openers(): Map<ColumnKind, string[]> {
  const out = new Map<ColumnKind, string[]>()
  for (const cmd of COMMANDS) {
    const opens = cmd.palette?.opens
    if (!opens) continue
    out.set(opens, [...(out.get(opens) ?? []), cmd.palette!.id])
  }
  return out
}

describe('palette covers every column destination', () => {
  test('each ColumnKind has a palette row or a documented exemption', () => {
    const by = openers()
    const uncovered = COLUMN_KINDS.filter((kind) => !by.has(kind) && !NO_PALETTE_ROW[kind])
    expect(uncovered, `column destinations with no palette row and no exemption: ${uncovered.join(', ')}`).toEqual([])
  })

  test('every exemption names a reason', () => {
    for (const [kind, reason] of Object.entries(NO_PALETTE_ROW)) {
      expect(reason?.trim(), `exemption for '${kind}' must say why`).toBeTruthy()
    }
  })

  test('the exemption list holds no destination that does have a row', () => {
    const by = openers()
    const stale = Object.keys(NO_PALETTE_ROW).filter((k) => by.has(k as ColumnKind))
    expect(stale, `exempted but now covered — delete the exemption: ${stale.join(', ')}`).toEqual([])
  })

  test('no two palette rows claim the same destination', () => {
    for (const [kind, ids] of openers()) {
      expect(ids, `${kind} is opened by more than one palette row`).toHaveLength(1)
    }
  })
})

/*
 * GDK-1796: the same audit for the layout grips. A grip is keyboard-capable
 * (arrows resize, Shift steps 1px, Backspace resets — LayoutResizeHandle) but
 * it sat roughly 150 tab stops deep, behind the whole sidebar and the whole
 * issue list: capability without reachability, and nothing measured
 * reachability. A palette row that focuses the grip is this app's grammar for
 * a motion whose only home is a keystroke (UX_PRINCIPLES §3), and this block
 * is what stops the next draggable axis from shipping without one.
 *
 * The axis list is read off a registry for the same reason the block above
 * reads COLUMN_KINDS: the Record type is compiler-checked in both directions
 * (every axis declares itself, no entry names a stranger), so this file
 * keeps no second list to drift.
 *
 * GDK-1815 moved WHICH registry, and that move is the finding. It used to be
 * LAYOUT_DRAG_CLAMP — the map of `ui.tokens.layout` drag ranges, two entries
 * — so this gate quantified over "axes whose width is a config token" while
 * claiming to quantify over "axes you can drag". The terminal dock is
 * draggable and is not a token, so the one axis in the app with no keyboard
 * door was the one axis this gate could not see, and it stayed green for it.
 * RESIZE_GRIP_ORIENTATION is keyed by DraggableLayoutAxis itself, which is
 * the set the grips are quantified over, so a new draggable seam cannot
 * enter the app without entering this list.
 */
const DRAGGABLE_AXES = Object.keys(RESIZE_GRIP_ORIENTATION) as DraggableLayoutAxis[]

/**
 * Axes whose grip legitimately has no palette row. Empty today — kept (and
 * kept the same shape as NO_PALETTE_ROW) so the first future exemption is
 * forced to carry its reason rather than quietly delete a row.
 */
const NO_GRIP_ROW: Partial<Record<DraggableLayoutAxis, string>> = {}

function gripFocusers(): Map<DraggableLayoutAxis, string[]> {
  const out = new Map<DraggableLayoutAxis, string[]>()
  for (const cmd of COMMANDS) {
    const axis = cmd.palette?.axis
    if (!axis) continue
    out.set(axis, [...(out.get(axis) ?? []), cmd.palette!.id])
  }
  return out
}

describe('palette reaches every draggable axis grip', () => {
  test('each DraggableLayoutAxis has a grip-focusing palette row or a documented exemption', () => {
    const by = gripFocusers()
    const uncovered = DRAGGABLE_AXES.filter((axis) => !by.has(axis) && !NO_GRIP_ROW[axis])
    expect(
      uncovered,
      `draggable axes with no grip-focusing palette row and no exemption: ${uncovered.join(', ')}`,
    ).toEqual([])
  })

  test('every grip exemption names a reason', () => {
    for (const [axis, reason] of Object.entries(NO_GRIP_ROW)) {
      expect(reason?.trim(), `exemption for '${axis}' must say why`).toBeTruthy()
    }
  })

  test('the grip exemption list holds no axis that does have a row', () => {
    const by = gripFocusers()
    const stale = Object.keys(NO_GRIP_ROW).filter((a) => by.has(a as DraggableLayoutAxis))
    expect(stale, `exempted but now covered — delete the exemption: ${stale.join(', ')}`).toEqual([])
  })

  test('no two palette rows claim the same axis', () => {
    for (const [axis, ids] of gripFocusers()) {
      expect(ids, `${axis} is focused by more than one palette row`).toHaveLength(1)
    }
  })
})
