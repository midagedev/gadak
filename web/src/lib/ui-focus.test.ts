/*
 * GDK-960: same `at` is applied once. App.svelte cannot be mounted in the
 * vitest unit project (no svelte plugin — skeleton-grace.test.ts /
 * gdk-944.test.ts), so the apply decision lives here as plain functions.
 *
 * GDK-989: the App.svelte substring scan this file carried is gone. The
 * wiring half ("App calls these") cannot be asserted without a mount, and
 * half of what it pinned was a name that no longer exists anywhere — a
 * regression to it is an import error, which typecheck already is. What
 * made the wiring safe is asserted directly instead: the focus poll the
 * app runs every 500ms degrades to an empty poll on every way the
 * endpoint can disappoint, so the loop cannot be thrown out of.
 */
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  UI_FOCUS_KEY,
  decideMirrorPull,
  readLastFocusKey,
  rememberFocusKey,
  shouldApplyUIFocus,
  uiFocusKey,
} from './ui-focus'
import { pollUIFocus } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GDK-960 shouldApplyUIFocus', () => {
  const H = 'pj=NMA'

  test('the same payload is applied only once', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, H, null)).toBe(true)
    expect(shouldApplyUIFocus(at, H, uiFocusKey(at, H))).toBe(false)
  })

  test('a newer at is applied', () => {
    const last = uiFocusKey('2026-08-26T00:00:00Z', H)
    expect(shouldApplyUIFocus('2026-08-26T00:01:00Z', H, last)).toBe(true)
  })

  /*
   * GDK-981: `at` is a wall-clock stamp and can repeat — an older server
   * writes it at second resolution, so `views open A && views open B` gives
   * both the same one. Keyed on `at` alone the tab that applied A would drop
   * B in silence, which is the symptom GDK-960 set out to fix.
   */
  test('a second hash under the same at is still applied', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, 'pj=NMB', uiFocusKey(at, 'pj=NMA'))).toBe(true)
  })

  test('the same hash under the same at is not applied twice', () => {
    const at = '2026-08-26T00:00:00Z'
    expect(shouldApplyUIFocus(at, 'pj=NMA', uiFocusKey(at, 'pj=NMA'))).toBe(false)
  })

  test('missing at cannot be deduped and is applied', () => {
    const last = uiFocusKey('2026-08-26T00:00:00Z', H)
    expect(shouldApplyUIFocus('', H, last)).toBe(true)
    expect(shouldApplyUIFocus(null, H, last)).toBe(true)
    expect(shouldApplyUIFocus(undefined, H, null)).toBe(true)
  })
})

describe('GDK-960 last-applied key storage', () => {
  const AT = '2026-08-26T00:00:00Z'
  const H = 'pj=NMA'
  const KEY = uiFocusKey(AT, H)

  test('memory is preferred over sessionStorage', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => 'from-store',
      setItem: () => {
        throw new Error('should not write')
      },
    })
    expect(readLastFocusKey('from-memory')).toBe('from-memory')
  })

  test('sessionStorage fills in after a refresh (memory empty)', () => {
    const store: Record<string, string> = {}
    vi.stubGlobal('sessionStorage', {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => {
        store[k] = v
      },
    })
    rememberFocusKey(KEY)
    expect(store[UI_FOCUS_KEY]).toBe(KEY)
    expect(readLastFocusKey(null)).toBe(KEY)
    expect(shouldApplyUIFocus(AT, H, readLastFocusKey(null))).toBe(false)
  })

  test('blocked sessionStorage is treated as not yet applied', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    })
    expect(readLastFocusKey(null)).toBeNull()
    expect(() => rememberFocusKey(KEY)).not.toThrow()
    expect(shouldApplyUIFocus(AT, H, readLastFocusKey(null))).toBe(true)
  })
})

/*
 * GDK-1170: a `gadak claim` in a terminal reaches an open board on the same
 * 500ms poll. The decision is a pure function for the same reason the focus
 * one is — App.svelte cannot be mounted in the vitest unit project — and
 * because the two ways to get it wrong are both silent: pulling on the first
 * sighting re-fetches on every boot, and stacking pulls on a 500ms tick is a
 * new defect rather than a fixed one.
 */
