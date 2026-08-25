import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// Recurrence layer for GDK-870 / GDK-879: the viewport gate cannot grow
// (its spec file is only allowed to unskip the 44pt test). These read the
// source so a later round cannot silently put the status action back in
// the header, reorder Detail to description-first, or paint Unpair with
// a status token.

const src = join(dirname(fileURLToPath(import.meta.url)), '..')

function read(rel: string): string {
  return readFileSync(join(src, rel), 'utf8')
}

describe('GDK-870 Detail contracts', () => {
  const detail = read('screens/Detail.svelte')

  it('renders Comments before Description', () => {
    const comments = detail.indexOf('<h3>Comments')
    const desc = detail.indexOf('<h3>Description')
    expect(comments).toBeGreaterThan(-1)
    expect(desc).toBeGreaterThan(-1)
    expect(comments).toBeLessThan(desc)
  })

  it('opens the transition sheet from the composer, not the header chips', () => {
    const chips = detail.indexOf('class="chips"')
    const composer = detail.indexOf('composer-slab')
    const statusBtn = detail.indexOf('class="status"')
    expect(chips).toBeGreaterThan(-1)
    expect(composer).toBeGreaterThan(chips)
    expect(statusBtn).toBeGreaterThan(composer)
  })
})

describe('GDK-879 pairing / spine contracts', () => {
  it('does not borrow a status token for Unpair', () => {
    const pairing = read('screens/PairingTab.svelte')
    const styles = pairing.slice(pairing.indexOf('<style>'))
    const unpair = styles.match(/\.unpair\s*\{[^}]+\}/)
    const armed = styles.match(/\.unpair\.armed\s*\{[^}]+\}/)
    expect(unpair?.[0]).toBeTruthy()
    expect(armed?.[0]).toBeTruthy()
    expect(unpair?.[0]).not.toMatch(/--color-status-/)
    expect(armed?.[0]).not.toMatch(/--color-status-/)
  })

  it('maps the new spine through --color-spine-new, not the raw status token', () => {
    const css = read('app.css')
    expect(css).toMatch(/--color-spine-new:\s*var\(--color-accent\)/)
    expect(read('ui/Row.svelte')).toMatch(/var\(--color-spine-new\)/)
    expect(read('screens/Detail.svelte')).toMatch(/var\(--color-spine-new\)/)
  })
})

describe('GDK-867 tap floor owner', () => {
  it('sets the 44pt floor on button in app.css', () => {
    expect(read('app.css')).toMatch(/button\s*\{[^}]*min-height:\s*var\(--spacing-control\)/s)
  })

  it('does not use --spacing-control-sm as a button tap size', () => {
    const files = [
      'screens/Issues.svelte',
      'screens/Search.svelte',
      'screens/Detail.svelte',
      'screens/PairingTab.svelte',
      'ui/Sheet.svelte',
      'ui/ScopeSheet.svelte',
    ]
    for (const rel of files) {
      const text = read(rel)
      for (const m of text.matchAll(/\.([a-z0-9-]+)[^{]*\{[^}]*min-height:\s*var\(--spacing-control-sm\)/gi)) {
        const cls = m[1]
        expect(text, `${rel} .${cls} is a button using the visual-chip token`).not.toMatch(
          new RegExp(`<button[^>]*class="${cls}"`),
        )
      }
    }
  })
})
