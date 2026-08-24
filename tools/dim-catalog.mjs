#!/usr/bin/env node
/*
 * Dimension catalog generator + drift check (dim-token chunk 1).
 *
 * Single source of the v1 dimension-override surface: token names, cssVar,
 * default, tier, unit and the numeric ranges tokencheck.ValidateDimensions
 * enforces. Go embeds the generated dim-catalog.json
 * (internal/config/tokencheck) as its tier table, and `--check` keeps the
 * committed copy from drifting.
 *
 * Where each default comes from (deliberately split, unlike colors):
 *   - spacing/type defaults are READ from web/src/app.css @theme
 *     (--spacing-*, --text-* including the --line-height pairs), exactly like
 *     token-catalog.mjs reads --color-*.
 *   - layout defaults live in an explicit table below because CSS does not
 *     own them yet: viewport-regime.ts constants (sidebar 272, list-min 390,
 *     detail-min 438 — single owner since GDK-201), the app.css 760px media
 *     step (sidebar-narrow 208) and its clamp/literal maxima (detail-max 720,
 *     overlay-max 560, shell-max 2200). The css-literal chunk moves the
 *     literals to var() consumption; until then this table is the honest
 *     mirror. docked-min is not typed at all: it is the SUM of the three
 *     track mins, recomputed here so it cannot drift from its parts.
 *
 * Tier policy (census scratch/dimwave/census.md Q6, adopted for v1):
 *   validated-range  19 tokens — open for override, checked at write time by
 *                    length format, min/max range and cross-token relations.
 *   locked            1 token  — layout.docked-min, the derived dock floor.
 *                    Writes are refused; the runtime recomputes it from the
 *                    three track tokens.
 *   wordmark is deliberately NOT in the catalog (v1 keeps it closed).
 *
 * Usage:
 *   node tools/dim-catalog.mjs          regenerate internal/config/tokencheck/dim-catalog.json
 *   node tools/dim-catalog.mjs --check  exit 1 on drift (regen diff, @theme
 *                                       set parity for spacing/type, tier and
 *                                       unit whitelists, defaults inside their
 *                                       own ranges, docked-min == sum)
 */
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { findAtTheme } from './lib/theme-parse.mjs'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const CSS_PATH = join(ROOT, 'web/src/app.css')
const CATALOG_PATH = join(ROOT, 'internal/config/tokencheck/dim-catalog.json')

const TIERS = ['validated-range', 'locked']

// Numeric ranges per catalog name (census Q6). min omitted where the bound is
// a cross-token relation instead of a constant (row-excerpt ≥ row+8,
// detail-max ≥ detail-min — enforced in Go, see dimRelations).
const RANGE = {
  'spacing.row': { min: 36, max: 56 },
  'spacing.row-excerpt': { max: 72 },
  'spacing.control': { min: 28, max: 40 },
  'spacing.control-sm': { min: 20, max: 32 },
  'layout.sidebar': { min: 208, max: 320 },
  'layout.sidebar-narrow': { min: 160, max: 240 },
  'layout.list-min': { min: 320, max: 480 },
  'layout.detail-min': { min: 360, max: 520 },
  'layout.detail-max': { max: 900 },
  'layout.overlay-max': { min: 360, max: 720 },
  'layout.shell-max': { min: 1600, max: 2800 },
  'type.micro': { min: 11, max: 13 },
  'type.body': { min: 12, max: 16 },
  'type.title': { min: 14, max: 18 },
  'type.heading': { min: 18, max: 28 },
  'type.micro-line-height': { min: 1.15, max: 1.6 },
  'type.body-line-height': { min: 1.15, max: 1.6 },
  'type.title-line-height': { min: 1.15, max: 1.6 },
  'type.heading-line-height': { min: 1.15, max: 1.6 },
}