describe('GDK-1170 decideMirrorPull', () => {
  const A = '1788053238.352256/1788053255.259592'
  const B = '1788053238.352256/1788053266.358472'

  test('the first sighting is a baseline, never a pull', () => {
    expect(decideMirrorPull(A, null, false)).toBe('baseline')
  })

  test('a moved version pulls', () => {
    expect(decideMirrorPull(B, A, false)).toBe('pull')
  })

  test('an unmoved version does nothing', () => {
    expect(decideMirrorPull(A, A, false)).toBe('ignore')
  })

  test('no signal does nothing — the 15s backstop owns that board', () => {
    // Older server, a serve with no mirror, and every failed poll.
    expect(decideMirrorPull('', null, false)).toBe('ignore')
    expect(decideMirrorPull('', A, false)).toBe('ignore')
    expect(decideMirrorPull(undefined, A, false)).toBe('ignore')
    expect(decideMirrorPull(null, A, false)).toBe('ignore')
  })

  test('a move during a pull waits instead of stacking a second delta', () => {
    expect(decideMirrorPull(B, A, true)).toBe('wait')
  })

  test('waiting keeps the old baseline, so the next tick still pulls', () => {
    // The tick that answered `wait` must not have adopted B — the caller
    // holds A, and 500ms later the pull is no longer in flight.
    expect(decideMirrorPull(B, A, false)).toBe('pull')
  })

  test('a first sighting during a pull is still only a baseline', () => {
    expect(decideMirrorPull(A, null, true)).toBe('baseline')
  })
})

describe('GDK-960 the focus poll degrades, never throws', () => {
  const EMPTY = { hash: null, at: '', configVersion: '', mirrorVersion: '' }

  test('404 — serve without the endpoint — is the empty poll', async () => {
    vi.stubGlobal('fetch', async () => new Response('no such route', { status: 404 }))
    expect(await pollUIFocus()).toEqual(EMPTY)
  })

  test('204 — an older server with nothing pending — is the empty poll', async () => {
    vi.stubGlobal('fetch', async () => new Response(null, { status: 204 }))
    expect(await pollUIFocus()).toEqual(EMPTY)
  })

  test('an answered 5xx is the empty poll too: the loop keeps looping', async () => {
    vi.stubGlobal(
      'fetch',
      async () =>
        new Response('{"error":"boom"}', {
          status: 500,
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    expect(await pollUIFocus()).toEqual(EMPTY)
  })

  test('a pending payload is mapped, not echoed', async () => {
    vi.stubGlobal(
      'fetch',
      async () =>
        new Response(
          JSON.stringify({
            hash: '#/?pj=NMA',
            at: '2026-08-29T00:00:00Z',
            configVersion: 'in-42',
            mirrorVersion: 'm-7',
          }),
          { headers: { 'Content-Type': 'application/json' } },
        ),
    )
    expect(await pollUIFocus()).toEqual({
      hash: '#/?pj=NMA',
      at: '2026-08-29T00:00:00Z',
      configVersion: 'in-42',
      mirrorVersion: 'm-7',
    })
  })

  test('a blank hash is null and a missing at is the empty string', async () => {
    vi.stubGlobal(
      'fetch',
      async () =>
        new Response(JSON.stringify({ hash: '  ', configVersion: 'in-42' }), {
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    expect(await pollUIFocus()).toEqual({
      hash: null,
      at: '',
      configVersion: 'in-42',
      mirrorVersion: '',
    })
  })

  /*
   * GDK-1170: a server that predates mirrorVersion is the normal path, not an
   * error. Folding a missing or wrong-typed field to '' is what makes
   * decideMirrorPull answer `ignore` there — anything else and an old server
   * would either pull every 500ms or read as "nothing changed".
   */
  test('a server with no mirrorVersion folds to the empty string', async () => {
    vi.stubGlobal(
      'fetch',
      async () =>
        new Response(JSON.stringify({ configVersion: 'in-42' }), {
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    expect((await pollUIFocus()).mirrorVersion).toBe('')
  })

  test('a wrong-typed mirrorVersion folds to the empty string', async () => {
    vi.stubGlobal(
      'fetch',
      async () =>
        new Response(JSON.stringify({ configVersion: 'in-42', mirrorVersion: 17 }), {
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    expect((await pollUIFocus()).mirrorVersion).toBe('')
  })

  test('a fetch throw is the empty poll', async () => {
    vi.stubGlobal('fetch', async () => {
      throw new TypeError('fetch failed')
    })
    expect(await pollUIFocus()).toEqual(EMPTY)
  })
})
