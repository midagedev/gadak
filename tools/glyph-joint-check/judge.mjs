#!/usr/bin/env node
/*
 * glyph-joint-check — GDK-1043 standing instrument, promoted 2026-08-28 from
 * the investigation scratch (scratch/glyph-matrix). Static page + Playwright,
 * DSF 2. Loads @xterm/xterm from THIS repo's node_modules (inlined into the
 * page), renders a box-drawing pattern through the pane's own options,
 * screenshots the terminal element, and judges joints numerically from the
 * PNG. No gadak serve, no ports.
 *
 * macOS only (CoreText resolution is the whole subject — see README.md), and
 * CI must never assert font pixels from this tool; the text-order contract
 * gate is web/src/lib/terminal/font-stack.test.ts.
 *
 * .mjs, not the scratch's .js: the repo root package.json is "type":
 * "module", so a CommonJS script cannot run from inside it.
 *
 * Usage (from the repo root):
 *   node tools/glyph-joint-check/judge.mjs [--engine=webkit|chromium] [config ...]
 *   node tools/glyph-joint-check/judge.mjs --engine=webkit --stack="Menlo, monospace"
 *   (default: all configs on their default engine)
 */
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..', '..');
const OUT = path.join(__dirname, 'out');
const require_ = createRequire(import.meta.url);
const { chromium, webkit } = require_(path.join(ROOT, 'node_modules', 'playwright'));

const XTERM_JS = fs.readFileSync(path.join(ROOT, 'node_modules/@xterm/xterm/lib/xterm.js'), 'utf8');
const XTERM_CSS = fs.readFileSync(path.join(ROOT, 'node_modules/@xterm/xterm/css/xterm.css'), 'utf8');

// The two stacks the pane can ride. baseline == --font-mono as shipped
// (web/src/app.css), menlo-first == --font-mono-terminal (GDK-1043 fix).
// fontSize 13 (--text-terminal: 13px), chromeTheme light tokens — the exact
// values web/src/lib/terminal/renderer.ts puts on the Terminal.
const STACK =
  "ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', " +
  "'Apple SD Gothic Neo', 'Noto Sans CJK KR', 'Malgun Gothic', monospace";
const TERMINAL_STACK =
  "Menlo, ui-monospace, SFMono-Regular, 'SF Mono', Consolas, 'Liberation Mono', " +
  "'Apple SD Gothic Neo', 'Noto Sans CJK KR', 'Malgun Gothic', monospace";
const THEME = {
  background: '#f4efe4',
  foreground: '#1c1812',
  cursor: '#2e4560',
  cursorAccent: '#f4efe4',
  selectionBackground: '#cfc0a4',
};

const PATTERN = ['┌─┬─┐', '│ │ │', '├─┼─┤', '│ │ │', '└─┴─┘', '', '│', '│', '│', '│', '──────', '한글'];
// Row map: 0-4 box, 5 blank, 6-9 solid │ col0, 10 ─ x6, 11 한글
const ROW_BOX = { top: 0, bottom: 4 };
const ROW_SOLID = { top: 6, bottom: 9 };
const ROW_HRUN = 10;
const ROW_CJK = 11;
const BOX_COLS = [0, 2, 4];

const CONFIGS = [
  { name: 'baseline', fontFamily: STACK, note: '--font-mono as shipped; WebKit resolves ui-monospace to SF Mono → the GDK-1043 defect' },
  { name: 'menlo-first', fontFamily: TERMINAL_STACK, note: '--font-mono-terminal (GDK-1043 fix)' },
  { name: 'font-sfmono-only', fontFamily: "'SF Mono', monospace", note: 'the broken resolution isolated' },
  { name: 'font-monaco', fontFamily: 'Monaco, monospace', note: 'rejected candidate — narrower cell, sans-class reach destroys the grid' },
];

const ENGINE_DEFAULT = 'chromium';

// ---------------------------------------------------------------- page build

