#!/usr/bin/env node
/*
 * Per-glyph platform-font probe (GDK-1532 / GDK-1533).
 *
 * CSS font fallback is per glyph, not per element: a stack that names a
 * Korean face before any Japanese one hands Japanese readers Korean hanja
 * shapes and drops the shinjitai the Korean face lacks onto whatever comes
 * next — on this machine, a Simplified Chinese face. Nothing in the build
 * sees that. `font-family` parses, the page renders, every gate is green.
 * The only instrument that sees it is the one the browser uses to pick the
 * face: CDP's CSS.getPlatformFontsForNode.
 *
 * This is that instrument, made standing. It renders one span per sample
 * glyph under each locale's real `<html lang>` value, with the surface's
 * real stylesheet, and reports which platform face the browser actually
 * chose — then classifies it (Japanese / Korean / Chinese / Latin) and
 * separates the two failure kinds a font bug comes in:
 *
 *   STACK   the stack names no face for this script at all — a code defect,
 *           fixable in the repo. Exit 1.
 *   ABSENT  the stack names the right faces but none is installed here —
 *           an environment gap, not a code defect. Exit 0 (2 with --strict,
 *           for a CI image that is supposed to carry the face).
 *
 * Surfaces and how they are measured
 *   web   web/src/app.css, served with `@import 'tailwindcss'` dropped and
 *         `@theme {` rewritten to `:root {`. Both the token block and the
 *         locale override are unlayered in the real build's cascade order
 *         too (@theme lands in @layer theme, the override is unlayered), so
 *         the precedence the probe measures is the precedence that ships.
 *   site  site/src/styles/tokens.css verbatim, served over HTTP from the
 *         repo root so its Pretendard @import resolves out of
 *         site/node_modules — the real first family, not a stand-in.
 *
 * What it does NOT measure: the built bundle. It reads the sources, so a
 * defect introduced by a build step would not show here. Everything the
 * two files themselves decide — family order, selector match, cascade
 * layer — is measured for real.
 *
 * Usage:
 *   node tools/font-probe.mjs                  both surfaces, all locales
 *   node tools/font-probe.mjs --surface web    one surface
 *   node tools/font-probe.mjs --json           machine-readable
 *   node tools/font-probe.mjs --strict         ABSENT is a failure too (CI)
 */
import { createServer } from 'node:http'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, normalize, extname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')

// ── Samples ────────────────────────────────────────────────────────────────
// One span per glyph so getPlatformFontsForNode returns exactly one face.
// The kanji set is the one GDK-1532 measured: 課 is shared with Korean
// hanja, 担/当/検 are shinjitai a Korean face does not carry (they fell to
// PingFang SC), あ/ア are the kana that pin the script outright.
const GLYPHS = [
  { ch: 'A', script: 'latin', note: 'Latin' },
  { ch: '가', script: 'hangul', note: 'Hangul' },
  { ch: '課', script: 'han', note: 'Han (shared ko/ja)' },
  { ch: 'あ', script: 'kana', note: 'Hiragana' },
  { ch: 'ア', script: 'kana', note: 'Katakana' },
  { ch: '担', script: 'kanji-jp', note: 'shinjitai' },
  { ch: '当', script: 'kanji-jp', note: 'shinjitai' },
  { ch: '検', script: 'kanji-jp', note: 'shinjitai' },
]

// The `<html lang>` value each surface actually ships, so the probe tests
// the selector as written: the app syncs documentElement.lang to the BCP 47
// tag (web/src/lib/i18n/index.ts localeTag), the site writes t.htmlLang.
const SURFACES = {
  web: {
    css: 'web/src/app.css',
    langs: { en: 'en-US', ko: 'ko-KR', ja: 'ja-JP' },
    roles: [
      { name: 'sans', family: 'var(--font-sans)' },
      { name: 'display', family: 'var(--font-display)' },
      { name: 'mono', family: 'var(--font-mono)' },
      { name: 'mono-terminal', family: 'var(--font-mono-terminal)' },
    ],
  },
  site: {
    css: 'site/src/styles/tokens.css',
    langs: { en: 'en', ko: 'ko', ja: 'ja' },
    roles: [
      { name: 'sans', family: 'var(--font-sans)' },
      { name: 'mono', family: 'var(--font-mono)' },
    ],
  },
}

