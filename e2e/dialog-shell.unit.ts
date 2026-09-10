import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-620 / GDK-649: the dialog-shell registry used to live inside the
 * Playwright spec. Walking the source tree does not need a browser — same
 * rail as no-bare-timeout.unit.ts.
 *
 * Every .svelte under web/src that imports DialogShell.svelte must have a
 * registry entry below, and every entry must have its importer.
 *
 * GDK-725: the browser spec is a representative pair (one commit-shaped row,
 * one content-only row), so this file is where the full six-dialog registry
 * lives. The two tests below keep the shrink honest from both ends: a
 * seventh DialogShell importer cannot skip the registry, and the browser
 * pair cannot stop being a pair.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(E2E_DIR, '../web/src')
const SPEC = join(E2E_DIR, 'dialog-shell.spec.ts')

/** Importer file name -> registry id, for the two whose names disagree. */
const IMPORTER_ROW: Record<string, string> = {
  'SettingsDialog.svelte': 'settings',
  'NewIssueDialog.svelte': 'new-issue',
  'ShortcutsDialog.svelte': 'shortcuts',
  'JiraKeySettings.svelte': 'jira-credentials', // file name predates the dialog id
  'QuickComment.svelte': 'quick-comment',
  'WorkspacesTab.svelte': 'workspaces-remove', // the nested removal confirm
}

function shellImporters(root: string): string[] {
  const out: string[] = []
  const walk = (dir: string): void => {
    for (const name of readdirSync(dir)) {
      if (name === 'node_modules' || name === 'dist') continue
      const p = join(dir, name)
      if (statSync(p).isDirectory()) walk(p)
      else if (name.endsWith('.svelte')) {
        if (/from\s+'[^']*DialogShell\.svelte'/.test(readFileSync(p, 'utf8'))) out.push(name)
      }
    }
  }
  walk(root)
  return out
}

function specRows(src: string): { id: string; hasCommit: boolean }[] {
  const start = src.indexOf('const DIALOGS:')
  expect(start, 'e2e/dialog-shell.spec.ts must declare DIALOGS').toBeGreaterThanOrEqual(0)
  const end = src.indexOf('\n]', start)
  expect(end, 'DIALOGS array must close').toBeGreaterThan(start)
  const body = src.slice(start, end)
  const ids = [...body.matchAll(/^\s*id: '([^']+)',\s*$/gm)].map((m) => m[1])
  const commits = [...body.matchAll(/^\s*hasCommit: (true|false),?\s*$/gm)].map(
    (m) => m[1] === 'true',
  )
  expect(
    ids.length,
    'every DIALOGS row needs id: and hasCommit: on their own lines',
  ).toBe(commits.length)
  return ids.map((id, i) => ({ id, hasCommit: commits[i] }))
}

test('the registry covers every component that imports DialogShell', () => {
  const importers = shellImporters(WEB_SRC)
  const unmapped = importers.filter((f) => !(f in IMPORTER_ROW))
  expect(
    unmapped,
    `DialogShell importer(s) without a registry entry — add the IMPORTER_ROW mapping: ${unmapped}`,
  ).toEqual([])
  expect(
    Object.values(IMPORTER_ROW).sort(),
    'every registry entry keeps a real importer; every importer gets an entry',
  ).toHaveLength(importers.length)
  expect(
    new Set(Object.values(IMPORTER_ROW)).size,
    'registry ids are unique — two importers must not share one id',
  ).toBe(importers.length)
})

test('the browser smoke is a subset of the registry and keeps both shapes (GDK-725)', () => {
  const rows = specRows(readFileSync(SPEC, 'utf8'))
  const registry = new Set(Object.values(IMPORTER_ROW))
  const unknown = rows.filter((r) => !registry.has(r.id))
  expect(
    unknown.map((r) => r.id),
    'browser rows must be registry ids — a dialog outside the registry is the skip hole',
  ).toEqual([])
  expect(
    rows.some((r) => r.hasCommit),
    'the browser pair must keep a commit-shaped row (footer dismiss + primary)',
  ).toBe(true)
  expect(
    rows.some((r) => !r.hasCommit),
    'the browser pair must keep a content-only row (X + Esc, no footer buttons)',
  ).toBe(true)
})