function pageHtml(cfg) {
  return (
    '<!DOCTYPE html><html><head><meta charset="utf-8"><style>\n' +
    XTERM_CSS +
    '\nhtml,body{margin:0;padding:0;background:#fff}#terminal{width:480px;height:420px}\n' +
    '</style></head><body><div id="terminal"></div>\n' +
    '<script>/* @xterm/xterm inlined from repo node_modules */\n' +
    XTERM_JS +
    '\n</script>\n' +
    '<script>\n' +
    'const CONFIG = ' + JSON.stringify(cfg) + ';\n' +
    'const THEME = ' + JSON.stringify(THEME) + ';\n' +
    'const PATTERN = ' + JSON.stringify(cfg.pattern || PATTERN) + ';\n' +
    pageScript +
    '\n</script></body></html>'
  );
}

const pageScript = `
const consoleErrors = [];
window.addEventListener('error', e => consoleErrors.push(String(e.message)));
(function run() {
  // termOptions() parity: fontSize 13, allowTransparency:false, scrollback:5000,
  // cursorBlink:false (web/src/lib/terminal/renderer.ts). No lineHeight,
  // no letterSpacing, no customGlyphs in the shipped pane.
  const term = new Terminal(Object.assign({
    fontFamily: CONFIG.fontFamily,
    fontSize: 13,
    allowTransparency: false,
    scrollback: 5000,
    cursorBlink: false,
    theme: THEME,
    cols: 30,
    rows: 14,
  }, CONFIG.extra || {}));
  term.open(document.getElementById('terminal'));
  term.write(PATTERN.join('\\r\\n'));
  const settle = 150;
  requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(() => {
    window.__probe = collectProbe(term, consoleErrors);
    window.__ready = true;
  }, settle)));
})();

function collectProbe(term, consoleErrors) {
  const el = term.element;
  const elRect = el.getBoundingClientRect();
  // cell dimensions from internals first; record which path worked
  let cell = null, cellSrc = 'none';
  try {
    const d = term._core && term._core._renderService && term._core._renderService.dimensions;
    if (d && d.css && d.css.cell && d.css.cell.width) {
      cell = { w: d.css.cell.width, h: d.css.cell.height };
      cellSrc = '_core._renderService.dimensions.css.cell';
    } else if (d && d.cell && d.cell.width) {
      cell = { w: d.cell.width, h: d.cell.height };
      cellSrc = '_core._renderService.dimensions.cell';
    }
  } catch (e) { cellSrc = 'threw ' + e; }
  const rows = [];
  const rowNodes = el.querySelectorAll('.xterm-rows > div');
  rowNodes.forEach((n) => {
    const r = n.getBoundingClientRect();
    rows.push({ top: r.top - elRect.top, left: r.left - elRect.left, width: r.width, height: r.height });
  });
  let rowsSrc = '.xterm-rows > div rects';
  if (!rows.length && term.rows > 0 && elRect.height > 0) {
    // a future renderer without DOM rows: synthesize from element box / rows
    const h = elRect.height / term.rows;
    for (let i = 0; i < term.rows; i++) rows.push({ top: i * h, left: 0, width: elRect.width, height: h });
    rowsSrc = 'synthesized(element/rows) — renderer exposed no DOM rows';
  }
  if (!cell && rows.length >= 2 && term.cols > 0) {
    cell = { w: elRect.width / term.cols, h: rows[1].top - rows[0].top };
    cellSrc = 'derived(element/cols, row pitch)';
  }
  const charSize = (() => {
    try {
      const s = term._core && term._core._charSizeService;
      return s ? { w: s.width, h: s.height } : null;
    } catch (e) { return null; }
  })();
  return {
    ua: navigator.userAgent,
    platform: navigator.platform,
    dpr: window.devicePixelRatio,
    fontFamilyRequested: CONFIG.fontFamily,
    extraOptions: CONFIG.extra || null,
    cols: term.cols, rows: term.rows,
    elementRect: { w: elRect.width, h: elRect.height },
    cell, cellSrc, charSize,
    rowCount: rows.length,
    rowsSrc,
    rows: rows.slice(0, 14),
    fonts: fontProbe(),
    consoleErrors,
  };
}

function fontProbe() {
  const FAMS = ['ui-monospace', 'SFMono-Regular', 'SF Mono', 'Menlo', 'Consolas',
    'Liberation Mono', 'Apple SD Gothic Neo', 'Noto Sans CJK KR', 'Malgun Gothic',
    'Monaco', 'Cascadia Mono', 'Helvetica Neue', 'monospace'];
  const CHARS = ['└', '─', '│', '한', 'A'];
  const out = { check: {}, ink: {}, bare: {} };
  for (const fam of FAMS) {
    out.check[fam] = {};
    for (const ch of CHARS) {
      let v;
      try { v = document.fonts.check('13px "' + fam + '"', ch); }
      catch (e) { v = 'threw'; }
      out.check[fam][ch] = v;
    }
    out.ink[fam] = {};
    for (const ch of ['└', '│', '─', '한']) {
      out.ink[fam][ch] = canvasInk('13px "' + fam + '", monospace', ch);
    }
    // bare: no fallback appended — unknown families land on the UA default
    // (sans). The advance separates Menlo 7.83 / SFNSMono 8.04 / Helvetica
    // full-width box 13.00 / Helvetica Latin ~7.2.
    out.bare[fam] = {};
    for (const ch of ['─', 'A']) {
      out.bare[fam][ch] = canvasInk('13px "' + fam + '"', ch);
    }
  }
  // what the full stack resolves to, per glyph (the browser's own pick)
  out.ink['<full stack>'] = {};
  for (const ch of ['└', '│', '─', '한']) {
    out.ink['<full stack>'][ch] = canvasInk('13px ' + CONFIG.fontFamily, ch);
  }
  return out;
}

function canvasInk(fontSpec, ch) {
  const S = 72;
  const cv = document.createElement('canvas');
  cv.width = S; cv.height = S;
  const ctx = cv.getContext('2d', { willReadFrequently: true });
  ctx.fillStyle = '#ffffff'; ctx.fillRect(0, 0, S, S);
  ctx.fillStyle = '#000000';
  ctx.font = fontSpec;
  ctx.textBaseline = 'alphabetic';
  const m = ctx.measureText(ch);
  const bx = 16, by = 48;
  ctx.fillText(ch, bx, by);
  const d = ctx.getImageData(0, 0, S, S).data;
  let x0 = S, y0 = S, x1 = -1, y1 = -1, dark = 0;
  for (let y = 0; y < S; y++) for (let x = 0; x < S; x++) {
    const i = (y * S + x) * 4;
    if (d[i] < 128) {
      dark++;
      if (x < x0) x0 = x; if (x > x1) x1 = x;
      if (y < y0) y0 = y; if (y > y1) y1 = y;
    }
  }
  return {
    advance: m.width,
    bbAscent: m.actualBoundingBoxAscent, bbDescent: m.actualBoundingBoxDescent,
    ink: x1 < 0 ? null : {
      h: y1 - y0 + 1, w: x1 - x0 + 1, dark,
      aboveBaseline: by - y0, belowBaseline: y1 - by,
    },
  };
}
`;