// ── Face classification ────────────────────────────────────────────────────
// Order matters: the Chinese test runs first because "Hiragino Sans GB" and
// "Hiragino Sans TC" are Chinese faces wearing a Japanese family's name.
// 蘋方/苹方 is PingFang under a localized family name — CDP reports the
// system UI face as ".蘋方 UI-簡", which no ASCII pattern would catch.
const CN = /\b(SC|TC|HK|GB|CNS|CN|TW)\b|PingFang|蘋方|苹方|黑体|宋体|Songti|STHeiti|STSong|SimSun|SimHei|Heiti|Microsoft YaHei|Kaiti|FangSong/i
const JP = /Hiragino|Yu ?Gothic|Yu ?Mincho|YuMincho|Meiryo|MS ?[PU]?(Gothic|Mincho)|Osaka|Klee|Tsukushi|Toppan|CJK JP|Sans JP|Serif JP|Han Sans JP|Han Serif JP|Kozuka/i
const KO = /Apple ?SD|AppleGothic|AppleMyungjo|산돌|Malgun|Nanum|나눔|Pretendard|Gothic A1|Spoqa|CJK KR|Sans KR|Serif KR|Han Sans K|Han Serif K|Dotum|Gulim|Batang/i

/** 'jp' | 'ko' | 'cn' | 'other' — what script community this face serves. */
export function classifyFace(name) {
  if (!name) return 'other'
  if (CN.test(name)) return 'cn'
  if (JP.test(name)) return 'jp'
  if (KO.test(name)) return 'ko'
  return 'other'
}

/**
 * Which face community a glyph must be drawn by, per locale. Keyed by the
 * glyph's script, not by the locale alone: Hangul is Korean on a Japanese
 * page too, and 課 is the character that has to move — Korean shapes under
 * ko, Japanese shapes under ja. A script with no entry for a locale is not
 * asserted (Japanese prose on a Korean page is nobody's contract here, and
 * `en` asserts nothing at all).
 */
const EXPECT = {
  latin: {},
  hangul: { ko: 'ko', ja: 'ko' },
  han: { ko: 'ko', ja: 'jp' },
  kana: { ja: 'jp' },
  'kanji-jp': { ja: 'jp' },
}

// ── Static server: repo files from disk, probe pages from memory ───────────
const MIME = { '.css': 'text/css', '.html': 'text/html', '.woff2': 'font/woff2', '.woff': 'font/woff', '.ttf': 'font/ttf', '.otf': 'font/otf', '.js': 'text/javascript', '.json': 'application/json' }

function serve(virtual) {
  return new Promise((resolve) => {
    const server = createServer((req, res) => {
      const path = decodeURIComponent(new URL(req.url, 'http://x').pathname)
      if (virtual.has(path)) {
        const { body, type } = virtual.get(path)
        res.writeHead(200, { 'content-type': type })
        res.end(body)
        return
      }
      const file = join(ROOT, normalize(path).replace(/^(\.\.[/\\])+/, ''))
      if (!file.startsWith(ROOT) || !existsSync(file) || !statSync(file).isFile()) {
        res.writeHead(404)
        res.end('not found')
        return
      }
      res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' })
      res.end(readFileSync(file))
    })
    server.listen(0, '127.0.0.1', () => resolve(server))
  })
}

/**
 * app.css as a browser can parse it. `@import 'tailwindcss'` pulls the
 * framework (nothing here needs it) and `@theme` is a Tailwind at-rule a
 * browser drops whole — with the token block inside it. Rewriting the
 * opener to `:root` keeps both the values and their cascade position: in
 * the shipped build @theme compiles into @layer theme, which every
 * unlayered rule already outranks, and an unlayered `:root` is outranked by
 * a later unlayered rule of equal specificity. Either way the locale
 * override below wins, which is the thing under test.
 */
