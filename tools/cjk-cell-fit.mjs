#!/usr/bin/env node
/*
 * cjk-cell-fit — does a CJK glyph fill the two cells xterm gave it? (GDK-1597)
 *
 * The defect this measures is perceptual, and the axis that missed it was
 * the absence of a perceptual axis: `web/src/lib/terminal/font-stack.test.ts`
 * pins the font stack as *text*, and every other gate was green while the
 * Korean in docs/media/terminal-hero-poster.ko.png read as
 * "라 벨 별  비 율 ... 만 들 겠 습 니 다" — a void after every syllable.
 * A gate that measures the name of a font can never see that.
 *
 * So this measures pixels. xterm lays a CJK codepoint across two cells,
 * derives the cell from the *Latin* advance, and pads the difference with
 * per-span letterSpacing rather than scaling the glyph. The visible
 * signature is therefore very specific: inside a run of CJK with no spaces,
 * voids appear at a fixed width that is a large fraction of a cell — wider
 * than the hairline breaks inside a syllable, narrower than a space.
 *
 * The metric is that band. For one text line:
 *
 *   midVoidRatio = (columns belonging to a void run whose width is in
 *                   [0.4 cell, 0.8 cell]) / (columns in the line's extent)
 *
 * Latin sits near zero because Menlo's inter-letter gaps are 1-3px (below
 * 0.4 cell) and its spaces are 9-14px (above 0.8 cell) — the band is empty
 * unless something is padding cells.
 *
 * Threshold derivation (2026-09-09, GDK-1597, every value from a run
 * recorded in this round; --png numbers off
 * docs/media/terminal-hero-poster.ko.png crop 40,670,900,230 at cell 8px,
 * --render numbers off both engines at deviceScaleFactor 2):
 *
 *   padded   13.13 15.34 18.45 19.75 19.93 22.70 23.96 27.95   min 13.13
 *   filled    1.26  1.26  1.26  6.31  6.33  7.33                max  7.33
 *   Latin     0.00  0.00  0.00  0.00  1.88                      max  1.88
 *
 * The filled set is not zero because a glyph's own side bearings live in
 * the band: a corrected Hangul row measures 1.26% and a corrected kana row
 * 6.3-7.3%, because Hiragino's kana genuinely do not fill their em box. A
 * first cut of this tool put the bar at 6.0% / +5.0 and failed that kana
 * row on WebKit — the bar was a target, not a contract. Re-derived to sit
 * inside the real gap:
 *
 *   FAIL when a CJK line's midVoidRatio exceeds MAX_ABS (9.0%)
 *        or exceeds the Latin reference by more than MAX_OVER_LATIN (8.0)
 *
 * 9.0 is 1.7 points above the highest correct measurement and 4.1 below the
 * lowest defective one. Widening it further would start admitting the
 * defect; narrowing it fails a font's own design. The narrow side of that
 * margin is kana side bearings, so a face change on the Japanese side is
 * the thing most likely to move this bar — re-derive from a fresh table
 * rather than nudging the constant.
 *
 * In --render mode there is a second, exact assertion that needs no
 * threshold at all: xterm's per-span `letterSpacing` *is* the padding, and
 * it is readable from the DOM. 4.405px on the Hangul spans before this
 * round, -0.001px after. Pixels are what a capture gives; letterSpacing is
 * what the live pane gives, and the live pane is asserted on the exact
 * number.
 *
 * Every measurement is printed, not just the verdict — "what is the CJK
 * spacing in this capture" is meant to be one command.
 *
 * macOS note: like tools/glyph-joint-check, the verdicts are about font
 * resolution on this platform. A Linux run can explore; it must not gate.
 *
 * Usage
 *   node tools/cjk-cell-fit.mjs                      render the pane, judge
 *   node tools/cjk-cell-fit.mjs --no-fix             ... without GDK-1597
 *   node tools/cjk-cell-fit.mjs --engine=webkit      the desktop/phone engine
 *   node tools/cjk-cell-fit.mjs --advances           candidate-face table
 *   node tools/cjk-cell-fit.mjs --png docs/media/terminal-hero-poster.ko.png \
 *        --crop 40,670,900,230 --cell 8
 *
 * Shots and JSON land in tools/cjk-cell-fit.out/ (untracked, delete freely)
 * unless --out says otherwise. Nothing outside that directory is written and
 * nothing is downloaded: the faces are the system's own.
 */
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const ROOT = path.resolve(__dirname, '..')
const require_ = createRequire(import.meta.url)
const { chromium, webkit } = require_(path.join(ROOT, 'node_modules', 'playwright'))
const esbuild = require_(path.join(ROOT, 'node_modules', 'esbuild'))