// ---------------------------------------------------------------- png decode

function decodePng(buf) {
  if (buf.readUInt32BE(0) !== 0x89504e47) throw new Error('not a png');
  let pos = 8, w, h, bitDepth, colorType, interlace;
  const idat = [];
  while (pos + 8 <= buf.length) {
    const len = buf.readUInt32BE(pos);
    const type = buf.toString('ascii', pos + 4, pos + 8);
    const data = buf.slice(pos + 8, pos + 8 + len);
    if (type === 'IHDR') {
      w = data.readUInt32BE(0); h = data.readUInt32BE(4);
      bitDepth = data[8]; colorType = data[9]; interlace = data[12];
    } else if (type === 'IDAT') idat.push(data);
    pos += 12 + len;
    if (type === 'IEND') break;
  }
  if (bitDepth !== 8 || (colorType !== 6 && colorType !== 2) || interlace !== 0)
    throw new Error('unsupported png depth=' + bitDepth + ' color=' + colorType + ' interlace=' + interlace);
  const bpp = colorType === 6 ? 4 : 3;
  const raw = zlib.inflateSync(Buffer.concat(idat));
  const stride = w * bpp;
  const out = Buffer.alloc(h * stride);
  let p = 0;
  for (let y = 0; y < h; y++) {
    const filter = raw[p++];
    const row = raw.slice(p, p + stride); p += stride;
    const prev = y > 0 ? out.slice((y - 1) * stride, y * stride) : null;
    const cur = out.slice(y * stride, (y + 1) * stride);
    for (let x = 0; x < stride; x++) {
      const a = x >= bpp ? cur[x - bpp] : 0;
      const b = prev ? prev[x] : 0;
      const c = x >= bpp && prev ? prev[x - bpp] : 0;
      let v = row[x];
      if (filter === 1) v = (v + a) & 0xff;
      else if (filter === 2) v = (v + b) & 0xff;
      else if (filter === 3) v = (v + ((a + b) >> 1)) & 0xff;
      else if (filter === 4) {
        const pa = Math.abs(b - c), pb = Math.abs(a - c), pc = Math.abs(a + b - 2 * c);
        const pr = (pa <= pb && pa <= pc) ? a : (pb <= pc ? b : c);
        v = (v + pr) & 0xff;
      } else if (filter !== 0) throw new Error('bad filter ' + filter);
      cur[x] = v;
    }
  }
  return { w, h, bpp, data: out };
}

