/*
 * Chrome vocabulary gate (GDK-142 V3/V10/V13/V14, GDK-1093 C-3).
 *
 * The visual audit's finding was not five separate bugs — it was that the same
 * meaning is drawn differently in each file that draws it. So this file does
 * not check five looks; it checks that each piece of vocabulary has exactly
 * one owner, and that the owner's numbers hold in every palette.
 *
 * The prose half of the rule lives in docs/project/UX_PRINCIPLES.md §16.
 */
import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { FOCUSABLE } from './focus-trap'
import { EMPTY_VALUE, INLINE_ACTION } from './chrome'

const SRC = join(dirname(fileURLToPath(import.meta.url)), '..')
const ROOT = join(SRC, '../..')

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n)
    return statSync(p).isDirectory() ? walk(p) : [p]
  })
}
const svelte = walk(join(SRC, 'components')).filter((p) => p.endsWith('.svelte'))

// The palette parser and the colour math are tools/lib/theme-parse.mjs — the
// same module tools/theme-check.mjs and tools/token-catalog.mjs use, so this
// gate can never disagree with them about what a palette is. Loaded through a
// computed URL: a static import would pull an untyped .mjs into svelte-check's
// input set (29 implicit-any errors and a "would overwrite input file" warning).
const { contrast, hexOf, parseAppCss } = (await import(
  /* @vite-ignore */ new URL('../../../tools/lib/theme-parse.mjs', import.meta.url).href
)) as {
  contrast: (a: string, b: string) => number
  hexOf: (pal: Record<string, string>, name: string) => string
  parseAppCss: (css: string) => {
    light: Record<string, string>
    themeNames: string[]
    palettes: Record<string, Record<string, string>>
  }
}
const read = (p: string) => readFileSync(p, 'utf8')
const rel = (p: string) => p.slice(SRC.length + 1)

describe('window seams vs in-surface dividers (GDK-1093 C-3)', () => {
  // A seam between two things that must read as *different windows* is
  // border-strong; a divider *inside* one surface stays border-subtle. The
  // audit measured the dark terminal seam at RGB (39,42,49) on (13,14,16) —
  // border-subtle, which is 1.28:1 against bg-panel and reads as one body.
  const SEAMS: [string, string][] = [
    ['components/terminal/TerminalPane.svelte', 'border-l'],
    ['components/terminal/TerminalPane.svelte', 'border-t border-border-strong'],
    ['components/terminal/TerminalPane.svelte', 'terminal-roster'],
    ['components/shell/Sidebar.svelte', 'issue-sidebar'],
    ['components/shell/LoadingShell.svelte', 'issue-sidebar'],
    ['components/shell/RightPanel.svelte', 'border-border-strong'],
  ]
  for (const [file, near] of SEAMS) {
    it(`${file} — the ${near} seam is border-strong`, () => {
      const line = read(join(SRC, file))
        .split('\n')
        .find((l) => l.includes(near))
      expect(line, `no line containing ${near}`).toBeTruthy()
      expect(line).toContain('border-border-strong')
    })
  }

  it('the two border tokens stay far enough apart to be two words', () => {
    const { light, themeNames, palettes } = parseAppCss(readFileSync(join(SRC, 'app.css'), 'utf8'))
    for (const [name, pal] of [
      ['light', light] as const,
      ...themeNames.map((n) => [n, palettes[n]] as const),
    ]) {
      for (const ground of ['bg-base', 'bg-panel']) {
        const strong = contrast(hexOf(pal, 'border-strong'), hexOf(pal, ground))
        const subtle = contrast(hexOf(pal, 'border-subtle'), hexOf(pal, ground))
        // Measured 2026-09-10: strong 1.77–2.31, subtle 1.26–1.43.
        expect(strong, `${name} border-strong on ${ground}`).toBeGreaterThanOrEqual(1.7)
        expect(strong / subtle, `${name} strong/subtle ratio on ${ground}`).toBeGreaterThan(1.3)
      }
    }
  })
})

