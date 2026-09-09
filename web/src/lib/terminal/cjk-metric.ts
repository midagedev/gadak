/*
 * The CJK cell fit (GDK-1597) — one owner for three surfaces.
 *
 * It lives in its own module rather than beside the wire protocol: what it
 * knows about is font metrics, and web/src/lib/terminal/renderer.ts,
 * mobile/src/lib/terminal/renderer.ts and the desktop bundle all reach it
 * here. The phone imports across the repo boundary the same way it already
 * imports the protocol.
 *
 * xterm lays a CJK codepoint across two cells and derives the cell from the
 * *Latin* face's advance, then pads the difference with per-span
 * letterSpacing. It does not scale the glyph. So when the face that draws
 * 한글 is narrower than two cells, every syllable gets a visible void after
 * it and a sentence reads as if it were spaced out — measured on
 * docs/media/terminal-hero-poster.ko.png (2026-09-08) and reproduced from
 * the DOM: Menlo at 13px gives cell.width 7.8333, two cells 15.667, while
 * Apple SD Gothic Neo's 가 advances 0.865em = 11.245 — so xterm stamps
 * `letterSpacing: 4.41667px` on the Hangul spans (Hiragino's 1.0em kana get
 * 2.66667px). The Latin on the same row is untouched, which is why every
 * gate stayed green: nothing in the repo measured the gap.
 *
 * No installed face closes it by selection. The cell wants a CJK advance of
 * exactly 2 x the Latin advance — 1.2042em against Menlo — and no CJK face
 * ships one; the 1:2 families that do (Sarasa Mono, D2Coding, Noto Sans
 * Mono CJK) are 0.5em-Latin designs that would have to lead the stack and
 * are not installed on any of the three platforms (measured 2026-09-09,
 * tools/cjk-cell-fit.mjs --advances). Only macOS has a 1:2 face at all
 * (Osaka-Mono, M 0.5 / あ 1.0) and it carries no Hangul.
 *
 * So the advance is adjusted instead of chosen: a @font-face whose src is
 * the same system faces the stack already names, carrying a `size-adjust`
 * that scales the glyph and its advance to exactly two cells, and a
 * `unicode-range` that keeps it away from Latin and from the box-drawing
 * block (U+2500-257F) so GDK-1043's joint survives untouched.
 *
 * The percentage is measured at runtime, not written down, because it is a
 * ratio between the resolved Latin lead and the resolved CJK face and both
 * differ per platform: Menlo 0.60205em on macOS and iOS, but Consolas
 * 0.5498em where ui-monospace resolves to it. A constant tuned to Menlo
 * would turn a 4.4px gap into a 1.4px overlap on that platform. Measuring
 * closes the class instead of one instance of it.
 *
 * One owner for three surfaces: this module is what the phone imports
 * directly (web/src/lib/terminal/renderer.ts and
 * mobile/src/lib/terminal/renderer.ts both call installCjkMetricFaces), and
 * the desktop app runs the web bundle.
 */

/** The scripts that get an advance-corrected face, and the faces each may
 *  be drawn by. Every entry is `[label, ...local() names]`.
 *
 *  Two families, not one: the Han block is shared between the two scripts
 *  and has to follow `lang` (GDK-1532), so each script keeps its own
 *  candidate list and the caller orders the pair by document language.
 *  Korean faces only in `hangul`, Japanese only in `kana` — a pan-Unicode
 *  face like Arial Unicode MS measures well and would destroy that
 *  distinction.
 *
 *  The list is not a priority order. The face is picked by measurement:
 *  whichever installed candidate needs the *least* correction wins (see
 *  pickCjkMetricFace). Ordering it by preference would be the wrong
 *  instinct — the nicest face here is the worst fit.
 *
 *  Faces are named twice, PostScript name first. Chromium's `local()`
 *  matches only the PostScript/full name — `local('Apple SD Gothic Neo')`
 *  fails there and `local('AppleSDGothicNeo-Regular')` resolves — while
 *  WebKit matches either (measured 2026-09-09 on both engines). Nothing new
 *  is asked of the machine and nothing is downloaded. */
