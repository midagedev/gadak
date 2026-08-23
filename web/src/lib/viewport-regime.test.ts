import { describe, expect, test } from 'vitest'
import {
  LAYOUT_DETAIL_MIN_PX,
  LAYOUT_LIST_MIN_PX,
  LAYOUT_SIDEBAR_PX,
  VIEWPORT_DOCKED_MIN_PX,
} from './viewport-regime'

describe('viewport docked floor (GDK-766)', () => {
  test('track mins sum to VIEWPORT_DOCKED_MIN_PX (±0)', () => {
    expect(LAYOUT_SIDEBAR_PX + LAYOUT_LIST_MIN_PX + LAYOUT_DETAIL_MIN_PX).toBe(
      VIEWPORT_DOCKED_MIN_PX,
    )
    expect(VIEWPORT_DOCKED_MIN_PX).toBe(1100)
  })
})
