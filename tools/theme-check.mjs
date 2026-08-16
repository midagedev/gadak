#!/usr/bin/env node
/*
 * Standing theme gate (GDK-154 / GDK-156).
 *
 * Parses web/src/app.css token blocks and asserts:
 *   (a) dark ground hue/chroma anti-slop (warm band, C ≥ 1.25× loudest ref)
 *   (b) WCAG floors from the design: body ≥7, secondary ≥4.5, muted ≥3
 *       on every ground (base / panel / elevated / hover / active)
 *   (c) every --color-* in @theme exists in BOTH dark blocks with the same value
 *
 * Extra check.mjs ports that prevent quiet palette drift: status-ink floors
 * and rank, text-primary chroma, mat darker than the page.
 *
 * Usage: node tools/theme-check.mjs
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const CSS_PATH = join(ROOT, 'web/src/app.css')
const css = readFileSync(CSS_PATH, 'utf8')

let fails = 0
const fail = (m) => {
  fails++
  console.log('  FAIL  ' + m)
}

const hex2rgb = (h) => {
  const s = h.replace('#', '')
  const full = s.length === 3 ? [...s].map((c) => c + c).join('') : s
  return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16) / 255)
}
const lin = (c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
const luminance = (hex) => {
  const [r, g, b] = hex2rgb(hex).map(lin)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
const contrast = (a, b) => {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (l1 + 0.05) / (l2 + 0.05)
}
const cr = (a, b) => Math.round(contrast(a, b) * 100) / 100

const hex2oklab = (hex) => {
  const [r, g, b] = hex2rgb(hex).map(lin)
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ]
}
const hex2oklch = (hex) => {
  const [L, a, b] = hex2oklab(hex)
  const C = Math.hypot(a, b)
  let H = (Math.atan2(b, a) * 180) / Math.PI
  if (H < 0) H += 360
  return [L, C, H]
}

function extractBraceBlock(source, openBraceIndex) {
  let depth = 0
  for (let i = openBraceIndex; i < source.length; i++) {
    const ch = source[i]
    if (ch === '{') depth++
    else if (ch === '}') {
      depth--
      if (depth === 0) return source.slice(openBraceIndex + 1, i)
    }
  }
  return null
}

function parseColorDecls(block) {
  const out = {}
  if (!block) return out
  const re = /--color-([a-z0-9-]+)\s*:\s*([^;]+);/gi
  let m
  while ((m = re.exec(block))) {
    out[m[1]] = m[2].trim()
  }
  return out
}

function findAtTheme(source) {
  const idx = source.search(/@theme\b/)
  if (idx < 0) return null
  const brace = source.indexOf('{', idx)
  return brace < 0 ? null : extractBraceBlock(source, brace)
}

function findSelectorBlocks(source, selectorRe) {
  const blocks = []
  const re = new RegExp(selectorRe.source, selectorRe.flags.includes('g') ? selectorRe.flags : `${selectorRe.flags}g`)
  let m
  while ((m = re.exec(source))) {
    const brace = source.indexOf('{', m.index + m[0].length - 1)
    if (brace < 0) continue
    const body = extractBraceBlock(source, brace)
    if (body) blocks.push(body)
  }
  return blocks
}

function richestColorDecls(blocks) {
  let best = {}
  for (const block of blocks) {
    const decls = parseColorDecls(block)
    if (Object.keys(decls).length > Object.keys(best).length) best = decls
  }
  return best
}

const light = parseColorDecls(findAtTheme(css))
const darkExplicit = richestColorDecls(findSelectorBlocks(css, /:root\[data-theme=['"]dark['"]\]/))
const darkMediaInner = (() => {
  // Skip the unlayered color-scheme-only media query; take the richest
  // token block under any prefers-color-scheme: dark section.
  const mediaRe = /@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)/g
  const inner = []
  let media
  while ((media = mediaRe.exec(css))) {
    const mediaBrace = css.indexOf('{', media.index)
    const mediaBody = extractBraceBlock(css, mediaBrace)
    if (!mediaBody) continue
    inner.push(
      ...findSelectorBlocks(mediaBody, /:root:not\(\s*\[data-theme=['"]light['"]\]\s*\)/),
    )
  }
  return richestColorDecls(inner)
})()

console.log('=== theme-check: parse ===')
console.log('  light --color-*', Object.keys(light).length)
console.log('  dark [data-theme=dark] --color-*', Object.keys(darkExplicit).length)
console.log('  dark prefers-color-scheme --color-*', Object.keys(darkMediaInner).length)

if (Object.keys(light).length === 0) fail('no --color-* tokens in @theme')
if (Object.keys(darkExplicit).length === 0) {
  fail("no --color-* tokens under :root[data-theme='dark'] — a color must not live only in a media query")
}
if (Object.keys(darkMediaInner).length === 0) {
  fail("no --color-* tokens under @media (prefers-color-scheme: dark) :root:not([data-theme='light'])")
}

console.log('\n=== (c) every light --color-* exists in every theme block ===')
const lightKeys = Object.keys(light).sort()
for (const key of lightKeys) {
  if (!(key in darkExplicit)) fail(`--color-${key} missing from [data-theme=dark]`)
  if (!(key in darkMediaInner)) fail(`--color-${key} missing from prefers-color-scheme dark block`)
  if (key in darkExplicit && key in darkMediaInner && darkExplicit[key] !== darkMediaInner[key]) {
    fail(`--color-${key} differs between dark blocks: ${darkExplicit[key]} vs ${darkMediaInner[key]}`)
  }
}
for (const key of Object.keys(darkExplicit)) {
  if (!(key in light)) fail(`--color-${key} is in dark but not in @theme (light)`)
}
if (fails === 0) console.log('  token parity: ok')

const hexOf = (pal, name) => {
  const v = pal[name]
  if (!v) return null
  const m = v.match(/#([0-9a-fA-F]{3,8})\b/)
  return m ? (m[0].length === 4 ? null : m[0].toLowerCase()) : null
}

const dark = darkExplicit
const GROUNDS = ['bg-base', 'bg-panel', 'bg-elevated', 'bg-hover', 'bg-active']
const FLOOR = { 'text-primary': 7, 'text-secondary': 4.5, 'text-muted': 3 }

console.log('\n=== (a) anti-slop: dark ground hue + chroma ===')
const baseHex = hexOf(dark, 'bg-base')
if (!baseHex) {
  fail('dark --color-bg-base is not a hex token')
} else {
  const [, ourC, ourH] = hex2oklch(baseHex)
  // Warm band: paper family. Cool (H>180) is the GitHub/Linear/VS Code slop ground.
  if (ourH > 180) fail(`ground hue ${ourH.toFixed(0)} is cool — that is the slop ground`)
  if (ourH < 50 || ourH > 110) fail(`ground hue ${ourH.toFixed(0)} left the warm paper band (50–110)`)
  const refGrounds = ['#0d1117', '#161b22', '#010409', '#08090a', '#1e1e1e', '#252526']
  const refMaxC = Math.max(...refGrounds.map((h) => hex2oklch(h)[1]))
  console.log(
    `  gadak ground ${baseHex} C ${ourC.toFixed(3)} H ${ourH.toFixed(0)} vs loudest ref C ${refMaxC.toFixed(3)} (need ≥ ${(refMaxC * 1.25).toFixed(3)})`,
  )
  if (ourC < refMaxC * 1.25) {
    fail(`ground chroma ${ourC.toFixed(3)} is within noise of the references (${refMaxC.toFixed(3)})`)
  }
}

console.log('\n=== (b) WCAG floors (dark text × grounds) ===')
for (const fg of Object.keys(FLOOR)) {
  const fgHex = hexOf(dark, fg)
  if (!fgHex) {
    fail(`dark --color-${fg} is not a hex token`)
    continue
  }
  for (const g of GROUNDS) {
    const gHex = hexOf(dark, g)
    if (!gHex) {
      fail(`dark --color-${g} is not a hex token`)
      continue
    }
    const v = cr(fgHex, gHex)
    process.stdout.write(`  ${fg} on ${g}: ${v}`)
    if (v < FLOOR[fg]) {
      console.log(`  < ${FLOOR[fg]}`)
      fail(`${fg} on ${g} = ${v}, floor ${FLOOR[fg]}`)
    } else console.log()
  }
}

console.log('\n=== status ink floors + rank (check.mjs §5) ===')
const STATS = ['status-new', 'status-inprogress', 'status-done', 'status-reopen', 'status-stale']
const rankOf = (pal) =>
  STATS.map((s) => [s, cr(hexOf(pal, s), hexOf(pal, 'bg-base'))])
    .sort((a, b) => b[1] - a[1])
    .map(([k]) => k)
    .join(' > ')
for (const s of STATS) {
  const fgHex = hexOf(dark, s)
  if (!fgHex) {
    fail(`dark --color-${s} is not a hex token`)
    continue
  }
  for (const g of GROUNDS) {
    const gHex = hexOf(dark, g)
    if (!gHex) continue
    const v = cr(fgHex, gHex)
    const floor = g === 'bg-hover' || g === 'bg-active' ? 3 : 4.5
    if (v < floor) fail(`${s} on ${g} = ${v}, floor ${floor}`)
  }
}
const lightHex = (name) => hexOf(light, name)
if (STATS.every((s) => hexOf(dark, s) && hexOf(light, s) && hexOf(dark, 'bg-base') && hexOf(light, 'bg-base'))) {
  const lr = rankOf(light)
  const dr = rankOf(dark)
  console.log('  light rank', lr.replace(/status-/g, ''))
  console.log('  dark  rank', dr.replace(/status-/g, ''))
  if (lr !== dr) fail('status inks changed their relative loudness in translation')
}

console.log('\n=== extras: text chroma, mat below the page ===')
const textHex = hexOf(dark, 'text-primary')
if (textHex) {
  const tC = hex2oklch(textHex)[1]
  console.log(`  text-primary chroma ${tC.toFixed(3)}`)
  if (tC < 0.01) fail(`text-primary chroma ${tC.toFixed(3)} — grey text is the second slop tell`)
}
const shellHex = hexOf(dark, 'shell')
const pageHex = hexOf(dark, 'bg-base')
if (shellHex && pageHex) {
  const shellL = hex2oklch(shellHex)[0]
  const pageL = hex2oklch(pageHex)[0]
  console.log(`  shell L ${(shellL * 100).toFixed(1)} vs page L ${(pageL * 100).toFixed(1)}`)
  if (shellL >= pageL) fail('the mat is lighter than the page — it will glow around the layout')
}

console.log(`\n=== ${fails === 0 ? 'ALL CHECKS PASS' : fails + ' FAILURES'} ===`)
process.exit(fails === 0 ? 0 : 1)