// ── Thresholds ─────────────────────────────────────────────────────────────
// Derived 2026-09-09 (GDK-1597) from the three populations tabulated in the
// header: padded 13.13-27.95, correctly filled 1.26-7.33, Latin 0.00-1.88.
// 9.0 / +8.0 sits inside the 7.33-13.13 gap. The header records why the
// first cut at 6.0 / +5.0 was wrong.
const MAX_ABS = Number(process.env.CJK_MAX_ABS ?? 9.0)
const MAX_OVER_LATIN = Number(process.env.CJK_MAX_OVER_LATIN ?? 8.0)
// The exact contract, --render only: xterm's per-span letterSpacing is the
// padding itself. Zero within a rounding hair; 0.05px is a tenth of the
// smallest padding this defect has ever produced.
const MAX_SPAN_PAD = Number(process.env.CJK_MAX_SPAN_PAD ?? 0.05)
/** Every codepoint xterm lays across two cells — the whole padded surface.
 *  Kept in step with CJK_METRIC_UNICODE_RANGE in
 *  web/src/lib/terminal/cjk-metric.ts. */
const WIDE_RE =
  /[\u1100-\u11FF\u2E80-\u2EFF\u3000-\u303F\u3040-\u30FF\u3130-\u318F\u31F0-\u31FF\u3200-\u32FF\u3400-\u4DBF\u4E00-\u9FFF\uA960-\uA97F\uAC00-\uD7AF\uF900-\uFAFF\uFE30-\uFE4F\uFF00-\uFF60\uFFE0-\uFFE6]/
/*
 * The subset the padding is *asserted* on: the syllables, kana and
 * ideographs that carry the text, and the ones this defect was reported
 * against. CJK punctuation and the fullwidth/compatibility forms are
 * measured and printed but not asserted, and the reason is a real limit
 * rather than a convenience: `size-adjust` is a single uniform factor, and
 * a CJK face's punctuation does not share its syllables' advance. Measured
 * 2026-09-09 — in Apple SD Gothic Neo 가 advances 0.865em while 。 advances
 * 0.7738em, so the factor that lands 가 on two cells (139.202%) leaves 。
 * 1.66px short. That residual is 70% smaller than the 5.59px it was before
 * this round, and no font trick closes it; a face with uniform CJK advances
 * would, which is the bundling question left to the lead. Asserting it here
 * would make the gate a target instead of a contract.
 */
const ASSERT_RE =
  /[\u1100-\u11FF\u3040-\u30FF\u3130-\u318F\u31F0-\u31FF\u3400-\u4DBF\u4E00-\u9FFF\uA960-\uA97F\uAC00-\uD7AF\uF900-\uFAFF]/
// The void-run band, in cells. Below MID_LO are the hairline breaks inside a
// syllable and between Latin letters; above MID_HI is a space.
const MID_LO = 0.4
const MID_HI = 0.8
// Ink threshold: |luminance - background| in 0-255. Recorded per run, and
// the verdict is re-reported at 30 and 60 so a result that flips on the
// threshold is visible rather than silent.
const TH = Number(process.env.CJK_TH ?? 40)

// ── Sample text ────────────────────────────────────────────────────────────
// Row 0 and 1 are the two scripts the recordings actually show, taken from
// the poster's own output so the measurement is of the shipped sentence.
// Row 2 is the Latin reference on the same frame — same face, same size,
// same background — which is what makes the comparison a comparison.
const ROWS = [
  '라벨별 비율 대시보드를 만들겠습니다.',
  'ラベル別の比率ダッシュボードを作ります。',
  'Made 1 scratchpad edit, ran 1 shell command',
]
const CJK_ROWS = [0, 1]
const LATIN_ROWS = [2]

