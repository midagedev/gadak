import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  LAYOUT_DETAIL_MIN_PX,
  LAYOUT_LIST_MIN_PX,
  LAYOUT_SIDEBAR_PX,
  VIEWPORT_DOCKED_MIN_PX,
  applyLayoutDimOverrides,
  effectiveLayout,
  layoutTokenStyle,
  subscribeViewportRegime,
} from './viewport-regime'

describe('viewport docked floor (GDK-766)', () => {
  test('track mins sum to VIEWPORT_DOCKED_MIN_PX (±0)', () => {
    expect(LAYOUT_SIDEBAR_PX + LAYOUT_LIST_MIN_PX + LAYOUT_DETAIL_MIN_PX).toBe(
      VIEWPORT_DOCKED_MIN_PX,
    )
    expect(VIEWPORT_DOCKED_MIN_PX).toBe(1100)
  })
})

describe('layout dim overrides (GDK-842 chunk 3)', () => {
  afterEach(() => {
    applyLayoutDimOverrides(null)
    vi.unstubAllGlobals()
  })

  test('effectiveLayout ships the defaults and keeps dockedMin a sum', () => {
    const eff = effectiveLayout()
    expect(eff).toEqual({ sidebar: 272, listMin: 390, detailMin: 438, dockedMin: 1100 })
    expect(eff.dockedMin).toBe(eff.sidebar + eff.listMin + eff.detailMin)
  })

  test('a sidebar override moves the docked floor and the token style with it', () => {
    applyLayoutDimOverrides({ '--layout-sidebar': '300px' })
    const eff = effectiveLayout()
    expect(eff.sidebar).toBe(300)
    expect(eff.dockedMin).toBe(300 + 390 + 438)
    expect(layoutTokenStyle()).toContain('--layout-sidebar:300px')
    expect(layoutTokenStyle()).toContain('--layout-docked-min:1128px')
  })

  test('malformed or non-positive values fall back to the defaults', () => {
    // The number is deliberate: an untrusted doc can carry a non-string, and
    // the cast is the test-side way to say so past the Record<string,string> type.
    applyLayoutDimOverrides({
      '--layout-sidebar': 'bogus',
      '--layout-list-min': '0px',
      '--layout-detail-min': 438,
    } as unknown as Record<string, string>)
    expect(effectiveLayout()).toEqual({
      sidebar: 272,
      listMin: 390,
      detailMin: 438,
      dockedMin: 1100,
    })
  })

  test('clearing the overrides restores the shipped floor', () => {
    applyLayoutDimOverrides({ '--layout-sidebar': '300px' })
    expect(effectiveLayout().dockedMin).toBe(1128)
    applyLayoutDimOverrides(null)
    expect(effectiveLayout()).toEqual({
      sidebar: 272,
      listMin: 390,
      detailMin: 438,
      dockedMin: 1100,
    })
  })

  test('a changed floor re-subscribes matchMedia under the new threshold', () => {
    const mqls: Array<{
      media: string
      matches: boolean
      listeners: Set<() => void>
      addEventListener: (t: string, fn: () => void) => void
      removeEventListener: (t: string, fn: () => void) => void
    }> = []
    vi.stubGlobal('window', {
      matchMedia: vi.fn((media: string) => {
        const listeners = new Set<() => void>()
        const mq = {
          media,
          matches: false,
          listeners,
          addEventListener: (_t: string, fn: () => void) => void listeners.add(fn),
          removeEventListener: (_t: string, fn: () => void) => void listeners.delete(fn),
        }
        mqls.push(mq)
        return mq
      }),
    })

    const seen: string[] = []
    const unsub = subscribeViewportRegime((r) => seen.push(r))
    expect(mqls).toHaveLength(1)
    expect(mqls[0].media).toContain('1100px')
    expect(mqls[0].listeners.size).toBe(1)
    expect(seen).toHaveLength(1) // subscription fires once immediately

    applyLayoutDimOverrides({ '--layout-sidebar': '300px' })
    expect(mqls).toHaveLength(2) // new floor ⇒ a new MediaQueryList…
    expect(mqls[0].listeners.size).toBe(0) // …old one released…
    expect(mqls[1].media).toContain('1128px')
    expect(mqls[1].listeners.size).toBe(1)
    expect(seen).toHaveLength(2) // …and subscribers re-notified with the new regime

    unsub()
    expect(mqls[1].listeners.size).toBe(0)
  })
})
