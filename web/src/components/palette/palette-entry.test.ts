/*
 * GDK-472 palette entry copy, moved from e2e/palette.spec.ts by the
 * GDK-1702 cost ladder: the case opened the palette to read three strings
 * that live in the source and the catalog. Entry click-through (open,
 * type, Enter) stays in e2e — usearch.spec.ts and the palette spec keep
 * the real path.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en } from '../../lib/i18n/en'

const HERE = dirname(fileURLToPath(import.meta.url))
const SEARCHBOX = join(HERE, '../list/SearchBox.svelte')
const PALETTE = join(HERE, 'CommandPalette.svelte')

describe('GDK-472: palette entry names its scope; empty palette is one phrase', () => {
  const searchbox = readFileSync(SEARCHBOX, 'utf8')
  const palette = readFileSync(PALETTE, 'utf8')

  test('the entry button says what it searches and shows the shortcut', () => {
    // Blocks: a button that is only an icon (scope unnamed) or that hides
    // the kbd — the e2e asserted the entry contains 'Search everything'
    // and a visible <kbd>.
    const button = searchbox.slice(
      searchbox.indexOf('data-testid="palette-open"'),
      searchbox.indexOf('</button>', searchbox.indexOf('data-testid="palette-open"')),
    )
    expect(button, 'the entry button block').toBeTruthy()
    expect(button).toMatch(/\{t\('palette\.entryLabel'\)\}/)
    expect(button).toMatch(/<kbd/)
    expect(en['palette.entryLabel']).toBe('Search everything')
  })

  test('the palette input placeholder is the catalog key, and the en sentence says what it does', () => {
    // Blocks: a hand-written placeholder drifting from the catalog (ko/ja
    // would silently keep the old sentence) — or the sentence forgetting
    // it names two things: jump and search.
    expect(palette).toMatch(/placeholder=\{t\('palette\.placeholder'\)\}/)
    expect(en['palette.placeholder']).toBe('Jump to an issue, or search everything…')
  })

  test('an empty palette is one phrase — no empty-state hint element', () => {
    // Blocks: the removed two-phrase empty state coming back. The e2e
    // asserted palette-empty-hint count 0 on open; it is absent from the
    // source, and this keeps it that way.
    expect(palette).not.toMatch(/palette-empty-hint/)
  })
})
