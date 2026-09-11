/*
 * GDK-965 contract: a space picker row says what the space costs, and says
 * nothing where the origin could not answer.
 *
 * FAIL-first, before pageCountLabel existed:
 *   Error: Failed to load url ./space-cost (resolved id: ./space-cost)
 * and with the module present but the absent case falling through to a
 * number, the first test read:
 *   expected '0 pages' to be null
 */
import { describe, expect, test } from 'vitest'

import { pageCountLabel } from './space-cost'

describe('GDK-965 space page-count label', () => {
  test('an origin that could not answer draws nothing — not 0, not "unknown"', () => {
    expect(pageCountLabel(undefined)).toBeNull()
    expect(pageCountLabel(null)).toBeNull()
  })

  test('a counted space says how many pages', () => {
    expect(pageCountLabel(7)).toBe('7 pages')
  })

  test('one page is not "1 pages"', () => {
    expect(pageCountLabel(1)).toBe('1 page')
  })

  test('a known-empty space is a real answer, and reads as one', () => {
    expect(pageCountLabel(0)).toBe('0 pages')
  })

  test('a large count is grouped for the locale', () => {
    expect(pageCountLabel(1240)).toBe('1,240 pages')
  })

  test('a nonsense count is treated as no answer', () => {
    expect(pageCountLabel(Number.NaN)).toBeNull()
    expect(pageCountLabel(-3)).toBeNull()
  })
})