export const CJK_METRIC_SCRIPTS = {
  hangul: {
    family: 'Gadak Terminal Hangul',
    probe: '가',
    candidates: [
      ['AppleGothic', 'AppleGothic'],
      ['Malgun Gothic', 'MalgunGothic', 'Malgun Gothic'],
      ['Noto Sans CJK KR', 'NotoSansCJKkr-Regular', 'Noto Sans CJK KR'],
      ['Noto Sans KR', 'NotoSansKR-Regular', 'Noto Sans KR'],
      ['NanumGothic', 'NanumGothic'],
      ['Apple SD Gothic Neo', 'AppleSDGothicNeo-Regular', 'Apple SD Gothic Neo'],
    ],
  },
  kana: {
    family: 'Gadak Terminal Kana',
    probe: 'あ',
    candidates: [
      ['Hiragino Sans', 'HiraginoSans-W3', 'Hiragino Sans'],
      ['Hiragino Kaku Gothic ProN', 'HiraKakuProN-W3', 'Hiragino Kaku Gothic ProN'],
      ['Yu Gothic', 'YuGothic-Medium', 'Yu Gothic'],
      ['Noto Sans CJK JP', 'NotoSansCJKjp-Regular', 'Noto Sans CJK JP'],
      ['Meiryo', 'Meiryo'],
    ],
  },
} as const

export type CjkMetricScript = keyof typeof CJK_METRIC_SCRIPTS

/*
 * The codepoints each adjusted face is allowed to draw.
 *
 * Deliberately excludes U+2500-257F (box drawing) and U+2580-259F (blocks):
 * those are what GDK-1043 tuned the Latin lead for, they are single-cell,
 * and a scaled face reaching them would break the grid the same way a
 * sans-class fallback did (measured there: vgaps=150).
 *
 * Split by script rather than shared, and that split is load-bearing. With
 * one range covering everything, the family that came first drew *both*
 * scripts — so on a Korean page the kana were drawn by the Korean face,
 * whose kana are narrow inside their em box, and the corrected kana row
 * measured worse than the uncorrected one (13.55% against 4.64%, measured
 * 2026-09-09). Each script now owns its own blocks; only the Han block and
 * the shared punctuation are contested, and those are the ones that must
 * follow `lang` (GDK-1532), which the family order decides.
 */
const CJK_RANGE_SHARED = [
  'U+2E80-2EFF', // CJK Radicals Supplement
  'U+3000-303F', // CJK Symbols and Punctuation
  'U+3200-32FF', // Enclosed CJK Letters and Months
  'U+3400-4DBF', // CJK Extension A
  'U+4E00-9FFF', // CJK Unified Ideographs
  'U+F900-FAFF', // CJK Compatibility Ideographs
  'U+FE30-FE4F', // CJK Compatibility Forms
  'U+FF00-FF60', // Fullwidth Forms
  'U+FFE0-FFE6', // Fullwidth Signs
]
const CJK_RANGE_BY_SCRIPT = {
  hangul: [
    'U+1100-11FF', // Hangul Jamo
    'U+3130-318F', // Hangul Compatibility Jamo
    'U+A960-A97F', // Hangul Jamo Extended-A
    'U+AC00-D7AF', // Hangul Syllables
    'U+D7B0-D7FF', // Hangul Jamo Extended-B
  ],
  kana: [
    'U+3040-30FF', // Hiragana, Katakana
    'U+31F0-31FF', // Katakana Phonetic Extensions
  ],
} as const

/** The `unicode-range` for one script's adjusted face. */
export function cjkMetricUnicodeRange(script: CjkMetricScript): string {
  return [...CJK_RANGE_BY_SCRIPT[script], ...CJK_RANGE_SHARED].join(', ')
}

/** Every codepoint any adjusted face may draw — what the perceptual gate
 *  measures against, and what must never include the box-drawing block. */
export const CJK_METRIC_UNICODE_RANGE = [
  ...CJK_RANGE_BY_SCRIPT.hangul,
  ...CJK_RANGE_BY_SCRIPT.kana,
  ...CJK_RANGE_SHARED,
].join(', ')

/** A `size-adjust` percentage that lands `glyph` on exactly `cells` cells,
 *  or null when there is nothing to correct or nothing to measure.
 *
 *  null on a non-finite or non-positive input (a canvas that measured
 *  nothing), and null when the face is already within `tolerance` of the
 *  target — the 1:2 face case, where declaring an adjusted face would only
 *  add a rounding error. Rounded to three decimals: at 13px, one unit in
 *  the third decimal is 0.00013px. */
export function cjkMetricAdjust(
  latinAdvance: number,
  glyphAdvance: number,
  cells = 2,
  tolerance = 0.002,
): number | null {
  if (!Number.isFinite(latinAdvance) || !Number.isFinite(glyphAdvance)) return null
  if (latinAdvance <= 0 || glyphAdvance <= 0) return null
  const factor = (latinAdvance * cells) / glyphAdvance
  if (Math.abs(factor - 1) <= tolerance) return null
  return Math.round(factor * 100 * 1000) / 1000
}

