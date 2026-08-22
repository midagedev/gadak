#!/usr/bin/env node
/*
 * Standing theme gate (GDK-154 / GDK-156 / GDK-190).
 *
 * Palettes are discovered from app.css, not listed here: every
 * :root[data-theme='NAME'] block is parsed and measured, so adding a palette
 * cannot mean adding an unmeasured one.
 *
 * Parses web/src/app.css token blocks and asserts:
 *   (a) per-palette ground identity — the hue/saturation band each theme owns
 *       (GROUND_SPECS), plus an ascending shell→active lightness ladder
 *   (b) WCAG floors from the design: body ≥7, secondary ≥4.5, muted ≥3
 *       on every ground (base / panel / elevated / hover / active), and the
 *       stricter GDK-190 floors 10 / 5.5 / 4.5 on the page ground
 *   (c) every --color-* in @theme exists in EVERY palette block, and the two
 *       dark blocks (media query + data-theme) carry the same value
 *
 * Extra check.mjs ports that prevent quiet palette drift: status-ink floors
 * and rank, text-primary chroma, mat darker than the page.
 *
 * GDK-190 also asserts palette separation: two picker entries whose grounds are
 * within ΔEok 0.015 are the same theme wearing two names.
 *
 * GDK-157 / GDK-159 axes (every palette):
 *   pairwise status-ink ΔEok (normal ≥0.05, deuteranopia ≥0.04)
 *   search-match token isolation (ΔEok ≥0.06 vs every ground and chip token)
 *   search-match text contrast (the ink Marks.svelte actually paints ≥4.5)
 *   per-role ink contrast (text ≥4.5 on base/panel/elevated, dot ≥3.0 on hover/active)
 *
 * GDK-129: web/src .svelte/.ts must not carry arbitrary text-[Npx] utilities.
 * Size lives on the four type-scale tokens (and the .wordmark owner).
 *
 * Usage: node tools/theme-check.mjs
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
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
const dEok = (a, b) => {
  const A = hex2oklab(a)
  const B = hex2oklab(b)
  return Math.hypot(A[0] - B[0], A[1] - B[1], A[2] - B[2])
}
const rgb2hex = (rgb) =>
  '#' +
  rgb
    .map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255).toString(16).padStart(2, '0'))
    .join('')
// Machado 2009 deuteranopia, severity 1.0. Same matrix as the GDK-157
// design scripts (analyse.mjs / verify.mjs). Do not substitute another.
const DEUT_M = [
  [0.367322, 0.860646, -0.227968],
  [0.280085, 0.672501, 0.047413],
  [-0.01182, 0.04294, 0.968881],
]
const deut = (hex) => {
  const [r, g, b] = hex2rgb(hex)
  return rgb2hex(DEUT_M.map((row) => row[0] * r + row[1] * g + row[2] * b))
}
const alphaOver = (fg, bg, a) => {
  const f = hex2rgb(fg)
  const b = hex2rgb(bg)
  return rgb2hex([0, 1, 2].map((i) => f[i] * a + b[i] * (1 - a)))
}
const hex2oklch = (hex) => {
  const [L, a, b] = hex2oklab(hex)
  const C = Math.hypot(a, b)
  let H = (Math.atan2(b, a) * 180) / Math.PI
  if (H < 0) H += 360
  return [L, C, H]
}
// HSL, because the GDK-190 ground contract is written in HSL hue/saturation:
// at ground lightness oklch chroma is a three-decimal number nobody can aim at,
// while "hue 220, S 10%" is a value a designer can read off the token.
const hex2hsl = (hex) => {
  const [r, g, b] = hex2rgb(hex)
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const L = (max + min) / 2
  const d = max - min
  if (d === 0) return [0, 0, L * 100]
  const S = d / (1 - Math.abs(2 * L - 1))
  let H
  if (max === r) H = 60 * (((g - b) / d) % 6)
  else if (max === g) H = 60 * ((b - r) / d + 2)
  else H = 60 * ((r - g) / d + 4)
  if (H < 0) H += 360
  return [H, S * 100, L * 100]
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

/*
 * Every palette declares itself with one :root[data-theme='NAME'] block, so the
 * gate discovers them instead of being told. A palette added to app.css and
 * THEMES but forgotten here would ship unmeasured — that is the failure this
 * enumeration closes.
 */