function lum(png, x, y) {
  if (x < 0 || y < 0 || x >= png.w || y >= png.h) return 255;
  const i = (y * png.w + x) * png.bpp;
  return (png.data[i] * 299 + png.data[i + 1] * 587 + png.data[i + 2] * 114) / 1000;
}

// ------------------------------------------------------------ joint checks

/** Vertical continuity in [xa,xb) x [ytop,ybot). Returns the dominant stroke
 *  column x*, then walks every y and counts rows with no dark pixel within
 *  ±1px of x*. joined = zero gap rows. */
function verticalCheck(png, xa, xb, ytop, ybot, th) {
  xa = Math.max(0, Math.round(xa)); xb = Math.min(png.w, Math.round(xb));
  ytop = Math.max(0, Math.round(ytop)); ybot = Math.min(png.h, Math.round(ybot));
  let xStar = -1, best = -1;
  for (let x = xa; x < xb; x++) {
    let c = 0;
    for (let y = ytop; y < ybot; y++) if (lum(png, x, y) < th) c++;
    if (c > best) { best = c; xStar = x; }
  }
  if (xStar < 0) return { xStar: -1, gapRows: -1, runs: [], firstInkY: -1, lastInkY: -1, note: 'no ink' };
  const lo = Math.max(xa, xStar - 1), hi = Math.min(xb - 1, xStar + 1);
  const gap = new Array(ybot - ytop).fill(true);
  let firstInkY = -1, lastInkY = -1;
  for (let y = ytop; y < ybot; y++) {
    let dark = false;
    for (let x = lo; x <= hi; x++) if (lum(png, x, y) < th) { dark = true; break; }
    gap[y - ytop] = !dark;
    if (dark) { if (firstInkY < 0) firstInkY = y; lastInkY = y; }
  }
  const runs = [];
  let s = -1;
  for (let i = 0; i <= gap.length; i++) {
    if (i < gap.length && gap[i]) { if (s < 0) s = i; }
    else if (s >= 0) { runs.push({ y0: ytop + s, y1: ytop + i - 1, len: i - s }); s = -1; }
  }
  const gapRows = runs.reduce((a, r) => a + r.len, 0);
  // interior gaps only: gap rows strictly between the first and last ink row.
  // The void above ┌'s arm / below └'s arm is normal glyph shape, not a seam.
  const interiorRuns = runs.filter((r) => r.y0 > firstInkY && r.y1 < lastInkY);
  const gapRowsInterior = interiorRuns.reduce((a, r) => a + r.len, 0);
  return { xStar, gapRows, gapRowsInterior, interiorRuns, runs, firstInkY, lastInkY };
}

