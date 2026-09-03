/*
 * GDK-1357: the dock appearance store's pure half. The attribute is held
 * by e2e/terminal-theme.spec.ts in a real browser; this pins the parse
 * (unknown → dark, the product default), that the store is a no-op without
 * a document, like lib/theme.ts — and the write-through contract that a
 * theme click and a dock-appearance click each spread the other's field
 * and serialize on one chain (GDK-1376: that was an e2e, and it is logic).
 */
import { beforeEach, describe, expect, test, vi } from 'vitest'
import {
  TERMINAL_APPEARANCES,
  applyTerminalAppearance,
  hydrateTerminalAppearance,
  parseTerminalAppearance,
  persistTerminalAppearance,
  readTerminalAppearance,
} from './appearance'
import { persistThemePreference } from '../theme'
import type { GadakSettings } from '../api'

// The settings document as the server holds it; each write-through GETs it
// and PUTs it back whole, so the mock is a tiny server.
let serverDoc: GadakSettings = {} as GadakSettings
vi.mock('../api', () => ({
  getSettings: vi.fn(async () => serverDoc),
  putSettings: vi.fn(async (doc: GadakSettings) => {
    serverDoc = doc
    return doc
  }),
}))
vi.mock('../config', () => ({ isHostedDemo: () => false }))

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

  describe('write-through keeps the sibling field (GDK-1357)', () => {
    beforeEach(() => {
      serverDoc = { appearance: { theme: 'light', terminal: 'follow' } } as GadakSettings
    })

    test('a theme click does not reset the dock appearance', async () => {
      await persistThemePreference('ember')
      expect(serverDoc.appearance).toEqual({ theme: 'ember', terminal: 'follow' })
    })

    test('a dock-appearance click does not reset the theme', async () => {
      await persistTerminalAppearance('dark')
      expect(serverDoc.appearance).toEqual({ theme: 'light', terminal: 'dark' })
    })

    test('two clicks in a row both land — one chain, each from the last document', async () => {
      await Promise.all([persistThemePreference('ink'), persistTerminalAppearance('dark')])
      expect(serverDoc.appearance).toEqual({ theme: 'ink', terminal: 'dark' })
    })
  })
})
