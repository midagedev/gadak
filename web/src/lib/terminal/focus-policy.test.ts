/*
 * GDK-1986. The measurement behind this contract is in focus-policy.ts's
 * header: a focus outside a touch gesture cannot raise an iOS keyboard, and
 * it spends the focus a later tap would have needed.
 */
import { describe, expect, test } from 'vitest'
import {
  COARSE_POINTER_QUERY,
  attachFocusAllowed,
  mayStealFocusOnAttach,
} from './focus-policy'

describe('attach-time focus (GDK-1986)', () => {
  test('a mouse keeps the desktop behaviour', () => {
    expect(mayStealFocusOnAttach({ matches: false })).toBe(true)
  })

  test('a finger does not: the tap is the only focus that can raise a keyboard', () => {
    expect(mayStealFocusOnAttach({ matches: true })).toBe(false)
  })

  test('a surface that cannot answer is treated as a desktop', () => {
    // Headless capture and any engine without the media feature. Answering
    // "coarse" there would silently stop focusing the shell on every
    // machine that cannot say otherwise.
    expect(mayStealFocusOnAttach(null)).toBe(true)
    expect(mayStealFocusOnAttach(undefined)).toBe(true)
  })

  test('the live reader asks exactly one query, and tolerates no matchMedia', () => {
    const asked: string[] = []
    const win = {
      matchMedia: (q: string) => {
        asked.push(q)
        return { matches: true } as MediaQueryList
      },
    } as unknown as Window
    expect(attachFocusAllowed(win)).toBe(false)
    expect(asked).toEqual([COARSE_POINTER_QUERY])
    expect(attachFocusAllowed({} as Window)).toBe(true)
    expect(attachFocusAllowed(undefined)).toBe(true)
  })
})