function discoverThemeNames(source) {
  const names = new Set()
  const re = /:root\[data-theme=['"]([a-z0-9-]+)['"]\]/gi
  let m
  while ((m = re.exec(source))) {
    if (m[1] !== 'light') names.add(m[1])
  }
  return [...names].sort()
}
const THEME_NAMES = discoverThemeNames(css)
const paletteOf = (name) =>
  richestColorDecls(
    findSelectorBlocks(css, new RegExp(`:root\\[data-theme=['"]${name}['"]\\]`)),
  )
const PALETTES = Object.fromEntries(THEME_NAMES.map((n) => [n, paletteOf(n)]))
const darkExplicit = PALETTES.dark ?? {}
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
console.log('  discovered data-theme palettes:', THEME_NAMES.join(', ') || '(none)')
for (const n of THEME_NAMES) console.log(`  ${n} --color-*`, Object.keys(PALETTES[n]).length)
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
  for (const n of THEME_NAMES) {
    if (!(key in PALETTES[n])) fail(`--color-${key} missing from [data-theme=${n}]`)
  }
  if (!(key in darkMediaInner)) fail(`--color-${key} missing from prefers-color-scheme dark block`)
  if (key in darkExplicit && key in darkMediaInner && darkExplicit[key] !== darkMediaInner[key]) {
    fail(`--color-${key} differs between dark blocks: ${darkExplicit[key]} vs ${darkMediaInner[key]}`)
  }
}
for (const n of THEME_NAMES) {
  for (const key of Object.keys(PALETTES[n])) {
    if (!(key in light)) fail(`--color-${key} is in ${n} but not in @theme (light)`)
  }
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

/*
 * GDK-190 ground contract, one row per palette. Each dark-family palette owns a
 * ground identity and the gate holds it there, so "the dark theme drifted warm
 * again" cannot happen quietly in either direction.
 *
 * The warm-paper anti-slop axis (GDK-154: hue 50–110, chroma ≥ 1.25× the
 * loudest reference ground) is NOT relaxed — it moved to `ember`, which is a
 * byte-for-byte copy of the palette that axis was written against. `dark` is
 * now a neutral-cool charcoal by user decision (2026-08-17: "기반 다크 테마가
 * 색상이 너무 불그스레한 게 맘에 안 든다"), so holding it in the warm band would
 * assert the opposite of the product's intent. Anyone reintroducing a warm
 * default should retarget this row, not delete it.
 */
const GROUND_SPECS = {
  dark: {
    label: 'neutral-cool charcoal',
    // hue band OR fully neutral (S ≤ 4, where hue is not perceptible anyway)
    hslHue: [210, 250],
    hslSat: [0, 12],
    neutralSatEscape: 4,
  },
  ink: {
    label: 'deep blue-black',
    hslHue: [210, 225],
    hslSat: [15, 35],
    neutralSatEscape: 0,
    baseHslL: [6, 10],
  },
  ember: {
    label: 'warm paper (GDK-154 anti-slop axis lives here)',
    warmPaper: true,
  },
}

const LADDER = ['shell', 'bg-base', 'bg-panel', 'bg-elevated', 'bg-hover', 'bg-active']

// Both directions, or the axis leaks. An unspecced palette ships unmeasured;
// a spec whose palette was renamed or deleted stops asserting anything and the
// gate still reports green — which is how the warm-paper axis could be lost.
for (const name of Object.keys(GROUND_SPECS)) {
  if (!THEME_NAMES.includes(name)) {
    fail(`GROUND_SPECS has '${name}' but app.css declares no :root[data-theme='${name}'] — that axis is not running`)
  }
}

console.log('\n=== (a) ground identity per palette (hue / saturation bands) ===')
for (const name of THEME_NAMES) {
  const pal = PALETTES[name]
  const spec = GROUND_SPECS[name]
  console.log(`  ${name}${spec ? '' : '  (no ground spec — add one to GROUND_SPECS)'}`)
  if (!spec) {
    fail(`palette '${name}' has no entry in GROUND_SPECS — an unmeasured ground ships silently`)
    continue
  }
  for (const g of LADDER) {
    const hex = hexOf(pal, g)
    if (!hex) {
      fail(`${name} --color-${g} is not a hex token`)
      continue
    }
    const [h, s, l] = hex2hsl(hex)
    const [oL, oC, oH] = hex2oklch(hex)
    console.log(
      `    ${g.padEnd(12)} ${hex}  hsl(${h.toFixed(0)} ${s.toFixed(1)}% ${l.toFixed(1)}%)  okL ${(oL * 100).toFixed(1)} okC ${oC.toFixed(3)} okH ${oH.toFixed(0)}`,
    )
    if (spec.warmPaper) continue
    const neutral = s <= spec.neutralSatEscape
    if (!neutral && (h < spec.hslHue[0] || h > spec.hslHue[1])) {
      fail(
        `${name} ${g} ${hex} hue ${h.toFixed(0)}° outside [${spec.hslHue}] (${spec.label}); S ${s.toFixed(1)}% is above the neutral escape ${spec.neutralSatEscape}%`,
      )
    }
    if (s < spec.hslSat[0] || s > spec.hslSat[1]) {
      fail(`${name} ${g} ${hex} saturation ${s.toFixed(1)}% outside [${spec.hslSat}] (${spec.label})`)
    }
  }
  if (spec.baseHslL) {
    const hex = hexOf(pal, 'bg-base')
    if (hex) {
      const l = hex2hsl(hex)[2]
      if (l < spec.baseHslL[0] || l > spec.baseHslL[1]) {
        fail(`${name} bg-base ${hex} HSL L ${l.toFixed(1)}% outside [${spec.baseHslL}]`)
      }
    }
  }
  if (spec.warmPaper) {
    const baseHex = hexOf(pal, 'bg-base')
    if (!baseHex) {
      fail(`${name} --color-bg-base is not a hex token`)
    } else {
      const [, ourC, ourH] = hex2oklch(baseHex)
      // Warm band: paper family. Cool (H>180) is the GitHub/Linear/VS Code slop ground.
      if (ourH > 180) fail(`${name} ground hue ${ourH.toFixed(0)} is cool — that is the slop ground`)
      if (ourH < 50 || ourH > 110) {
        fail(`${name} ground hue ${ourH.toFixed(0)} left the warm paper band (50–110)`)
      }
      const refGrounds = ['#0d1117', '#161b22', '#010409', '#08090a', '#1e1e1e', '#252526']
      const refMaxC = Math.max(...refGrounds.map((h) => hex2oklch(h)[1]))
      console.log(
        `    ${name} ground ${baseHex} C ${ourC.toFixed(3)} H ${ourH.toFixed(0)} vs loudest ref C ${refMaxC.toFixed(3)} (need ≥ ${(refMaxC * 1.25).toFixed(3)})`,
      )
      if (ourC < refMaxC * 1.25) {
        fail(`${name} ground chroma ${ourC.toFixed(3)} is within noise of the references (${refMaxC.toFixed(3)})`)
      }
    }
  }
  // The ladder is the depth cue: shell is the mat, active is the nearest
  // surface. Out of order, elevation reads backwards on every panel.
  const ls = LADDER.map((g) => [g, hexOf(pal, g)]).filter(([, h]) => h).map(([g, h]) => [g, hex2oklch(h)[0]])
  for (let i = 1; i < ls.length; i++) {
    if (ls[i][1] <= ls[i - 1][1]) {
      fail(
        `${name} ladder not ascending at ${ls[i - 1][0]} okL ${(ls[i - 1][1] * 100).toFixed(1)} → ${ls[i][0]} okL ${(ls[i][1] * 100).toFixed(1)}`,
      )
    }
  }
}

/*
 * Two floors, not one. FLOOR is the original per-ground minimum every surface
 * must clear. BASE_FLOOR is the stricter GDK-190 floor on the page ground only:
 * the page is where the reading happens, and a palette that only just clears 7
 * there reads dim even though no single pair is illegal.
 */
const BASE_FLOOR = { 'text-primary': 10, 'text-secondary': 5.5, 'text-muted': 4.5 }

console.log('\n=== (b) WCAG floors (text × grounds, every dark palette) ===')
for (const name of THEME_NAMES) {
  const pal = PALETTES[name]
  console.log(`  ${name}`)
  for (const fg of Object.keys(FLOOR)) {
    const fgHex = hexOf(pal, fg)
    if (!fgHex) {
      fail(`${name} --color-${fg} is not a hex token`)
      continue
    }
    for (const g of GROUNDS) {
      const gHex = hexOf(pal, g)
      if (!gHex) {
        fail(`${name} --color-${g} is not a hex token`)
        continue
      }
      const v = cr(fgHex, gHex)
      const floor = g === 'bg-base' ? Math.max(FLOOR[fg], BASE_FLOOR[fg]) : FLOOR[fg]
      process.stdout.write(`    ${fg} on ${g}: ${v}`)
      if (v < floor) {
        console.log(`  < ${floor}`)
        fail(`${name} ${fg} on ${g} = ${v}, floor ${floor}`)
      } else console.log()
    }
  }
}

console.log('\n=== status ink floors + rank (check.mjs §5) ===')
const STATS = ['status-new', 'status-inprogress', 'status-done', 'status-reopen', 'status-stale']
const rankOf = (pal) =>
  STATS.map((s) => [s, cr(hexOf(pal, s), hexOf(pal, 'bg-base'))])
    .sort((a, b) => b[1] - a[1])
    .map(([k]) => k)
    .join(' > ')
for (const name of THEME_NAMES) {
  const pal = PALETTES[name]
  for (const s of STATS) {
    const fgHex = hexOf(pal, s)
    if (!fgHex) {
      fail(`${name} --color-${s} is not a hex token`)
      continue
    }
    for (const g of GROUNDS) {
      const gHex = hexOf(pal, g)
      if (!gHex) continue
      const v = cr(fgHex, gHex)
      const floor = g === 'bg-hover' || g === 'bg-active' ? 3 : 4.5
      if (v < floor) fail(`${name} ${s} ${fgHex} on ${g} ${gHex} = ${v}, floor ${floor}`)
    }
  }
}
const lr = rankOf(light)
console.log('  light rank', lr.replace(/status-/g, ''))
for (const name of THEME_NAMES) {
  const pal = PALETTES[name]
  if (!STATS.every((s) => hexOf(pal, s)) || !hexOf(pal, 'bg-base')) continue
  const dr = rankOf(pal)
  console.log(`  ${name.padEnd(6)} rank`, dr.replace(/status-/g, ''))
  if (lr !== dr) {
    const dump = (p) =>
      STATS.map((s) => {
        const h = hexOf(p, s)
        return `${s.replace('status-', '')} ${h} cr=${cr(h, hexOf(p, 'bg-base'))}`
      }).join(', ')
    fail(
      `status inks changed their relative loudness in translation — light [${dump(light)}] vs ${name} [${dump(pal)}]`,
    )
  }
}

console.log('\n=== extras: text chroma, mat below the page ===')
for (const name of THEME_NAMES) {
  const pal = PALETTES[name]
  const textHex = hexOf(pal, 'text-primary')
  if (textHex) {
    const tC = hex2oklch(textHex)[1]
    console.log(`  ${name} text-primary chroma ${tC.toFixed(3)}`)
    if (tC < 0.01) {
      fail(`${name} text-primary chroma ${tC.toFixed(3)} — grey text is the second slop tell`)
    }
  }
  const shellHex = hexOf(pal, 'shell')
  const pageHex = hexOf(pal, 'bg-base')
  if (shellHex && pageHex) {
    const shellL = hex2oklch(shellHex)[0]
    const pageL = hex2oklch(pageHex)[0]
    console.log(`  ${name} shell L ${(shellL * 100).toFixed(1)} vs page L ${(pageL * 100).toFixed(1)}`)
    if (shellL >= pageL) {
      fail(`${name}: the mat is lighter than the page — it will glow around the layout`)
    }
  }
}

// ── GDK-157 / GDK-159 ────────────────────────────────────────────────
const PAIR_N = 0.05
const PAIR_D = 0.04
const MARK_DE = 0.06
const CHIP_GROUNDS = ['bg-base', 'bg-elevated', 'bg-active']

function checkPairwise(label, pal) {
  console.log(`  ${label}`)
  const hexes = STATS.map((s) => [s, hexOf(pal, s)])
  if (hexes.some(([, h]) => !h)) {
    for (const [s, h] of hexes) {
      if (!h) fail(`${label} --color-${s} is not a hex token`)
    }
    return
  }
  for (let i = 0; i < hexes.length; i++) {
    for (let j = i + 1; j < hexes.length; j++) {
      const [aN, aH] = hexes[i]
      const [bN, bH] = hexes[j]
      const n = dEok(aH, bH)
      const d = dEok(deut(aH), deut(bH))
      const a = aN.replace('status-', '')
      const b = bN.replace('status-', '')
      console.log(`    ${a}/${b}  ΔEok ${n.toFixed(3)}  deut ${d.toFixed(3)}`)
      if (n < PAIR_N) fail(`${label} ${a}/${b} ${aH}/${bH} ΔEok ${n.toFixed(3)} < ${PAIR_N}`)
      if (d < PAIR_D) {
        fail(
          `${label} ${a}/${b} ${deut(aH)}/${deut(bH)} deuteranopia ΔEok ${d.toFixed(3)} < ${PAIR_D}`,
        )
      }
    }
  }
}

console.log('\n=== pairwise status-ink ΔEok (every palette) ===')
checkPairwise('light', light)
for (const name of THEME_NAMES) checkPairwise(name, PALETTES[name])

function checkSearchMatch(label, pal) {
  console.log(`  ${label}`)
  const mark = hexOf(pal, 'search-match')
  if (!mark) {
    fail(`${label} missing --color-search-match`)
    const stale = hexOf(pal, 'status-stale')
    const base = hexOf(pal, 'bg-base')
    const active = hexOf(pal, 'bg-active')
    if (stale && base && active) {
      const washed = alphaOver(stale, base, 0.3)
      console.log(
        `    current stale/30 on base vs bg-active ΔEok ${dEok(washed, active).toFixed(3)} (the collision this token replaces)`,
      )
    }
    return
  }
  const names = [...new Set([...GROUNDS, ...CHIP_GROUNDS])]
  for (const g of names) {
    const gh = hexOf(pal, g)
    if (!gh) {
      fail(`${label} --color-${g} is not a hex token`)
      continue
    }
    const v = dEok(mark, gh)
    console.log(`    vs ${g}: ${v.toFixed(3)}`)
    if (v < MARK_DE) fail(`${label} search-match ${mark} vs ${g} ${gh} ΔEok ${v.toFixed(3)} < ${MARK_DE}`)
  }
}

// Coupled to Marks.svelte: the hit <mark> is `bg-search-match text-text-primary`
// (not text-inherit — muted/secondary cannot clear 4.5:1 on any visible dark
// mark). If that class list changes, this axis is invalid until retargeted.
const MARK_TEXT_FLOOR = 4.5
function checkSearchMatchText(label, pal) {
  console.log(`  ${label}`)
  const mark = hexOf(pal, 'search-match')
  const fg = hexOf(pal, 'text-primary')
  if (!mark) {
    fail(`${label} missing --color-search-match`)
    return
  }
  if (!fg) {
    fail(`${label} missing --color-text-primary`)
    return
  }
  const v = cr(fg, mark)
  process.stdout.write(`    text-primary ${fg} on search-match ${mark}: ${v}`)
  if (v < MARK_TEXT_FLOOR) {
    console.log(`  < ${MARK_TEXT_FLOOR}`)
    fail(`${label} text-primary ${fg} on search-match ${mark} = ${v} < ${MARK_TEXT_FLOOR}`)
  } else console.log()
}

console.log('\n=== search-match isolation (every palette) ===')
checkSearchMatch('light', light)
for (const name of THEME_NAMES) checkSearchMatch(name, PALETTES[name])

console.log('\n=== search-match text contrast (Marks.svelte text-text-primary) ===')
checkSearchMatchText('light', light)
for (const name of THEME_NAMES) checkSearchMatchText(name, PALETTES[name])

function checkRoleFloors(label, pal) {
  console.log(`  ${label}`)
  for (const s of STATS) {
    const fgHex = hexOf(pal, s)
    if (!fgHex) {
      fail(`${label} --color-${s} is not a hex token`)
      continue
    }
    for (const g of GROUNDS) {
      const gHex = hexOf(pal, g)
      if (!gHex) continue
      const v = cr(fgHex, gHex)
      const floor = g === 'bg-hover' || g === 'bg-active' ? 3 : 4.5
      const role = floor === 4.5 ? 'text' : 'dot'
      process.stdout.write(`    ${s.replace('status-', '')} ${role} on ${g}: ${v}`)
      if (v < floor) {
        console.log(`  < ${floor}`)
        fail(`${label} ${s} ${fgHex} ${role} on ${g} ${gHex} = ${v}, floor ${floor}`)
      } else console.log()
    }
  }
}

console.log('\n=== per-role ink contrast floors (every palette) ===')
checkRoleFloors('light', light)
for (const name of THEME_NAMES) checkRoleFloors(name, PALETTES[name])

/*
 * Palettes must be told apart. Two themes in the picker that render the same
 * screen are worse than one — the user pays a choice and gets nothing. The floor
 * is on ground ΔEok, because the ground is what a whole screen is made of.
 */
const PALETTE_DE = 0.015
console.log('\n=== palette separation (a picker entry must change the screen) ===')
for (let i = 0; i < THEME_NAMES.length; i++) {
  for (let j = i + 1; j < THEME_NAMES.length; j++) {
    const [a, b] = [THEME_NAMES[i], THEME_NAMES[j]]
    const worst = GROUNDS.map((g) => {
      const ah = hexOf(PALETTES[a], g)
      const bh = hexOf(PALETTES[b], g)
      return ah && bh ? [g, dEok(ah, bh)] : null
    }).filter(Boolean).sort((x, y) => x[1] - y[1])[0]
    if (!worst) continue
    console.log(`  ${a}/${b}: closest ground ${worst[0]} ΔEok ${worst[1].toFixed(3)} (need ≥ ${PALETTE_DE})`)
    if (worst[1] < PALETTE_DE) {
      fail(`${a} and ${b} are the same theme: ${worst[0]} ΔEok ${worst[1].toFixed(3)} < ${PALETTE_DE}`)
    }
  }
}

/*
 * GDK-129. The four type-scale tokens (micro/body/title/heading) plus the
 * .wordmark owner are the only sizes. An arbitrary text-[Npx] utility is the
 * same class of leak as an unmeasured palette: it ships a fifth size inside
 * the 11–13 band and the screen reads as noise. Walker shape matches
 * DialogShell.test.ts (svelteFiles) — ts included because class strings live
 * in settings/controls.ts and similar.
 */
function walkWebSrc(dir, acc = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walkWebSrc(p, acc)
    else if (name.endsWith('.svelte') || name.endsWith('.ts')) acc.push(p)
  }
  return acc
}

const ARB_TEXT_PX = /text-\[[0-9]+px\]/g
console.log('\n=== no arbitrary text-[Npx] in web/src (.svelte / .ts) ===')
const webSrc = join(ROOT, 'web/src')
const srcFiles = walkWebSrc(webSrc).sort()
let arbHits = 0
for (const file of srcFiles) {
  const rel = file.slice(ROOT.length + 1)
  const lines = readFileSync(file, 'utf8').split('\n')
  for (let i = 0; i < lines.length; i++) {
    ARB_TEXT_PX.lastIndex = 0
    let m
    while ((m = ARB_TEXT_PX.exec(lines[i]))) {
      arbHits++
      fail(`${rel}:${i + 1}: ${m[0]}`)
    }
  }
}
if (arbHits === 0) console.log('  no text-[Npx] utilities: ok')

console.log(`\n=== ${fails === 0 ? 'ALL CHECKS PASS' : fails + ' FAILURES'} ===`)
process.exit(fails === 0 ? 0 : 1)
