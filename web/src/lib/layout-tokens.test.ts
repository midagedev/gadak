import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'
import {
  LAYOUT_DETAIL_MIN_PX,
  LAYOUT_LIST_MIN_PX,
  LAYOUT_SIDEBAR_PX,
  VIEWPORT_DOCKED_MIN_PX,
  layoutTokenStyle,
} from './viewport-regime'

/*
 * GDK-826: moved from e2e/narrow-clip.spec.ts ("docked track mins sum to
 * VIEWPORT_DOCKED_MIN_PX"). That test booted a browser to read four CSS
 * custom properties off one element — but the properties are an inline
 * style generated from the TS constants (layoutTokenStyle), so the whole
 * chain is ownable here:
 *
 *   viewport-regime.ts derives the token string from the constants whose sum
 *   viewport-regime.test.ts pins → App.svelte mounts it on
 *   [data-testid="issue-layout"] → app.css consumes var(--layout-*) without
 *   restating px. The painted geometry tests (trail at 740/1100) stay in
 *   narrow-clip.spec.ts.
 *
 * GDK-842 (dim wave chunk 2, 2026-08-25): app.css's leftover dimension
 * literals were converged onto var() consumption so the JS-owned tokens and
 * the painted geometry cannot fork a second time. Three contracts were
 * added: raw px may only live in definitions and var() fallbacks, the three
 * CSS-owned maxima are defined exactly once in :root, and every converted
 * declaration resolves to the exact pre-conversion geometry (the visual
 * no-op contract).
 */

const HERE = dirname(fileURLToPath(import.meta.url))

const CSS_TEXT = readFileSync(join(HERE, '../app.css'), 'utf8')
/** app.css with comments blanked in place — offsets and line structure preserved, so the line numbers the assertions quote are real. */
const CSS_CODE = CSS_TEXT.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, ' '))

