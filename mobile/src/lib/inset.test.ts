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
  it('.safe-bottom and every .sheet share one padding-bottom formula with this floor', () => {
    // GDK-902 2026-09-15: the selector was `.detail-layer .sheet`, which
    // exempted a sheet in the column because the tab bar stood under it.
    // With no tab bar the exemption became a gap, so the rule covers every
    // sheet — and this assertion is narrowed with it: a re-added
    // `.detail-layer` qualifier fails here, which is the regression that
    // would otherwise only show as a control under the home indicator.
    const css = readFileSync(join(srcDir, 'app.css'), 'utf8')
    const formula = new RegExp(
      `\\.safe-bottom,\\s*\\.sheet,\\s*\\.key-bar:not\\(\\[data-keyboard-inset\\]\\)\\s*\\{[\\s\\S]*?padding-bottom:\\s*max\\(var\\(--safe-bottom\\),\\s*${SHEET_INSET_FLOOR_PX}px\\)`,
    )
    expect(css).toMatch(formula)
  })
})

/*
 * GDK-902 2026-09-15 — the key bar joined that selector list, and why it is
 * a third selector rather than a second copy of the number.
 *
 * The tab bar used to stand between the key bar and the home indicator, so
 * the bar owed nothing; it was the precedent app.css cited for "KeyBar never
 * composes .safe-bottom". With no tab bar (DESIGN.md §2) the bar is the
 * bottom-most painted surface of the column while the keyboard is down, and
 * a bottom-most surface owes the inset — the same sentence the .sheet rule
 * was widened by. FAIL-first, measured 2026-09-15 by the shell e2e on the
 * built bundle: padding-bottom 0px, --safe-bottom 0px, expected 12.
 *
 * The `:not([data-keyboard-inset])` half is the other direction of the same
 * defect. While the keyboard is up the bar rides `keyboardInset`'s
 * translate, so the inset below it is no longer the home indicator but a
 * dead strip between the keys and the keyboard's top edge — the exact thing
 * `.composer.safe-bottom:focus-within` exists to remove. The action is the
 * only honest witness of "the keyboard is up" (`:focus-within` is false on
 * the bar — the focus lives in the sibling IME field, and it would also be
 * false under a hardware keyboard, when the bar DOES sit on the home
 * indicator), so the action stamps the attribute and CSS reads it.
 */
describe('the key bar is a bottom-most surface (GDK-902)', () => {
  it('KeyBar carries the class the rule names, and does not restate the number', () => {
    const bar = readFileSync(join(srcDir, 'ui/KeyBar.svelte'), 'utf8')
    expect(bar).toMatch(/class="bar key-bar"/)
    expect(bar).toContain('use:keyboardInset')
    // The floor has one owner. A literal `12px` here would be the second.
    expect(bar).not.toMatch(/padding-bottom:\s*max\(/)
  })

  it('keyboardInset stamps the attribute the rule switches on, and clears it', () => {
    // GDK-1995: the module moved to the shared home (web/src); the contract
    // this pins — stamp while the band exists, clear when it closes — moved
    // with it unchanged.
    const kb = readFileSync(
      join(
        dirname(fileURLToPath(import.meta.url)),
        '..',
        '..',
        '..',
        'web',
        'src',
        'lib',
        'keyboard.ts',
      ),
      'utf8',
    )
    expect(kb).toContain('keyboardInset')
    expect(kb).toMatch(/dataset\.keyboardInset/)
    // Both edges: set while the band exists, removed when it closes and on
    // destroy — a stuck attribute is a bar that never pays the inset again.
    expect(kb.match(/dataset\.keyboardInset/g)?.length ?? 0).toBeGreaterThanOrEqual(2)
  })

  it('app.css zeroes nothing by hand — the rule simply stops matching', () => {
    const css = readFileSync(join(srcDir, 'app.css'), 'utf8')
    expect(css).not.toMatch(/\.key-bar\[data-keyboard-inset\]\s*\{[^}]*padding-bottom:\s*0/)
  })
})
