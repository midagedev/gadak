import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'

/*
 * Recurrence layer for GDK-884: "the phone invents a word" must fail here,
 * not in a reviewer's eye.
 *
 * Three closes, in order of how a regression actually happens:
 *  1. A source scan for the invented nouns this round removed (Queue / Mine /
 *     All), so they cannot come back under a new component.
 *  2. One seam: only lib/i18n.ts may reach into web/, so the catalog cannot be
 *     imported from ten places — or, worse, copied into one.
 *  3. A ko-locale run of the scope builder. A hardcoded English string passes
 *     an English eye and fails this: the desk's word for the default plate is
 *     내 담당, and only t() knows that.
 */

const srcDir = join(dirname(fileURLToPath(import.meta.url)), '..')

function sourceFiles(): string[] {
  const out: string[] = []
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) walk(path)
      else if (/\.(ts|svelte)$/.test(entry.name)) out.push(path)
    }
  }
  walk(srcDir)
  return out.filter((p) => !p.endsWith('.test.ts'))
}

/** Strips // and /* *​/ comments and <!-- --> so prose about the ban is not the ban. */
function code(path: string): string {
  return readFileSync(path, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

describe('GDK-884 the phone does not invent nouns', () => {
  it('has no Queue / Mine / All left in shipped source', () => {
    const hits: string[] = []
    for (const path of sourceFiles()) {
      const text = code(path)
      for (const re of [/\bQueue\b/, /['">]Mine['"<]/, /['">]All['"<]/]) {
        if (re.test(text)) hits.push(`${relative(srcDir, path)} :: ${re}`)
      }
    }
    expect(hits).toEqual([])
  })

  it('crosses into the desktop catalog through exactly one module', () => {
    const importers = sourceFiles()
      .filter((p) => /web\/src\/lib\/i18n/.test(code(p)))
      .map((p) => relative(srcDir, p))
    expect(importers).toEqual(['lib/i18n.ts'])
  })

  it('takes the scope names from the catalog, in whatever locale is set', async () => {
    // A fresh module graph with gadak_locale=ko: web/src/lib/i18n runs
    // initLocale() at import time, so the catalog table is Korean before
    // domain.ts asks for a single word.
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (k === 'gadak_locale' ? 'ko' : null),
      setItem: () => {},
      removeItem: () => {},
    })
    vi.resetModules()
    try {
      const { locale } = await import('./i18n')
      const { buildScopes, SCOPE_ALL_OPEN, SCOPE_ME } = await import('./domain')
      expect(locale()).toBe('ko')
      const scopes = buildScopes([], [], { email: 'dev@example.com', account_id: 'acct-1', name: 'Dev' })
      expect(scopes.find((s) => s.id === SCOPE_ME)?.name).toBe('내 담당')
      expect(scopes.find((s) => s.id === SCOPE_ALL_OPEN)?.name).toBe('전체 미해결')
    } finally {
      vi.unstubAllGlobals()
      vi.resetModules()
    }
  })
})

describe('GDK-885 the picker wears the desktop section headings', () => {
  const sheet = code(join(srcDir, 'ui/ScopeSheet.svelte'))

  it('uses the sidebar keys, not phone-authored section labels', () => {
    for (const key of [
      'personal.myIssues',
      'sidebar.builtinViews',
      'sidebar.myViews',
      'sidebar.jiraFilters',
    ]) {
      expect(sheet, `ScopeSheet is missing ${key}`).toContain(key)
    }
  })

  it('titles the tab and the sheet with the object, from the catalog', () => {
    expect(code(join(srcDir, 'ui/TabBar.svelte'))).toContain("t('doc.issues')")
    expect(sheet).toContain("t('doc.issues')")
  })

  it('does not consume the view list for writing', () => {
    for (const path of sourceFiles()) {
      const text = code(path)
      expect(text, `${relative(srcDir, path)} writes to the view list`).not.toMatch(
        /issues\/views\/[^'"]*['"],\s*\{[^}]*method:\s*['"](POST|DELETE|PUT)/,
      )
    }
  })
})

describe('status-category folding stays parity with the desktop', () => {
  it('accepts every alias web/src/lib/view-config.ts accepts', async () => {
    const owner = readFileSync(
      join(srcDir, '../../web/src/lib/view-config.ts'),
      'utf8',
    )
    const body = owner.slice(owner.indexOf('export function effectiveCategory'))
    const fn = body.slice(0, body.indexOf('\n}'))
    // Every quoted alias the desk compares `sc` against.
    const aliases = [...fn.matchAll(/sc === '([a-z]+)'/g)].map((m) => m[1])
    expect(aliases.length).toBeGreaterThan(5)
    const { categoryAliases } = await import('./domain')
    expect(Object.keys(categoryAliases()).sort()).toEqual([...aliases].sort())
  })
})
