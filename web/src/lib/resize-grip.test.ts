/*
 * GDK-1815 — the grip contract that is the same for every seam.
 *
 * The defect this round closed was the terminal dock having a pointer and no
 * keyboard, and the structural half was that the keyboard could not be given
 * to it without also moving its height into the config document. `ResizeGrip`
 * is the seam that split those two, and this file pins the half that is now
 * shared: what an arrow press means, and that a grip's cadence is paint-many
 * / save-once.
 *
 * Deliberately one rung below e2e (the repo's ladder is type > unit >
 * integration > e2e). The arrow mapping is pure, so it does not need a
 * browser — and it is the piece most likely to be got wrong for the vertical
 * axis, because "up" and "bigger" only coincide on a grip that is on the TOP
 * edge of the thing it sizes.
 *
 * The two vi.mock lines mirror layout-resize.test.ts: importing
 * lib/layout-resize pulls settings-write, which pulls the write store.
 */
import { afterEach, describe, expect, test, vi } from 'vitest'

vi.mock('./config', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./config')>()
  return { ...actual, isHostedDemo: () => false }
})
vi.mock('../stores/write.svelte', () => ({ write: { toast: vi.fn() } }))

import {
  KEY_COMMIT_DELAY_MS,
  RESIZE_STEP_FINE_PX,
  RESIZE_STEP_PX,
  handleGripKey,
  stepForKey,
  type ResizeGrip,
} from './layout-resize'
import { RESIZE_GRIP_ORIENTATION, type DraggableLayoutAxis } from './viewport-regime'

afterEach(() => {
  vi.useRealTimers()
})

describe('one arrow mapping for every grip (GDK-1815)', () => {
  test('a horizontal grip takes Left/Right and a vertical grip takes Up/Down', () => {
    expect(stepForKey('ArrowRight', false, 'horizontal')).toBe(RESIZE_STEP_PX)
    expect(stepForKey('ArrowLeft', false, 'horizontal')).toBe(-RESIZE_STEP_PX)
    expect(stepForKey('ArrowUp', false, 'vertical')).toBe(RESIZE_STEP_PX)
    expect(stepForKey('ArrowDown', false, 'vertical')).toBe(-RESIZE_STEP_PX)
  })

  test('an arrow on the other axis is not this grip’s key', () => {
    /*
     * FAIL-first shape: a handler that answered every arrow would swallow
     * ArrowUp/ArrowDown on a focused column grip, which is how the issue
     * list scrolls while the seam has focus.
     */
    expect(stepForKey('ArrowUp', false, 'horizontal')).toBeNull()
    expect(stepForKey('ArrowDown', false, 'horizontal')).toBeNull()
    expect(stepForKey('ArrowLeft', false, 'vertical')).toBeNull()
    expect(stepForKey('ArrowRight', false, 'vertical')).toBeNull()
    expect(stepForKey('Enter', false, 'vertical')).toBeNull()
  })

  test('Shift is the fine step on both orientations', () => {
    expect(stepForKey('ArrowRight', true, 'horizontal')).toBe(RESIZE_STEP_FINE_PX)
    expect(stepForKey('ArrowUp', true, 'vertical')).toBe(RESIZE_STEP_FINE_PX)
  })

  test('up is taller on the dock, because its grip is on the top edge', () => {
    // Not a restatement of the row above: this is the direction claim the
    // pointer drag has always made (TerminalPane, GDK-1194) and the keyboard
    // now has to agree with, or the two gestures fight each other.
    expect(RESIZE_GRIP_ORIENTATION.terminal).toBe('vertical')
    expect(stepForKey('ArrowUp', false, RESIZE_GRIP_ORIENTATION.terminal)).toBeGreaterThan(0)
  })

  test('every draggable axis declares an orientation', () => {
    // The registry the coverage gate quantifies over. A Record is checked in
    // both directions by the compiler; this is the runtime half — no entry
    // may be left undefined by a later hand-edit.
    for (const [axis, orientation] of Object.entries(RESIZE_GRIP_ORIENTATION)) {
      expect(['horizontal', 'vertical'], `${axis} orientation`).toContain(orientation)
    }
  })
})