describe('meter bars have one owner (GDK-142 V13)', () => {
  // Two bars are deliberately not meters and keep their own markup:
  //  - board/SprintStrip: a stacked partition (every segment is a category,
  //    there is no empty half to make legible).
  //  - shell/Onboarding: an indeterminate activity bar — a pulsing third of a
  //    rail, because the total is unknown and a percent would be dishonest.
  const NOT_METERS = /ui\/MeterBar\.svelte$|board\/SprintStrip\.svelte$|shell\/Onboarding\.svelte$/

  it('no component but MeterBar draws a track', () => {
    const offenders = svelte
      .filter((p) => !NOT_METERS.test(p))
      .filter((p) => /overflow-hidden rounded-full bg-(bg|border|text)-/.test(read(p)))
      .map(rel)
    expect(offenders, 'these re-spell a meter track; use ui/MeterBar.svelte').toEqual([])
  })

  it('the track is legible on every ground in every palette', () => {
    const { light, themeNames, palettes } = parseAppCss(readFileSync(join(SRC, 'app.css'), 'utf8'))
    const track = read(join(SRC, 'components/ui/MeterBar.svelte')).match(
      /overflow-hidden rounded-full bg-(bg-[a-z]+)/,
    )?.[1]
    expect(track, 'MeterBar must name a ground token for its track').toBeTruthy()
    for (const [name, pal] of [
      ['light', light] as const,
      ...themeNames.map((n) => [n, palettes[n]] as const),
    ]) {
      for (const ground of ['bg-base', 'bg-panel', 'bg-elevated']) {
        const v = contrast(hexOf(pal, track!), hexOf(pal, ground))
        // Measured 2026-09-10 for bg-active: 1.28–1.76. The old bg-elevated
        // track scored 1.00 against its own ground — the underline bug.
        expect(v, `${name} track ${track} on ${ground}`).toBeGreaterThanOrEqual(1.25)
      }
    }
  })
})

describe('status dots have one owner (GDK-142 V10)', () => {
  it('no component but StatusDot tints a round dot by category', () => {
    const offenders = svelte
      .filter((p) => !/ui\/StatusDot\.svelte$|list\/IssueRow\.svelte$/.test(p))
      .filter((p) =>
        read(p)
          .split('\n')
          .some((l) => l.includes('categoryMetaOf') && /rounded-full|style:background/.test(l)),
      )
      .map(rel)
    expect(offenders, 'these draw their own dot; use ui/StatusDot.svelte').toEqual([])
  })

  it("IssueRow's lead dot — the one exception — uses the shared size", () => {
    const src = read(join(SRC, 'components/list/IssueRow.svelte'))
    expect(src).toContain('DOT_CLASS')
  })
})

describe('a value and an action are not the same costume (GDK-142 V14)', () => {
  it('nothing spells the empty-value or inline-action classes by hand', () => {
    const offenders = svelte
      .filter((p) => /italic\s+text-text-muted|text-text-muted\s+italic/.test(read(p)))
      .map(rel)
    expect(offenders, 'use EMPTY_VALUE / INLINE_ACTION from lib/chrome.ts').toEqual([])
  })

  it('the two costumes share nothing', () => {
    const empty = EMPTY_VALUE.split(/\s+/)
    const action = INLINE_ACTION.split(/\s+/)
    expect(empty.length).toBeGreaterThan(0)
    expect(action.length).toBeGreaterThan(0)
    expect(empty).toContain('italic')
    expect(action).not.toContain('italic')
    expect(empty.filter((c) => action.includes(c))).toEqual([])
  })
})

describe('the focus trap agrees with roving tabindex (GDK-142 V3)', () => {
  it('FOCUSABLE excludes every tabindex="-1" element, not only generic ones', () => {
    expect(FOCUSABLE.length).toBeGreaterThan(0)
    for (const clause of FOCUSABLE.split(',')) {
      expect(clause, `${clause} would match a roving tabindex="-1" element`).toContain(
        ':not([tabindex="-1"])',
      )
    }
  })
})