function webCss(src) {
  const out = src.replace(/^@import\s+['"]tailwindcss['"];?\s*$/m, '')
  // Anchored to the start of a line: app.css's own header comment says the
  // word "@theme", and matching that one silently truncates the token block
  // into an unterminated comment (measured 2026-09-07 — the probe reported
  // "Times" for every stack until this was anchored).
  const m = /^[ \t]*@theme[^{]*\{/m.exec(out)
  if (!m) throw new Error('font-probe: no @theme block in app.css')
  return out.slice(0, m.index) + ':root {' + out.slice(m.index + m[0].length)
}

/** tokens.css verbatim, with the bare-specifier @import pointed at the file. */
function siteCss(src) {
  return src.replace(
    /@import\s+['"]pretendard\/([^'"]+)['"];/,
    (_m, rest) => `@import url("/site/node_modules/pretendard/${rest}");`,
  )
}

function probeHtml(cssHref, lang, roles) {
  const rows = roles
    .map((role) =>
      GLYPHS.map(
        (g) =>
          `<span data-role="${role.name}" data-ch="${g.ch}" style="font-family:${role.family};font-size:32px">${g.ch}</span>`,
      ).join(''),
    )
    .join('')
  return `<!doctype html><html lang="${lang}"><head><meta charset="utf-8">
<link rel="stylesheet" href="${cssHref}"></head>
<body><div id="probe">${rows}</div></body></html>`
}

// ── Measure ────────────────────────────────────────────────────────────────
async function measure(browser, url) {
  const page = await browser.newPage()
  const cdp = await page.context().newCDPSession(page)
  await page.goto(url, { waitUntil: 'load' })
  // The webfont @import must land before the first face is picked, or a
  // Pretendard-covered glyph reports its fallback and the row lies.
  await page.evaluate(() => document.fonts.ready)
  await cdp.send('DOM.enable')
  await cdp.send('CSS.enable')
  const { root } = await cdp.send('DOM.getDocument', { depth: -1 })
  const { nodeIds } = await cdp.send('DOM.querySelectorAll', {
    nodeId: root.nodeId,
    selector: '#probe span',
  })
  const stacks = await page.evaluate(() =>
    Object.fromEntries(
      [...new Set([...document.querySelectorAll('#probe span')].map((s) => s.dataset.role))].map((role) => [
        role,
        getComputedStyle(document.querySelector(`#probe span[data-role="${role}"]`)).fontFamily,
      ]),
    ),
  )
  const out = []
  for (const nodeId of nodeIds) {
    const { attributes } = await cdp.send('DOM.getAttributes', { nodeId })
    const attr = {}
    for (let i = 0; i < attributes.length; i += 2) attr[attributes[i]] = attributes[i + 1]
    const { fonts } = await cdp.send('CSS.getPlatformFontsForNode', { nodeId })
    const face = fonts.sort((a, b) => b.glyphCount - a.glyphCount)[0]
    out.push({ role: attr['data-role'], ch: attr['data-ch'], face: face?.familyName ?? '(none)' })
  }
  await page.close()
  return { rows: out, stacks }
}

/**
 * Does this stack name any face for `script` at all? Separates a code
 * defect (nothing named) from an environment gap (named, not installed).
 */
function namesFaceFor(stack, script) {
  return stack
    .split(',')
    .map((f) => f.trim().replace(/^['"]|['"]$/g, ''))
    .some((f) => classifyFace(f) === script)
}

// ── Report ─────────────────────────────────────────────────────────────────
const args = process.argv.slice(2)
const asJson = args.includes('--json')
const strict = args.includes('--strict')
const only = args.includes('--surface') ? args[args.indexOf('--surface') + 1] : null
const surfaces = only ? { [only]: SURFACES[only] } : SURFACES
if (only && !SURFACES[only]) {
  console.error(`font-probe: unknown surface ${only} (have: ${Object.keys(SURFACES).join(', ')})`)
  process.exit(64)
}

const virtual = new Map()
for (const [name, s] of Object.entries(surfaces)) {
  const src = readFileSync(join(ROOT, s.css), 'utf8')
  virtual.set(`/probe/${name}.css`, {
    body: name === 'web' ? webCss(src) : siteCss(src),
    type: 'text/css',
  })
  for (const [loc, lang] of Object.entries(s.langs)) {
    virtual.set(`/probe/${name}.${loc}.html`, {
      body: probeHtml(`/probe/${name}.css`, lang, s.roles),
      type: 'text/html',
    })
  }
}

const server = await serve(virtual)
const port = server.address().port
const browser = await chromium.launch()
const result = {}
try {
  for (const [name, s] of Object.entries(surfaces)) {
    result[name] = { css: s.css, langs: s.langs, roles: {}, stacks: {} }
    for (const loc of Object.keys(s.langs)) {
      const { rows, stacks } = await measure(browser, `http://127.0.0.1:${port}/probe/${name}.${loc}.html`)
      result[name].stacks[loc] = stacks
      for (const r of rows) {
        result[name].roles[r.role] ??= {}
        result[name].roles[r.role][r.ch] ??= {}
        result[name].roles[r.role][r.ch][loc] = r.face
      }
    }
  }
} finally {
  await browser.close()
  server.close()
}

// A finding is one (surface, role, locale, wanted script) that failed.
const findings = []
for (const [name, s] of Object.entries(surfaces)) {
  for (const role of s.roles) {
    for (const loc of Object.keys(s.langs)) {
      const stack = result[name].stacks[loc][role.name] ?? ''
      const wanted = [...new Set(GLYPHS.map((g) => EXPECT[g.script][loc]).filter(Boolean))]
      for (const want of wanted) {
        const bad = GLYPHS.filter(
          (g) =>
            EXPECT[g.script][loc] === want &&
            classifyFace(result[name].roles[role.name][g.ch][loc]) !== want,
        )
        if (!bad.length) continue
        findings.push({
          surface: name,
          role: role.name,
          locale: loc,
          want,
          kind: namesFaceFor(stack, want) ? 'ABSENT' : 'STACK',
          stack,
          glyphs: bad.map((g) => ({
            ch: g.ch,
            note: g.note,
            fellTo: result[name].roles[role.name][g.ch][loc],
            fellToScript: classifyFace(result[name].roles[role.name][g.ch][loc]),
          })),
        })
      }
    }
  }
}

if (asJson) {
  console.log(JSON.stringify({ result, findings }, null, 2))
} else {
  for (const [name, s] of Object.entries(surfaces)) {
    console.log(`\n── ${name} (${s.css}) ─────────────────────────────────────`)
    const locs = Object.keys(s.langs)
    for (const role of s.roles) {
      console.log(`\n  role ${role.name}  ${role.family}`)
      console.log(`    glyph            ${locs.map((l) => `${l} (lang=${s.langs[l]})`.padEnd(22)).join('')}`)
      for (const g of GLYPHS) {
        const cells = locs
          .map((l) => {
            const face = result[name].roles[role.name][g.ch][l]
            return `${face} [${classifyFace(face)}]`.padEnd(22)
          })
          .join('')
        console.log(`    ${g.ch} ${g.note.padEnd(15)}${cells}`)
      }
    }
  }
  console.log('')
  if (!findings.length) {
    console.log('font-probe: every locale draws its own script. OK')
  }
  for (const f of findings) {
    const list = f.glyphs.map((g) => `${g.ch}→${g.fellTo} [${g.fellToScript}]`).join(', ')
    if (f.kind === 'STACK') {
      console.log(
        `font-probe: STACK  ${f.surface}/${f.role} lang=${f.locale}: the stack names no ${f.want} face.\n` +
          `            ${list}\n            stack: ${f.stack}`,
      )
    } else {
      console.log(
        `font-probe: ABSENT ${f.surface}/${f.role} lang=${f.locale}: the stack names a ${f.want} face, none installed here — fell through.\n` +
          `            ${list}\n            stack: ${f.stack}\n` +
          `            install: macOS Japanese language support, or \`brew install --cask font-noto-sans-cjk\`; CI images want fonts-noto-cjk.`,
      )
    }
  }
}

const hasStack = findings.some((f) => f.kind === 'STACK')
const hasAbsent = findings.some((f) => f.kind === 'ABSENT')
process.exit(hasStack ? 1 : hasAbsent && strict ? 2 : 0)