/** A grip over a plain number, with its writes counted. */
function fakeGrip(axis: DraggableLayoutAxis, start: number): ResizeGrip & {
  value: number
  commits: number[]
  resets: number
} {
  const g = {
    axis,
    orientation: RESIZE_GRIP_ORIENTATION[axis],
    value: start,
    commits: [] as number[],
    resets: 0,
    min: () => 100,
    max: () => 500,
    current: () => g.value,
    fromPointer: (event: PointerEvent, s: { value: number; clientY: number }) =>
      s.value + (s.clientY - event.clientY),
    paint: (px: number) => {
      g.value = Math.min(500, Math.max(100, Math.round(px)))
    },
    commit: (px: number) => {
      g.commits.push(px)
    },
    reset: () => {
      g.resets += 1
      g.value = start
    },
  }
  return g as ResizeGrip & { value: number; commits: number[]; resets: number }
}

describe('a grip paints many and saves once (GDK-1815)', () => {
  test('a burst of arrow presses is one save, at the clamped value', async () => {
    /*
     * FAIL-first: the dock's old handler called persistHeight on every
     * pointermove frame and had no keyboard at all. The contract the columns
     * already had — and the one this test now holds for every grip — is that
     * the store is written once per intent, and with the number the person
     * actually ended on rather than the number the arithmetic asked for.
     */
    vi.useFakeTimers()
    const g = fakeGrip('terminal', 300)
    for (let i = 0; i < 10; i++) expect(handleGripKey(g, 'ArrowUp', false)).toBe(true)
    expect(g.value, 'painted every press').toBe(300 + 10 * RESIZE_STEP_PX)
    expect(g.commits, 'nothing saved mid-burst').toEqual([])
    await vi.advanceTimersByTimeAsync(KEY_COMMIT_DELAY_MS + 50)
    expect(g.commits).toEqual([380])
  })

  test('the saved value is the clamped one, not the requested one', async () => {
    vi.useFakeTimers()
    const g = fakeGrip('terminal', 496)
    handleGripKey(g, 'ArrowUp', false)
    await vi.advanceTimersByTimeAsync(KEY_COMMIT_DELAY_MS + 50)
    expect(g.commits, 'the ceiling, not 504').toEqual([500])
  })

  test('Backspace and Delete are the keyboard’s double-click', () => {
    const g = fakeGrip('terminal', 300)
    handleGripKey(g, 'ArrowUp', false)
    expect(handleGripKey(g, 'Backspace', false)).toBe(true)
    expect(g.resets).toBe(1)
    expect(handleGripKey(g, 'Delete', false)).toBe(true)
    expect(g.resets).toBe(2)
  })

  test('a reset cancels a pending save rather than racing it', async () => {
    /*
     * Arrow, then Backspace before the burst settles. Without the cancel the
     * debounced write lands AFTER the reset and quietly restores the width
     * the person just threw away — a defect only a clock can see.
     */
    vi.useFakeTimers()
    const g = fakeGrip('terminal', 300)
    handleGripKey(g, 'ArrowUp', false)
    handleGripKey(g, 'Backspace', false)
    await vi.advanceTimersByTimeAsync(KEY_COMMIT_DELAY_MS + 50)
    expect(g.commits, 'the stale arrow save never fired').toEqual([])
  })

  test('a key that is not the grip’s is refused, so the caller can let it through', () => {
    const g = fakeGrip('sidebar', 272)
    expect(handleGripKey(g, 'ArrowUp', false), 'vertical key on a column grip').toBe(false)
    expect(handleGripKey(g, 'Tab', false)).toBe(false)
    expect(g.value, 'nothing painted').toBe(272)
  })
})
