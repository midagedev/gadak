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
 * DIALOGS row, and every row must have its importer.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(E2E_DIR, '../web/src')
const SPEC = join(E2E_DIR, 'dialog-shell.spec.ts')

/** Importer file name -> DIALOGS row id, for the two whose names disagree. */
const IMPORTER_ROW: Record<string, string> = {
  'SettingsDialog.svelte': 'settings',
  'NewIssueDialog.svelte': 'new-issue',
  'ShortcutsDialog.svelte': 'shortcuts',
  'JiraKeySettings.svelte': 'jira-credentials', // file name predates the dialog id
  'QuickComment.svelte': 'quick-comment',
  'SidebarNav.svelte': 'update-notes', // hosts the update-notes modal
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

function dialogIdsFromSpec(src: string): string[] {
  const start = src.indexOf('const DIALOGS:')
  expect(start, 'e2e/dialog-shell.spec.ts must declare DIALOGS').toBeGreaterThanOrEqual(0)
  const end = src.indexOf('\n]', start)
  expect(end, 'DIALOGS array must close').toBeGreaterThan(start)
  return [...src.slice(start, end).matchAll(/^\s*id: '([^']+)'/gm)].map((m) => m[1])
}

test('dialog-shell table covers every component that imports DialogShell', () => {
  const importers = shellImporters(WEB_SRC)
  const unmapped = importers.filter((f) => !(f in IMPORTER_ROW))
  expect(
    unmapped,
    `DialogShell importer(s) without a DIALOGS row — add the row and the IMPORTER_ROW entry: ${unmapped}`,
  ).toEqual([])
  const specIds = dialogIdsFromSpec(readFileSync(SPEC, 'utf8'))
  expect(
    specIds.sort(),
    'every row keeps a real importer; every importer gets a row',
  ).toEqual(importers.map((f) => IMPORTER_ROW[f]).sort())
})
