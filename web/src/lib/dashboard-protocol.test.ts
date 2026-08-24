import { describe, expect, test, vi } from 'vitest'
import {
  createRefreshThrottle,
  dataMessageFromError,
  FRAME_MESSAGE_TYPES,
  parseFrameMessage,
} from './dashboard-protocol'

/*
 * The frame→parent whitelist is a security decision expressed as a list:
 * `refresh` is the only verb an authored dashboard gets. These tests exist so
 * that widening it (adding 'open', 'navigate', anything URL-shaped) is a
 * change someone sees here first, not an accident that ships.
 */

describe('parseFrameMessage', () => {
  test('accepts the one whitelisted verb', () => {
    expect(parseFrameMessage({ type: 'refresh' })).toEqual({ type: 'refresh' })
  })

  test('rejects every other type — URL and navigation verbs included', () => {
    for (const data of [
      { type: 'open', url: 'https://evil.example' },
      { type: 'navigate', to: '/admin' },
      { type: 'refreshAll' },
      { type: 'REFRESH' },
      { type: '' },
      {},
      { type: 7 },
      { type: null },
      'refresh',
      null,
      undefined,
      [],
      [{ type: 'refresh' }],
    ]) {
      expect(parseFrameMessage(data), `${JSON.stringify(data)} must not parse`).toBeNull()
    }
  })

  test('extra fields do not smuggle a second verb in', () => {
    // The whitelist checks the type, and only the type survives parsing —
    // a payload cannot ride along on a whitelisted message.
    expect(parseFrameMessage({ type: 'refresh', href: 'https://evil.example' })).toEqual({
      type: 'refresh',
    })
  })

  test('the whitelist is exactly one verb', () => {
    expect([...FRAME_MESSAGE_TYPES]).toEqual(['refresh'])
  })
})

describe('dataMessageFromError', () => {
  test('a failed datasource answers the same shape as a successful one', () => {
    expect(dataMessageFromError('by_status', 'datasource failed: sql_error')).toEqual({
      type: 'data',
      name: 'by_status',
      columns: [],
      rows: [],
      truncated: false,
      warning: 'datasource failed: sql_error',
    })
  })
})

describe('createRefreshThrottle', () => {
  function fakeTimer() {
    const timers: { fn: () => void; ms: number }[] = []
    return {
      timer: {
        setTimeout(fn: () => void, ms: number) {
          timers.push({ fn, ms })
          return timers.length
        },
      },
      pending: () => timers.length > 0,
      fire() {
        const t = timers.shift()
        t?.fn()
      },
    }
  }

  test('first call runs immediately, a burst coalesces to one trailing run', () => {
    const { timer, pending, fire } = fakeTimer()
    const throttle = createRefreshThrottle(2000, timer)
    const run = vi.fn()

    expect(throttle.run(run)).toBe(true) // first is free
    expect(run).toHaveBeenCalledTimes(1)

    // Flood: five immediate re-requests before the interval elapses.
    for (let i = 0; i < 5; i++) expect(throttle.run(run)).toBe(false)
    expect(run).toHaveBeenCalledTimes(1) // nothing ran yet
    expect(pending()).toBe(true) // one trailing run scheduled, not five

    fire()
    expect(run).toHaveBeenCalledTimes(2) // the burst became exactly one run
    expect(pending()).toBe(false)
  })

  test('flush runs a pending trailing call once', () => {
    const { timer } = fakeTimer()
    const throttle = createRefreshThrottle(5000, timer)
    const run = vi.fn()
    throttle.run(run)
    throttle.run(run)
    expect(run).toHaveBeenCalledTimes(1)
    throttle.flush()
    expect(run).toHaveBeenCalledTimes(2)
    throttle.flush() // nothing pending — a second flush is a no-op
    expect(run).toHaveBeenCalledTimes(2)
  })
})
