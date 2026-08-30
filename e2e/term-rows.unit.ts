import { describe, expect, it } from 'vitest'
import { stitchTermRows, type TermRow } from './helpers'

/*
 * The wrap seam (2026-08-30). Five terminal specs read the pane's buffer row
 * by row and joined with '\n'; on the CI runner's 24-column prompt every
 * command they typed crossed the window edge, xterm stored the tail as a
 * separate row, and the specs saw a command cut in half. The product was
 * correct — the read was not.
 *
 * These cases are the buffer as xterm hands it over: `wrapped` is
 * IBufferLine.isWrapped, and a wrapped row's predecessor is full width, so
 * the rows are untrimmed here exactly as readTerm collects them.
 */

/** A full-width row, padded the way translateToString(false) pads. */
function row(text: string, opts: { cols?: number; wrapped?: boolean } = {}): TermRow {
  const cols = opts.cols ?? 24
  return { text: text.padEnd(cols, ' '), wrapped: !!opts.wrapped }
}

describe('stitchTermRows', () => {
  it('joins a wrapped row onto the row it continues, with no newline between', () => {
    const rows: TermRow[] = [
      row("runner@runnervmgx7h7:~$ printf 'GDK1162%s\\n' -RA", { cols: 48 }),
      row('N', { cols: 48, wrapped: true }),
    ]
    const text = stitchTermRows(rows)
    expect(text).toContain("printf 'GDK1162%s\\n' -RAN")
    expect(text).toBe("runner@runnervmgx7h7:~$ printf 'GDK1162%s\\n' -RAN")
  })

  it('keeps a real newline between rows the shell actually ended', () => {
    const rows: TermRow[] = [row('NMA-140'), row('runner@runnervmgx7h7:~$ ')]
    expect(stitchTermRows(rows)).toBe('NMA-140\nrunner@runnervmgx7h7:~$ '.replace(/\s+$/, ''))
    expect(stitchTermRows(rows).split('\n')).toHaveLength(2)
  })

  it('keeps the spaces a wrapped row carries across the seam', () => {
    // The first row ends in a space the shell printed; trimming it before the
    // join would glue two words together and invent a match that is not there.
    const rows: TermRow[] = [row('echo one two ', { cols: 13 }), row('three', { cols: 13, wrapped: true })]
    expect(stitchTermRows(rows)).toBe('echo one two three')
  })

  it('stitches a line that wrapped more than once', () => {
    const rows: TermRow[] = [
      row('aaaa', { cols: 4 }),
      row('bbbb', { cols: 4, wrapped: true }),
      row('cc', { cols: 4, wrapped: true }),
    ]
    expect(stitchTermRows(rows)).toBe('aaaabbbbcc')
  })

  it('trims each stitched line, not each row', () => {
    expect(stitchTermRows([row('done', { cols: 40 })])).toBe('done')
  })

  it('drops a leading wrapped row rather than losing it to no predecessor', () => {
    // Scrollback can start mid-line; there is nothing above to stitch onto,
    // so the row stands as its own line instead of vanishing.
    expect(stitchTermRows([row('tail', { cols: 8, wrapped: true }), row('next', { cols: 8 })])).toBe(
      'tail\nnext',
    )
  })

  it('is empty for an empty buffer', () => {
    expect(stitchTermRows([])).toBe('')
  })
})
