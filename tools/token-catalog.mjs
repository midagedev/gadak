#!/usr/bin/env node
/*
 * Token catalog generator + drift check (GDK-787).
 *
 * Single source of truth for the color-token surface: names, tier, per-palette
 * values, one-line role description, and the write-time rule ids. Go embeds
 * the generated catalog.json (internal/config/tokencheck) as its tier table,
 * and `--check` keeps the committed copy from drifting from app.css.
 *
 * Tier policy (GDK-769 investigation Q2 recommendation, adopted 2026-08-24;
 * research original: scratch/gdk-dash/tokens-report.md). The boundary the
 * tiers follow is the one tools/theme-check.mjs already drew — every floor
 * and threshold below is DERIVED from a current theme-check assertion; no
 * number in this file is invented here:
 *
 *   locked (10) — theme-check asserts palette-level structure on exactly
 *     these tokens: the 5 grounds (GROUNDS, theme-check.mjs "const GROUNDS"),
 *     the 3 text tiers (FLOOR / BASE_FLOOR + text-chroma axis), search-match
 *     (isolation ΔEok ≥0.06 + mark-ink 4.5), shell (ladder bottom, mat<page).
 *     A runtime override would have to re-run those palette-authoring
 *     assertions, so overrides are refused — the custom-palette scope (GDK-789).
 *
 *   validated (12) — accent 4, border 2, status 5, focus-ring. Open for
 *     override, checked at write time with the same formulas theme-check uses
 *     (see the rule derivation table in internal/config/tokencheck/
 *     tokencheck.go). NOTE: theme-check carries NO named assertion for
 *     accent, accent-hover, accent-subtle, border-subtle, border-strong or
 *     focus-ring today — for those
 *     the only carried rule is hex validity, and the catalog says so via an
 *     empty rules list. When theme-check gains assertions for them, the rules
 *     lists here and the Go rule table grow together (GDK-786 hand-off).
 *
 *   free (22) — lozenge 6, avatar 8, dept 6, scrim, scrollbar-hover: tokens
 *     theme-check asserts nothing about; any valid hex passes.
 *
 * Usage:
 *   node tools/token-catalog.mjs          regenerate internal/config/tokencheck/catalog.json
 *   node tools/token-catalog.mjs --check  exit 1 on drift (regen diff, set
 *                                        parity with @theme, tier whitelist,
 *                                        palette-axis parity with THEMES)
 */
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { hex2hsl, hexOf, parseAppCss } from './lib/theme-parse.mjs'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const CSS_PATH = join(ROOT, 'web/src/app.css')
const THEMES_PATH = join(ROOT, 'web/src/lib/theme.ts')
const CATALOG_PATH = join(ROOT, 'internal/config/tokencheck/catalog.json')

// locked: the tokens theme-check's palette-structure assertions are written
// against (GROUNDS, FLOOR keys, search-match isolation, shell ladder).
const LOCKED = new Set([
  'bg-base',
  'bg-panel',
  'bg-elevated',
  'bg-hover',
  'bg-active',
  'text-primary',
  'text-secondary',
  'text-muted',
  'search-match',
  'shell',
])
// validated: open for override with write-time checks. Rule ids are consumed
// by internal/config/tokencheck.ValidateTokens; derivation of each floor is
// documented in that file's header.
const VALIDATED = new Set([
  'accent',
  'accent-hover',
  'accent-subtle',
  'accent-text',
  'border-subtle',
  'border-strong',
  'status-new',
  'status-inprogress',
  'status-done',
  'status-reopen',
  'status-stale',
  'focus-ring',
])
// Everything else in @theme is free: theme-check carries no assertion that
// names it (GDK-769 investigation §0-3: the gate protects exactly 15 tokens;
// the other 29 live outside it).

const TIERS = ['free', 'validated', 'locked']

