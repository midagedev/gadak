import { describe, expect, test, vi } from 'vitest'
import {
  createVerbThrottle,
  dataMessageFromError,
  FRAME_MESSAGE_TYPES,
  parseFrameMessage,
} from './dashboard-protocol'

/*
 * The frame→parent whitelist is a security decision expressed as a list:
 * `refresh` and `open` are the only verbs an authored dashboard gets. These
 * tests exist so that widening it (adding 'navigate', anything URL-shaped)
 * is a change someone sees here first, not an accident that ships.
 */

describe('parseFrameMessage', () => {
  test('accepts the one whitelisted verb', () => {
    expect(parseFrameMessage({ type: 'refresh' })).toEqual({ type: 'refresh' })
  })

  test('rejects every other type — URL and navigation verbs included', () => {
    for (const data of [
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

  test('the whitelist is exactly two verbs', () => {
    expect([...FRAME_MESSAGE_TYPES]).toEqual(['refresh', 'open'])
  })
})

describe('parseFrameMessage — the open verb', () => {
  test('accepts an in-app hash', () => {
    expect(parseFrameMessage({ type: 'open', hash: '#/?issue=GDK-1' })).toEqual({
      type: 'open',
      hash: '#/?issue=GDK-1',
    })
  })

  test('rejects every non-hash payload', () => {
    for (const data of [
      { type: 'open' }, // no hash at all
      { type: 'open', url: 'https://evil.example' }, // a URL field is not a hash
      { type: 'open', hash: 7 },
      { type: 'open', hash: null },
      { type: 'open', hash: ['#/'] },
      { type: 'open', hash: '' }, // not '#'-prefixed…
      { type: 'open', hash: '/?issue=GDK-1' }, // …even when it looks like a route
      { type: 'open', hash: 'https://evil.example' },
      { type: 'open', hash: '#/\n' }, // raw control characters never ride a click
    ]) {
      expect(parseFrameMessage(data), `${JSON.stringify(data)} must not parse`).toBeNull()
    }
  })

  test('the length ceiling: 2048 parses, 2049 does not', () => {
    // Deliberately the literal, not the module constant: the ceiling is a
    // security boundary, and moving it should be a two-place decision.
    expect(parseFrameMessage({ type: 'open', hash: '#' + 'a'.repeat(2047) })).not.toBeNull()
    expect(parseFrameMessage({ type: 'open', hash: '#' + 'a'.repeat(2048) })).toBeNull()
  })

  test('extra fields do not ride along on open', () => {
    expect(parseFrameMessage({ type: 'open', hash: '#/', url: 'https://evil.example' })).toEqual({
      type: 'open',
      hash: '#/',
    })
  })

  test('a URL-shaped string behind # is still just a fragment', () => {
    // It parses on purpose: a fragment is same-document by construction, so
    // the app's own grammar (parseHash) coerces this to an unknown path and
    // renders the default view. No navigation to the named host happens —
    // this test pins that the payload being URL-*shaped* is not itself the
    // threat; leaving the document is, and only the host can navigate.
    expect(parseFrameMessage({ type: 'open', hash: '#https://evil.example' })).toEqual({
      type: 'open',
      hash: '#https://evil.example',
    })
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

describe('createVerbThrottle', () => {
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
    const throttle = createVerbThrottle(2000, timer)
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
    const throttle = createVerbThrottle(5000, timer)
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
