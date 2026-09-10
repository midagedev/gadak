/*
 * GDK-645: the palette highlight is stored raw and read clamped (SearchBox
 * sugIdxRaw / sugIdx), not rewritten from an $effect that also scrolls.
 *
 * vitest is node, no svelte plugin — importing the .svelte file fails
 * (SearchBox.test.ts). Rendered arrow/hover behaviour is Playwright's
 * (e2e/palette.spec.ts GDK-461).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(HERE, 'CommandPalette.svelte'), 'utf8')

describe('CommandPalette highlight index (GDK-645)', () => {
  test('stores the raw index and reads it clamped, like SearchBox sugIdx', () => {
    expect(SRC).toMatch(/let idxRaw = \$state/)
    expect(SRC).toMatch(/const idx = \$derived/)
    expect(SRC).toContain('function firstSafeIndex')
    expect(SRC).not.toMatch(/if \(idx !== next\) idx = next/)
    expect(SRC).not.toMatch(/idx = list\.length/)
    expect(SRC).toMatch(/idxRaw = idx < 0 \? 0 : \(idx \+ 1\) % items\.length/)
    expect(SRC).toMatch(/idxRaw = i/)
  })

  test('scrollIntoView stays an effect over the clamped index, not a rewrite of it', () => {
    expect(SRC).toMatch(/listEl\?\.querySelector\(`\[data-idx="\$\{i\}"\]`\)\?\.scrollIntoView/)
    expect(SRC).not.toMatch(/idx = list\.length \? list\.length - 1 : -1/)
  })
})

/*
 * GDK-143 (visual audit V8): in a mixed list the leading icon must not move
 * where the text starts. Rows without an icon used to begin one gap earlier
 * than their iconed neighbours — the same class the audit flagged on the
 * history rows ("아이콘 유무로 텍스트 시작 x 갈림"). The row now reserves the
 * icon slot: an icon or a same-width spacer, never neither.
 */
describe('palette rows reserve the icon slot (GDK-143)', () => {
  test('an iconless row renders a same-width spacer, not an earlier start', () => {
    const at = SRC.indexOf('<Icon name={item.icon}')
    expect(at, 'the row still leads with the item icon').toBeGreaterThan(-1)
    // From the {#if item.icon} just above the icon to its {/if}: both
    // branches live there, and the else branch must hold the slot.
    const open = SRC.lastIndexOf('{#if item.icon}', at)
    const block = SRC.slice(open, SRC.indexOf('{/if}', at))
    expect(block).toContain('<Icon name={item.icon}')
    expect(block, 'icon absent: a 14px spacer holds the slot').toMatch(/<span class="w-3\.5 flex-none"/)
  })
})
