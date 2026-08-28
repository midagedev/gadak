import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { messages } from './i18n/catalog'
import {
  COMMANDS,
  collectLabelKeys,
  dumpKeyBindings,
  duplicateBindingKeys,
  helpSections,
  resolveGlobalKey,
  keyContext,
  type CommandDef,
  type MessageKey,
} from './commands'

const HERE = dirname(fileURLToPath(import.meta.url))
const KEYMAP = join(HERE, 'keymap.svelte.ts')
const PALETTE = join(HERE, '../components/palette/CommandPalette.svelte')
const SHEET = join(HERE, '../components/shell/ShortcutsDialog.svelte')

describe('command registry integrity', () => {
  test('every registry label key exists in the catalog', () => {
    const missing = collectLabelKeys().filter((key) => !(key in messages))
    expect(missing, missing.join('\n')).toEqual([])
  })

  test('no two keymap commands share a chord in the same scope', () => {
    expect(duplicateBindingKeys()).toEqual([])
  })

  test('ShortcutsDialog and CommandPalette derive from the registry', () => {
    const sheet = readFileSync(SHEET, 'utf8')
    const palette = readFileSync(PALETTE, 'utf8')
    const keymap = readFileSync(KEYMAP, 'utf8')
    expect(sheet).toContain('helpSections')
    expect(sheet).not.toContain("['j', t('shortcuts.moveDown')]")
    expect(palette).toContain('paletteActionItems')
    expect(palette).not.toContain("id: 'a:triage-status'")
    expect(palette).not.toContain("kbd: 's'")
    expect(keymap).toContain("from './commands'")
    expect(keymap).not.toContain("key === 's' || key === 'a' || key === 'l' || key === 'p'")
  })
})

describe('integrity checkers FAIL when the contract is violated', () => {
  test('missing i18n key is reported', () => {
    const bad: CommandDef[] = [
      {
        id: 'ghost',
        chords: [],
        help: {
          group: 'global',
          kbd: 'g',
          labelKey: 'shortcuts.doesNotExist' as MessageKey,
          sort: 1,
        },
      },
    ]
    const missing = collectLabelKeys(bad).filter((key) => !(key in messages))
    expect(missing).toContain('shortcuts.doesNotExist')
  })

  test('duplicate chord+scope is reported', () => {
    const dispatch = () => ({ type: 'ignore' as const })
    const bad: CommandDef[] = [
      {
        id: 'one',
        scope: 'list',
        chords: [{ key: 's' }],
        dispatch,
      },
      {
        id: 'two',
        scope: 'list',
        chords: [{ key: 's' }],
        dispatch,
      },
    ]
    expect(duplicateBindingKeys(bad)).toEqual(['s scope=list (one ∩ two)'])
  })
})

describe('dumpKey', () => {
  test('dumpKeyBindings answers what s is bound to', () => {
    const dump = dumpKeyBindings('s')
    expect(dump).toBe(
      [
        's\tlist-status\tscope=list\tkeymap\tpalette:a:triage-status\tlist:shortcuts.listStatus',
        's\tdetail-status\tscope=detail\tkeymap\t-\tdetail:shortcuts.focusStatus',
      ].join('\n'),
    )
  })

  test('helpSections keep the previous group order and compose glyph', () => {
    const sections = helpSections('⌘')
    expect(sections.map((s) => s.titleKey)).toEqual([
      'shortcuts.sectionGlobal',
      'shortcuts.sectionList',
      'shortcuts.sectionColumnViews',
      'shortcuts.sectionDetail',
      'shortcuts.sectionSearch',
      'shortcuts.sectionPalette',
      'shortcuts.sectionCompose',
    ])
    const compose = sections.find((s) => s.titleKey === 'shortcuts.sectionCompose')
    expect(compose?.rows).toEqual([{ kbd: '⌘ ↵', labelKey: 'shortcuts.submitComment' }])
    const global = sections.find((s) => s.titleKey === 'shortcuts.sectionGlobal')
    expect(global?.rows.map((r) => r.kbd)).toEqual(['⌘ K', 'Ctrl+`', ',', 'c', '?', 'Esc'])
    const list = sections.find((s) => s.titleKey === 'shortcuts.sectionList')
    expect(list?.rows.map((r) => r.kbd)).toEqual(['j', 'k', '↵', 'o', 'x', 's', 'p', 'a', 'l', 'c', 'Esc'])
    const detail = sections.find((s) => s.titleKey === 'shortcuts.sectionDetail')
    expect(detail?.rows.map((r) => r.kbd)).toEqual(['o', 'o', 's', 'p', 'a', 'l', 'c'])
  })
})

describe('registry matches previous keymap contracts', () => {
  test('COMMANDS is the owner; count is the census', () => {
    expect(COMMANDS.length).toBeGreaterThanOrEqual(40)
    expect(COMMANDS.filter((c) => c.dispatch).length).toBeGreaterThanOrEqual(20)
    expect(COMMANDS.filter((c) => c.palette).length).toBeGreaterThanOrEqual(20)
  })

  test('resolveGlobalKey still comes from this module', () => {
    expect(resolveGlobalKey(keyContext({ key: '?' }))).toEqual({ type: 'open-shortcuts' })
  })

  /*
   * GDK-945: the registry order is the Esc chain's documentation (GDK-827
   * idiom). The overlay terminal covers the content track, so it takes its
   * Esc before any column view — the X button and Ctrl+` remain the ways to
   * close it from inside the VT, where Esc is the PTY's, not chrome's.
   */
  test('the Esc ladder order is browse, bulk, detail, terminal overlay, feed, dashboard, history, docs (GDK-945)', () => {
    const escLadder = COMMANDS.filter((c) => c.chords.some((ch) => !ch.mod && ch.key === 'Escape')).map(
      (c) => c.id,
    )
    expect(escLadder).toEqual([
      'hide-browse',
      'clear-bulk',
      'clear-selection-esc',
      'close-terminal-overlay',
      'close-feed',
      'close-dashboard',
      'close-history',
      'close-docs',
    ])
  })
})
