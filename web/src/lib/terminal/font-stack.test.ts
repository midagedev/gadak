import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

/*
 * 2026-08-28 — GDK-1043 recurrence gate. A Linux CI cannot assert font
 * pixels — fontconfig resolves the same families differently — but it can
 * hold the ORDER that makes box glyphs join on macOS WebKit: Menlo first
 * (its ink overshoots the cell xterm derives; SF Mono undershoots it), and
 * the Hangul gothics before the bare generic so a sans-class fallback can
 * never reach the box glyphs (measured vgaps=150, cell 12.03 when it does).
 * Pinned as text against app.css, which is the single owner of the stack.
 *
 * Read with node:fs, not an `app.css?raw` import: this repo's vitest (4.1.10,
 * unit project) stubs CSS modules to an empty string even through `?raw` —
 * measured 2026-08-28 while writing this test. The spec asked for `?raw`;
 * the file read is the same contract with a mechanism that actually carries
 * bytes.
 */
const appCss = readFileSync(new URL('../../app.css', import.meta.url), 'utf8')

describe('--font-mono-terminal stack contract (GDK-1043)', () => {
  const decl = appCss.match(/--font-mono-terminal:\s*([^;]+);/)
  const stack = decl?.[1] ?? ''

  test('is declared in app.css', () => {
    expect(decl, 'app.css must declare --font-mono-terminal').not.toBeNull()
  })

  test('leads with Menlo, the face whose box ink overshoots the cell on both engines', () => {
    expect(stack.trimStart().startsWith('Menlo')).toBe(true)
  })

  test("lists 'Apple SD Gothic Neo' ahead of the bare monospace generic", () => {
    const cjk = stack.indexOf("'Apple SD Gothic Neo'")
    const generic = stack.search(/,\s*monospace\s*$/)
    expect(cjk).toBeGreaterThan(-1)
    expect(generic).toBeGreaterThan(-1)
    expect(cjk).toBeLessThan(generic)
  })
})
