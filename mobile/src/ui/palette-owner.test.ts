import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * GDK-1949 — the palette's owner rows say where they lead.
 *
 * The owner list mixed two kinds of row in one column: scope rows (views,
 * projects) carrying a match count on the right, and owner rows (Sprints,
 * Terminal) carrying nothing there — so the one empty cell read as a number
 * that failed to load rather than a number that does not exist. The rule
 * pinned here is the shape of the fix, not the pixels: every row of the
 * owner list is either a scope row carrying a count or an owner row
 * carrying the disclosure chevron — no row has an empty right slot. A test
 * that only checked "Sprints has a chevron" would not close the class; the
 * next owner row added would reopen it.
 *
 * Same style as this tree's other component tests (KeyBar, detail-sheets):
 * source contracts over the .svelte, no DOM mount harness. Geometry stays
 * in the viewport gate.
 */

const here = dirname(fileURLToPath(import.meta.url))
const palette = readFileSync(join(here, 'Palette.svelte'), 'utf8')
const deskRow = readFileSync(join(here, 'DeskRow.svelte'), 'utf8')

/** The owner list only: the column the finding is about. Search mode ranks
 *  matches and is uniformly count-less, so it has no single gap cell. */
const ownerList = palette.slice(
  palette.indexOf("{#if mode === 'empty'}"),
  palette.indexOf("{:else if mode === 'short'}"),
)

/** Every `<button class="palette-row" …>…</button>` in the owner list. */
function ownerRows(): string[] {
  const rows: string[] = []
  const re = /<button\s+class="palette-row"[\s\S]*?<\/button>/g
  let m: RegExpExecArray | null
  while ((m = re.exec(ownerList)) !== null) rows.push(m[0])
  return rows
}

describe('GDK-1949 — no owner-list row has an empty right slot', () => {
  it('every palette row carries a count or the disclosure chevron', () => {
    const rows = ownerRows()
    // Scope rows + Sprints + Terminal. If this number moves, the new row
    // must satisfy the rule too — that is the point of counting here.
    expect(rows.length).toBeGreaterThanOrEqual(3)
    for (const row of rows) {
      const name = row.match(/\{[^}]*\.name\}|\{t\('[^']*'\)\}/)?.[0] ?? row.slice(0, 60)
      const hasCount = row.includes('class="n"')
      const hasChevron = row.includes('m9 18 6-6-6-6')
      expect(hasCount || hasChevron, `row with no right slot: ${name}`).toBe(true)
    }
  })

  it('the scope rows still earn their count through the null branch', () => {
    // The fix must not invent a sprint count beside issue counts (two units
    // in one column). Scope rows keep `{#if n !== null}`; owner rows take
    // the chevron instead.
    expect(palette).toContain('{#if n !== null}')
  })

  it('the chevron is decorative and reuses the muted ink', () => {
    const svgs = palette.match(/<svg(\s[^>]*)?>[\s\S]*?<\/svg>/g) ?? []
    const chevrons = svgs.filter((s) => s.includes('m9 18 6-6-6-6'))
    expect(chevrons.length).toBeGreaterThanOrEqual(2)
    for (const svg of chevrons) {
      expect(svg).toContain('aria-hidden="true"')
      expect(svg).not.toMatch(/tabindex|focusable/)
    }
    // Same right-edge slot idiom as ListSheet's scope-row chevron (16px,
    // muted), and the muted ink the count itself uses — no new colour.
    expect(palette).toMatch(/\.palette-row \.go\s*\{[^}]*color:\s*var\(--color-text-muted\)/)
  })

  it('desk rows keep their own trailing affordance and take no chevron', () => {
    // A desk row means "this is the desk's job", not "tap to go here" — a
    // chevron would claim navigation the inert row does not offer.
    expect(deskRow).toContain('class="why"')
    expect(deskRow).not.toContain('m9 18 6-6-6-6')
  })
})
