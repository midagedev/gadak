import { describe, expect, test } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

/*
 * The web boundary, as a gate (GDK-1497). The phone may share the web's
 * pure modules — the i18n catalog, view-config, terminal/protocol, and now
 * the ADF renderer — but never its runtime-config store (lib/config) or
 * any of its stores: those own web-only state (site base URL, shell
 * sessions, locale persistence) that would drag the store graph into the
 * phone bundle and silently couple the two surfaces' behavior.
 *
 * Text scanning, not a module-graph walk: it sees import(), `import type`,
 * re-exports, and .svelte <script> blocks the same way, with no bundler in
 * the loop. The banned prefixes are assembled from pieces so this file's
 * own source cannot match itself.
 */
const BANNED = ['web/src/' + 'lib/config', 'web/src/' + 'stores/']

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) walk(full, out)
    else if (/\.(ts|svelte)$/.test(entry.name)) out.push(full)
  }
  return out
}

/** Importer lines that reach a banned web path, as `file:line` entries. */
function violations(root: string): string[] {
  const found: string[] = []
  for (const file of walk(root)) {
    const rel = file.slice(root.length + 1)
    if (rel === join('lib', 'web-boundary.test.ts')) continue
    const lines = readFileSync(file, 'utf8').split('\n')
    lines.forEach((line, i) => {
      // `from '<spec>'` (static import/export) or `import('<spec>')`.
      if (!/(?:from|import\()\s*['"]/.test(line)) return
      for (const banned of BANNED) {
        if (line.includes(banned)) found.push(`${rel}:${i + 1}: ${line.trim()}`)
      }
    })
  }
  return found
}

describe('web boundary (GDK-1497)', () => {
  test('no mobile/src file imports the web config store or web stores', () => {
    const srcDir = fileURLToPath(new URL('..', import.meta.url))
    expect(violations(srcDir)).toEqual([])
  })
})
