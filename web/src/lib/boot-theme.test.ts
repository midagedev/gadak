/*
 * index.html's boot script is the only code that runs before the bundle, so
 * it cannot import anything — the theme storage key is spelled there by hand.
 * These pin the two halves together. The failure they block is silent and
 * total: a dark-mode user's stored choice is ignored on every cold boot and
 * they see the cream shell flash before the app corrects itself.
 */
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

import { THEME_STORAGE_KEY } from './storage'
import { THEMES } from './theme'

const html = readFileSync(join(__dirname, '../../index.html'), 'utf8')

describe('the boot script agrees with the app', () => {
  it('reads the same storage key storage.ts writes', () => {
    // Hand-spelled: the boot script cannot import themeStorageKey().
    expect(html).toContain(`'${THEME_STORAGE_KEY}'`)
    expect(html).toContain(`'${THEME_STORAGE_KEY}:'`)
    expect(html).toMatch(/localStorage\.getItem\(\s*key\s*\)/)
  })

  it('derives the workspace key from the same /w/<name> path as storage.ts', () => {
    const storageSrc = readFileSync(join(__dirname, 'storage.ts'), 'utf8')
    const re = '/^\\/w\\/([A-Za-z0-9_-]+)(\\/|$)/'
    expect(storageSrc, 'storage.ts must own the path regex').toContain(re)
    expect(html, 'boot script must hand-spell the same path regex').toContain(re)
    expect(html).toContain(`${THEME_STORAGE_KEY}:`)
  })

  it('honors every registered theme by name', () => {
    // A theme added to THEMES but not to the boot script would apply only
    // after hydration — the flash this whole mechanism exists to prevent.
    for (const theme of THEMES) {
      expect(html, `boot script must accept '${theme.name}'`).toContain(`'${theme.name}'`)
    }
  })

  it('paints a boot shell for every dark-family theme', () => {
    // Accepting the name is only half of it. Without --boot-* for that theme
    // the attribute is set and the shell still paints from :root — the cream
    // flash, for exactly the users who chose a dark palette to avoid it.
    for (const theme of THEMES) {
      if (theme.name === 'light') continue
      const block = html.match(
        new RegExp(`:root\\[data-theme='${theme.name}'\\]\\s*\\{([^}]*)\\}`),
      )
      expect(block, `boot stylesheet needs a :root[data-theme='${theme.name}'] block`).not.toBeNull()
      for (const v of ['--boot-bg', '--boot-panel', '--boot-line', '--boot-skeleton']) {
        expect(block![1], `${theme.name} boot shell must set ${v}`).toContain(v)
      }
      expect(block![1], `${theme.name} must set color-scheme: dark`).toContain('color-scheme: dark')
    }
  })

  it('ships no hardcoded data-theme on <html>', () => {
    // A baked-in attribute wins over prefers-color-scheme forever: it is
    // exactly what made the OS setting unreadable at boot.
    expect(html).not.toMatch(/<html[^>]*data-theme=/)
  })

  it('paints the boot shell from tokens, with the inline values as fallback', () => {
    // The inline stylesheet is unlayered and would otherwise outrank app.css's
    // layered rules forever, pinning the mat to the light value in both themes.
    expect(html).toContain('var(--color-shell, var(--boot-bg))')
  })
})
