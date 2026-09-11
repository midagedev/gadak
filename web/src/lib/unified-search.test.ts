import { describe, expect, test, vi } from 'vitest'
import {
  createUnifiedSearch,
  excludeLocalKeys,
  isStale,
  projectUnifiedHits,
} from './unified-search'

describe('unified search — cancel / project', () => {
  test('isStale is true only when the request id is not current', () => {
    expect(isStale(1, 2)).toBe(true)
    expect(isStale(4, 4)).toBe(false)
    expect(isStale(0, 1)).toBe(true)
  })

  test('excludeLocalKeys drops keys the local section already shows', () => {
    expect(excludeLocalKeys(['A', 'B', 'C'], new Set(['B']))).toEqual(['A', 'C'])
    expect(excludeLocalKeys(['A'], new Set())).toEqual(['A'])
  })

  test('projectUnifiedHits dedupes local keys and caps the preview', () => {
    const keys = Array.from({ length: 12 }, (_, i) => `NMB-${i}`)
    const pages = Array.from({ length: 9 }, (_, i) => ({
      key: `p-${i}`,
      title: `Page ${i}`,
      space_key: 'ENG',
      parent_id: null,
      author: null,
      updated_at: null,
      version: 1,
      url: '',
    }))
    const res = {
      keys,
      total: 21,
      pages,
      matches: { 'NMB-3': { field: 'body' as const, snippet: 'body hit' } },
    }
    const out = projectUnifiedHits(res, new Set(['NMB-0', 'NMB-1']), new Set(['p-0']))
    expect(out.issues.map((h) => h.key)).toEqual(keys.slice(2, 10))
    expect(out.pages.map((h) => h.key)).toEqual(pages.slice(1, 7).map((p) => p.key))
    expect(out.issues[1]?.match?.field).toBe('body')
    expect(out.truncated).toBe(true)
  })

  test('createUnifiedSearch drops a stale response after a newer query', async () => {
    let finishSlow: (value: { keys: string[]; total: number; pages: []; matches: {} }) => void
    const slow = new Promise<{ keys: string[]; total: number; pages: []; matches: {} }>((resolve) => {
      finishSlow = resolve
    })
    const views: { status: string; query: string }[] = []
    const handle = createUnifiedSearch({
      debounceMs: 1,
      fetch: (q) => {
        if (q === 'old') return slow
        return Promise.resolve({ keys: ['NEW-1'], total: 1, pages: [], matches: {} })
      },
      onView: (v) => views.push({ status: v.status, query: v.query }),
    })
    handle.request('old')
    await new Promise((r) => setTimeout(r, 15))
    handle.request('new')
    await new Promise((r) => setTimeout(r, 15))
    finishSlow!({ keys: ['OLD-1'], total: 1, pages: [], matches: {} })
    await new Promise((r) => setTimeout(r, 15))
    const ready = views.filter((v) => v.status === 'ready')
    expect(ready).toEqual([{ status: 'ready', query: 'new' }])
    handle.cancel()
  })

  test('createUnifiedSearch cancel drops an in-flight response (closed palette)', async () => {
    let finish: (value: { keys: string[]; total: number; pages: []; matches: {} }) => void
    const pending = new Promise<{ keys: string[]; total: number; pages: []; matches: {} }>((resolve) => {
      finish = resolve
    })
    const views: string[] = []
    const handle = createUnifiedSearch({
      debounceMs: 1,
      fetch: () => pending,
      onView: (v) => views.push(v.status),
    })
    handle.request('workaround')
    await new Promise((r) => setTimeout(r, 15))
    handle.cancel()
    finish!({ keys: ['NMB-42'], total: 1, pages: [], matches: {} })
    await new Promise((r) => setTimeout(r, 15))
    expect(views.filter((s) => s === 'ready')).toEqual([])
  })
})

describe('unified search — the debounce window (GDK-1790)', () => {
  // The palette's production shape: UNIFIED_DEBOUNCE_MS = 250 (restated here,
  // not imported, so changing the constant fails this test loudly instead of
  // silently testing some other window). These pin what the e2e 'no request
  // while typing' assertion leans on: a burst collapses to one fetch for the
  // final query, and a pause wider than the window costing a search is the
  // designed behavior, not a leak.

  function recorder(fetched: string[]) {
    return createUnifiedSearch({
      debounceMs: 250,
      fetch: (q) => {
        fetched.push(q)
        return Promise.resolve({ keys: [], total: 0, pages: [], matches: {} })
      },
      onView: () => {},
    })
  }

  test('a typing burst fires one fetch, only after the last keystroke', () => {
    vi.useFakeTimers()
    try {
      const fetched: string[] = []
      const handle = recorder(fetched)
      // 'runbook' typed at the e2e spec's 20ms delay: request() per keystroke.
      for (const partial of ['r', 'ru', 'run', 'runb', 'runbo', 'runboo', 'runbook']) {
        handle.request(partial)
        vi.advanceTimersByTime(20)
      }
      // 20ms after the last keystroke: still inside the window — nothing fired.
      expect(fetched).toEqual([])
      vi.advanceTimersByTime(229)
      expect(fetched).toEqual([])
      // 250ms after the last keystroke, to the millisecond: the whole burst is
      // one request, and it carries the final query, not any prefix.
      vi.advanceTimersByTime(1)
      expect(fetched).toEqual(['runbook'])
    } finally {
      vi.useRealTimers()
    }
  })

  test('a pause wider than the window fires mid-typing — by design', () => {
    // GDK-1790's verdict: when key gaps stretch past the window (a loaded
    // machine, a human who stopped typing), the search request that appears
    // is this rule working, not a debounce leak.
    vi.useFakeTimers()
    try {
      const fetched: string[] = []
      const handle = recorder(fetched)
      handle.request('run')
      vi.advanceTimersByTime(251)
      expect(fetched).toEqual(['run'])
      handle.request('runbook')
      vi.advanceTimersByTime(251)
      expect(fetched).toEqual(['run', 'runbook'])
    } finally {
      vi.useRealTimers()
    }
  })
})
