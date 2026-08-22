import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en } from './en'
import { ja } from './ja'
import { ko } from './ko'

/*
 * GDK-621: one fact, one on-screen spelling — across surfaces that print
 * the same thing from different places. Two contracts live here:
 *
 *   keycap notation — the cheat sheet (ShortcutsDialog.svelte) owns the
 *     glyphs: `↵` for Enter, the chord rendered in code as `${mod} ↵`.
 *     Catalog strings that print keycaps (the composer's kbd chip, the
 *     one-line kbd hints) must use those glyphs, not the word "Enter".
 *
 *   close affordances — an X button's accessible name (aria-label) and its
 *     hover tooltip (title) must be the same string, or a screen reader and
 *     a hovering cursor disagree about the same control.
 *
 * Not here: catalog completeness (catalog.test.ts owns that), and prose
 * that names a key inside a sentence (`list.searchOpen` "Open with Enter"
 * is a sentence, not a keycap run).
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const SHEET = join(HERE, '../../components/shell/ShortcutsDialog.svelte')
const WEB_SRC = join(HERE, '../..')

const CATALOGS = [
  ['en', en],
  ['ko', ko],
  ['ja', ja],
] as const

describe('GDK-621 keycap notation: the cheat sheet is the owner', () => {
  test('every locale prints the submit-comment chord as the sheet does', () => {
    // The sheet renders the chord in code (`${mod} ↵`); the composer's kbd
    // chip renders it from write.commentShortcut. If either side drifts,
    // the same physical key wears two notations on two screens.
    const src = readFileSync(SHEET, 'utf8')
    expect(
      src,
      'ShortcutsDialog must keep the compose row as `[`${mod} ↵`, t(...)]`',
    ).toContain("[`${mod} ↵`, t('shortcuts.submitComment')]")
    for (const [locale, table] of CATALOGS) {
      expect(table['write.commentShortcut'], locale).toBe('{mod} ↵')
    }
  })

  test('one owner of comment-shortcut; both composers render it', () => {
    // GDK-650: the page composer used to omit the chip. The chord is owned
    // by one component (write.commentShortcut + modifierSymbol); issue and
    // page composers both render that owner — a second kbd with a local
    // string is the class this gate closes.
    const files: string[] = []
    const walk = (dir: string): void => {
      for (const name of readdirSync(dir)) {
        const p = join(dir, name)
        if (statSync(p).isDirectory()) walk(p)
        else if (name.endsWith('.svelte')) files.push(p)
      }
    }
    walk(WEB_SRC)
    const owners = files.filter((p) =>
      readFileSync(p, 'utf8').includes('data-testid="comment-shortcut"'),
    )
    expect(owners, `comment-shortcut owners:\n${owners.join('\n')}`).toHaveLength(1)
    const ownerSrc = readFileSync(owners[0], 'utf8')
    expect(ownerSrc).toContain("t('write.commentShortcut'")
    expect(ownerSrc).toContain('modifierSymbol()')

    const ownerBase = owners[0].slice(owners[0].lastIndexOf('/') + 1)
    const composers = [
      join(WEB_SRC, 'components/write/CommentComposer.svelte'),
      join(WEB_SRC, 'components/detail/DocumentPanel.svelte'),
    ]
    for (const c of composers) {
      if (c === owners[0]) continue
      const src = readFileSync(c, 'utf8')
      expect(src, `${c} must render the comment-shortcut owner (${ownerBase})`).toContain(
        ownerBase,
      )
    }
  })

  test('one-line kbd hints use the ↵ glyph, never the word Enter', () => {
    // palette.hintNav (palette footer) and settings.scopeHint (scope
    // picker) are keycap runs in the sheet's grammar. This list is the set
    // of kbd-run strings; a new hint line joins it, a sentence does not.
    const HINTS = ['palette.hintNav', 'settings.scopeHint'] as const
    const failures: string[] = []
    for (const [locale, table] of CATALOGS) {
      for (const key of HINTS) {
        const value = table[key]
        if (/Enter/.test(value)) {
          failures.push(`${locale}.${key} spells the key out: ${JSON.stringify(value)}`)
        }
        if (!value.includes('↵')) {
          failures.push(`${locale}.${key} has no ↵ glyph: ${JSON.stringify(value)}`)
        }
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })
})

describe('GDK-621 close affordances agree with themselves', () => {
  test('a close-family label uses one key for aria-label and title', () => {
    // Adjacent-pair scan: every aria-label={t('…')} looks for the first
    // title={t('…')} on its own line or the next two, and when either key
    // is close-family the two keys must be equal. The codebase formats
    // attributes aria-first (every pair at authoring time); a reversed or
    // far-apart pair is outside this gate.
    const aria = /aria-label=\{t\('([^']+)'\)\}/
    const title = /title=\{t\('([^']+)'\)\}/
    const files: string[] = []
    const walk = (dir: string): void => {
      for (const name of readdirSync(dir)) {
        const p = join(dir, name)
        if (statSync(p).isDirectory()) walk(p)
        else if (name.endsWith('.svelte')) files.push(p)
      }
    }
    walk(WEB_SRC)
    const failures: string[] = []
    for (const p of files) {
      const lines = readFileSync(p, 'utf8').split('\n')
      for (let i = 0; i < lines.length; i++) {
        const ma = aria.exec(lines[i])
        if (!ma) continue
        for (const next of lines.slice(i, i + 3)) {
          const mt = title.exec(next)
          if (!mt) continue
          if (/close/i.test(ma[1]) || /close/i.test(mt[1])) {
            if (ma[1] !== mt[1]) failures.push(`${p}: aria=${ma[1]} vs title=${mt[1]}`)
          }
          break
        }
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })
})
