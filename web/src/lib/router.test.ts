/*
 * The router's write rules, without a browser (GDK-1327).
 *
 * lib/router.svelte.ts coalesces history writes: one entry per user action,
 * where "one task" is the unit of action — a second push in the same task
 * becomes a replace, a setTimeout(0) ends the task, and so does the next
 * input event (Chromium runs input ahead of timers). Until now only
 * e2e/history-nav.spec.ts exercised this, so every coalescing regression
 * cost a browser round. The mechanism is pure (history + location + window
 * events + timers), which is exactly what a vitest with stubbed globals and
 * fake timers can drive exhaustively — the e2e keeps the per-surface
 * back/forward smoke, this file owns the mechanism.
 *
 * Everything here runs under the pages-store project (svelte plugin): the
 * module uses runes. The globals are stubbed BEFORE the dynamic import
 * because the module reads location/window once, at import time.
 */
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

type Op = { op: 'push' | 'replace'; hash: string; state: unknown }

let ops: Op[]
let listeners: Map<string, Array<(ev: { type: string }) => void>>
let currentHash: string
let historyState: unknown
let fakeWindow: { addEventListener: (t: string, fn: (ev: { type: string }) => void) => void }

type Router = typeof import('./router.svelte')

async function loadRouter(): Promise<Router> {
  return (await import('./router.svelte')) as Router
}

function fire(type: string): void {
  for (const fn of listeners.get(type) ?? []) fn({ type })
}

beforeEach(() => {
  vi.resetModules()
  vi.useFakeTimers()
  ops = []
  listeners = new Map()
  currentHash = '#/'
  historyState = null
  fakeWindow = {
    addEventListener: (type, fn) => {
      listeners.set(type, [...(listeners.get(type) ?? []), fn])
    },
  }
  vi.stubGlobal('window', fakeWindow)
  vi.stubGlobal('location', {
    get hash() {
      return currentHash
    },
    set hash(v: string) {
      currentHash = v
    },
  })
  vi.stubGlobal('history', {
    get state() {
      return historyState
    },
    pushState: (state: unknown, _unused: string, hash: string) => {
      ops.push({ op: 'push', hash, state })
      historyState = state
      currentHash = hash
    },
    replaceState: (state: unknown, _unused: string, hash: string) => {
      ops.push({ op: 'replace', hash, state })
      historyState = state
      currentHash = hash
    },
  })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('one entry per user action (the coalescing rules)', () => {
  test('a second write in the same task replaces; back walks actions, not writes', async () => {
    const { navigate, setParams } = await loadRouter()
    navigate('/board')
    setParams({ g: 'none' })
    expect(ops).toEqual([
      { op: 'push', hash: '#/board', state: null },
      { op: 'replace', hash: '#/board?g=none', state: null },
    ])
  })

  test('a setTimeout(0) ends the task: the next write pushes again', async () => {
    const { navigate } = await loadRouter()
    navigate('/board')
    await vi.advanceTimersByTimeAsync(1)
    navigate('/inbox')
    expect(ops.map((o) => o.op)).toEqual(['push', 'push'])
  })

  test.each(['pointerdown', 'keydown', 'input'])(
    'a %s event ends the task outright, before the timer',
    async (type) => {
      const { navigate } = await loadRouter()
      navigate('/board')
      // No timer flush: input arriving mid-task must still get its own entry.
      fire(type)
      navigate('/inbox')
      expect(ops.map((o) => o.op)).toEqual(['push', 'push'])
    },
  )

  test('writing the hash the location already holds adds no history entry', async () => {
    const { navigate } = await loadRouter()
    navigate('/board')
    // Same task: coalesced — at worst a same-hash replace, never a second
    // entry. (The product's no-op guard fires on the push branch only; a
    // same-task write lands in the coalescing branch. Either way: one push.)
    navigate('/board')
    expect(ops.filter((o) => o.op === 'push')).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    // New task, same hash: guarded to a no-op outright.
    navigate('/board')
    expect(ops.filter((o) => o.op === 'push')).toHaveLength(1)
    expect(ops.every((o) => o.hash === '#/board')).toBe(true)
  })
})

describe('entry markers (dialogs: rule 2)', () => {
  test('a replace keeps the pushed entry’s marker — tab switches never erase it', async () => {
    const { setParams, atEntry } = await loadRouter()
    // A dialog opening pushes one marked entry…
    setParams({ settings: 'sync' }, false, 'dialog:settings')
    expect(ops[0]).toMatchObject({ op: 'push', state: { entry: 'dialog:settings' } })
    expect(atEntry('dialog:settings')).toBe(true)
    // …its tabs are continuous input, rewrites that must keep the marker.
    setParams({ settings: 'about' }, true)
    expect(ops[1]).toMatchObject({ op: 'replace', state: { entry: 'dialog:settings' } })
    expect(atEntry('dialog:settings')).toBe(true)
  })

  test('atEntry is false for a plain push and after the state moves on', async () => {
    const { navigate, setParams, atEntry } = await loadRouter()
    // Each action gets its own task boundary — without the flushes this
    // would all coalesce into one entry, which is the rule, not a leak.
    navigate('/board')
    await vi.advanceTimersByTimeAsync(1)
    expect(atEntry('dialog:settings')).toBe(false)
    setParams({ settings: 'workspaces' }, false, 'dialog:settings')
    await vi.advanceTimersByTimeAsync(1)
    expect(atEntry('dialog:settings')).toBe(true)
    navigate('/inbox')
    expect(atEntry('dialog:settings')).toBe(false)
  })
})

describe('the reactive hash is in sync at commit time (GDK-1292)', () => {
  test('a second write in one flush builds on the first, without a hashchange', async () => {
    const { navigate, setParams, router } = await loadRouter()
    navigate('/a')
    setParams({ q: 'x' })
    expect(router.path).toBe('/a')
    expect(router.params.get('q')).toBe('x')
  })

  test('a hashchange the browser fires (back/forward) updates current', async () => {
    const { router } = await loadRouter()
    currentHash = '#/inbox?p=2'
    fire('hashchange')
    expect(router.path).toBe('/inbox')
    expect(router.params.get('p')).toBe('2')
  })
})