// Layout axis: explicit table (JS-owned constants, see header). docked-min is
// appended after the sum is known.
const LAYOUT_ORDER = [
  'sidebar',
  'sidebar-narrow',
  'list-min',
  'detail-min',
  'detail-max',
  'overlay-max',
  'shell-max',
]
const LAYOUT_DEFAULT = {
  sidebar: '272px', // web/src/lib/viewport-regime.ts LAYOUT_SIDEBAR_PX
  'sidebar-narrow': '208px', // app.css @media (max-width: 760px) LNB step
  'list-min': '390px', // viewport-regime.ts LAYOUT_LIST_MIN_PX
  'detail-min': '438px', // viewport-regime.ts LAYOUT_DETAIL_MIN_PX
  'detail-max': '720px', // app.css docked clamp upper bound
  'overlay-max': '560px', // app.css overlay panel cap
  'shell-max': '2200px', // app.css .issue-layout max-width
}

const pxOf = (v) => Number.parseFloat(v)
const lineHeightOf = (v) => Number.parseFloat(v)

function parsePrefixDecls(block, prefix) {
  const out = {}
  if (!block) return out
  const re = new RegExp(`${prefix}([a-z0-9-]+)\\s*:\\s*([^;]+);`, 'g')
  let m
  while ((m = re.exec(block))) out[m[1]] = m[2].trim()
  return out
}

// @theme --text-micro--line-height → catalog name micro-line-height
function typeName(cssName) {
  const lh = '--line-height'
  return cssName.endsWith(lh) ? cssName.slice(0, -lh.length) + '-line-height' : cssName
}

function entry(axis, name, cssVar, def, tier, unit) {
  const e = { cssVar, default: def, tier, unit }
  const r = RANGE[`${axis}.${name}`]
  if (r) {
    if (r.min !== undefined) e.min = r.min
    if (r.max !== undefined) e.max = r.max
  }
  return e
}

function buildCatalog() {
  const theme = findAtTheme(readFileSync(CSS_PATH, 'utf8'))
  const spacingDecls = parsePrefixDecls(theme, '--spacing-')
  const textDecls = parsePrefixDecls(theme, '--text-')

  const spacing = {}
  for (const [cssName, value] of Object.entries(spacingDecls)) {
    spacing[cssName] = entry('spacing', cssName, `--spacing-${cssName}`, value, 'validated-range', 'px')
  }
  const type = {}
  for (const [cssName, value] of Object.entries(textDecls)) {
    const name = typeName(cssName)
    const unit = name.endsWith('-line-height') ? 'none' : 'px'
    type[name] = entry('type', name, `--text-${cssName}`, value, 'validated-range', unit)
  }
  const layout = {}
  for (const name of LAYOUT_ORDER) {
    layout[name] = entry('layout', name, `--layout-${name}`, LAYOUT_DEFAULT[name], 'validated-range', 'px')
  }
  const sum =
    pxOf(LAYOUT_DEFAULT.sidebar) + pxOf(LAYOUT_DEFAULT['list-min']) + pxOf(LAYOUT_DEFAULT['detail-min'])
  layout['docked-min'] = {
    cssVar: '--layout-docked-min',
    default: `${sum}px`,
    tier: 'locked',
    unit: 'px',
  }
  return {
    $comment:
      'Generated by tools/dim-catalog.mjs — do not edit by hand; regenerate. Spacing/type defaults from web/src/app.css @theme; layout defaults from the explicit table in the generator (JS-owned constants, not yet CSS vars). Consumed via go:embed by internal/config/tokencheck.',
    source: 'web/src/app.css @theme + generator layout table',
    axes: [
      { id: 'spacing', cssPrefix: '--spacing-', tokens: spacing },
      { id: 'layout', cssPrefix: '--layout-', tokens: layout },
      { id: 'type', cssPrefix: '--text-', tokens: type },
    ],
  }
}

function catalogText(doc) {
  return JSON.stringify(doc, null, 2) + '\n'
}