// Rule ids per token (mirrored by the Go rule table; the Go test asserts the
// two stay in sync). Floors all come from theme-check:
//   status-pair                ΔEok ≥ 0.05 (theme-check.mjs PAIR_N)
//   status-pair-deuteranopia   ΔEok ≥ 0.04 after the Machado matrix (PAIR_D)
//   status-role-floor          4.5 on base/panel/elevated, 3 on hover/active
//                              (status ink floors + per-role floors sections)
//   accent-text-contrast       ≥ 4.5 — the text-role floor theme-check applies
//                              to ink-on-ground pairs, applied to the pair
//                              accent-text is actually painted on (measured:
//                              every shipped palette clears it on accent-subtle
//                              and all five grounds)
const RULES = {
  'status-new': ['hex', 'status-pair', 'status-pair-deuteranopia', 'status-role-floor'],
  'status-inprogress': ['hex', 'status-pair', 'status-pair-deuteranopia', 'status-role-floor'],
  'status-done': ['hex', 'status-pair', 'status-pair-deuteranopia', 'status-role-floor'],
  'status-reopen': ['hex', 'status-pair', 'status-pair-deuteranopia', 'status-role-floor'],
  'status-stale': ['hex', 'status-pair', 'status-pair-deuteranopia', 'status-role-floor'],
  'accent-text': ['hex', 'accent-text-contrast'],
  'accent-subtle': ['hex', 'accent-text-contrast'],
}

// Role text: mechanical, from the name groups app.css itself documents. The
// hue family is measured off the light value so the catalog stays honest
// when a palette changes (regeneration re-derives it).
const ROLE = {
  'bg-base': 'page ground (lightest surface of the paper ladder)',
  'bg-panel': 'sidebar / right-panel ground',
  'bg-elevated': 'card / group-header ground',
  'bg-hover': 'row-hover ground',
  'bg-active': 'selected-row ground',
  'text-primary': 'primary text ink',
  'text-secondary': 'secondary text ink',
  'text-muted': 'muted text ink (metadata)',
  'border-subtle': 'hairline border',
  'border-strong': 'strong border + scrollbar track',
  accent: 'accent thread — selection marks, bars',
  'accent-hover': 'accent hover shade + inset ring on .bg-accent',
  'accent-subtle': 'subtle accent ground (badges, panels)',
  'accent-text': 'accent ink, painted on accent-subtle and the grounds',
  'status-new': 'status ink — new',
  'status-inprogress': 'status ink — in progress',
  'status-done': 'status ink — done',
  'status-reopen': 'status ink — reopened',
  'status-stale': 'status ink — stale badge',
  'search-match': 'search-hit mark ground',
  shell: 'mat outside the max-width layout',
  'focus-ring': ':focus-visible ring',
  scrim: 'modal scrim (alpha value, not a flat hex)',
  'scrollbar-hover': 'scrollbar hover thumb',
}
function roleOf(name) {
  if (ROLE[name]) return ROLE[name]
  if (name.startsWith('lozenge-')) return `Jira ADF status lozenge — ${name.slice('lozenge-'.length)}`
  if (name.startsWith('avatar-')) return `initials avatar disc ${name.slice('avatar-'.length)}`
  if (name.startsWith('dept-')) return `department ring fallback ${name.slice('dept-'.length)}`
  return 'color token'
}

function hueFamily(hex) {
  const [h, s] = hex2hsl(hex)
  if (s <= 8) return 'neutral'
  if (h < 15 || h >= 345) return 'red family'
  if (h < 45) return 'orange family'
  if (h < 70) return 'yellow family'
  if (h < 150) return 'green family'
  if (h < 200) return 'teal family'
  if (h < 255) return 'blue family'
  if (h < 290) return 'violet family'
  return 'magenta family'
}

function tierOf(name) {
  if (LOCKED.has(name)) return 'locked'
  if (VALIDATED.has(name)) return 'validated'
  return 'free'
}

