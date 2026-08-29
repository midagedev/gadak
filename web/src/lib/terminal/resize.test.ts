import { describe, expect, it } from 'vitest'
import { RESIZE_SETTLE_MS, settleResize } from './resize'

// GDK-1154. FAIL-first is on the pane, not here: before the fix, the shell
// sent its size at create and at socket-open and never again, and sessions
// were measured at cols 10 / rows 5 while the pane rendered 48x42. What is
// unit-testable is the schedule that closes the window — that it re-checks
// more than once, that the first check is immediate, that it reaches past
// the frame in which the socket opened, and that leaving the pane cancels
// the ticks that have not fired.

describe('settleResize — the post-open size reconciliation schedule', () => {
  function fakeTimers() {
    const pending = new Map<number, { fn: () => void; ms: number }>()
    let next = 1
    const schedule = (fn: () => void, ms: number) => {
      const h = next++
      pending.set(h, { fn, ms })
      return h as unknown as ReturnType<typeof setTimeout>
    }
    const cancel = (h: ReturnType<typeof setTimeout>) => {
      pending.delete(h as unknown as number)
    }
    const fireAll = () => {
      for (const [h, t] of [...pending]) {
        pending.delete(h)
        t.fn()
      }
    }
    return { pending, schedule, cancel, fireAll }
  }

  it('re-checks more than once, starting immediately and reaching past the open frame', () => {
    expect(RESIZE_SETTLE_MS.length).toBeGreaterThan(1)
    expect(RESIZE_SETTLE_MS[0]).toBe(0)
    // A single frame is ~16ms; a layout that settles a few frames after the
    // socket opens is exactly the case that produced 10x5.
    expect(Math.max(...RESIZE_SETTLE_MS)).toBeGreaterThanOrEqual(500)
  })

  it('runs the check once per delay', () => {
    const t = fakeTimers()
    let calls = 0
    settleResize(() => calls++, t.schedule, t.cancel)
    expect(calls).toBe(0) // nothing runs synchronously — the caller owns the first send
    t.fireAll()
    expect(calls).toBe(RESIZE_SETTLE_MS.length)
  })

  it('cancel stops every tick that has not fired', () => {
    const t = fakeTimers()
    let calls = 0
    const stop = settleResize(() => calls++, t.schedule, t.cancel)
    stop()
    t.fireAll()
    expect(calls).toBe(0)
    expect(t.pending.size).toBe(0)
  })
})