// ── Candidate faces (--advances) ───────────────────────────────────────────
// The 1:2 question, answered by measurement rather than by reputation. A
// face is "present" when its advance differs from a deliberately absent
// family's: a canvas asked for a missing family answers in the default face
// without complaining, so equality with the absent baseline is the only
// available absence test.
const CANDIDATES = [
  'Menlo', 'Monaco', 'SF Mono', 'Andale Mono', 'Courier New', 'Consolas',
  'Liberation Mono', 'DejaVu Sans Mono', 'Cascadia Mono', 'JetBrains Mono',
  'Apple SD Gothic Neo', 'AppleGothic', 'Malgun Gothic', 'NanumGothic',
  'NanumGothicCoding', 'D2Coding', 'Sarasa Mono K', 'Sarasa Mono J',
  'Sarasa Term K', 'Noto Sans Mono CJK KR', 'Noto Sans Mono CJK JP',
  'Noto Sans CJK KR', 'Noto Sans KR', 'Source Han Mono K',
  'Hiragino Sans', 'Hiragino Kaku Gothic ProN', 'Yu Gothic', 'Meiryo',
  'MS Gothic', 'Osaka-Mono', 'Osaka', 'Arial Unicode MS', 'Iosevka Term',
]

// ── args ───────────────────────────────────────────────────────────────────
const argv = process.argv.slice(2)
const flag = (n) => argv.includes(n)
const val = (n, d = null) => {
  const eq = argv.find((a) => a.startsWith(n + '='))
  if (eq) return eq.slice(n.length + 1)
  const i = argv.indexOf(n)
  return i >= 0 && argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[i + 1] : d
}
const engineName = val('--engine', 'chromium')
if (!['chromium', 'webkit'].includes(engineName)) {
  console.error(`cjk-cell-fit: unknown engine ${engineName} (chromium|webkit)`)
  process.exit(64)
}
const OUT = path.resolve(val('--out', path.join(__dirname, 'cjk-cell-fit.out')))
const applyFix = !flag('--no-fix')
const asJson = flag('--json')

// ── The measurement, as a string the page evaluates ────────────────────────
// One implementation for both modes: given an ImageData-backed grid it finds
// text lines, then per line the void runs inside the line's ink extent.
const MEASURE_FN = `
function measureBand(data, w, h, th) {
  const lum = (i) => 0.2126 * data[i] + 0.7152 * data[i + 1] + 0.0722 * data[i + 2]
  const hist = new Map()
  for (let i = 0; i < data.length; i += 4) {
    const k = Math.round(lum(i) / 4) * 4
    hist.set(k, (hist.get(k) ?? 0) + 1)
  }
  const bg = [...hist.entries()].sort((a, b) => b[1] - a[1])[0][0]
  const ink = (x, y) => Math.abs(lum((y * w + x) * 4) - bg) > th
  const rowInk = []
  for (let y = 0; y < h; y++) { let n = 0; for (let x = 0; x < w; x++) if (ink(x, y)) n++; rowInk.push(n) }
  const bands = []
  let s = -1
  for (let y = 0; y <= h; y++) {
    const on = y < h && rowInk[y] > 0
    if (on && s < 0) s = y
    if (!on && s >= 0) { if (y - s >= 5) bands.push([s, y - 1]); s = -1 }
  }
  const lines = []
  for (const [y0, y1] of bands) {
    const prof = []
    for (let x = 0; x < w; x++) { let n = 0; for (let y = y0; y <= y1; y++) if (ink(x, y)) n++; prof.push(n) }
    const first = prof.findIndex((v) => v > 0)
    const last = prof.length - 1 - [...prof].reverse().findIndex((v) => v > 0)
    if (first < 0 || last <= first) continue
    const voids = []
    let vs = -1
    for (let x = first; x <= last + 1; x++) {
      const on = x <= last && prof[x] > 0
      if (!on && vs < 0) vs = x
      if (on && vs >= 0) { voids.push(x - vs); vs = -1 }
    }
    // period: autocorrelation of the ink profile over the extent. Reported
    // for orientation and for --png auto-classification; never a verdict.
    const seg = prof.slice(first, last + 1)
    const mean = seg.reduce((a, b) => a + b, 0) / seg.length
    const z = seg.map((v) => v - mean)
    const den = z.reduce((a, b) => a + b * b, 0) || 1
    let peak = { k: 0, r: -1 }
    for (let k = 4; k <= 24; k++) {
      let acc = 0
      for (let i = 0; i + k < z.length; i++) acc += z[i] * z[i + k]
      const r = acc / den
      if (r > peak.r) peak = { k, r }
    }
    lines.push({ y0, y1, first, last, extent: last - first + 1, voids, period: peak.k, periodR: peak.r })
  }
  return { bg, lines }
}
`