/** y-band of dark ink within one cell's x-range. */
function cellBand(png, xa, xb, ytop, ybot, th) {
  xa = Math.max(0, Math.round(xa)); xb = Math.min(png.w, Math.round(xb));
  ytop = Math.max(0, Math.round(ytop)); ybot = Math.min(png.h, Math.round(ybot));
  let minY = -1, maxY = -1, darkCount = 0;
  for (let y = ytop; y < ybot; y++)
    for (let x = xa; x < xb; x++)
      if (lum(png, x, y) < th) { darkCount++; if (minY < 0) minY = y; maxY = y; }
  return { minY, maxY, darkCount };
}

function bandsOverlap(a, b, tol) {
  if (a.maxY < 0 || b.maxY < 0) return false;
  return a.maxY >= b.minY - tol && b.maxY >= a.minY - tol;
}

// ---------------------------------------------------------------- analysis

function analyze(png, probe, cfgName, th, dsf) {
  const cellW = probe.cell.w * dsf;
  const rowTop = (r) => probe.rows[r].top * dsf;
  const rowBot = (r) => (probe.rows[r].top + probe.rows[r].height) * dsf;
  const colX = (j) => j * cellW;

  const res = { config: cfgName, threshold: th, checks: {} };

  // 1. solid │ column, rows 6..9, col 0
  res.checks.solidCol = verticalCheck(png, colX(0), colX(1), rowTop(ROW_SOLID.top), rowBot(ROW_SOLID.bottom), th);

  // 2. box verticals (cols 0,2,4 across rows 0..4) + float of └ row
  res.checks.boxCols = {};
  for (const c of BOX_COLS) {
    res.checks.boxCols[c] = verticalCheck(png, colX(c), colX(c + 1), rowTop(ROW_BOX.top), rowBot(ROW_BOX.bottom), th);
  }
  res.checks.cornerFloat = {};
  for (const c of [0, 2, 4]) {
    const v = res.checks.boxCols[c];
    res.checks.cornerFloat[c] = v.lastInkY >= 0 ? rowBot(ROW_BOX.bottom) - v.lastInkY - 1 : null; // px of empty cell below └'s ink
  }

  // 3. horizontal runs: row 10 (six ─) and every row of the box
  const hruns = {};
  const seg = (row, cols) => {
    const bands = cols.map((j) => cellBand(png, colX(j), colX(j + 1), rowTop(row), rowBot(row), th));
    let fails = 0;
    const pairs = [];
    for (let i = 0; i + 1 < bands.length; i++) {
      const ok = bandsOverlap(bands[i], bands[i + 1], 1);
      if (!ok) fails++;
      pairs.push({ cells: [cols[i], cols[i + 1]], ok });
    }
    return { bands, fails, pairs };
  };
  hruns['row10'] = seg(ROW_HRUN, [0, 1, 2, 3, 4, 5]);
  for (const r of [0, 2, 4]) hruns['row' + r] = seg(r, [0, 1, 2, 3, 4]);
  res.checks.hruns = hruns;
  res.checks.hgapTotal = Object.values(hruns).reduce((a, s) => a + s.fails, 0);

  // 4. baseline arm check on row 4: └ ┴ ┘ ink must reach the ─ band (±1px)
  const row4 = hruns['row4'];
  const hBand = { minY: Math.min(row4.bands[1].minY, row4.bands[3].minY), maxY: Math.max(row4.bands[1].maxY, row4.bands[3].maxY) };
  // bands array index == position in cols array [0,1,2,3,4]
  const armOk = { '└': bandsOverlap(row4.bands[0], hBand, 1), '┴': bandsOverlap(row4.bands[2], hBand, 1), '┘': bandsOverlap(row4.bands[4], hBand, 1) };
  const armFloat = {
    '└': hBand.minY - row4.bands[0].maxY, // >0: glyph ink ends above the ─ band (floats)
    '┴': hBand.minY - row4.bands[2].maxY,
    '┘': hBand.minY - row4.bands[4].maxY,
  };
  res.checks.arm = { hBand, armOk, armFloat, ok: armOk['└'] && armOk['┴'] && armOk['┘'] };

  // 5. CJK cells still inked
  const cjk = [0, 1].map((j) => cellBand(png, colX(j), colX(j + 1), rowTop(ROW_CJK), rowBot(ROW_CJK), th));
  res.checks.cjk = { bands: cjk, inked: cjk.every((b) => b.darkCount > 0) };

  // blank-pane detector
  const all = cellBand(png, 0, png.w, 0, png.h, th);
  res.checks.totalDark = all.darkCount;

  // verdict — "joined = zero gap rows across every cell boundary": interior
  // gaps of the vertical members + horizontal band continuity + arm reach.
  const vgaps = res.checks.solidCol.gapRowsInterior + BOX_COLS.reduce((a, c) => a + Math.max(0, res.checks.boxCols[c].gapRowsInterior), 0);
  res.vgaps = vgaps;
  res.rawVgaps = res.checks.solidCol.gapRows + BOX_COLS.reduce((a, c) => a + Math.max(0, res.checks.boxCols[c].gapRows), 0);
  res.cellMetrics = { cellWcss: probe.cell.w, cellHcss: probe.cell.h, note: '7.83=Menlo 7.80=Monaco 8.04=SFNSMono (CoreText advances at 13px)' };
  res.hgaps = res.checks.hgapTotal;
  res.joined = vgaps === 0 && res.checks.hgapTotal === 0 && res.checks.arm.ok && res.checks.cjk.inked && all.darkCount > 500;
  return res;
}

