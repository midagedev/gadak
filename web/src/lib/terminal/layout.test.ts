/*
 * The dock's height math is pure (layout.ts), so it is held here rather than
 * by an e2e that opened a browser to read one constant (GDK-1376; the band
 * test it replaces sat in e2e/terminal.spec.ts). The "not too tall" e2e
 * still measures the real dock against the real window.
 */
import { describe, expect, test } from 'vitest'
import {
  TERMINAL_DEFAULT_HEIGHT_RATIO,
  TERMINAL_MAX_HEIGHT_RATIO,
  TERMINAL_MIN_HEIGHT_PX,
  dockDefaultHeight,
  dockMaxHeight,
} from './layout'

describe('dock height (GDK-1352)', () => {
  test('the first open is a band: a quarter of the window at 900px', () => {
    const h = dockDefaultHeight(900)
    expect(h / 900).toBeGreaterThan(0.22)
    expect(h / 900).toBeLessThan(0.28)
    expect(h).toBe(Math.round(900 * TERMINAL_DEFAULT_HEIGHT_RATIO))
  })

  test('never under the minimum, whatever the window', () => {
    expect(dockDefaultHeight(300)).toBe(TERMINAL_MIN_HEIGHT_PX)
    expect(dockMaxHeight(100)).toBe(TERMINAL_MIN_HEIGHT_PX)
  })

  test('the ceiling leaves the list standing', () => {
    expect(dockMaxHeight(900)).toBe(Math.round(900 * TERMINAL_MAX_HEIGHT_RATIO))
    expect(TERMINAL_MAX_HEIGHT_RATIO).toBeLessThan(1)
    expect(TERMINAL_DEFAULT_HEIGHT_RATIO).toBeLessThan(TERMINAL_MAX_HEIGHT_RATIO)
  })
})