/** Per-line ratios for a given cell width. Pure, so both modes share it. */
function scoreLine(line, cell, th) {
  const lo = MID_LO * cell
  const hi = MID_HI * cell
  const mid = line.voids.filter((v) => v >= lo && v <= hi)
  const midCols = mid.reduce((a, b) => a + b, 0)
  return {
    ...line,
    th,
    cell,
    midBand: [Number(lo.toFixed(2)), Number(hi.toFixed(2))],
    midRuns: mid.length,
    midCols,
    midVoidRatio: Number(((midCols / line.extent) * 100).toFixed(2)),
    voidHist: [...line.voids.reduce((m, v) => m.set(v, (m.get(v) ?? 0) + 1), new Map())]
      .sort((a, b) => a[0] - b[0])
      .map(([k, n]) => `${k}px x${n}`)
      .join(' '),
  }
}

/** The exact axis: every span whose text is wide must carry no padding.
 *  --render only — a PNG has no DOM. Returns finding strings. */
function spanPadFindings(spans) {
  const findings = []
  const noted = []
  for (const [i, row] of spans.entries()) {
    for (const s of row) {
      if (!s.t || !WIDE_RE.test(s.t)) continue
      const pad = s.ls ? parseFloat(s.ls) : 0
      if (Math.abs(pad) <= MAX_SPAN_PAD) continue
      const line =
        `row${i} ${JSON.stringify(s.t.slice(0, 12))}: xterm padded the cell by ${pad}px ` +
        `(|pad| > ${MAX_SPAN_PAD})`
      if (ASSERT_RE.test(s.t)) findings.push(`${line} — the glyph does not fill its two cells`)
      else noted.push(`${line} — punctuation/compatibility form, measured not asserted`)
    }
  }
  return { findings, noted }
}

function verdict(cjk, latin, spanFindings = []) {
  const ref = latin.length ? Math.max(...latin.map((l) => l.midVoidRatio)) : 0
  const findings = [...spanFindings]
  for (const c of cjk) {
    if (c.midVoidRatio > MAX_ABS) {
      findings.push(`line y=${c.y0}-${c.y1}: midVoidRatio ${c.midVoidRatio}% > MAX_ABS ${MAX_ABS}%`)
    } else if (c.midVoidRatio - ref > MAX_OVER_LATIN) {
      findings.push(
        `line y=${c.y0}-${c.y1}: midVoidRatio ${c.midVoidRatio}% exceeds the Latin reference ` +
          `${ref}% by ${(c.midVoidRatio - ref).toFixed(2)} > ${MAX_OVER_LATIN}`,
      )
    }
  }
  return { latinRef: ref, findings, pass: findings.length === 0 }
}

function report(title, cjk, latin, extra = {}, spanFindings = []) {
  const v = verdict(cjk, latin, spanFindings)
  console.log(`\n── ${title} ───────────────────────────────────`)
  for (const [k, x] of Object.entries(extra)) console.log(`   ${k}: ${x}`)
  console.log(
    `   ink threshold ${TH}   mid-void band [${MID_LO}, ${MID_HI}] cell = ` +
      `${(cjk[0] ?? latin[0])?.midBand?.join('..')}px   cell ${(cjk[0] ?? latin[0])?.cell}px`,
  )
  console.log('   role   y-range   extent  period  midRuns  midCols  midVoidRatio')
  const row = (r, role) =>
    console.log(
      `   ${role.padEnd(6)} ${(r.y0 + '-' + r.y1).padStart(8)} ${String(r.extent).padStart(7)} ` +
        `${String(r.period).padStart(7)} ${String(r.midRuns).padStart(8)} ${String(r.midCols).padStart(8)} ` +
        `${String(r.midVoidRatio).padStart(12)}%`,
    )
  for (const r of cjk) row(r, 'CJK')
  for (const r of latin) row(r, 'Latin')
  console.log('   void-run histograms (inside the line extent):')
  for (const r of [...cjk, ...latin]) console.log(`     y=${r.y0}-${r.y1}: ${r.voidHist}`)
  console.log(`   Latin reference ${v.latinRef}%   MAX_ABS ${MAX_ABS}%   MAX_OVER_LATIN ${MAX_OVER_LATIN}`)
  if (v.pass) {
    console.log('   PASS — every CJK line fills its cells.')
  } else {
    console.log('   FAIL — a CJK line is padded, not filled:')
    for (const f of v.findings) console.log(`     ${f}`)
  }
  return v
}

