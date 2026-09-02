/*
 * GDK-1357: the dock appearance store's pure half. The attribute and the
 * write-through are held by e2e/terminal-theme.spec.ts in a real browser;
 * this pins the parse (unknown → dark, the product default) and that the
 * store is a no-op without a document, like lib/theme.ts.
 */
import { describe, expect, test } from 'vitest'
import {
  TERMINAL_APPEARANCES,
  applyTerminalAppearance,
  hydrateTerminalAppearance,
  parseTerminalAppearance,
  readTerminalAppearance,
} from './appearance'

describe('terminal appearance', () => {
  test('unknown and missing values are dark', () => {
    expect(parseTerminalAppearance(undefined)).toBe('dark')
    expect(parseTerminalAppearance(null)).toBe('dark')
    expect(parseTerminalAppearance('light')).toBe('dark')
    expect(parseTerminalAppearance('follow')).toBe('follow')
    expect(parseTerminalAppearance('dark')).toBe('dark')
  })

  test('the picker list is exactly the two the server validates', () => {
    expect(TERMINAL_APPEARANCES.map((a) => a.name)).toEqual(['dark', 'follow'])
  })

  test('no document, no localStorage: reads dark and applies nothing', () => {
    expect(readTerminalAppearance()).toBe('dark')
    expect(() => applyTerminalAppearance('follow')).not.toThrow()
    expect(() => hydrateTerminalAppearance({})).not.toThrow()
    expect(() => hydrateTerminalAppearance({ appearance: { terminal: 'follow' } })).not.toThrow()
  })
})