/*
 * How much scaling is too much (measured 2026-09-09, GDK-1597).
 *
 * `size-adjust` scales the glyph as well as its advance, and a CJK face
 * whose ink nearly fills its own advance has no side bearing to give. Four
 * faces rendered through the real pane at the factor each one needs, read
 * off the shot:
 *
 *   AppleGothic          1.0000em  120.410%  clean, evenly spaced
 *   Arial Unicode MS     1.0000em  120.410%  clean (rejected: pan-Unicode)
 *   NanumGothic          0.9400em  128.096%  clean, slightly tight
 *   Apple SD Gothic Neo  0.8650em  139.202%  strokes collide — 라벨별 reads 라멜멸
 *
 * So the fit is not free above roughly 130%: the syllables join and then
 * keep going until they overlap, which is a worse defect than the gap. The
 * cap sits between the highest good measurement and the failing one, and a
 * script with no candidate under it gets no face at all — a visible gap is
 * a legibility problem, overlapping strokes are a correctness one.
 */
export const CJK_METRIC_MAX_ADJUST = 132

/**
 * The installed candidate that needs the least correction, or null when
 * none is installed or none is within CJK_METRIC_MAX_ADJUST.
 *
 * "Installed" is the only test a canvas offers: asked for a family it does
 * not have, it answers in the default face without complaining, so a
 * candidate counts as present only when its advance differs from a
 * deliberately absent family's.
 */
export function pickCjkMetricFace(
  candidates: readonly (readonly string[])[],
  latinAdvance: number,
  probe: string,
  measure: AdvanceMeasure,
  maxAdjust: number = CJK_METRIC_MAX_ADJUST,
): { label: string; sources: string[]; advance: number; adjustPct: number } | null {
  const absent = measure(ABSENT_FAMILY, probe)
  let best: { label: string; sources: string[]; advance: number; adjustPct: number } | null = null
  for (const [label, ...names] of candidates) {
    const advance = measure(names.map((n) => `'${n}'`).join(', '), probe)
    if (!Number.isFinite(advance) || advance <= 0 || advance === absent) continue
    const adjustPct = cjkMetricAdjust(latinAdvance, advance)
    if (adjustPct === null || adjustPct > maxAdjust) continue
    // Least correction = largest natural advance, since the target is fixed.
    if (!best || advance > best.advance) best = { label, sources: [...names], advance, adjustPct }
  }
  return best
}

/*
 * The four styles one corrected family has to answer for (GDK-1743).
 *
 * A `@font-face` that declares no `font-weight`/`font-style` is a regular
 * face, and on WebKit a bold request does not resolve to it — the run falls
 * through to the raw system face, draws at that face's own advance, and
 * xterm pulls the difference back with letterSpacing. Measured on WebKit
 * 2026-09-10, which is the engine the desktop app and the phone run:
 * -0.427px per syllable on every bold and bold-italic Hangul run, against
 * 0.0104px at regular weight. Chromium resolves the same request to the
 * corrected face and reads 0.0104px in all four styles, which is why the
 * defect had a platform and no gate — every measurement this module was
 * ever built on came from a regular-weight Chromium row.
 *
 * So the same face is declared four times, once per style, and the
 * descriptors are the whole fix: they are what makes a bold request land on
 * the corrected face instead of past it.
 *
 * The factor stays one factor. Correcting each style by its own
 * canvas-measured advance was tried first and made WebKit worse — a
 * synthesised bold measures ~2.7% wider on a canvas than it renders through
 * `@font-face`, so the per-style factor overshot and turned -0.427px into
 * +0.448px (measured the same day). The canvas is a faithful proxy for the
 * face's own advance and not for the synthesis on top of it.
 */
export const CJK_METRIC_STYLES = [
  { weight: '400', style: 'normal' },
  { weight: '700', style: 'normal' },
  { weight: '400', style: 'italic' },
  { weight: '700', style: 'italic' },
] as const

/** One @font-face rule, as text. `sources` are quoted for `local()`. */
export function cjkMetricFaceCss(
  family: string,
  sources: readonly string[],
  adjustPct: number,
  unicodeRange: string,
  descriptors?: { weight: string; style: string },
): string {
  const src = sources.map((s) => `local('${s}')`).join(', ')
  // Without these the rule answers regular requests only, and a bold cell
  // resolves past the correction to the raw face (GDK-1743).
  const d = descriptors ? `font-weight:${descriptors.weight};font-style:${descriptors.style};` : ''
  return (
    `@font-face{font-family:'${family}';src:${src};${d}` +
    `unicode-range:${unicodeRange};size-adjust:${adjustPct}%;}`
  )
}

