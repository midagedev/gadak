/*
 * GDK-645: the palette highlight is stored raw and read clamped (SearchBox
 * sugIdxRaw / sugIdx), not rewritten from an $effect that also scrolls.
 *
 * vitest is node, no svelte plugin — importing the .svelte file fails
 * (SearchBox.test.ts). Rendered arrow/hover behaviour is Playwright's
 * (e2e/ux-f7.spec.ts GDK-461).
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
