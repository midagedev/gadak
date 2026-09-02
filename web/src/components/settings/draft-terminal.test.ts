/*
 * GDK-1357: the Terminal tab's half of the settings form model. PUT
 * replaces the `ui` block whole, so the tab must carry the GET block
 * verbatim and change only its two leaves — and send it only when touched.
 * The display fields always ride along; the server merges them onto the
 * stored block (shell and workingDir are not in the document, GDK-1069).
 */
import { describe, expect, test } from 'vitest'
import { toDraft, toSettings, withTerminalTokens } from './draft'

describe('terminal tab draft', () => {
  test('GET → form: stored values as typed, 0 as empty, px stripped', () => {
    const d = toDraft({
      terminal: { scrollback: 20000, cursorBlink: true },
      ui: { tokens: { type: { terminal: '15px', heading: '24px' }, fonts: { 'mono-terminal': 'Menlo' } } },
    })
    expect(d.terminalScrollbackText).toBe('20000')
    expect(d.terminalCursorBlink).toBe(true)
    expect(d.terminalFontSizeText).toBe('15')
    expect(d.terminalFontFamily).toBe('Menlo')
    expect(d.uiTouched).toBe(false)
    expect(toDraft({ terminal: { scrollback: 0, cursorBlink: false } }).terminalScrollbackText).toBe('')
    expect(toDraft({}).terminalFontSizeText).toBe('')
  })

  test('form → PUT: terminal always, ui only when touched', () => {
    const d = toDraft({ ui: { tokens: { colors: { accent: '#7a4bd0' } } } })
    d.terminalScrollbackText = '900'
    d.terminalCursorBlink = true
    let out = toSettings(d, false)
    expect(out.terminal).toEqual({ scrollback: 900, cursorBlink: true })
    expect(out).not.toHaveProperty('ui')

    d.uiTouched = true
    d.terminalFontSizeText = '15'
    d.terminalFontFamily = ' Menlo, monospace '
    out = toSettings(d, false)
    expect(out.ui).toEqual({
      tokens: {
        colors: { accent: '#7a4bd0' },
        type: { terminal: '15px' },
        fonts: { 'mono-terminal': 'Menlo, monospace' },
      },
    })
    // Neither of the two leaves is ever sent as an absolute path or a shell.
    expect(JSON.stringify(out.terminal)).not.toMatch(/shell|workingDir/)
  })

  test('withTerminalTokens keeps the rest of the block and drops empty axes', () => {
    const ui = {
      tokens: { colors: { accent: '#7a4bd0' }, type: { terminal: '15px', heading: '24px' } },
      dataColors: { status: { done: '#00aa00' } },
    }
    const cleared = withTerminalTokens(ui, '', '')
    expect(cleared).toEqual({
      tokens: { colors: { accent: '#7a4bd0' }, type: { heading: '24px' } },
      dataColors: { status: { done: '#00aa00' } },
    })
    // The caller's object is untouched.
    expect(ui.tokens.type.terminal).toBe('15px')
    expect(withTerminalTokens(undefined, '', '')).toEqual({})
    expect(withTerminalTokens(undefined, '12', 'Menlo')).toEqual({
      tokens: { type: { terminal: '12px' }, fonts: { 'mono-terminal': 'Menlo' } },
    })
    // A unit the user typed themselves is passed through for the server to judge.
    expect(withTerminalTokens(undefined, '1.2rem', '').tokens?.type?.terminal).toBe('1.2rem')
    // <input type="number"> binds a number, not a string (the e2e caught
    // `e.trim is not a function` on Save).
    expect(
      withTerminalTokens(undefined, 15 as unknown as string, '').tokens?.type?.terminal,
    ).toBe('15px')
  })
})