/** The whole family: the same corrected face declared for all four styles. */
export function cjkMetricFamilyCss(
  family: string,
  sources: readonly string[],
  adjustPct: number,
  unicodeRange: string,
): string[] {
  return CJK_METRIC_STYLES.map((d) =>
    cjkMetricFaceCss(family, sources, adjustPct, unicodeRange, d),
  )
}

/** The scripts in the order the document's language wants them tried.
 *  Japanese first under `lang="ja*"`, Korean first otherwise — the same
 *  precedence app.css's locale override already gives the raw faces, so the
 *  shared Han block keeps drawing in the reader's own shapes. */
export function cjkMetricOrder(lang: string | null | undefined): CjkMetricScript[] {
  return /^ja\b/i.test(lang ?? '') ? ['kana', 'hangul'] : ['hangul', 'kana']
}

/** Put `families` at the front of a font stack, skipping any already there. */
export function withCjkMetricFamilies(stack: string, families: readonly string[]): string {
  const add = families.filter((f) => !stack.includes(f))
  if (!add.length) return stack
  return add.map((f) => `'${f}'`).join(', ') + ', ' + stack
}

/** How a caller measures an advance: the width of `text` in `family`, in em. */
export type AdvanceMeasure = (family: string, text: string) => number

/** A family name no machine has, for telling "absent" from "measured".
 *  A canvas asked for a missing family silently answers in the default
 *  face, so the only way to know a face is present is that its advance
 *  differs from this one's. */
const ABSENT_FAMILY = "'__gadak_absent_family__'"

/** A canvas-backed AdvanceMeasure, or null where there is no canvas (node,
 *  jsdom). Measures at 100px and divides, so the result is a ratio and the
 *  terminal's own font size never enters. */
export function canvasAdvanceMeasure(doc: Document): AdvanceMeasure | null {
  const ctx = doc.createElement('canvas')?.getContext?.('2d')
  if (!ctx) return null
  return (family, text) => {
    ctx.font = `100px ${family}`
    return ctx.measureText(text).width / 100
  }
}

/**
 * Declare the advance-corrected CJK faces for `stack` and return the stack
 * that uses them. The stack comes back unchanged — never broken — when
 * there is no canvas to measure with, no CJK face installed, or nothing to
 * correct.
 *
 * `styleId` keeps one <style> element per document, rewritten in place, so
 * a font-size change or a second pane re-measures instead of stacking
 * rules.
 */
export function installCjkMetricFaces(opts: {
  stack: string
  doc?: Document | null
  lang?: string | null
  measure?: AdvanceMeasure | null
  styleId?: string
  /** Override CJK_METRIC_MAX_ADJUST — for the tools that measure it. */
  maxAdjust?: number
}): string {
  const doc = opts.doc ?? (typeof document === 'undefined' ? null : document)
  if (!doc) return opts.stack
  const measure = opts.measure ?? canvasAdvanceMeasure(doc)
  if (!measure) return opts.stack

  // The cell: what the resolved Latin lead of this very stack advances 'W'
  // by. Reading it off the stack rather than off a named face is what makes
  // this correct on a platform whose lead is not Menlo, and what keeps a
  // user's own terminalFontFamily honoured.
  const latin = measure(opts.stack, 'W')
  if (!Number.isFinite(latin) || latin <= 0) return opts.stack

  const rules: string[] = []
  const families: string[] = []
  const chosen: Record<string, string> = {}
  for (const script of cjkMetricOrder(opts.lang ?? doc.documentElement?.lang)) {
    const s = CJK_METRIC_SCRIPTS[script]
    const face = pickCjkMetricFace(s.candidates, latin, s.probe, measure, opts.maxAdjust)
    if (!face) continue
    rules.push(...cjkMetricFamilyCss(s.family, face.sources, face.adjustPct, cjkMetricUnicodeRange(script)))
    families.push(s.family)
    chosen[script] = `${face.label} @ ${face.adjustPct}%`
  }
  if (!rules.length) return opts.stack

  const id = opts.styleId ?? 'gadak-cjk-metric-faces'
  let el = doc.getElementById(id) as HTMLStyleElement | null
  if (!el) {
    el = doc.createElement('style')
    el.id = id
    doc.head.appendChild(el)
  }
  el.textContent = rules.join('\n')
  // The choice, where a debugger can find it without re-deriving it:
  // `document.getElementById('gadak-cjk-metric-faces').dataset`.
  if (el.dataset) {
    for (const [k, v] of Object.entries(chosen)) el.dataset[k] = v
    el.dataset.latinAdvance = String(latin)
  }
  return withCjkMetricFamilies(opts.stack, families)
}
