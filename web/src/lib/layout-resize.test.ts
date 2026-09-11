/*
 * GDK-759 contracts for the column resize handles.
 *
 * Three things are pinned here, each one a defect this round could otherwise
 * have shipped silently:
 *   1. the drag limit IS the dim catalog's range (read off disk), so the
 *      handle can never paint a width outside the tested range (the server
 *      carries an out-of-range dimension with a warning, so the clamp is the
 *      only thing standing there);
 *   2. a reset DELETES ui.tokens.layout.<axis> — it does not write the
 *      shipped px, because only an absent token lets app.css's per-track
 *      var() fallbacks resolve (GDK-769);
 *   3. a burst of arrow presses is ONE PUT, not one per press.
 * The "one PUT per pointer drag" half is measured against the real server in
 * e2e/layout-resize.spec.ts — there is no pointer here.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

vi.mock('./config', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./config')>()
  return { ...actual, isHostedDemo: () => false }
})
vi.mock('../stores/write.svelte', () => ({ write: { toast: vi.fn() } }))

import {
  KEY_COMMIT_DELAY_MS,
  RESIZE_STEP_FINE_PX,
  RESIZE_STEP_PX,
  handleResizeKey,
  persistLayoutWidth,
  resetLayoutWidth,
  widthForKey,
} from './layout-resize'
import { LAYOUT_DRAG_CLAMP, applyLayoutDimOverrides, clampLayoutPx } from './viewport-regime'

const HERE = dirname(fileURLToPath(import.meta.url))
const CATALOG = join(HERE, '../../../internal/config/tokencheck/dim-catalog.json')

type Call = { method: string; body: unknown }
const calls: Call[] = []
let stored: Record<string, unknown> = {}
const originalFetch = globalThis.fetch

beforeEach(() => {
  calls.length = 0
  stored = { projects: ['NMB'], ui: { tokens: { layout: { sidebar: '300px' } } } }
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase()
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method, body })
    if (method === 'PUT') stored = body as Record<string, unknown>
    void input
    return new Response(JSON.stringify(method === 'GET' ? stored : (body ?? {})), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as typeof fetch
})

afterEach(() => {
  globalThis.fetch = originalFetch
  applyLayoutDimOverrides(null)
  vi.useRealTimers()
})

describe('the drag limit is the dim catalog (GDK-759)', () => {
  test('LAYOUT_DRAG_CLAMP equals the catalog range for both draggable axes', () => {
    /*
     * FAIL-first: this is the assertion that fails if someone widens
     * layout.list in dim-catalog.json (or narrows layout.sidebar) without
     * moving the handle with it. Without it, the handle would happily paint
     * a width outside the range the catalog says is tested — which the
     * server carries with a standing advisory rather than refusing, so
     * nothing downstream would ever surface the mistake.
     */
    const doc = JSON.parse(readFileSync(CATALOG, 'utf8')) as {
      axes: { id: string; tokens: Record<string, { min?: number; max?: number }> }[]
    }
    const layout = doc.axes.find((a) => a.id === 'layout')
    expect(layout, 'the catalog has a layout axis').toBeTruthy()
    for (const axis of ['sidebar', 'list'] as const) {
      const entry = layout?.tokens[axis]
      expect(entry, `catalog has layout.${axis}`).toBeTruthy()
      expect({ min: entry?.min, max: entry?.max }, `layout.${axis} range`).toEqual(
        LAYOUT_DRAG_CLAMP[axis],
      )
    }
  })

  test('clampLayoutPx stops at the range ends and rounds to a whole pixel', () => {
    expect(clampLayoutPx('sidebar', 40)).toBe(LAYOUT_DRAG_CLAMP.sidebar.min)
    expect(clampLayoutPx('sidebar', 9_000)).toBe(LAYOUT_DRAG_CLAMP.sidebar.max)
    expect(clampLayoutPx('list', 10)).toBe(LAYOUT_DRAG_CLAMP.list.min)
    expect(clampLayoutPx('list', 9_000)).toBe(LAYOUT_DRAG_CLAMP.list.max)
    expect(clampLayoutPx('sidebar', 271.6)).toBe(272)
  })

  test('an arrow press moves one step and never leaves the range', () => {
    expect(widthForKey('sidebar', 272, 'ArrowRight', false)).toBe(272 + RESIZE_STEP_PX)
    expect(widthForKey('sidebar', 272, 'ArrowLeft', true)).toBe(272 - RESIZE_STEP_FINE_PX)
    expect(widthForKey('sidebar', LAYOUT_DRAG_CLAMP.sidebar.max, 'ArrowRight', false)).toBe(
      LAYOUT_DRAG_CLAMP.sidebar.max,
    )
    expect(widthForKey('sidebar', 272, 'Enter', false)).toBeNull()
  })
})