// Keeps the gate honest about where it lives.
it('UX_PRINCIPLES carries the prose half of these rules', () => {
  const doc = readFileSync(join(ROOT, 'docs/project/UX_PRINCIPLES.md'), 'utf8')
  for (const anchor of ['window seam', 'meter', 'StatusDot', 'EMPTY_VALUE', 'section label']) {
    expect(doc, `§16 must name ${anchor}`).toContain(anchor)
  }
})

/*
 * GDK-141: a section label is one class, and its hierarchy signal is weight,
 * not case. The utility dialect (micro/500/uppercase/tracked/muted) carried
 * its label-ness in uppercase — a signal Hangul does not have, so once the
 * :lang(ko)/:lang(ja) rules turned the utility off, a Korean section label
 * wore the same costume as metadata. The class owns the whole recipe and
 * weighs 600 in every script; uppercase stays as the Latin bonus, killed
 * per language at the same owner.
 */
describe('section labels: one owner, weight as the signal (GDK-141)', () => {
  const css = read(join(SRC, 'app.css'))

  it('app.css owns the recipe with the numeric contract', () => {
    // Anchor inside the components layer: the :lang kill rule earlier in
    // the file also ends a selector line with ".section-label {".
    const open = css.indexOf('.section-label {', css.indexOf('@layer components'))
    const block = css.slice(open, css.indexOf('}', open))
    expect(block, '.section-label must exist in app.css').toContain('font-size: var(--text-micro)')
    expect(block).toContain('line-height: var(--text-micro--line-height)')
    // 600 — the one signal every script in the product renders (the ko/ja
    // system stacks ship a real SemiBold); 500 was the collapsed hierarchy.
    expect(block).toContain('font-weight: 600')
    expect(block).toContain('letter-spacing: 0.025em')
    expect(block).toContain('text-transform: uppercase')
    expect(block).toContain('color: var(--color-text-muted)')
  })

  it('the CJK case-kill covers the class, not just the utility', () => {
    const kill = css.match(/:lang\(ko\) [^{]*\{[^}]*text-transform: none/)
    expect(kill, 'the :lang(ko) kill rule must exist').toBeTruthy()
    // The kill is unlayered, so it outranks the @layer components recipe.
    expect(css.indexOf('.section-label'), 'recipe lives in a layer (checked below)').toBeGreaterThan(0)
    expect(kill![0]).toContain('.section-label')
    const killJa = css.match(/:lang\(ja\) [^{]*\{[^}]*text-transform: none/)
    expect(killJa![0]).toContain('.section-label')
  })

  it('the utility dialect survives only where a chip justifies it', () => {
    // A chip (doc badge, palette badge) is a rounded badge: shape carries
    // its meaning, so the case-free CJK rendering is fine there. A section
    // heading is text alone — it must carry the class instead.
    const offenders: string[] = []
    for (const p of svelte) {
      const src = read(p)
      for (const m of src.matchAll(/class="([^"]*)"/g)) {
        const attrs = m[1]
        if (/text-micro (?:font-medium )?uppercase tracking-wide text-text-muted/.test(attrs) && !attrs.includes('rounded')) {
          offenders.push(`${rel(p)}: ${attrs.slice(0, 60)}`)
        }
      }
    }
    expect(offenders, 'section headings must use .section-label, not the utility dialect').toEqual([])
  })

  it('the migration actually happened (floor, not the contract)', () => {
    const files = svelte.filter((p) => read(p).includes('section-label')).map(rel)
    // 21 sites in 18 files moved in GDK-141; a floor keeps a silent
    // unmigration visible without pinning every future label.
    expect(files.length).toBeGreaterThanOrEqual(15)
  })
})
