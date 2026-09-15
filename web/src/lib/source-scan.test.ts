import { describe, expect, test } from 'vitest'
import { blankBlockComments, stripComments } from './source-scan'

/*
 * GDK-1878: the trap this file pins is that the naive block-comment regex
 * eats from the slash-star inside `image/*` to the next comment terminator,
 * and a gate built on it passes because it cannot see the file. The FAIL-first
 * test below keeps that measured behaviour asserted, so a future "simpler"
 * stripper has a red waiting for it. (This very comment says "terminator"
 * instead of writing the two characters out — same trap, one line up.)
 */

describe('GDK-1878 source-scan', () => {
  const PICKER = `<input accept="image/*" /><button data-testid="send">send</button><style>/* a */</style>`

  test('FAIL-first, recorded: the naive regex loses data-testid="send" on the picker markup', () => {
    const naive = PICKER.replace(/\/\*[\s\S]*?\*\//g, '')
    // The trap: `/*` in accept="image/*" opens a comment for the naive regex,
    // swallowing the button up to the stylesheet's `*/`.
    expect(naive).not.toContain('data-testid="send"')
  })

  test('stripComments keeps data-testid="send" and removes the real comment', () => {
    const out = stripComments(PICKER)
    expect(out).toContain('data-testid="send"')
    expect(out).not.toContain('/* a */')
  })

  test('blankBlockComments keeps url(a/*.png) and color: red, blanks the trailing comment, and preserves length and newline indices', () => {
    const src = `.x { background: url(a/*.png) }\n.y { color: red } /* note */`
    const out = blankBlockComments(src)
    expect(out).toContain('url(a/*.png)')
    expect(out).toContain('color: red')
    expect(out).not.toContain('note')
    expect(out).toHaveLength(src.length)
    const newlines = (s: string) => s.split('').flatMap((c, i) => (c === '\n' ? [i] : []))
    expect(newlines(out)).toEqual(newlines(src))
  })

  test('a real multi-line block comment on its own lines is blanked with its newlines kept', () => {
    const src = 'a {}\n/* one\n   two */\nb {}'
    const out = blankBlockComments(src)
    expect(out).not.toContain('one')
    expect(out).not.toContain('two')
    expect(out).toBe('a {}\n      \n         \nb {}')
  })

  test('whole-line // comments are removed; a trailing // inside code is kept', () => {
    const src = 'const a = 1 // keep\n// gone\nconst b = 2'
    // The phone's regex clears the line's characters and leaves the empty
    // line behind — identical behaviour is the contract.
    expect(stripComments(src)).toBe('const a = 1 // keep\n\nconst b = 2')
  })
})