// which candidate family's canvas fingerprint matches the terminal's own glyph
function identifyFont(probe) {
  const fams = Object.keys(probe.fonts.ink).filter((f) => f !== '<full stack>');
  const out = {};
  for (const ch of ['└', '│']) {
    const stackInk = probe.fonts.ink['<full stack>'][ch].ink;
    if (!stackInk) { out[ch] = { note: 'stack ink null' }; continue; }
    const ranked = fams
      .map((f) => {
        const ink = probe.fonts.ink[f][ch].ink;
        const adv = probe.fonts.ink[f][ch].advance;
        const dInk = ink ? Math.abs(ink.h - stackInk.h) + Math.abs(ink.w - stackInk.w) + Math.abs(ink.aboveBaseline - stackInk.aboveBaseline) + Math.abs(ink.belowBaseline - stackInk.belowBaseline) : 9999;
        return { fam: f, dInk, adv, ink };
      })
      .sort((a, b) => a.dInk - b.dInk);
    out[ch] = { stackInk, ranked: ranked.slice(0, 3) };
  }
  return out;
}

// ---------------------------------------------------------------- driver

function probeCellW(res) {
  const w = res.probe.cell && res.probe.cell.w;
  return w ? w.toFixed(2) : '?';
}

async function runConfig(browser, cfg, th, engineName) {
  const page = await browser.newPage({ viewport: { width: 560, height: 500 }, deviceScaleFactor: 2 });
  const consoleMsgs = [];
  page.on('console', (m) => { if (m.type() === 'error' || m.type() === 'warning') consoleMsgs.push(m.type() + ': ' + m.text()); });
  page.on('pageerror', (e) => consoleMsgs.push('pageerror: ' + String(e)));
  await page.setContent(pageHtml(cfg), { waitUntil: 'load' });
  await page.waitForFunction('window.__ready === true', null, { timeout: 20000 });
  const probe = await page.evaluate('window.__probe');
  probe.consoleErrors = (probe.consoleErrors || []).concat(consoleMsgs);
  const shot = path.join(OUT, 'shot-' + engineName + '-' + cfg.name + '.png');
  await page.locator('#terminal').screenshot({ path: shot });
  await page.close();
  const png = decodePng(fs.readFileSync(shot));
  const res = analyze(png, probe, cfg.name, th, 2);
  // custom one-line patterns have no box/solid/cjk structure — analyze
  // tolerates missing rows as no-ink; note that the row map does not apply
  if (cfg.pattern) res.patternNote = 'custom pattern: ' + JSON.stringify(cfg.pattern);
  res.engine = engineName;
  res.shot = path.relative(ROOT, shot);
  res.pngSize = { w: png.w, h: png.h };
  res.probe = probe;
  res.fontIdentity = identifyFont(probe);
  return res;
}

