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
 *   keycap notation — the command registry (lib/commands.ts) owns the
 *     glyphs: `↵` for Enter, the chord rendered as `{mod} ↵`. The sheet
 *     renders those rows; catalog strings that print keycaps (the
 *     composer's kbd chip, the one-line kbd hints) must use those glyphs,
 *     not the word "Enter".
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
const COMMANDS = join(HERE, '../commands.ts')
const WEB_SRC = join(HERE, '../..')

const CATALOGS = [
  ['en', en],
  ['ko', ko],
  ['ja', ja],
] as const

describe('GDK-621 keycap notation: the cheat sheet is the owner', () => {
  test('every locale prints the submit-comment chord as the sheet does', () => {
    // GDK-674: the registry owns the compose row (`{mod} ↵`); the sheet
    // renders helpSections(). The composer's kbd chip still renders
    // write.commentShortcut. If either side drifts, the same physical key
    // wears two notations on two screens.
    const src = readFileSync(COMMANDS, 'utf8')
    expect(src, 'registry must keep the compose chord as `{mod} ↵`').toContain("kbd: '{mod} ↵'")
    expect(src).toContain("'shortcuts.submitComment'")
    expect(readFileSync(SHEET, 'utf8')).toContain('helpSections')
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

const PALETTE = join(WEB_SRC, 'components/palette/CommandPalette.svelte')
const FEED = join(WEB_SRC, 'components/personal/PersonalFeed.svelte')
const NEED_CREDENTIAL_KEYS = [
  'personal.needCredentials',
  'feed.needCredentials',
  'write.commentNeedCredentials',
  'doc.commentNeedCredentials',
  // GDK-731: the write-gate recovery used "token" on two surfaces.
  'write.needToken',
  'sidebar.jiraCredsMissing',
  // GDK-831: the docs-fetch recovery is the same credentials check, and the
  // hint predated the lock — it still said token/토큰/トークン.
  'sidebar.docsFetchFailedHint',
] as const

describe('GDK-651 palette registers sibling views and cursor issue actions', () => {
  test('CommandPalette owns a:docs, a:feed, a:favorite, a:watch', () => {
    // GDK-674: ids and label keys live on the registry; the palette still
    // wires favorites/watches/feed through the host it passes in.
    const registry = readFileSync(COMMANDS, 'utf8')
    for (const id of ["id: 'a:docs'", "id: 'a:feed'", "id: 'a:favorite'", "id: 'a:watch'"]) {
      expect(registry, `missing ${id}`).toContain(id)
    }
    expect(registry).toContain("'palette.actionDocs'")
    expect(registry).toContain("'palette.actionFeed'")
    expect(registry).toContain("'palette.actionFavorite'")
    expect(registry).toContain("'palette.actionWatch'")
    const src = readFileSync(PALETTE, 'utf8')
    expect(src).toContain('favorites.toggle')
    expect(src).toContain('watches.toggle')
    expect(src).toContain('me.openFeed')
    expect(src).toContain("feature('feed')")
    expect(src).toContain('me.identified')
  })
})

describe('GDK-651 credentials affordance: one noun, one settings action', () => {
  test('need-credentials surfaces use the credentials noun, not token', () => {
    const noun = /credential|자격증명|資格情報/i
    const token = /token|토큰|トークン/i
    const failures: string[] = []
    for (const [locale, table] of CATALOGS) {
      for (const key of NEED_CREDENTIAL_KEYS) {
        const value = table[key]
        if (!noun.test(value)) {
          failures.push(`${locale}.${key} has no credentials noun: ${JSON.stringify(value)}`)
        }
        if (token.test(value)) {
          failures.push(`${locale}.${key} still names a token: ${JSON.stringify(value)}`)
        }
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('PersonalFeed empty-identity state uses common.setCredentials to open settings', () => {
    const src = readFileSync(FEED, 'utf8')
    expect(src).toContain("t('feed.needCredentials')")
    expect(src).toContain("t('common.setCredentials')")
    expect(src).toContain('write.openSettings')
    expect(src).toContain('actionLabel')
    expect(src).toContain('onAction')
  })
})

describe('GDK-651 cheat sheet documents Tab on column views', () => {
  test('ShortcutsDialog names Tab and Esc for documents, history, and feed', () => {
    const src = readFileSync(COMMANDS, 'utf8')
    expect(src).toContain("'shortcuts.sectionColumnViews'")
    expect(src).toContain("'shortcuts.tabMoveRows'")
    expect(src).toContain("'shortcuts.closeColumnView'")
    expect(readFileSync(SHEET, 'utf8')).toContain('helpSections')
  })
})

function svelteFiles(): string[] {
  const files: string[] = []
  const walk = (dir: string): void => {
    for (const name of readdirSync(dir)) {
      const p = join(dir, name)
      if (statSync(p).isDirectory()) walk(p)
      else if (name.endsWith('.svelte')) files.push(p)
    }
  }
  walk(WEB_SRC)
  return files
}

const KBD_CHIP_CLASS = 'rounded border border-border-subtle px-1 text-micro text-text-muted'
const SEARCH_BOX = join(WEB_SRC, 'components/list/SearchBox.svelte')
const DOCS_FILTER = join(WEB_SRC, 'components/docs/DocsFilter.svelte')
const HISTORY_VIEW = join(WEB_SRC, 'components/history/HistoryView.svelte')
const BULK_BAR = join(WEB_SRC, 'components/list/BulkBar.svelte')
const KEYMAP = join(WEB_SRC, 'lib/keymap.svelte.ts')
const SETTINGS = join(WEB_SRC, 'components/settings/SettingsDialog.svelte')
const COMMENT_FOOTER = join(WEB_SRC, 'components/write/CommentSubmitFooter.svelte')

describe('GDK-652 in-field X: one name, aria-label on SearchBox', () => {
  test('SearchBox, DocsFilter, and HistoryView clear-X share list.searchClear', () => {
    const search = readFileSync(SEARCH_BOX, 'utf8')
    const docs = readFileSync(DOCS_FILTER, 'utf8')
    const history = readFileSync(HISTORY_VIEW, 'utf8')
    expect(search).toContain("title={t('list.searchClear')}")
    expect(search).toContain("aria-label={t('list.searchClear')}")
    expect(docs).toContain("title={t('list.searchClear')}")
    expect(docs).toContain("aria-label={t('list.searchClear')}")
    expect(history).toContain("title={t('list.searchClear')}")
    expect(history).toContain("aria-label={t('list.searchClear')}")
    expect(docs).not.toContain("docs.filterClear")
    expect(history).not.toContain("history.filterClear")
  })
})

describe('GDK-652 back-arrow: one key for the arrow-left close', () => {
  test('every arrow-left back/close button uses feed.backToList', () => {
    const failures: string[] = []
    for (const p of svelteFiles()) {
      const src = readFileSync(p, 'utf8')
      if (!src.includes('name="arrow-left"')) continue
      if (src.includes("t('docs.backToIssues')")) {
        failures.push(`${p} still uses docs.backToIssues`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
    const feed = readFileSync(FEED, 'utf8')
    expect(feed).toContain("t('feed.backToList')")
  })
})

describe('GDK-652 bulk bar kbd chips match palette ∩ keymap', () => {
  test('BulkBar chips s/a/l/Esc in the comment-shortcut class; no p chip', () => {
    const footer = readFileSync(COMMENT_FOOTER, 'utf8')
    expect(footer).toContain(KBD_CHIP_CLASS)

    const bar = readFileSync(BULK_BAR, 'utf8')
    expect(bar).toContain(KBD_CHIP_CLASS)
    for (const glyph of ['s', 'a', 'l', 'Esc']) {
      expect(bar, `missing kbd ${glyph}`).toMatch(
        new RegExp(`<kbd[^>]*>\\s*${glyph}\\s*</kbd>`),
      )
    }
    expect(bar, 'palette does not teach p on triage items').not.toMatch(/<kbd[^>]*>\s*p\s*<\/kbd>/)

    const registry = readFileSync(COMMANDS, 'utf8')
    expect(registry).toContain("type: 'clear-bulk'")
    expect(registry).toContain("chords: [{ key: 's' }]")
    expect(registry).toContain("chords: [{ key: 'a' }]")
    expect(registry).toContain("chords: [{ key: 'l' }]")
    expect(registry).toContain("chords: [{ key: 'p' }]")
    expect(registry).toContain("kbd: 's'")
    expect(registry).toContain("kbd: 'a'")
    expect(registry).toContain("kbd: 'l'")
    expect(registry).toContain("kbd: 'Esc'")
    expect(readFileSync(KEYMAP, 'utf8')).toContain("from './commands'")
  })
})

describe('GDK-652 settings loading uses LoadingState', () => {
  test('SettingsDialog mounts LoadingState instead of a static paragraph', () => {
    const src = readFileSync(SETTINGS, 'utf8')
    expect(src).toContain('LoadingState')
    expect(src).not.toContain("t('settings.loading')")
  })
})

describe('GDK-652 EmptyState icons: omissions filled', () => {
  test('DocsView, SpaceDocsView, PersonalFeed do not pass icon=""', () => {
    const files = [
      join(WEB_SRC, 'components/docs/DocsView.svelte'),
      join(WEB_SRC, 'components/docs/SpaceDocsView.svelte'),
      join(WEB_SRC, 'components/personal/PersonalFeed.svelte'),
    ]
    const failures: string[] = []
    for (const p of files) {
      const src = readFileSync(p, 'utf8')
      if (/icon=""/.test(src)) failures.push(`${p} still has icon=""`)
    }
    expect(failures, failures.join('\n')).toEqual([])
  })
})
