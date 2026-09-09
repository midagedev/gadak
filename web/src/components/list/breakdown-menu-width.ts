/*
 * GDK-1560 (#17): single owner of the breakdown menu's cell geometry.
 *
 * The menu is a two-column grid of grouping axes. It was `w-64` with no
 * wrap rule, which gives each cell
 *   (256 − 12 padding − 4 gap) / 2 − 20 horizontal padding = 100px
 * of text. That fits every English label, so the overflow only appeared in
 * the translated builds: ja 'ソースプロジェクト' is 9 full-width glyphs at
 * --text-body 13px ≈ 117px and ko '복제 원본 프로젝트' ≈ 124px, so both wrapped
 * to a second line and made one row of the grid taller than its neighbour.
 *
 * A wrapped row is the symptom; the class is "a label that outgrows the cell
 * fails silently, in a locale nobody is looking at". So the width lives here
 * as arithmetic over the real catalog rather than as a Tailwind class chosen
 * by eye, the buttons are nowrap (a future overgrowth ellipsizes with its
 * full text in the title, instead of reflowing the grid), and the test beside
 * this file re-derives the requirement from every locale's labels — a new
 * translation that does not fit turns the gate red.
 *
 * Width model, deliberately crude and deliberately pessimistic: a CJK or
 * Hangul syllable is one em, everything else is half. It only has to be an
 * upper bound on the real advance width, and 13px system-stack Latin runs
 * well under 0.5em average.
 */

/** --text-body, from app.css. */
export const BODY_PX = 13

/** Tailwind `w-80` on the menu, `p-1.5` around the grid, `gap-1` between
 *  columns, `px-2.5` inside a button. */
export const MENU_PX = 320
const MENU_PADDING_PX = 12
const COLUMN_GAP_PX = 4
const BUTTON_PADDING_PX = 20

/** Text width available to one option label, in pixels. */
export const CELL_TEXT_PX = (MENU_PX - MENU_PADDING_PX - COLUMN_GAP_PX) / 2 - BUTTON_PADDING_PX

/** Wide (one-em) ranges: CJK ideographs, kana, Hangul, and full-width forms. */
function isWide(cp: number): boolean {
  return (
    (cp >= 0x1100 && cp <= 0x115f) || // Hangul Jamo
    (cp >= 0x2e80 && cp <= 0x303e) || // CJK radicals, punctuation
    (cp >= 0x3041 && cp <= 0x33ff) || // kana, Hangul compat jamo, CJK compat
    (cp >= 0x3400 && cp <= 0x4dbf) || // CJK ext A
    (cp >= 0x4e00 && cp <= 0x9fff) || // CJK unified
    (cp >= 0xa960 && cp <= 0xa97f) || // Hangul Jamo ext A
    (cp >= 0xac00 && cp <= 0xd7a3) || // Hangul syllables
    (cp >= 0xf900 && cp <= 0xfaff) || // CJK compat ideographs
    (cp >= 0xfe30 && cp <= 0xfe6f) || // CJK compat forms
    (cp >= 0xff00 && cp <= 0xff60) || // full-width forms
    (cp >= 0xffe0 && cp <= 0xffe6)
  )
}

/** Pessimistic advance width of a label at --text-body. */
export function labelWidthPx(label: string): number {
  let em = 0
  for (const ch of label) em += isWide(ch.codePointAt(0) ?? 0) ? 1 : 0.5
  return em * BODY_PX
}

/** Does this label render on one line in a menu cell? */
export function fitsCell(label: string): boolean {
  return labelWidthPx(label) <= CELL_TEXT_PX
}
