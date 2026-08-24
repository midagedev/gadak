/*
 * GDK-786 user-tokens unit tests. Three contracts:
 *
 *   1. buildUserTokenStyles mirrors app.css's palette cascade — the media
 *      fallback and the data-theme blocks must both exist or a system-dark
 *      user and an explicit-dark user see different overrides.
 *   2. The boot cache key derives from the path exactly like the theme key
 *      (index.html's blocking script re-spells it — see the parity tests).
 *   3. Untrusted documents (hand-edited config.json, downgrade) degrade to
 *      dropped declarations, never to injected junk.
 *
 * Node environment, no DOM: applyUserTokens is a no-op here by design; the
 * DOM half is Playwright's job (live-reflect spec).
 */
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

import {
  USER_TOKENS_STORAGE_KEY,
  USER_TOKENS_STYLE_ATTR,
  buildUserTokenStyles,
  isHexColor,
  parseUiDoc,
  userTokensStorageKeyFromPath,
} from './user-tokens'

const html = readFileSync(join(__dirname, '../../index.html'), 'utf8')

describe('buildUserTokenStyles mirrors the app.css cascade', () => {
  const vars = {
    light: { '--color-accent': '#7a4bd0' },
    dark: { '--color-accent': '#9a6be0' },
    ember: { '--color-accent': '#d07a3a' },
  }

  it('puts light at :root and every other palette behind its data-theme', () => {
    const css = buildUserTokenStyles(vars)
    expect(css).toContain(':root{--color-accent:#7a4bd0;}')
    expect(css).toContain(`:root[data-theme='dark']{--color-accent:#9a6be0;}`)
    expect(css).toContain(`:root[data-theme='ember']{--color-accent:#d07a3a;}`)
  })

  it('gives system-dark users the dark values without a data-theme', () => {
    const css = buildUserTokenStyles(vars)
    expect(css).toContain(
      "@media (prefers-color-scheme: dark){:root:not([data-theme='light']){--color-accent:#9a6be0;}}",
    )
  })

  it('emits nothing for an empty or malformed map', () => {
    expect(buildUserTokenStyles({})).toBe('')
    expect(buildUserTokenStyles(null)).toBe('')
    expect(buildUserTokenStyles('nope')).toBe('')
  })

  it('drops declarations that are not --color-* vars or #rgb/#rrggbb values', () => {
    const css = buildUserTokenStyles({
      light: {
        '--color-accent': '#7a4bd0', // fine
        '--color-x': 'javascript:alert(1)', // scheme payload as a value
        '--not-a-color-var': '#7a4bd0', // wrong var shape
        '--color-x2': 'rgb(1 2 3)', // functional color
        'margin-top': '4px', // stray CSS property
      },
    })
    expect(css).toBe(':root{--color-accent:#7a4bd0;}')
  })

  it('refuses palette names that could escape the attribute selector', () => {
    const css = buildUserTokenStyles({
      "x']{}body{display:none": { '--color-accent': '#7a4bd0' },
    })
    expect(css).toBe('')
  })
})

describe('the boot cache key follows the theme key rules', () => {
  it('stays unscoped on the primary mount', () => {
    expect(userTokensStorageKeyFromPath('/')).toBe(USER_TOKENS_STORAGE_KEY)
    expect(userTokensStorageKeyFromPath('/?issue=GDK-1')).toBe(USER_TOKENS_STORAGE_KEY)
  })

  it('scopes to /w/<name> mounts', () => {
    expect(userTokensStorageKeyFromPath('/w/oss/')).toBe(`${USER_TOKENS_STORAGE_KEY}:oss`)
    expect(userTokensStorageKeyFromPath('/w/work/issues/GDK-1')).toBe(
      `${USER_TOKENS_STORAGE_KEY}:work`,
    )
  })
})

describe('parseUiDoc defends the boot path', () => {
  it('returns a sanitized empty doc for garbage', () => {
    expect(parseUiDoc(null)).toBeNull()
    expect(parseUiDoc('string')).toBeNull()
    const doc = parseUiDoc({ vars: 'nope', dataColors: 3 })
    expect(doc).not.toBeNull()
    expect(doc!.vars).toEqual({})
    expect(doc!.dataColors).toEqual({})
  })

  it('keeps only hex values and known families', () => {
    const doc = parseUiDoc({
      vars: { light: { '--color-accent': '#7a4bd0', '--color-bad': 'nope' } },
      dataColors: {
        label: { urgent: '#c03030', broken: 'red' },
        priority: { '1': '#ff0000' }, // family the UI does not render
      },
      warnings: [{ token: 'x', rule: 'unknown-token', message: 'ignored' }, 'junk'],
    })
    expect(doc!.vars.light).toEqual({ '--color-accent': '#7a4bd0' })
    expect(doc!.dataColors.label).toEqual({ urgent: '#c03030' })
    expect(Object.keys(doc!.dataColors)).toEqual(['label'])
    expect(doc!.warnings).toHaveLength(1)
  })

  it('isHexColor accepts only #rgb and #rrggbb', () => {
    expect(isHexColor('#abc')).toBe(true)
    expect(isHexColor('#a1b2c3')).toBe(true)
    expect(isHexColor('a1b2c3')).toBe(false)
    expect(isHexColor('#a1b2c3d4')).toBe(false)
    expect(isHexColor('#GGGGGG')).toBe(false)
    expect(isHexColor('')).toBe(false)
  })
})

describe('the index.html boot script agrees with this module', () => {
  // The boot script cannot import anything; the key, the path regex, and the
  // value filters are spelled there by hand. These pin the halves together
  // the same way boot-theme.test.ts does for the theme key — the failure they
  // block is a customized user seeing the default palette flash on cold boot.
  it('reads the same storage key user-tokens.ts writes', () => {
    expect(html).toContain(`'${USER_TOKENS_STORAGE_KEY}'`)
    expect(html).toContain(`'${USER_TOKENS_STORAGE_KEY}:'`)
  })

  it('derives the workspace key from the same /w/<name> path regex', () => {
    const re = '/^\\/w\\/([A-Za-z0-9_-]+)(\\/|$)/'
    expect(html, 'boot script must hand-spell the same path regex').toContain(re)
  })

  it('re-checks the var name and hex value shapes this module enforces', () => {
    expect(html).toContain('/^--color-[a-z0-9-]+$/i')
    expect(html).toContain('/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/')
  })

  it('installs the same marked style element applyUserTokens owns', () => {
    expect(html).toContain(USER_TOKENS_STYLE_ATTR)
  })

  it('mirrors the cascade: data-theme blocks and the system-dark fallback', () => {
    expect(html).toContain(":root[data-theme='")
    expect(html).toContain("@media (prefers-color-scheme: dark){:root:not([data-theme='light']){")
  })
})