describe('the save writes the same tokens the CLI writes (GDK-759)', () => {
  test('a width lands in ui.tokens.layout.<axis> as a px string', async () => {
    await persistLayoutWidth('list', 640)
    const put = calls.find((c) => c.method === 'PUT')
    expect(put, 'exactly one PUT').toBeTruthy()
    expect(calls.filter((c) => c.method === 'PUT')).toHaveLength(1)
    const body = put?.body as { ui: { tokens: { layout: Record<string, string> } } }
    expect(body.ui.tokens.layout).toEqual({ sidebar: '300px', list: '640px' })
  })

  test('a reset DELETES the key instead of writing the shipped px', async () => {
    /*
     * FAIL-first: an implementation that "resets" by writing 272px passes
     * every visual check and still breaks the list, whose three tracks
     * (1360px capped, 1fr docked, the browse clamp) only exist while the
     * token is ABSENT. The assertion is the absence, twice: the key is
     * gone, and no px for that axis appears anywhere in the document.
     */
    stored = {
      projects: ['NMB'],
      ui: { tokens: { layout: { sidebar: '300px', list: '640px' } } },
    }
    await persistLayoutWidth('list', null)
    const body = calls.at(-1)?.body as { ui: { tokens: { layout: Record<string, string> } } }
    expect(Object.keys(body.ui.tokens.layout)).toEqual(['sidebar'])
    expect('list' in body.ui.tokens.layout).toBe(false)
  })

  test('resetting the last axis drops the layout map, not just the key', async () => {
    stored = { projects: ['NMB'], ui: { tokens: { layout: { list: '640px' } } } }
    await persistLayoutWidth('list', null)
    const body = calls.at(-1)?.body as { ui: { tokens: { layout?: unknown } } }
    expect(body.ui.tokens.layout).toBeUndefined()
  })

  test('resetLayoutWidth persists the deletion and unsets the local paint', async () => {
    applyLayoutDimOverrides({ '--layout-list': '640px' })
    resetLayoutWidth('list')
    const { effectiveLayout } = await import('./viewport-regime')
    expect(effectiveLayout().list, 'local paint is unset, not a number').toBeUndefined()
    await vi.waitFor(() => expect(calls.some((c) => c.method === 'PUT')).toBe(true))
  })
})

describe('one save per intent, never per frame (GDK-759)', () => {
  test('a burst of arrow presses is a single PUT', async () => {
    /*
     * FAIL-first: the naive handler saves on every press. Ten presses of a
     * held arrow would be ten read-modify-write round trips through the
     * serialized settings queue, each rewriting config.json — which is the
     * same failure a per-frame pointer save would be, reachable from the
     * keyboard.
     */
    vi.useFakeTimers()
    for (let i = 0; i < 10; i++) handleResizeKey('sidebar', 'ArrowRight', false, null)
    expect(calls.filter((c) => c.method === 'PUT'), 'nothing saved mid-burst').toHaveLength(0)
    await vi.advanceTimersByTimeAsync(KEY_COMMIT_DELAY_MS + 50)
    vi.useRealTimers()
    await vi.waitFor(() =>
      expect(calls.filter((c) => c.method === 'PUT')).toHaveLength(1),
    )
  })
})