function check(committedText, generatedText, doc) {
  let fails = 0
  const fail = (m) => {
    fails++
    console.log('  FAIL  ' + m)
  }
  console.log('=== dim-catalog --check ===')
  if (committedText !== generatedText) {
    fail('committed dim-catalog.json differs from what the sources generate — regenerate with `node tools/dim-catalog.mjs`')
  } else {
    console.log('  committed catalog matches regeneration: ok')
  }

  // Set parity: every @theme spacing/text var must be catalogued with a
  // declared range, and vice versa — a new --spacing-* in app.css without a
  // conscious tier/range decision fails here instead of shipping unvalidated.
  const theme = findAtTheme(readFileSync(CSS_PATH, 'utf8'))
  const spacingDecls = parsePrefixDecls(theme, '--spacing-')
  const textDecls = parsePrefixDecls(theme, '--text-')
  const cssTypeNames = new Set(Object.keys(textDecls).map(typeName))
  const ax = Object.fromEntries(doc.axes.map((a) => [a.id, a.tokens]))
  for (const [axis, decls, toName] of [
    ['spacing', spacingDecls, (n) => n],
    ['type', textDecls, typeName],
  ]) {
    const cssNames = new Set(Object.keys(decls).map(toName))
    const catNames = new Set(Object.keys(ax[axis]))
    for (const n of cssNames) if (!catNames.has(n)) fail(`${axis}.${n} is in @theme but has no catalog entry (declare a tier and range in the generator)`)
    for (const n of catNames) if (!cssNames.has(n)) fail(`${axis}.${n} is in the catalog but not in @theme`)
    if (cssNames.size === catNames.size && [...cssNames].every((n) => catNames.has(n))) {
      console.log(`  ${axis} set parity (@theme ⇄ catalog, ${cssNames.size} tokens): ok`)
    }
  }

  // Layout axis: the generator table is the source; assert docked-min is the
  // live sum of the three track mins.
  const trackSum =
    pxOf(ax.layout.sidebar.default) +
    pxOf(ax.layout['list-min'].default) +
    pxOf(ax.layout['detail-min'].default)
  if (pxOf(ax.layout['docked-min'].default) !== trackSum) {
    fail(`layout.docked-min default ${ax.layout['docked-min'].default} != sidebar+list-min+detail-min (${trackSum}px) — the sum is the contract`)
  } else {
    console.log(`  docked-min == sidebar+list-min+detail-min (${trackSum}px): ok`)
  }

  let writable = 0
  const sectionFails = fails
  for (const axis of doc.axes) {
    for (const [name, tok] of Object.entries(axis.tokens)) {
      if (!TIERS.includes(tok.tier)) fail(`${axis.id}.${name} tier "${tok.tier}" is not one of ${TIERS.join(', ')}`)
      if (!['px', 'none'].includes(tok.unit)) fail(`${axis.id}.${name} unit "${tok.unit}" is not px or none`)
      if (axis.id === 'layout' && !RANGE[`layout.${name}`] && tok.tier !== 'locked') {
        fail(`layout.${name} has no declared range in the generator table`)
      }
      if (tok.tier === 'locked') continue
      writable++
      const v = tok.unit === 'none' ? lineHeightOf(tok.default) : pxOf(tok.default)
      const r = RANGE[`${axis.id}.${name}`] ?? {}
      if (Number.isNaN(v)) {
        fail(`${axis.id}.${name} default "${tok.default}" does not parse as ${tok.unit === 'none' ? 'a unitless number' : 'a px length'}`)
      } else if ((r.min !== undefined && v < r.min) || (r.max !== undefined && v > r.max)) {
        fail(`${axis.id}.${name} default "${tok.default}" sits outside its own range [${r.min ?? '—'}, ${r.max ?? '—'}] — move the CSS value or the range deliberately`)
      }
    }
  }
  const locked = Object.values(ax.layout).filter((t) => t.tier === 'locked').length
  const section = `tier/unit whitelists + defaults-in-range (${writable} writable, ${locked} locked)`
  console.log(`  ${section}: ${fails === sectionFails ? 'ok' : 'see FAIL lines above'}`)
  console.log(`=== ${fails === 0 ? 'DIM CATALOG IN SYNC' : fails + ' FAILURES'} ===`)
  return fails === 0
}

const generated = buildCatalog()
const generatedText = catalogText(generated)
if (process.argv.includes('--check')) {
  const committedText = readFileSync(CATALOG_PATH, 'utf8')
  process.exit(check(committedText, generatedText, generated) ? 0 : 1)
}
writeFileSync(CATALOG_PATH, generatedText)
const counts = {}
for (const a of generated.axes) for (const t of Object.values(a.tokens)) counts[a.id] = (counts[a.id] ?? 0) + 1
console.log(`wrote ${CATALOG_PATH} (${Object.entries(counts).map(([k, v]) => `${k} ${v}`).join(', ')})`)
