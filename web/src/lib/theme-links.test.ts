import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'
import { THEMES } from './theme'

/*
 * The registry contract is "token block in app.css + entry in THEMES"
 * (theme.ts header). Two links in that chain had no unit: boot-theme.test.ts
 * proves THEMES ↔ index.html's boot shell, and tools/theme-check.mjs proves
 * app.css's own internal parity — but nothing proved THEMES ↔ app.css in
 * either direction. A name in THEMES with no CSS block sits in the picker
 * and selects to nothing; a CSS palette missing from THEMES ships unpicked.
 * The painted half (picker selection paints its ground) stays in
 * e2e/theme.spec.ts; these are the static pins that used to ride inside its
 * picker loop, moved here following the layout-tokens.test.ts idiom.
 */

const HERE = dirname(fileURLToPath(import.meta.url))

const CSS_TEXT = readFileSync(join(HERE, '../app.css'), 'utf8')
/** app.css with comments blanked in place — offsets and line structure preserved, so the line numbers the assertions quote are real. */
const CSS_CODE = CSS_TEXT.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, ' '))

function lineOf(idx: number): number {
  return CSS_CODE.slice(0, idx).split('\n').length
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/** Body of the first rule whose selector contains `selector` and whose body
 *  declares `prop`, brace-walked so a selector-list rule (the dark ground
 *  shares its rule with the terminal dock) is read whole. ExtractBraceBlock
 *  idiom from theme-class.test.ts, iterated because a palette name appears
 *  in non-token rules too (color-scheme, .bg-accent) — the first
 *  prop-declaring occurrence is the token block. */
function ruleBodyDeclaring(selector: string, prop: string): { body: string; line: number } {
  const re = new RegExp(`${escapeRe(selector)}[^{}]*\\{`, 'g')
  for (const m of CSS_CODE.matchAll(re)) {
    const open = (m.index ?? 0) + m[0].length - 1
    let depth = 0
    for (let i = open; i < CSS_CODE.length; i++) {
      if (CSS_CODE[i] === '{') depth++
      else if (CSS_CODE[i] === '}' && --depth === 0) {
        const body = CSS_CODE.slice(open + 1, i)
        if (body.includes(`${prop}:`)) return { body, line: lineOf(m.index ?? 0) }
        break
      }
    }
  }
  throw new Error(`app.css has no ${selector} rule declaring ${prop}`)
}

/** The ground of a registered palette, from the CSS text: light is the
 *  un-themed default and lives in the @theme block; the rest each own a
 *  :root[data-theme] token block. */
function groundOf(name: string): string {
  const { body, line } =
    name === 'light'
      ? ruleBodyDeclaring('@theme', '--color-bg-base')
      : ruleBodyDeclaring(`:root[data-theme='${name}']`, '--color-bg-base')
  const decl = /--color-bg-base:\s*(#[0-9a-fA-F]{6})\b/.exec(body)
  expect(decl, `app.css line ${line}: ${name}'s token block must define --color-bg-base as 6-hex`)
    .toBeTruthy()
  return (decl?.[1] ?? '').toLowerCase()
}

test('every registered palette has a ground of its own in app.css', () => {
  for (const theme of THEMES) {
    // 6-hex, not a color() function or a 3-digit shorthand: the picker loop
    // in e2e/theme.spec.ts paints exactly this shape.
    expect(groundOf(theme.name), `${theme.name} must define a 6-hex --color-bg-base`).toMatch(
      /^#[0-9a-f]{6}$/,
    )
  }
})

test('every app.css data-theme palette is registered in THEMES', () => {
  // The direction theme-check.mjs cannot see: it discovers palettes from
  // app.css and checks them against its own GROUND_SPECS map, which could
  // drift from THEMES undetected. :not([data-theme=…]) does not match — only
  // a rule that actually applies a palette counts.
  const inCss = new Set(
    [...CSS_CODE.matchAll(/:root\[data-theme='([a-z]+)'\]/g)].map((m) => m[1]),
  )
  const registered = new Set<string>(THEMES.map((t) => t.name))
  expect([...inCss].filter((n) => !registered.has(n)), 'CSS palettes missing from THEMES').toEqual(
    [],
  )
  expect(
    [...registered].filter((n) => !inCss.has(n)),
    'THEMES entries with no :root[data-theme] presence in app.css',
  ).toEqual([])
})

test('no two palettes paint the same ground', () => {
  const grounds = THEMES.map((t) => t.name)
  const seen = new Map<string, string>()
  for (const name of grounds) {
    const hex = groundOf(name)
    const collision = [...seen].find(([, h]) => h === hex)
    expect(
      collision,
      `${name} and ${collision?.[0]} share the ground ${hex} — a palette nobody can tell apart`,
    ).toBeUndefined()
    seen.set(name, hex)
  }
})

test('the continuity grounds stay pinned', () => {
  // Moved from e2e/theme.spec.ts's picker loop, where they rode as static
  // pins. dark is the neutral-cool charcoal; ember carries the warm dark this
  // product shipped through 0.14 forward — its hexes are a copy, not a
  // redesign, and if they drift the continuity promise made to the users who
  // chose that ground is broken.
  expect(groundOf('dark')).toBe('#0d0e10')
  expect(groundOf('ink')).toBe('#101621')
  expect(groundOf('ember')).toBe('#160f04')
})