function lineOf(idx: number): number {
  return CSS_CODE.slice(0, idx).split('\n').length
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/** Brace-matched span of the first `@media …` block opening at `query`. Walker copied from theme-class.test.ts's extractBraceBlock (not exported there, and that file is outside this round's whitelist). */
function mediaBody(query: string): { start: number; end: number; body: string } {
  const at = CSS_CODE.indexOf(query)
  expect(at, `app.css must keep the ${query} block`).toBeGreaterThan(-1)
  const open = CSS_CODE.indexOf('{', at)
  let depth = 0
  for (let i = open; i < CSS_CODE.length; i++) {
    if (CSS_CODE[i] === '{') depth++
    else if (CSS_CODE[i] === '}' && --depth === 0) {
      return { start: open, end: i, body: CSS_CODE.slice(open + 1, i) }
    }
  }
  throw new Error(`unbalanced braces in the ${query} block`)
}

function tokens(): Record<string, number> {
  return Object.fromEntries(
    layoutTokenStyle().split(';').map((decl) => {
      const [k, v] = decl.split(':')
      return [k, parseFloat(v)]
    }),
  )
}

test('layoutTokenStyle carries every track min and they sum to the docked floor', () => {
  const t = tokens()
  expect(t['--layout-sidebar']).toBe(LAYOUT_SIDEBAR_PX)
  expect(t['--layout-list-min']).toBe(LAYOUT_LIST_MIN_PX)
  expect(t['--layout-detail-min']).toBe(LAYOUT_DETAIL_MIN_PX)
  expect(t['--layout-docked-min'], 'CSS --layout-docked-min follows VIEWPORT_DOCKED_MIN_PX').toBe(
    VIEWPORT_DOCKED_MIN_PX,
  )
  expect(
    t['--layout-sidebar'] + t['--layout-list-min'] + t['--layout-detail-min'],
    `sidebar ${t['--layout-sidebar']} + list ${t['--layout-list-min']} + detail ${t['--layout-detail-min']} must equal docked ${t['--layout-docked-min']} (was 272+390+440=1102 vs 1100)`,
  ).toBe(t['--layout-docked-min'])
})

test('App mounts the token style on the issue-layout element', () => {
  const app = readFileSync(join(HERE, '../App.svelte'), 'utf8')
  const idx = app.indexOf('data-testid="issue-layout"')
  expect(idx, 'App must render [data-testid="issue-layout"]').toBeGreaterThan(-1)
  const tag = app.slice(Math.max(0, idx - 200), idx + 200)
  expect(tag, 'the layout element carries the generated token style').toContain(
    'style={layoutTokenStyle()}',
  )
})

test('layout dimension literals live only in definitions and var() fallbacks', () => {
  /*
   * Pre-GDK-842 app.css forked the sidebar width five times as a raw 272px
   * (closed grid, .issue-sidebar, .browse-pane inset, the 1600px browse
   * grid, .browse-reentry) while the detail-open/overlay grids consumed
   * var(--layout-sidebar) — a token change would have split the layout in
   * half. Every consumption now goes through var(); a raw px may only
   * appear where a value is declared: the :root maxima, the 760px narrow
   * step, or a var() fallback.
   */
  const narrow = mediaBody('@media (max-width: 760px)')
  const offenders: string[] = []
  for (const m of CSS_CODE.matchAll(/(?:^|[^\w.-])(272|208|560|720|2200)px/g)) {
    const idx = m.index ?? 0
    const before = CSS_CODE.slice(Math.max(0, idx - 48), idx)
    if (/var\(--layout-[a-z-]+,\s*$/.test(before)) continue // var() fallback
    const declStart = Math.max(CSS_CODE.lastIndexOf(';', idx), CSS_CODE.lastIndexOf('{', idx))
    const defined = /(--layout-[a-z-]+):\s*$/.exec(CSS_CODE.slice(declStart + 1, idx))
    if (defined) {
      const name = defined[1]
      if (
        name === '--layout-detail-max' ||
        name === '--layout-overlay-max' ||
        name === '--layout-shell-max'
      ) {
        continue // :root base of a CSS-owned maximum
      }
      if (name === '--layout-sidebar' && m[1] === '208' && idx > narrow.start && idx < narrow.end) {
        continue // the 760px narrow step
      }
    }
    offenders.push(`line ${lineOf(idx)}: …${before.trim().split('\n').pop()} ${m[0]}`)
  }
  expect(offenders, 'raw layout px outside definitions/fallbacks (GDK-842 consumption ban)').toEqual(
    [],
  )
})

test('the CSS-owned layout maxima are defined exactly once, in :root', () => {
  /*
   * GDK-842 gives these three caps a CSS owner — unlike the four track
   * tokens, which viewport-regime.ts owns and app.css must never define.
   * One definition in one :root rule, so a future user-token style has a
   * single base to override. Defaults match the chunk-1 dim catalog
   * (internal/config/tokencheck/dim-catalog.json).
   */
  for (const [name, value] of [
    ['--layout-detail-max', '720px'],
    ['--layout-overlay-max', '560px'],
    ['--layout-shell-max', '2200px'],
  ] as const) {
    const hits = [...CSS_CODE.matchAll(new RegExp(`${escapeRe(name)}:\\s*${value}`, 'g'))]
    expect(hits.length, `${name} is defined exactly once`).toBe(1)
    const idx = hits[0]?.index ?? -1
    const open = CSS_CODE.lastIndexOf('{', idx)
    const enclosing = CSS_CODE.lastIndexOf('{', open - 1)
    const selector = CSS_CODE.slice(enclosing + 1, open).trim()
    expect(selector, `${name} sits directly in a :root rule`).toBe(':root')
  }
})

test('converted declarations resolve to the exact pre-conversion geometry', () => {
  /*
   * GDK-842's visual no-op contract: converging the literals onto var()
   * consumption must not move one track. Each converted declaration is
   * resolved with the values layoutTokenStyle() installs inline (plus the
   * CSS-owned maxima and the 760px narrow step) and pinned to the literal
   * strings that shipped before the conversion. Measured equivalent in
   * Chromium at 740px before the edit landed: grid "272px 452px", sidebar
   * 208px, browse-pane left 208px, re-entry left 224px, panel 468px.
   */
  const base: Record<string, string> = {
    '--layout-sidebar': `${LAYOUT_SIDEBAR_PX}px`,
    '--layout-list-min': `${LAYOUT_LIST_MIN_PX}px`,
    '--layout-detail-min': `${LAYOUT_DETAIL_MIN_PX}px`,
    '--layout-docked-min': `${VIEWPORT_DOCKED_MIN_PX}px`,
    '--layout-detail-max': '720px',
    '--layout-overlay-max': '560px',
    '--layout-shell-max': '2200px',
  }
  const narrowVars = { ...base, '--layout-sidebar': '208px' }
  const resolve = (decl: string, vars: Record<string, string> = base) =>
    decl.replace(/var\((--layout-[a-z-]+)(?:,\s*[^)]*)?\)/g, (_, name) => {
      const v = vars[name]
      if (!v) throw new Error(`no resolved value for ${name}`)
      return v
    })
  /** First `prop` declaration in the first rule matching `selector` inside `scope`. */
  const declOf = (scope: string, selector: string, prop: string): string => {
    const rule = new RegExp(`${escapeRe(selector)}[^{}]*\\{([^}]*)\\}`).exec(scope)
    expect(rule, `app.css must keep a ${selector} rule`).toBeTruthy()
    const decl = new RegExp(`${escapeRe(prop)}:\\s*([^;]+);`).exec(rule?.[1] ?? '')
    expect(decl, `${selector} must declare ${prop}`).toBeTruthy()
    return (decl?.[1] ?? '').trim().replace(/\s+/g, ' ')
  }
  const narrow = mediaBody('@media (max-width: 760px)').body
  const wide1600 = mediaBody('@media (min-width: 1600px)').body

  expect(resolve(declOf(CSS_CODE, '.issue-layout', 'grid-template-columns'))).toBe(
    '272px minmax(0, 1360px) minmax(0, 1fr)',
  )
  expect(resolve(declOf(CSS_CODE, '.issue-layout.detail-open', 'grid-template-columns'))).toBe(
    '272px minmax(390px, 1fr) clamp(438px, 34vw, 720px)',
  )
  expect(resolve(declOf(wide1600, '.issue-layout.browse-open', 'grid-template-columns'))).toBe(
    '272px clamp(640px, 40vw, 800px) minmax(0, 1fr)',
  )
  expect(resolve(declOf(CSS_CODE, '.issue-sidebar', 'width'))).toBe('272px')
  expect(resolve(declOf(CSS_CODE, '.browse-pane', 'inset'))).toBe('0 0 0 272px')
  expect(resolve(declOf(CSS_CODE, '.browse-reentry', 'left'))).toBe('calc(272px + 1rem)')
  expect(resolve(declOf(CSS_CODE, '.issue-layout', 'max-width'))).toBe('2200px')
  expect(
    resolve(
      declOf(
        CSS_CODE,
        ".issue-layout[data-viewport-regime='overlay'] .issue-detail-panel",
        'width',
      ),
    ),
  ).toBe('min(560px, calc(100vw - 272px))')

  // Narrow step (≤760px). The step redefines the token on the consuming
  // elements because the inline install on .issue-layout out-ranks any
  // redeclaration this block could place there — so on .issue-layout itself
  // the grid still resolves the inline 272px, exactly what painted before
  // the conversion, where the literal 208px declaration was already
  // out-ranked by the data-viewport-regime overlay rules. Handing the step
  // to JS is chunk 3's move (viewport-regime.ts owns the token).
  expect(resolve(declOf(narrow, '.issue-layout', 'grid-template-columns'))).toBe(
    '272px minmax(0, 1fr)',
  )
  expect(resolve(declOf(narrow, '.issue-sidebar', 'width'), narrowVars)).toBe('208px')
  expect(resolve(declOf(narrow, '.browse-pane', 'left'), narrowVars)).toBe('208px')
  expect(resolve(declOf(narrow, '.browse-reentry', 'left'), narrowVars)).toBe('calc(208px + 1rem)')
})

test('app.css consumes the tokens and never restates their px', () => {
  // viewport-regime.ts's own rule ("CSS must not restate the px"): a px
  // definition of a JS-owned track token would fork the floor a second time.
  expect(CSS_CODE, 'the docked grid sizes its tracks from the tokens').toContain(
    'minmax(var(--layout-list-min), 1fr)',
  )
  /*
   * GDK-842 refinement (2026-08-25), not a relaxation. The pre-refinement
   * ban /--layout-[a-z-]+:\s*\d/ covered every --layout-* name by spelling;
   * its intent (see the GDK-201 header in viewport-regime.ts) was only that
   * CSS must not fork the four JS-owned track tokens. The day the three
   * CSS-owned maxima landed in :root, that ban failed on them — measured:
   * this test went red on --layout-detail-max/-overlay-max/-shell-max and
   * the 208px narrow step before this refinement. Precise form: the four
   * JS-owned names may not be defined anywhere except the narrow-step
   * redefinition of --layout-sidebar inside the max-width: 760px block
   * (which by design sits on the consuming elements — the inline install on
   * .issue-layout out-ranks anything placed there). The CSS-owned maxima
   * are pinned by their own test above.
   */
  const narrow = mediaBody('@media (max-width: 760px)')
  expect(narrow.body, 'the narrow step redefines the sidebar token in place').toContain(
    '--layout-sidebar: 208px',
  )
  const outsideNarrow =
    CSS_CODE.slice(0, narrow.start) + ' '.repeat(narrow.end - narrow.start) + CSS_CODE.slice(narrow.end)
  expect(
    outsideNarrow,
    'the four JS-owned track tokens are never defined outside the narrow step',
  ).not.toMatch(/--layout-(sidebar|list-min|detail-min|docked-min):\s*[\d.]/)
})

test('the boot shell consumes the same tokens the app does (GDK-842 chunk 3)', () => {
  /*
   * index.html paints a sidebar and list rows before the bundle arrives.
   * Those two literals were the last geometry a token change could not
   * reach: a user with --layout-sidebar 300px saw a 272px flash, and a
   * --spacing-row override moved every row except the boot ones. The boot
   * style block now consumes var() with the shipped defaults as fallbacks —
   * byte-identical paint for everyone without overrides (the boot script's
   * user-token :root rule is unlayered, so it reaches this block), and a
   * cached override repaints the skeleton before first paint.
   */
  const html = readFileSync(join(HERE, '../../index.html'), 'utf8')
  const start = html.indexOf('.boot-sidebar')
  const end = html.indexOf('@keyframes')
  expect(start, 'index.html must keep a .boot-sidebar rule').toBeGreaterThan(-1)
  expect(end, 'index.html must keep the boot keyframes after the shell rules').toBeGreaterThan(
    start,
  )
  const boot = html.slice(start, end)

  const sidebarVar = `var(--layout-sidebar, ${LAYOUT_SIDEBAR_PX}px)`
  expect(boot, 'sidebar width consumes the token').toContain(`width: ${sidebarVar}`)
  expect(boot, 'flex-basis consumes the token too (it wins for flex items)').toContain(
    `flex: 0 0 ${sidebarVar}`,
  )
  expect(boot, 'boot rows consume the spacing token').toContain('height: var(--spacing-row, 42px)')

  const stripped = boot.replaceAll(sidebarVar, '').replaceAll('var(--spacing-row, 42px)', '')
  expect(stripped, 'no raw 272px survives outside the var() fallbacks').not.toContain('272px')
  expect(stripped, 'no raw 42px survives outside the var() fallbacks').not.toContain('42px')
})
