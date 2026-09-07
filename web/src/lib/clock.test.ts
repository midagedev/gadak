/*
 * lib/clock.svelte.ts (GDK-1584): one interval while anyone listens, none
 * when nobody does. Fake timers drive the tick; module state is isolated
 * per test with resetModules so one test's subscriber never leaks into the
 * next assertion.
 */
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

describe('shared wall clock (GDK-1584)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetModules()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  test('no interval at import; the first subscriber starts exactly one', async () => {
    const start = vi.spyOn(globalThis, 'setInterval')
    const clock = await import('./clock.svelte')
    expect(start).not.toHaveBeenCalled()

    const off = clock.subscribeWallClock(vi.fn())
    expect(start).toHaveBeenCalledTimes(1)
    expect(start).toHaveBeenLastCalledWith(expect.any(Function), clock.WALL_CLOCK_TICK_MS)
    expect(clock.wallClockSubscribers()).toBe(1)
    off()
  })

  test('listeners fire once per tick while subscribed', async () => {
    const clock = await import('./clock.svelte')
    const a = vi.fn()
    const off = clock.subscribeWallClock(a)

    vi.advanceTimersByTime(clock.WALL_CLOCK_TICK_MS)
    expect(a).toHaveBeenCalledTimes(1)
    // A partial tick does not fire, and a double-length one fires twice —
    // the cadence is the interval's, not the listener's.
    vi.advanceTimersByTime(clock.WALL_CLOCK_TICK_MS / 2)
    expect(a).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(clock.WALL_CLOCK_TICK_MS)
    expect(a).toHaveBeenCalledTimes(2)
    off()
  })

  test('the last unsubscribe clears the interval', async () => {
    const clock = await import('./clock.svelte')
    const clear = vi.spyOn(globalThis, 'clearInterval')
    const a = vi.fn()
    const off = clock.subscribeWallClock(a)
    expect(clear).not.toHaveBeenCalled()

    off()
    expect(clock.wallClockSubscribers()).toBe(0)
    expect(clear).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(clock.WALL_CLOCK_TICK_MS * 3)
    expect(a).not.toHaveBeenCalled()
  })

  test('one subscriber leaving does not stop the clock for the other', async () => {
    const clock = await import('./clock.svelte')
    const clear = vi.spyOn(globalThis, 'clearInterval')
    const a = vi.fn()
    const b = vi.fn()
    const offA = clock.subscribeWallClock(a)
    const offB = clock.subscribeWallClock(b)

    offA()
    expect(clear).not.toHaveBeenCalled()
    expect(clock.wallClockSubscribers()).toBe(1)

    vi.advanceTimersByTime(clock.WALL_CLOCK_TICK_MS)
    expect(a).not.toHaveBeenCalled()
    expect(b).toHaveBeenCalledTimes(1)
    offB()
  })
})