async function main() {
  const args = process.argv.slice(2);
  const engineArg = args.find((a) => a.startsWith('--engine='));
  const engineFilter = engineArg ? engineArg.split('=')[1] : null;
  const stackArg = args.find((a) => a.startsWith('--stack='));
  const rest = args.filter((a) => !a.startsWith('--engine=') && !a.startsWith('--stack='));
  let list = stackArg
    ? [{ name: 'ad-hoc', fontFamily: stackArg.slice('--stack='.length), note: 'ad-hoc --stack' }]
    : rest.length
      ? CONFIGS.filter((c) => rest.includes(c.name))
      : CONFIGS.slice();
  if (!stackArg && rest.length) {
    const unknown = rest.filter((n) => !CONFIGS.some((c) => c.name === n));
    if (unknown.length) { console.error('unknown config(s): ' + unknown.join(', ') + ' — known: ' + CONFIGS.map((c) => c.name).join(', ')); process.exit(2); }
  }
  const thDefault = Number(process.env.TH || 128);
  fs.mkdirSync(OUT, { recursive: true });
  const results = [];
  const engines = {};
  for (const cfg of list) {
    const engineName = engineFilter || cfg.engine || ENGINE_DEFAULT;
    if (engineName !== 'webkit' && engineName !== 'chromium') {
      console.error('unknown engine: ' + engineName + ' (webkit|chromium)');
      process.exit(2);
    }
    if (!engines[engineName]) {
      engines[engineName] = engineName === 'webkit' ? await webkit.launch() : await chromium.launch();
    }
    const browser = engines[engineName];
    const res = await runConfig(browser, cfg, thDefault, engineName);
    // threshold sensitivity at 100 and 160
    const png = decodePng(fs.readFileSync(path.join(ROOT, res.shot)));
    res.sensitivity = {
      th100: ((r) => ({ joined: r.joined, vgaps: r.vgaps, hgaps: r.hgaps, arm: r.checks.arm.ok }))(analyze(png, res.probe, cfg.name, 100, 2)),
      th160: ((r) => ({ joined: r.joined, vgaps: r.vgaps, hgaps: r.hgaps, arm: r.checks.arm.ok }))(analyze(png, res.probe, cfg.name, 160, 2)),
    };
    results.push(res);
    const a = res.checks.arm;
    console.log(
      engineName.padEnd(8), cfg.name.padEnd(18),
      res.joined ? 'JOINED' : 'BROKEN',
      '(vgaps=' + res.vgaps + ', hgaps=' + res.hgaps + ', arm=' + (a.ok ? 'ok' : 'miss') +
      ', float└/┴/┘=' + a.armFloat['└'] + '/' + a.armFloat['┴'] + '/' + a.armFloat['┘'] +
      ', cjk=' + (res.checks.cjk.inked ? 'inked' : 'EMPTY') +
      ', cell=' + probeCellW(res) +
      ', dark=' + res.checks.totalDark + ')'
    );
  }
  for (const name of Object.keys(engines)) await engines[name].close();
  for (const res of results) {
    fs.writeFileSync(path.join(OUT, res.engine + '-' + res.config + '.json'), JSON.stringify(res, null, 2));
  }
  fs.writeFileSync(path.join(OUT, 'summary.json'), JSON.stringify(results.map((r) => ({
    engine: r.engine, config: r.config, joined: r.joined, vgaps: r.vgaps, hgaps: r.hgaps, arm: r.checks.arm.ok,
    armFloat: r.checks.arm.armFloat, cell: r.cellMetrics, sensitivity: r.sensitivity, shot: r.shot,
  })), null, 2));
}

main().catch((e) => { console.error(e); process.exit(1); });
