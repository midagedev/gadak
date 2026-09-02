/*
 * Shared color math + app.css palette parsing for the theme tools.
 *
 * Extracted verbatim from tools/theme-check.mjs (GDK-787) so
 * tools/token-catalog.mjs consumes the SAME parser instead of growing a
 * second one — a divergent copy here would let the catalog and the build
 * gate disagree about what a palette even is. theme-check's assertions are
 * unchanged; they now import from this module.
 *
 * Everything in here is a pure function of its arguments (no fs, no argv):
 * that is what makes the Go port in internal/config/tokencheck testable
 * against golden vectors emitted from these exact formulas.
 */

export const hex2rgb = (h) => {
  const s = h.replace('#', '')
  const full = s.length === 3 ? [...s].map((c) => c + c).join('') : s
  return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16) / 255)
}
export const lin = (c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
export const luminance = (hex) => {
  const [r, g, b] = hex2rgb(hex).map(lin)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
export const contrast = (a, b) => {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (l1 + 0.05) / (l2 + 0.05)
}

export const hex2oklab = (hex) => {
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
export const dEok = (a, b) => {
  const A = hex2oklab(a)
  const B = hex2oklab(b)
  return Math.hypot(A[0] - B[0], A[1] - B[1], A[2] - B[2])
}
export const rgb2hex = (rgb) =>
  '#' +
  rgb
    .map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255).toString(16).padStart(2, '0'))
    .join('')
// Machado 2009 deuteranopia, severity 1.0. Same matrix as the GDK-157
// design scripts (analyse.mjs / verify.mjs). Do not substitute another.
export const DEUT_M = [
  [0.367322, 0.860646, -0.227968],
  [0.280085, 0.672501, 0.047413],
  [-0.01182, 0.04294, 0.968881],
]
export const deut = (hex) => {
  const [r, g, b] = hex2rgb(hex)
  return rgb2hex(DEUT_M.map((row) => row[0] * r + row[1] * g + row[2] * b))
}
export const alphaOver = (fg, bg, a) => {
  const f = hex2rgb(fg)
  const b = hex2rgb(bg)
  return rgb2hex([0, 1, 2].map((i) => f[i] * a + b[i] * (1 - a)))
}
export const hex2oklch = (hex) => {
  const [L, a, b] = hex2oklab(hex)
  const C = Math.hypot(a, b)
  let H = (Math.atan2(b, a) * 180) / Math.PI
  if (H < 0) H += 360
  return [L, C, H]
}
// HSL, because the GDK-190 ground contract is written in HSL hue/saturation:
// at ground lightness oklch chroma is a three-decimal number nobody can aim at,
// while "hue 220, S 10%" is a value a designer can read off the token.
export const hex2hsl = (hex) => {
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

export function extractBraceBlock(source, openBraceIndex) {
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

export function parseColorDecls(block) {
  const out = {}
  if (!block) return out
  const re = /--color-([a-z0-9-]+)\s*:\s*([^;]+);/gi
  let m
  while ((m = re.exec(block))) {
    out[m[1]] = m[2].trim()
  }
  return out
}

export function findAtTheme(source) {
  const idx = source.search(/@theme\b/)
  if (idx < 0) return null
  const brace = source.indexOf('{', idx)
  return brace < 0 ? null : extractBraceBlock(source, brace)
}

export function findSelectorBlocks(source, selectorRe) {
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

export function richestColorDecls(blocks) {
  let best = {}
  for (const block of blocks) {
    const decls = parseColorDecls(block)
    if (Object.keys(decls).length > Object.keys(best).length) best = decls
  }
  return best
}

export const hexOf = (pal, name) => {
  const v = pal[name]
  if (!v) return null
  const m = v.match(/#([0-9a-fA-F]{3,8})\b/)
  if (m) return m[0].length === 4 ? null : m[0].toLowerCase()
  // An ink token — `rgb(r g b / a)`, a wash meant to sit over any ground
  // (GDK-1341 made bg-hover one). The gate measures it the way the page
  // shows it: composited over this palette's bg-base.
  const ink = v.match(/rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)\s*(?:[/,]\s*([\d.]+%?))?\s*\)/)
  if (!ink) return null
  const base = pal['bg-base']?.match(/#[0-9a-fA-F]{6}\b/)?.[0]
  if (!base) return null
  let a = ink[4] === undefined ? 1 : parseFloat(ink[4])
  if (ink[4]?.endsWith('%')) a /= 100
  const fg = rgb2hex([+ink[1] / 255, +ink[2] / 255, +ink[3] / 255]) // rgb2hex takes 0..1
  return alphaOver(fg, base.toLowerCase(), a)
}

/*
 * Every palette declares itself with one :root[data-theme='NAME'] block, so the
 * gate discovers them instead of being told. A palette added to app.css and
 * THEMES but forgotten here would ship unmeasured — that is the failure this
 * enumeration closes.
 */
export function discoverThemeNames(source) {
  const names = new Set()
  const re = /:root\[data-theme=['"]([a-z0-9-]+)['"]\]/gi
  let m
  while ((m = re.exec(source))) {
    if (m[1] !== 'light') names.add(m[1])
  }
  return [...names].sort()
}

/*
 * The one entry point tools use: parse app.css source into the light (@theme)
 * declarations, the discovered data-theme palettes, and the richest
 * prefers-color-scheme: dark inner block. Order of themeNames is sorted, so
 * downstream iteration is deterministic.
 */
export function parseAppCss(source) {
  const light = parseColorDecls(findAtTheme(source))
  const themeNames = discoverThemeNames(source)
  const paletteOf = (name) =>
    richestColorDecls(
      findSelectorBlocks(source, new RegExp(`:root\\[data-theme=['"]${name}['"]\\]`)),
    )
  const palettes = Object.fromEntries(themeNames.map((n) => [n, paletteOf(n)]))
  const darkExplicit = palettes.dark ?? {}
  const darkMediaInner = (() => {
    // Skip the unlayered color-scheme-only media query; take the richest
    // token block under any prefers-color-scheme: dark section.
    const mediaRe = /@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)/g
    const inner = []
    let media
    while ((media = mediaRe.exec(source))) {
      const mediaBrace = source.indexOf('{', media.index)
      const mediaBody = extractBraceBlock(source, mediaBrace)
      if (!mediaBody) continue
      inner.push(
        ...findSelectorBlocks(mediaBody, /:root:not\(\s*\[data-theme=['"]light['"]\]\s*\)/),
      )
    }
    return richestColorDecls(inner)
  })()
  return { light, themeNames, palettes, darkExplicit, darkMediaInner }
}