function buildCatalog() {
  const css = readFileSync(CSS_PATH, 'utf8')
  const { light, themeNames, palettes } = parseAppCss(css)
  const paletteOrder = ['light', ...themeNames]
  const tokens = Object.keys(light).map((name) => {
    const values = {}
    for (const p of paletteOrder) values[p] = (p === 'light' ? light : palettes[p])[name] ?? null
    const lightHex = hexOf(light, name)
    const tier = tierOf(name)
    const desc = lightHex
      ? `${roleOf(name)}; light ${hueFamily(lightHex)} (${lightHex})`
      : roleOf(name)
    return {
      name,
      cssVar: `--color-${name}`,
      tier,
      description: desc,
      rules: tier === 'locked' ? [] : RULES[name] ?? ['hex'],
      values,
    }
  })
  return {
    $comment:
      'Generated by tools/token-catalog.mjs from web/src/app.css — do not edit by hand; regenerate. Consumed via go:embed by internal/config/tokencheck (GDK-785/787).',
    source: 'web/src/app.css',
    palettes: paletteOrder,
    tokens,
  }
}

function catalogText(doc) {
  return JSON.stringify(doc, null, 2) + '\n'
}

// Palette axis must equal web/src/lib/theme.ts THEMES — a palette added to
// app.css but not to the picker (or vice versa) is drift either way.
function themeRegistryNames() {
  const src = readFileSync(THEMES_PATH, 'utf8')
  const them = src.slice(src.indexOf('export const THEMES'), src.indexOf('export const THEME_MODES'))
  return [...them.matchAll(/name:\s*'([a-z0-9-]+)'/g)].map((m) => m[1]).sort()
}

function check(committedText, generatedText, doc) {
  let fails = 0
  const fail = (m) => {
    fails++
    console.log('  FAIL  ' + m)
  }
  console.log('=== token-catalog --check ===')
  if (committedText !== generatedText) {
    fail('committed catalog.json differs from what app.css generates — regenerate with `node tools/token-catalog.mjs`')
  } else {
    console.log('  committed catalog matches regeneration: ok')
  }
  const committed = JSON.parse(committedText)
  const css = readFileSync(CSS_PATH, 'utf8')
  const { light } = parseAppCss(css)
  const cssNames = new Set(Object.keys(light))
  const catNames = new Set(committed.tokens.map((t) => t.name))
  for (const n of cssNames) if (!catNames.has(n)) fail(`--color-${n} is in @theme but missing from the catalog`)
  for (const n of catNames) if (!cssNames.has(n)) fail(`--color-${n} is in the catalog but not in @theme`)
  if (cssNames.size === catNames.size && cssNames.size === [...cssNames].filter((n) => catNames.has(n)).length) {
    console.log(`  token set parity (@theme ⇄ catalog, ${cssNames.size} tokens): ok`)
  }
  for (const t of committed.tokens) {
    if (!TIERS.includes(t.tier)) fail(`--color-${t.name} has tier "${t.tier}" — not one of ${TIERS.join(', ')}`)
  }
  const registry = themeRegistryNames().join(',')
  const axis = [...doc.palettes].sort().join(',')
  if (registry !== axis) {
    fail(`catalog palette axis [${axis}] != THEMES registry [${registry}] in web/src/lib/theme.ts`)
  } else {
    console.log(`  palette axis == THEMES (${axis}): ok`)
  }
  console.log(`=== ${fails === 0 ? 'CATALOG IN SYNC' : fails + ' FAILURES'} ===`)
  return fails === 0
}

const generated = buildCatalog()
const generatedText = catalogText(generated)
if (process.argv.includes('--check')) {
  const committedText = readFileSync(CATALOG_PATH, 'utf8')
  process.exit(check(committedText, generatedText, generated) ? 0 : 1)
}
writeFileSync(CATALOG_PATH, generatedText)
console.log(`wrote ${CATALOG_PATH} (${generated.tokens.length} tokens, palettes: ${generated.palettes.join(', ')})`)
const byTier = {}
for (const t of generated.tokens) byTier[t.tier] = (byTier[t.tier] ?? 0) + 1
console.log(`tiers: ${Object.entries(byTier).map(([k, v]) => `${k} ${v}`).join(', ')}`)
