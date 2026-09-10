/*
 * GDK-911 — the .detail-layer sheet inset is a pinned contract, not an
 * accident of CSS.
 *
 * The walk (mobile/e2e/viewport.spec.ts) lost its only .detail-layer sheet
 * measurement when GDK-906 made the transition sheet unopenable on the
 * demo fixture, and nothing noticed that `padding-bottom:
 * max(var(--safe-bottom), 12px)` was asserted by nobody. These tests are
 * the unit half of the restored coverage: the formula itself, and the
 * stylesheet held to the same constant the formula owns. The e2e half
 * measures the painted sheet in the layer; a change to the CSS floor is
 * red *here* before it can be green there.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { SHEET_INSET_FLOOR_PX, sheetBottomInset } from './inset'

const srcDir = join(dirname(fileURLToPath(import.meta.url)), '..')

describe('sheetBottomInset — max(reported, floor)', () => {
  it('floors a zero or small inset — the headless rig reports 0', () => {
    expect(sheetBottomInset(0)).toBe(SHEET_INSET_FLOOR_PX)
    expect(sheetBottomInset(5)).toBe(SHEET_INSET_FLOOR_PX)
  })

  it('a real inset wins — 34px is the dev shell home indicator measurement', () => {
    expect(sheetBottomInset(34)).toBe(34)
  })
})

describe('app.css composes the same floor the function owns', () => {
  // The artifact/code divergence detector: the number exists in exactly
  // one place (SHEET_INSET_FLOOR_PX) and the stylesheet must compose it.
  // Mutation-tested: 12px → 8px in app.css makes this red.
  it('.safe-bottom and .detail-layer .sheet share one padding-bottom formula with this floor', () => {
    const css = readFileSync(join(srcDir, 'app.css'), 'utf8')
    const formula = new RegExp(
      `\\.safe-bottom,\\s*\\.detail-layer\\s+\\.sheet\\s*\\{[\\s\\S]*?padding-bottom:\\s*max\\(var\\(--safe-bottom\\),\\s*${SHEET_INSET_FLOOR_PX}px\\)`,
    )
    expect(css).toMatch(formula)
  })
})
