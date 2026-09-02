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
 *        node tools/theme-check.mjs --emit-vectors
 *          All assertions run either way; --emit-vectors additionally writes
 *          the golden-vector JSON consumed by internal/config/tokencheck
 *          (GDK-785). Deterministic output: same app.css in, same bytes out.
 */
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  alphaOver,
  contrast,
  dEok,
  deut,
  hex2hsl,
  hex2oklab,
  hex2oklch,
  hexOf,
  parseAppCss,
} from './lib/theme-parse.mjs'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const CSS_PATH = join(ROOT, 'web/src/app.css')
const css = readFileSync(CSS_PATH, 'utf8')

let fails = 0
const fail = (m) => {
  fails++
  console.log('  FAIL  ' + m)
}

const cr = (a, b) => Math.round(contrast(a, b) * 100) / 100

// Pure color math and the app.css palette parser live in ./lib/theme-parse.mjs
// since GDK-787 (shared with tools/token-catalog.mjs so the catalog and this
// gate can never disagree about what a palette is). Everything below is the
// assertion body and is unchanged by that extraction.

const { light, themeNames: THEME_NAMES, palettes: PALETTES, darkExplicit, darkMediaInner } =
  parseAppCss(css)

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
  // An ink token (`rgb(r g b / a)`, GDK-1341's bg-hover) is not a rung: it is
  // a wash that lands one step above whatever ground it sits on, so its place
  // in the ladder depends on the ground. The rule for it is that the step is
  // visible over the page.
  const isInk = (g) => /rgba?\(/.test(pal[g] ?? '') && !/#/.test(pal[g] ?? '')
  for (const g of LADDER.filter(isInk)) {
    const base = hexOf(pal, 'bg-base')
    const over = hexOf(pal, g)
    if (!base || !over) continue
    const step = Math.abs(hex2oklch(over)[0] - hex2oklch(base)[0]) * 100
    console.log(`    ${g.padEnd(12)} ink ${pal[g]} → over bg-base ${over}  ΔokL ${step.toFixed(1)}`)
    if (step < 1.2) fail(`${name} ${g} ink is not a visible step over bg-base (ΔokL ${step.toFixed(1)} < 1.2)`)
  }
  const ls = LADDER.filter((g) => !isInk(g))
    .map((g) => [g, hexOf(pal, g)])
    .filter(([, h]) => h)
    .map(([g, h]) => [g, hex2oklch(h)[0]])
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

// ── GDK-785: golden vectors ───────────────────────────────────────────
/*
 * --emit-vectors writes the measured color math this gate runs on — same
 * formulas, same shipped palette values — as the JSON the Go port
 * (internal/config/tokencheck) must reproduce to 4 decimal places. Two
 * implementations, one definition: the vectors are generated here because
 * this file is the original instrument. Every status-ink pair and every
 * (ink, ground) pair of every palette is included, so "the shipped palettes
 * pass the shipped floors" is itself recorded as data, not asserted.
 *
 * All numbers are rounded to 6 decimals and there is no timestamp: identical
 * app.css in, identical bytes out.
 */
const VECTORS_PATH = join(ROOT, 'internal/config/tokencheck/testdata/token-vectors.json')
const r6 = (x) => Math.round(x * 1e6) / 1e6

function emitGoldenVectors() {
  const contrastPairs = []
  const deltaPairs = []
  const oklab = []
  const deuts = []
  const pushContrast = (fg, bg) => contrastPairs.push({ fg, bg, contrast: r6(contrast(fg, bg)) })
  const pushDelta = (a, b) =>
    deltaPairs.push({
      a,
      b,
      deltaEok: r6(dEok(a, b)),
      deltaEokDeuteranopia: r6(dEok(deut(a), deut(b))),
    })
  const vectorPalettes = [{ name: 'light', pal: light }].concat(
    THEME_NAMES.map((n) => ({ name: n, pal: PALETTES[n] })),
  )
  for (const { pal } of vectorPalettes) {
    // Exactly the pairs the runtime validator can measure (GDK-785 rules),
    // plus the text tiers for range coverage.
    for (const s of STATS) {
      for (const g of GROUNDS) pushContrast(hexOf(pal, s), hexOf(pal, g))
    }
    const at = hexOf(pal, 'accent-text')
    pushContrast(at, hexOf(pal, 'accent-subtle'))
    for (const g of GROUNDS) pushContrast(at, hexOf(pal, g))
    for (const fg of Object.keys(FLOOR)) {
      for (const g of GROUNDS) pushContrast(hexOf(pal, fg), hexOf(pal, g))
    }
    for (let i = 0; i < STATS.length; i++) {
      for (let j = i + 1; j < STATS.length; j++) {
        pushDelta(hexOf(pal, STATS[i]), hexOf(pal, STATS[j]))
      }
    }
    for (const t of ['bg-base', 'text-primary', 'accent-text', 'status-new', 'status-stale', 'search-match']) {
      const hex = hexOf(pal, t)
      const [L, A, B] = hex2oklab(hex)
      oklab.push({ hex, L: r6(L), a: r6(A), b: r6(B) })
      deuts.push({ hex, deuteranopia: deut(hex) })
    }
  }
  // Boundary witnesses: values sitting at or near the floors the runtime
  // validator enforces (4.5:1, ΔEok 0.05/0.04), so a formula drift that
  // matters at the boundary cannot hide in the middle of the range.
  const SYNTH_CONTRAST = [
    ['#000000', '#ffffff'], // 21 — the ceiling
    ['#767676', '#ffffff'], // ≈4.54 — the classic AA boundary
    ['#777777', '#ffffff'], // just under 4.5
    ['#1c1812', '#1c1812'], // 1 — fg === bg
    ['#ffffff', '#767676'], // order must not matter (luminance is sorted)
    ['#734701', '#f4efe4'], // the historical stale ink on the light page ground
    ['#ffc579', '#cfc0a4'], // search-match on the most saturated light ground
  ]
  for (const [fg, bg] of SYNTH_CONTRAST) pushContrast(fg, bg)
  const SYNTH_DELTA = [
    ['#1c4c31', '#1c4c31'], // 0 — identical colors
    ['#734701', '#7e5904'], // the P3 near-collision the stale token comment records
    ['#7e5904', '#7f5a04'], // one 8-bit step above it
    ['#8f3530', '#1c4c31'], // red vs green — the deuteranopia collapse axis
    ['#000000', '#ffffff'],
    ['#d19b5a', '#b28a44'], // dark-family stale vs inprogress
  ]
  for (const [a, b] of SYNTH_DELTA) pushDelta(a, b)
  for (const hex of ['#000000', '#ffffff', '#777777', '#7e5904', '#8f3530', '#1c4c31']) {
    const [L, A, B] = hex2oklab(hex)
    oklab.push({ hex, L: r6(L), a: r6(A), b: r6(B) })
    deuts.push({ hex, deuteranopia: deut(hex) })
  }
  const doc = {
    meta: {
      generator: 'tools/theme-check.mjs --emit-vectors',
      source: 'web/src/app.css',
      rounding: 6,
      note: 'Golden vectors pinning the color math shared with internal/config/tokencheck (GDK-785). Compare to 4 decimals.',
      counts: {
        contrast: contrastPairs.length,
        deltaEok: deltaPairs.length,
        oklab: oklab.length,
        deut: deuts.length,
      },
    },
    contrast: contrastPairs,
    deltaEok: deltaPairs,
    oklab,
    deut: deuts,
  }
  writeFileSync(VECTORS_PATH, JSON.stringify(doc, null, 2) + '\n')
  console.log(
    `  wrote ${VECTORS_PATH} (${contrastPairs.length} contrast, ${deltaPairs.length} ΔEok, ${oklab.length} oklab, ${deuts.length} deuteranopia vectors)`,
  )
}

if (process.argv.includes('--emit-vectors')) emitGoldenVectors()

console.log(`\n=== ${fails === 0 ? 'ALL CHECKS PASS' : fails + ' FAILURES'} ===`)
process.exit(fails === 0 ? 0 : 1)
