import { describe, expect, test } from 'vitest'
import {
  applyToastReservation,
  resolveBrowseStack,
  type BrowseStackInput,
  type StackRect,
} from './browse-stack'

/*
 * Contract → assertion (GDK-76 / GDK-77). Each clause has a happy path
 * and a boundary. FAIL-first: these assert the desired stack, not the
 * pre-change "hide native, leave chrome in front, ignore toasts" logic.
 *
 * | Clause                                              | Assertions                          |
 * |-----------------------------------------------------|-------------------------------------|
 * | dialog open + browse visible → pane yields          | chromeYields, !nativeVisible        |
 * | dialog closed → pane restored                       | !chromeYields, nativeVisible        |
 * | toast during browse → reserve the toast host        | reserveToast, nativeVisible         |
 */

function stack(over: Partial<BrowseStackInput> = {}) {
  return resolveBrowseStack({
    paneOpen: false,
    dialogOpen: false,
    toastVisible: false,
    ...over,
  })
}

const frame: StackRect = { x: 272, y: 88, w: 1000, h: 700 }

describe('resolveBrowseStack — dialog vs pane', () => {
  test('dialog open + browse visible → chrome yields and native hides', () => {
    const s = stack({ paneOpen: true, dialogOpen: true })
    expect(s.nativeVisible).toBe(false)
    expect(s.chromeYields).toBe(true)
    expect(s.chromeMounted).toBe(true)
    expect(s.reserveToast).toBe(false)
  })

  test('dialog closed → pane restored (native + front-tier chrome)', () => {
    const s = stack({ paneOpen: true, dialogOpen: false })
    expect(s.nativeVisible).toBe(true)
    expect(s.chromeYields).toBe(false)
    expect(s.chromeMounted).toBe(true)
  })

  test('pane closed + dialog open → nothing of the pane is up', () => {
    const s = stack({ paneOpen: false, dialogOpen: true })
    expect(s.nativeVisible).toBe(false)
    expect(s.chromeYields).toBe(false)
    expect(s.chromeMounted).toBe(false)
    expect(s.reserveToast).toBe(false)
  })

  test('idle (pane closed, no dialog) is all-off', () => {
    const s = stack()
    expect(s).toEqual({
      nativeVisible: false,
      chromeMounted: false,
      chromeYields: false,
      reserveToast: false,
    })
  })
})

describe('resolveBrowseStack — toast during browse', () => {
  test('toast while the native view is up → reserve the toast host, do not hide native', () => {
    const s = stack({ paneOpen: true, toastVisible: true })
    expect(s.nativeVisible).toBe(true)
    expect(s.chromeYields).toBe(false)
    expect(s.reserveToast).toBe(true)
  })

  test('toast with no pane → no reservation (nothing to shrink)', () => {
    const s = stack({ toastVisible: true })
    expect(s.reserveToast).toBe(false)
    expect(s.nativeVisible).toBe(false)
  })

  test('dialog + toast together: yield to the dialog; do not also reserve (native is already hidden)', () => {
    const s = stack({ paneOpen: true, dialogOpen: true, toastVisible: true })
    expect(s.nativeVisible).toBe(false)
    expect(s.chromeYields).toBe(true)
    expect(s.reserveToast).toBe(false)
  })

  test('toast ending while still browsing restores the full rectangle (no reserve)', () => {
    const during = stack({ paneOpen: true, toastVisible: true })
    const after = stack({ paneOpen: true, toastVisible: false })
    expect(during.reserveToast).toBe(true)
    expect(after.reserveToast).toBe(false)
    expect(after.nativeVisible).toBe(true)
  })
})

describe('applyToastReservation', () => {
  test('overlapping bottom-right toast lifts the native bottom edge to the toast top', () => {
    const toast: StackRect = { x: 860, y: 740, w: 384, h: 48 }
    expect(applyToastReservation(frame, toast)).toEqual({
      x: 272,
      y: 88,
      w: 1000,
      h: 652,
    })
  })

  test('empty toast host is a no-op (disabled path stays the viewport box)', () => {
    expect(applyToastReservation(frame, { x: 1200, y: 780, w: 0, h: 0 })).toBe(frame)
  })

  test('toast that does not overlap the frame is a no-op', () => {
    const left: StackRect = { x: 16, y: 740, w: 200, h: 48 }
    expect(applyToastReservation(frame, left)).toBe(frame)
  })

  test('toast wholly above the frame is a no-op', () => {
    const above: StackRect = { x: 300, y: 10, w: 200, h: 40 }
    expect(applyToastReservation(frame, above)).toBe(frame)
  })
})
