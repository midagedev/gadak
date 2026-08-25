import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'Search.svelte'), 'utf8')

describe('GDK-905 Search plates are distinct and catalog-backed', () => {
  it('holds a searching flag and does not paint empty over an open fetch', () => {
    expect(src).toContain('searchPaint')
    expect(src).toMatch(/searching\s*=/)
    expect(src).not.toMatch(/else if results\.length === 0 && serverPages\.length === 0/)
  })

  it('records a failed server search instead of swallowing it', () => {
    expect(src).toMatch(/searchFailed\s*=\s*true/)
    expect(src).toContain("t('list.searchFailed')")
  })

  it('uses catalog keys for clear and for no-results', () => {
    expect(src).toContain("t('list.clearSearch')")
    expect(src).toContain("t('common.noResults')")
    expect(src).not.toContain('aria-label="Clear search"')
    expect(src).not.toContain('title="No matches"')
  })

  it('does not center a brochure hero on the idle empty screen', () => {
    expect(src).not.toContain('Search the mirror')
    // Recents stay the idle-label list; the empty idle is a hint, not EmptyState.
    expect(src).toMatch(/idle-hint|idleHint/)
  })
})
