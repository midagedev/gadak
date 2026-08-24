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
 * GDK-842 adds the dimension axis: one palette-agnostic `dims` map
 * (--spacing-row → 44px) that lands in a single :root rule, rides the same
 * boot cache, and recouples the JS geometry owners (rowMetrics cache,
 * viewport-regime floor) the moment applyUserTokens runs.
 *
 * Node environment, no DOM: applyUserTokens is a no-op here by design; the
 * DOM half is Playwright's job (live-reflect spec).
 */
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { rowMetrics } from './row-metrics'
import { effectiveLayout } from './viewport-regime'
import {
  USER_TOKENS_STORAGE_KEY,
  USER_TOKENS_STYLE_ATTR,
  applyUserTokens,
  buildUserTokenStyles,
  isDimValue,
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

  it('re-checks the dim var name and value shapes this module enforces', () => {
    expect(html).toContain('/^--(?:spacing|layout|text)-[a-z0-9-]+$/i')
    expect(html).toContain('/^[0-9]+(\\.[0-9])?px$/')
    expect(html).toContain('/--line-height$/')
    expect(html).toContain('/^[0-9]+\\.[0-9]{1,2}$/')
  })

  it('installs from a dims-only cache too, not just colors', () => {
    expect(html).toContain('udoc.dims')
    expect(html).toContain("ucss += ':root{' + ddecl + '}'")
  })
})

describe('dimension overrides land in one palette-agnostic rule (GDK-842)', () => {
  it('builds a single :root rule from the dims map', () => {
    const css = buildUserTokenStyles({}, {
      '--spacing-row': '48px',
      '--layout-sidebar': '300px',
      '--text-body': '14px',
      '--text-body--line-height': '1.45',
    })
    expect(css).toBe(
      ':root{--spacing-row:48px;--layout-sidebar:300px;--text-body:14px;--text-body--line-height:1.45;}',
    )
  })

  it('appends the dims rule after the palette cascade, once — not per palette', () => {
    const css = buildUserTokenStyles(
      {
        light: { '--color-accent': '#7a4bd0' },
        dark: { '--color-accent': '#9a6be0' },
      },
      { '--spacing-row': '48px' },
    )
    expect(css).toBe(
      ':root{--color-accent:#7a4bd0;}' +
        ":root[data-theme='dark']{--color-accent:#9a6be0;}" +
        "@media (prefers-color-scheme: dark){:root:not([data-theme='light']){--color-accent:#9a6be0;}}" +
        ':root{--spacing-row:48px;}',
    )
  })

  it('drops dim entries whose name or value the server would have refused', () => {
    const css = buildUserTokenStyles(
      {},
      {
        '--spacing-row': '48px', // fine
        '--spacing-row-excerpt': '1.2', // unitless on a px token
        '--color-accent': '#7a4bd0', // color name inside the dims map
        '--layout-sidebar': '0px', // not positive
        '--layout-list-min': '390', // missing px
        '--text-title': '15.125px', // two decimals
        '--text-title--line-height': '1', // no decimal point
        'margin-left': '4px', // stray property
        '--text-body--line-height': '1.35', // fine
      },
    )
    expect(css).toBe(':root{--spacing-row:48px;--text-body--line-height:1.35;}')
  })

  it('emits nothing for empty or malformed dims', () => {
    expect(buildUserTokenStyles({}, {})).toBe('')
    expect(buildUserTokenStyles({}, null)).toBe('')
    expect(buildUserTokenStyles({}, 'nope')).toBe('')
  })

  it('isDimValue mirrors the server dim gate — the unit is owned by the name', () => {
    expect(isDimValue('--spacing-row', '44px')).toBe(true)
    expect(isDimValue('--spacing-row', '44.5px')).toBe(true)
    expect(isDimValue('--spacing-row', '0px')).toBe(false)
    expect(isDimValue('--spacing-row', '44')).toBe(false)
    expect(isDimValue('--spacing-row', '44PX')).toBe(false)
    expect(isDimValue('--spacing-row', '1em')).toBe(false)
    expect(isDimValue('--text-body--line-height', '1.4')).toBe(true)
    expect(isDimValue('--text-body--line-height', '1.35')).toBe(true)
    expect(isDimValue('--text-body--line-height', '1.355')).toBe(false)
    expect(isDimValue('--text-body--line-height', '1')).toBe(false)
    expect(isDimValue('--text-body--line-height', '14px')).toBe(false)
  })
})

describe('parseUiDoc carries the dims axis', () => {
  it('sanitizes dims with the same name/value filters', () => {
    const doc = parseUiDoc({
      dims: {
        '--spacing-row': '48px',
        '--layout-sidebar': 'bogus',
        '--unknown-dim': '48px',
        'margin-left': '4px',
        '--text-body--line-height': '1.45',
      },
    })
    expect(doc!.dims).toEqual({
      '--spacing-row': '48px',
      '--text-body--line-height': '1.45',
    })
  })

  it('defaults dims to an empty object so callers never guard', () => {
    expect(parseUiDoc({})!.dims).toEqual({})
  })
})

describe('the boot cache carries dims (GDK-842)', () => {
  afterEach(() => {
    applyUserTokens(null)
    vi.unstubAllGlobals()
  })

  it('round-trips a dims-only document and clears once everything is empty', () => {
    const store = new Map<string, string>()
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
    })
    applyUserTokens({ vars: {}, dims: { '--spacing-row': '48px' }, dataColors: {} })
    const cached = store.get(USER_TOKENS_STORAGE_KEY) ?? ''
    expect(cached).toContain('"--spacing-row":"48px"')
    // What the boot script reads back must be what this module would rebuild.
    expect(parseUiDoc(JSON.parse(cached))!.dims).toEqual({ '--spacing-row': '48px' })

    applyUserTokens({ vars: {}, dims: {}, dataColors: {} })
    expect(store.has(USER_TOKENS_STORAGE_KEY)).toBe(false)
  })
})

describe('applyUserTokens recouples the JS geometry owners (GDK-842 chunk 3)', () => {
  afterEach(() => {
    applyUserTokens(null)
    vi.unstubAllGlobals()
  })

  it('invalidates the rowMetrics cache and recomputes the docked floor', () => {
    const computed: Record<string, string> = { '--spacing-row': '42px' }
    // A document stub that survives the full style-install path (querySelector
    // miss → createElement → head.appendChild), so the assertions see the
    // geometry wiring, not a TypeError from the stub itself.
    const styleEl = { setAttribute: () => {}, textContent: '' }
    vi.stubGlobal('document', {
      documentElement: {},
      head: { appendChild: () => {} },
      createElement: () => styleEl,
      querySelector: () => null,
    })
    vi.stubGlobal('getComputedStyle', () => ({
      getPropertyValue: (n: string) => computed[n] ?? '',
    }))

    expect(rowMetrics().row).toBe(42) // primes the module cache
    computed['--spacing-row'] = '48px' // what the style install will paint
    expect(rowMetrics().row).toBe(42) // still cached — the drift this round closes

    applyUserTokens({
      vars: {},
      dims: { '--spacing-row': '48px', '--layout-sidebar': '300px' },
      dataColors: {},
    })
    expect(rowMetrics().row).toBe(48)
    expect(effectiveLayout().dockedMin).toBe(300 + 390 + 438)
  })
})
