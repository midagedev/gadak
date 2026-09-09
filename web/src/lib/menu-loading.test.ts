/*
 * GDK-1566: the shared cap on menu origin round-trips.
 *
 * Two halves, both ladder moves from e2e/menu-loading.spec.ts:
 *
 *  - the timing contract (withMenuTimeout resolves/rejects/passes through)
 *    is a fake-timer unit here — in the browser it cost a 6 s stall per
 *    case and could only ever assert the two ends of one wait;
 *  - the picker's fallback structure — the detail priority picker falls
 *    back to the cached site catalog with the offline note when the
 *    per-key answer times out — is a source scan of PriorityPicker.svelte
 *    (the FeaturesTab.test.ts idiom: vitest is environment:'node', the
 *    unit project loads no svelte plugin, and copy/structure that cannot
 *    drift apart from the source it lives in does not need a browser).
 *
 * The bulk-menu half of the spec (failure sentence + Retry, and a second
 * capped wait after Retry) stays in e2e: it needs the real menu open.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { en } from './i18n/en'
import { MENU_ORIGIN_TIMEOUT_MS, MenuOriginTimeout, withMenuTimeout } from './menu-loading'

const HERE = dirname(fileURLToPath(import.meta.url))
const PICKER = join(HERE, '../components/write/PriorityPicker.svelte')

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

function deferred<T>(): { promise: Promise<T>; resolve(v: T): void; reject(e: unknown): void } {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

/** Settled-event recorder: one entry per settlement, so a promise that
 * settles twice — or never — is distinguishable from one that settles once. */
function eventsOf<T>(p: Promise<T>): { calls: string[]; value: () => T | undefined; error: () => unknown } {
  const calls: string[] = []
  let val: T | undefined
  let err: unknown
  p.then(
    (v) => {
      val = v
      calls.push('fulfilled')
    },
    (e) => {
      err = e
      calls.push('rejected')
    },
  )
  return { calls, value: () => val, error: () => err }
}

describe('withMenuTimeout (GDK-1566)', () => {
  test('an answer inside the cap is the call\'s own answer, and the timer is cleared', async () => {
    // Blocks: a cap that swallows in-time answers, or leaks a timer that
    // rejects an already-settled wait later in the menu's life.
    const d = deferred<string>()
    const events = eventsOf(withMenuTimeout(d.promise))

    d.resolve('site catalog')
    await vi.advanceTimersByTimeAsync(1)
    expect(events.calls).toEqual(['fulfilled'])
    expect(events.value()).toBe('site catalog')
    expect(vi.getTimerCount(), 'the cap timer must be cleared on settle').toBe(0)

    // Past the cap with no timer: nothing fires, the wait stays answered.
    await vi.advanceTimersByTimeAsync(MENU_ORIGIN_TIMEOUT_MS * 2)
    expect(events.calls).toEqual(['fulfilled'])
  })

  test('a silent origin rejects with MenuOriginTimeout at exactly the cap', async () => {
    // Blocks: an uncapped wait (the unreachable-origin 15 s socket hold this
    // cap exists for), or a cap that fires early/late.
    const events = eventsOf(withMenuTimeout(deferred<string>().promise))

    await vi.advanceTimersByTimeAsync(MENU_ORIGIN_TIMEOUT_MS - 1)
    expect(events.calls, 'one ms before the cap the menu is still waiting').toEqual([])

    await vi.advanceTimersByTimeAsync(1)
    expect(events.calls).toEqual(['rejected'])
    const e = events.error() as MenuOriginTimeout
    expect(e).toBeInstanceOf(MenuOriginTimeout)
    expect(e.name).toBe('MenuOriginTimeout')
    expect(e.message).toBe(`origin did not answer within ${MENU_ORIGIN_TIMEOUT_MS} ms`)
  })

  test('the cap is the shared constant, not a per-menu number', () => {
    // Blocks: the next copy of this menu reintroducing its own wait. The
    // audit found the three loaders by their shared shape; the constant is
    // the contract (e2e imports it so no spec re-derives it either).
    expect(MENU_ORIGIN_TIMEOUT_MS).toBe(6_000)
  })

  test('a store error passes through unwrapped — credential refusals keep their dialog path', async () => {
    // Blocks: MenuOriginTimeout swallowing an ApiError: the picker's catch
    // only closes on the timeout; anything else must rethrow as itself.
    const sentinel = new Error('refused')
    const events = eventsOf(withMenuTimeout(Promise.reject(sentinel)))

    await vi.advanceTimersByTimeAsync(1)
    expect(events.calls).toEqual(['rejected'])
    expect(events.error()).toBe(sentinel)
    expect(events.error()).not.toBeInstanceOf(MenuOriginTimeout)
    expect(vi.getTimerCount(), 'the cap timer must be cleared on settle').toBe(0)
  })

  test('a late success does not resurrect the wait', async () => {
    // Blocks: a race that "un-rejects" when the origin finally answers —
    // the menu already showed the fallback; flipping back mid-read is the
    // defect. The request itself is not aborted and its answer still lands
    // in the store for the next open; this promise settles once.
    const d = deferred<string>()
    const events = eventsOf(withMenuTimeout(d.promise))

    await vi.advanceTimersByTimeAsync(MENU_ORIGIN_TIMEOUT_MS)
    expect(events.calls).toEqual(['rejected'])

    d.resolve('late per-key catalog')
    await vi.advanceTimersByTimeAsync(1)
    expect(events.calls, 'a settled race stays settled').toEqual(['rejected'])
    expect(events.error()).toBeInstanceOf(MenuOriginTimeout)
  })
})

describe('detail picker timeout fallback (moved from e2e menu-loading case 2)', () => {
  const picker = readFileSync(PICKER, 'utf8')

  test('the per-key load runs under the shared cap and only the timeout degrades', () => {
    // Blocks: the picker waiting past the cap, or treating every failure as
    // "use the cache" — a credential refusal must still throw.
    expect(picker).toMatch(/withMenuTimeout\(write\.loadPrioritiesFor\(issue\.issue_key\)\)/)
    expect(picker).toMatch(/if \(!\(e instanceof MenuOriginTimeout\)\) throw e/)
  })

  test('a timeout falls back to the site catalog when one is cached, else the error row', () => {
    // Blocks: a timeout on a primed site catalog rendering the error row
    // (or vice versa) — the cached rows ARE the answer the user can act on.
    expect(picker).toMatch(/load = write\.priorities\.length \? 'cached' : 'error'/)
  })

  test('the cached rows carry the offline note, and only when they stand in', () => {
    // Blocks: the note naming a cache the user is not looking at. The note
    // renders only while the rows are the site catalog standing in for the
    // per-key answer the origin never gave.
    expect(picker).toMatch(/load === 'cached' && !write\.hasPrioritiesFor\(issue\.issue_key\)/)
    expect(picker).toMatch(/data-testid="menu-cached-note"[\s\S]{0,120}\{t\('app\.offlineBanner'\)\}/)
    expect(en['app.offlineBanner'], 'the note is real copy, not an empty key').toBeTruthy()
  })
})