// ── Mode: --advances ───────────────────────────────────────────────────────
async function modeAdvances(browser) {
  const page = await browser.newPage()
  await page.setContent('<body>x</body>')
  const rows = await page.evaluate((cands) => {
    const c = document.createElement('canvas').getContext('2d')
    const adv = (fam, ch) => { c.font = `100px ${fam}`; return c.measureText(ch).width / 100 }
    const ABSENT = "'__gadak_absent_family__'"
    return cands.map((f) => {
      const q = `'${f}'`
      const M = adv(q, 'M')
      const ko = adv(q, '가')
      const ja = adv(q, 'あ')
      const present = !(M === adv(ABSENT, 'M') && ko === adv(ABSENT, '가'))
      return { f, present, M, ko, ja }
    })
  }, CANDIDATES)
  console.log('\n── candidate faces on this machine (advance in em, 100px canvas) ──')
  console.log('   Note: a Latin-only face answers the CJK columns in whatever the')
  console.log('   engine fell back to, so those columns describe the *pair*, which')
  console.log('   is exactly what the cell sees.\n')
  console.log('   present  family                       M       가      あ     가:M   あ:M')
  for (const r of rows) {
    console.log(
      `   ${(r.present ? 'yes' : ' - ').padEnd(8)} ${r.f.padEnd(27)} ` +
        `${r.M.toFixed(4)} ${r.ko.toFixed(4)} ${r.ja.toFixed(4)} ` +
        `${(r.ko / r.M).toFixed(3)} ${(r.ja / r.M).toFixed(3)}`,
    )
  }
  console.log('\n   The cell wants 2.000. A face is a selection-only fix only if it')
  console.log('   both leads the stack for Latin and reaches 2.000 for its script.')
  await page.close()
  return rows
}

// ── Mode: --png ────────────────────────────────────────────────────────────
async function modePng(browser) {
  const file = val('--png')
  const crop = (val('--crop') ?? '').split(',').map(Number)
  if (!file || crop.length !== 4 || crop.some((n) => !Number.isFinite(n))) {
    console.error('cjk-cell-fit: --png <file> needs --crop x,y,w,h')
    process.exit(64)
  }
  const b64 = fs.readFileSync(path.resolve(file)).toString('base64')
  const page = await browser.newPage()
  await page.setContent('<body></body>')
  const raw = await page.evaluate(
    async ([b64, x, y, w, h, th, fn]) => {
      // eslint-disable-next-line no-eval
      eval(fn)
      const img = new Image()
      img.src = 'data:image/png;base64,' + b64
      await img.decode()
      const c = document.createElement('canvas')
      c.width = w
      c.height = h
      const g = c.getContext('2d')
      g.drawImage(img, x, y, w, h, 0, 0, w, h)
      // eslint-disable-next-line no-undef
      return measureBand(g.getImageData(0, 0, w, h).data, w, h, th)
    },
    [b64, crop[0], crop[1], crop[2], crop[3], TH, MEASURE_FN],
  )
  await page.close()

  const cellArg = val('--cell')
  // The Latin period is the cell. Auto-derivation is the *smallest* strong
  // period across the band's lines; it is a guess and is labelled as one, so
  // a capture whose cell matters gets --cell.
  const periods = raw.lines.map((l) => l.period).filter((k) => k >= 4)
  const cell = cellArg ? Number(cellArg) : Math.min(...periods)
  const pick = (n) => (val(n) ?? '').split(',').filter((s) => s !== '').map(Number)
  let cjkIdx = pick('--cjk')
  let latinIdx = pick('--latin')
  if (!cjkIdx.length && !latinIdx.length) {
    raw.lines.forEach((l, i) => {
      if (Math.abs(l.period - 2 * cell) <= 2) cjkIdx.push(i)
      else if (Math.abs(l.period - cell) <= 2 || Math.abs(l.period - cell / 2) <= 1) latinIdx.push(i)
    })
  }
  const scored = raw.lines.map((l) => scoreLine(l, cell, TH))
  console.log(`\n   all ${scored.length} text lines in the band (index: y-range  period  midVoidRatio):`)
  scored.forEach((l, i) =>
    console.log(`     [${i}] y=${l.y0}-${l.y1}  period ${l.period}  ${l.midVoidRatio}%`),
  )
  const v = report(
    `${path.relative(ROOT, path.resolve(file))} crop ${crop.join(',')}`,
    cjkIdx.map((i) => scored[i]).filter(Boolean),
    latinIdx.map((i) => scored[i]).filter(Boolean),
    {
      cell: `${cell}px ${cellArg ? '(given)' : '(auto: smallest strong period — a guess)'}`,
      'line roles': `CJK [${cjkIdx}] Latin [${latinIdx}]${
        pick('--cjk').length || pick('--latin').length ? ' (given)' : ' (auto by period)'
      }`,
      background: `luminance ${raw.bg}`,
    },
  )
  return { mode: 'png', file, crop, cell, lines: scored, verdict: v }
}

