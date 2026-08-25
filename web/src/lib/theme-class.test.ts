import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

/*
 * GDK-637: a class like `text-danger-text` shipped because the token is not
 * in app.css @theme, so the error line rendered as body copy. Two contracts:
 *
 *   color text-* in class position must name a --color-* @theme token (or a
 *     Tailwind non-color text utility / the type scale). That is the class
 *     of the defect, not just this one misspelling.
 *
 *   `text-danger-` is banned by spelling so the incident cannot hide inside
 *     a comment or a concatenated string the class scanner misses.
 *
 * Not here: palette contrast (tools/theme-check.mjs) and arbitrary text-[Npx]
 * (GDK-129, same file).
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')
const CSS_PATH = join(WEB_SRC, 'app.css')

const NON_COLOR_TEXT = new Set([
  'left',
  'center',
  'right',
  'justify',
  'start',
  'end',
  'wrap',
  'nowrap',
  'balance',
  'pretty',
  'ellipsis',
  'clip',
  'inherit',
  'current',
  'transparent',
  'white',
  'black',
])

/** Tailwind `text-<token>` (optional variant / opacity). */
const TEXT_UTIL =
  /(?:^|[^A-Za-z0-9_-])(?:[a-z0-9-]+:)*text-([a-z][a-z0-9-]*)(?:\/[0-9.]+)?/g

function extractBraceBlock(source: string, openBraceIndex: number): string {
  let depth = 0
  for (let i = openBraceIndex; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}') {
      depth--
      if (depth === 0) return source.slice(openBraceIndex + 1, i)
    }
  }
  return ''
}

function atThemeBlock(css: string): string {
  const idx = css.search(/@theme\b/)
  expect(idx, 'web/src/app.css must declare @theme').toBeGreaterThanOrEqual(0)
  const brace = css.indexOf('{', idx)
  expect(brace, '@theme must open a block').toBeGreaterThan(idx)
  return extractBraceBlock(css, brace)
}

function walkSrc(dir: string, acc: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      walkSrc(p, acc)
      continue
    }
    if ((name.endsWith('.svelte') || name.endsWith('.ts')) && !name.endsWith('.test.ts')) {
      acc.push(p)
    }
  }
  return acc
}

function isCommentLine(line: string): boolean {
  const t = line.trim()
  return t.startsWith('//') || t.startsWith('*') || t.startsWith('<!--')
}

describe('GDK-637 theme text-* classes exist in @theme', () => {
  test('text-<color> in web/src names a --color-* token (or a non-color text utility)', () => {
    const css = readFileSync(CSS_PATH, 'utf8')
    const theme = atThemeBlock(css)
    const colors = new Set(
      [...theme.matchAll(/--color-([a-z0-9-]+)\s*:/gi)].map((m) => m[1]),
    )
    expect(colors.size, '@theme must declare --color-* tokens').toBeGreaterThan(10)
    const textTokens = new Set(
      [...theme.matchAll(/--text-([a-z0-9-]+)\s*:/gi)]
        .map((m) => m[1])
        .filter((n) => !n.includes('--')),
    )
    /*
     * 2026-08-25 — GDK-864 (lead). `--text-terminal` is a `--text-*` token
     * that is deliberately **not a rung on the type ladder**: it sizes the VT
     * grid, which a person sets for themselves, and pulling it onto the
     * ladder would mean growing the terminal also grows the issue list.
     *
     * So it is subtracted here rather than added to the expected list. The
     * assertion below still says "the ladder is exactly these four", which is
     * the thing this gate was written to protect; a fifth *rung* still fails.
     * FAIL-first, measured on CI (run 32850920070, this file):
     *   AssertionError: type-scale tokens: expected [ 'body', 'heading',
     *   'micro', …(2) ] to deeply equal [ 'body', 'heading', 'micro', 'title' ]
     */
    const OFF_LADDER = ['terminal']
    for (const name of OFF_LADDER) {
      expect(textTokens.has(name), `--text-${name} must stay declared in @theme`).toBe(true)
    }
    const typeScale = new Set([...textTokens].filter((n) => !OFF_LADDER.includes(n)))
    expect([...typeScale].sort(), 'type-scale tokens').toEqual(['body', 'heading', 'micro', 'title'])

    const files = walkSrc(WEB_SRC)
    expect(files.length, 'the sweep found no sources to read').toBeGreaterThan(50)

    // textTokens, not typeScale: an off-ladder size is still a real utility,
    // so `text-terminal` in a template must not be reported as unknown.
    const allowed = new Set<string>([...NON_COLOR_TEXT, ...textTokens])
    const failures: string[] = []
    let sawStatusReopen = false
    let sawTextPrimary = false
    for (const file of files) {
      const rel = file.slice(WEB_SRC.length + 1)
      const lines = readFileSync(file, 'utf8').split('\n')
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i]
        if (isCommentLine(line)) continue
        TEXT_UTIL.lastIndex = 0
        let m: RegExpExecArray | null
        while ((m = TEXT_UTIL.exec(line))) {
          // `text-decoration:` is a CSS property, not a utility. The negative
          // lookahead `(?!:)` backtracks into the token (`text-decoratio`), so
          // reject by the character after the full match instead.
          const after = line[m.index + m[0].length]
          if (after === ':') continue
          const token = m[1]
          if (token === 'status-reopen') sawStatusReopen = true
          if (token === 'text-primary') sawTextPrimary = true
          if (allowed.has(token) || colors.has(token)) continue
          failures.push(`${rel}:${i + 1} text-${token}`)
        }
      }
    }
    expect(sawStatusReopen, 'regex must still see text-status-reopen').toBe(true)
    expect(sawTextPrimary, 'regex must still see text-text-primary').toBe(true)
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('text-danger- is gone from web/src (the misspelling that shipped)', () => {
    const files = walkSrc(WEB_SRC)
    const hits: string[] = []
    for (const file of files) {
      const rel = file.slice(WEB_SRC.length + 1)
      const lines = readFileSync(file, 'utf8').split('\n')
      for (let i = 0; i < lines.length; i++) {
        if (lines[i].includes('text-danger-')) hits.push(`${rel}:${i + 1}`)
      }
    }
    expect(hits, hits.join('\n')).toEqual([])
  })
})

describe('GDK-637 page comment POST is owned by the write store', () => {
  test('web/src/components does not import commentOnPage', () => {
    const components = join(WEB_SRC, 'components')
    const files = walkSrc(components)
    expect(files.length, 'no components to read').toBeGreaterThan(20)
    const hits: string[] = []
    for (const file of files) {
      const src = readFileSync(file, 'utf8')
      if (src.includes('commentOnPage')) hits.push(file.slice(WEB_SRC.length + 1))
    }
    expect(hits, `commentOnPage must be called from the write store, found in: ${hits.join(', ')}`).toEqual(
      [],
    )
  })
})
