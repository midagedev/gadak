/*
 * The single owner of "what does the terminal pane say" (GDK-1567).
 *
 * This module is deliberately import-free: mobile/e2e compiles against its own
 * @playwright/test install, so the page parameter is a structural type naming
 * only the surface readTerm touches. e2e/helpers.ts re-exports everything
 * here, so specs that already import from './helpers' keep working — but new
 * terminal code should import from the owner, and `translateToString(` may
 * appear nowhere else in e2e/, mobile/e2e/, or web/src/ (held by
 * e2e/term-read-owner.unit.ts).
 *
 * Why one owner at all: four copies of a readTerm that collected rows without
 * stitching xterm's wrapped continuations passed on a developer's short macOS
 * prompt and failed on the CI runner's 24-column one (2026-08-30 — five
 * terminal specs red in CI only, `e2e/ci-shell.sh` reproduces the runner).
 * Any second copy re-imports that fork, because the fix lives here.
 */

/**
 * Structural stand-in for Playwright's `Page` — the owner must stay importable
 * from mobile/e2e, whose own @playwright/test is a separate install. Both the
 * root and the mobile Page satisfy this.
 */
export interface TermPageLike {
  evaluate<R>(pageFunction: () => R): Promise<R>
}

/**
 * One row of the terminal buffer, as `readTerm` collects it.
 *
 * `text` is the row untrimmed — full width, padded with spaces — because a
 * row that continues into the next one must not lose the spaces at its end.
 * `wrapped` is xterm's own `IBufferLine.isWrapped`.
 */
export interface TermRow {
  text: string
  wrapped: boolean
}

/**
 * Buffer rows as the shell wrote them: continuation rows stitched back onto
 * the row they continue, and only then trimmed.
 *
 * xterm stores a line too long for the window as several rows and marks each
 * continuation `isWrapped`; no newline was ever printed there. Reading the
 * buffer row by row and joining with '\n' therefore invents line breaks
 * wherever the terminal ran out of columns — and how many columns are left
 * depends on how wide the shell's prompt is. That is what made five terminal
 * specs pass on a developer's short macOS prompt and fail on the CI runner's
 * 24-column one (2026-08-30): `printf 'GDK1162%s\n' -RAN` arrived as
 * `printf 'GDK1162%s\n' -RA` + '\n' + `N`, and every `toContain(COMMAND)`
 * missed. Pure and exported so `e2e/term-rows.unit.ts` can hold it to that
 * without a browser.
 */
export function stitchTermRows(rows: readonly TermRow[]): string {
  const lines: string[] = []
  for (const row of rows) {
    if (row.wrapped && lines.length > 0) lines[lines.length - 1] += row.text
    else lines.push(row.text)
  }
  return lines.map((line) => line.replace(/\s+$/, '')).join('\n')
}

/**
 * The terminal pane's buffer as text — the single owner for "what does the
 * pane say", so a spec never has to know what a wrapped row is.
 *
 * `__gadakTerm` is the pane's own hook (renderer.ts). The evaluate half only
 * collects rows; the stitching is stitchTermRows above, in node, where a unit
 * test can reach it.
 */
export async function readTerm(page: TermPageLike): Promise<string> {
  const rows = await page.evaluate(() => {
    type Hook = {
      buffer: {
        active: {
          length: number
          getLine: (
            y: number,
          ) => { translateToString: (trimRight?: boolean) => string; isWrapped: boolean } | undefined
        }
      }
    }
    const t = (window as unknown as { __gadakTerm?: Hook }).__gadakTerm
    if (!t) return [] as { text: string; wrapped: boolean }[]
    const buf = t.buffer.active
    const out: { text: string; wrapped: boolean }[] = []
    for (let i = 0; i < buf.length; i++) {
      const line = buf.getLine(i)
      // Trim every row except one whose successor continues it: those spaces
      // are inside the line the shell wrote, and dropping them would glue two
      // words together across the seam. Trimming the rest keeps a 30k-line
      // scrollback (terminal-burst) from crossing the bridge padded to full
      // width on every poll. stitchTermRows trims the stitched line anyway.
      const continues = !!buf.getLine(i + 1)?.isWrapped
      out.push({ text: line?.translateToString(!continues) ?? '', wrapped: !!line?.isWrapped })
    }
    return out
  })
  return stitchTermRows(rows)
}

/**
 * The same buffer, split at the line breaks the shell actually printed —
 * wraps stitched first, so a line is one entry however wide the prompt was.
 */
export async function readTermLines(page: TermPageLike): Promise<string[]> {
  return (await readTerm(page)).split('\n')
}