// ── Mode: --render (default) ───────────────────────────────────────────────
/** app.css's --font-mono-terminal, read from the file so the tool can never
 *  drift from the shipped token. Locale block last wins, like the browser. */
function shippedStack(lang) {
  const css = fs.readFileSync(path.join(ROOT, 'web/src/app.css'), 'utf8')
  const all = [...css.matchAll(/--font-mono-terminal:\s*([^;]+);/g)].map((m) =>
    m[1].replace(/\s+/g, ' ').trim(),
  )
  if (!all.length) throw new Error('cjk-cell-fit: app.css declares no --font-mono-terminal')
  // Two declarations today: the default block and the ja override. Pick by
  // which declaration the locale's own block carries.
  const jaBlockStart = css.search(/:root:lang\(ja\)|\[lang\^?=['"]?ja/)
  if (lang?.startsWith('ja') && all.length > 1) return all[all.length - 1]
  if (jaBlockStart < 0 && all.length > 1) return all[0]
  return all[0]
}

/** cjk-metric.ts's own installCjkMetricFaces, bundled for the page — the gate
 *  measures the shipped module, not a copy of its algorithm. */
function bundleProtocol() {
  const r = esbuild.buildSync({
    entryPoints: [path.join(ROOT, 'web/src/lib/terminal/cjk-metric.ts')],
    bundle: true,
    format: 'iife',
    globalName: 'GadakProtocol',
    write: false,
    platform: 'browser',
    target: 'es2020',
  })
  return r.outputFiles[0].text
}

async function modeRender(browser) {
  fs.mkdirSync(OUT, { recursive: true })
  const XTERM_JS = fs.readFileSync(path.join(ROOT, 'node_modules/@xterm/xterm/lib/xterm.js'), 'utf8')
  const XTERM_CSS = fs.readFileSync(path.join(ROOT, 'node_modules/@xterm/xterm/css/xterm.css'), 'utf8')
  const PROTOCOL = bundleProtocol()
  const lang = val('--lang', 'ko-KR')
  const stack = val('--stack', shippedStack(lang))
  const results = []

  for (const [tag, sample] of [['cjk', ROWS]]) {
    const html =
      `<!DOCTYPE html><html lang="${lang}"><head><meta charset="utf-8"><style>${XTERM_CSS}\n` +
      'html,body{margin:0;padding:0;background:#111}#t{width:760px;height:120px}\n' +
      `</style></head><body><div id="t"></div>\n<script>${XTERM_JS}</script>\n` +
      `<script>${PROTOCOL}</script>\n<script>\n` +
      `const STACK = ${JSON.stringify(stack)};\n` +
      `const APPLY = ${JSON.stringify(applyFix)};\n` +
      `const ROWS = ${JSON.stringify(sample)};\n` +
      `const used = APPLY ? GadakProtocol.installCjkMetricFaces({ stack: STACK }) : STACK;\n` +
      // fontSize 13 (--text-terminal), no lineHeight/letterSpacing — the
      // values web/src/lib/terminal/renderer.ts termOptions() ships.
      `const term = new Terminal({ fontFamily: used, fontSize: 13, allowTransparency: false,\n` +
      `  theme: { background: '#111111', foreground: '#e6e0d4' }, cols: 58, rows: ${sample.length + 1} });\n` +
      `term.open(document.getElementById('t'));\nterm.write(ROWS.join('\\r\\n'));\n` +
      `window.__diag = () => {\n` +
      `  const d = term._core._renderService.dimensions;\n` +
      `  const spans = [...document.querySelectorAll('.xterm-rows > div')].map(r =>\n` +
      `    [...r.children].map(s => ({ t: s.textContent, ls: s.style.letterSpacing || '' })));\n` +
      `  return { cell: d.css.cell, used, faces: (document.getElementById('gadak-cjk-metric-faces')||{}).textContent || '', spans };\n` +
      `};\n` +
      `requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(() => { window.__ready = 1 }, 200)));\n` +
      '</script></body></html>'

    const page = await browser.newPage({ viewport: { width: 800, height: 180 }, deviceScaleFactor: 2 })
    await page.setContent(html, { waitUntil: 'load' })
    await page.waitForFunction('window.__ready')
    const diag = await page.evaluate(() => window.__diag())
    const shot = path.join(OUT, `${tag}-${engineName}-${applyFix ? 'fix' : 'nofix'}.png`)
    await page.locator('#t').screenshot({ path: shot })
    const b64 = fs.readFileSync(shot).toString('base64')
    const raw = await page.evaluate(
      async ([b64, th, fn]) => {
        // eslint-disable-next-line no-eval
        eval(fn)
        const img = new Image()
        img.src = 'data:image/png;base64,' + b64
        await img.decode()
        const c = document.createElement('canvas')
        c.width = img.width
        c.height = img.height
        c.getContext('2d').drawImage(img, 0, 0)
        // eslint-disable-next-line no-undef
        return measureBand(c.getContext('2d').getImageData(0, 0, img.width, img.height).data, img.width, img.height, th)
      },
      [b64, TH, MEASURE_FN],
    )
    await page.close()

    // deviceScaleFactor 2, so the shot's cell is twice the CSS cell. The
    // tool knows the cell from xterm itself here — no guessing.
    const cell = diag.cell.width * 2
    const scored = raw.lines.map((l) => scoreLine(l, cell, TH))
    // Rows were written in a known order and every one of them inked, so
    // the row index maps straight onto the sample. Assert that rather than
    // assume it: a dropped row would silently move every role.
    if (scored.length !== sample.length) {
      console.error(
        `cjk-cell-fit: wrote ${sample.length} rows but found ${scored.length} inked text lines ` +
          `in ${shot} — the sample did not render as written, so the roles cannot be trusted.`,
      )
      process.exit(3)
    }
    console.log(`\n   stack in use: ${diag.used}`)
    console.log(`   xterm cell: ${diag.cell.width.toFixed(4)} x ${diag.cell.height} css px`)
    console.log(`   declared faces: ${diag.faces ? '\n     ' + diag.faces.split('\n').join('\n     ') : '(none)'}`)
    console.log('   xterm per-span letterSpacing (the padding, straight from the DOM):')
    for (const [i, r] of diag.spans.entries()) {
      const s = r.filter((x) => x.t && x.t.trim()).map((x) => `${JSON.stringify(x.t)}=${x.ls || 'default'}`)
      if (s.length) console.log(`     row${i}: ${s.join('  ')}`.slice(0, 300))
    }
    const { findings: spanFindings, noted: spanNoted } = spanPadFindings(diag.spans)
    for (const n of spanNoted) console.log(`   residual (not asserted): ${n}`)
    const v = report(
      `render ${engineName} ${applyFix ? '(GDK-1597 applied)' : '(--no-fix)'}`,
      CJK_ROWS.map((i) => scored[i]),
      LATIN_ROWS.map((i) => scored[i]),
      {
        shot: path.relative(ROOT, shot),
        lang,
        'max |span padding| (asserted glyphs)': `${Math.max(
          0,
          ...diag.spans.flat().filter((x) => x.t && ASSERT_RE.test(x.t)).map((x) => Math.abs(parseFloat(x.ls) || 0)),
        ).toFixed(5)}px (limit ${MAX_SPAN_PAD})`,
      },
      spanFindings,
    )
    results.push({ mode: 'render', engine: engineName, applyFix, shot, cell, diag, lines: scored, verdict: v })
  }
  return results[0]
}

// ── main ───────────────────────────────────────────────────────────────────
const browser = await (engineName === 'webkit' ? webkit : chromium).launch()
let result
try {
  if (flag('--advances')) result = { mode: 'advances', rows: await modeAdvances(browser) }
  else if (val('--png')) result = await modePng(browser)
  else result = await modeRender(browser)
} finally {
  await browser.close()
}
if (asJson) {
  fs.mkdirSync(OUT, { recursive: true })
  const f = path.join(OUT, `${result.mode}-${engineName}.json`)
  fs.writeFileSync(f, JSON.stringify(result, null, 2))
  console.log(`\n   json: ${path.relative(ROOT, f)}`)
}
console.log('')
process.exit(result.verdict && !result.verdict.pass ? 1 : 0)
